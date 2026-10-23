package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/core"
	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/nodeproto"
	"github.com/k9032431-cmd/cloudrix/internal/store"
)

// Node agents call the panel (see package nodeproto). The panel never runs a
// proxy core itself: all client traffic goes through nodes.

const (
	nodePollWait   = 15 * time.Second // long-poll window for config changes
	nodeStaleAfter = 90 * time.Second // no call for this long = node offline
)

// nodeHub wakes long-polling agents when users, inbounds or nodes change.
type nodeHub struct {
	mu     sync.Mutex
	ch     chan struct{}
	lastTS map[int64]int64 // newest accepted request timestamp per node (replay guard)
}

func newNodeHub() *nodeHub { return &nodeHub{ch: make(chan struct{}), lastTS: map[int64]int64{}} }

func (h *nodeHub) changed() <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.ch
}

func (h *nodeHub) bump() {
	h.mu.Lock()
	close(h.ch)
	h.ch = make(chan struct{})
	h.mu.Unlock()
}

// accept records ts for the node if it is newer than every earlier request.
func (h *nodeHub) accept(id, ts int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ts <= h.lastTS[id] {
		return false
	}
	h.lastTS[id] = ts
	return true
}

var certHashRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// nodeConfigs builds the core configs a node should run and their version.
func (s *Server) nodeConfigs(ctx context.Context, id int64) (xray, singbox []byte, files map[string]string, version string, err error) {
	inbounds, err := s.store.ListInbounds(ctx)
	if err != nil {
		return nil, nil, nil, "", err
	}
	users, err := s.store.AllUsers(ctx)
	if err != nil {
		return nil, nil, nil, "", err
	}
	// users of an expired admin, or one over its traffic limit, get no keys
	if held := s.suspendedAdmins(ctx); len(held) > 0 {
		users = slices.DeleteFunc(slices.Clone(users), func(u *model.User) bool { return held[u.AdminID] })
	}
	groups, err := s.store.GroupMap(ctx)
	if err != nil {
		return nil, nil, nil, "", err
	}
	node, err := s.store.GetNode(ctx, id)
	if err != nil {
		return nil, nil, nil, "", err
	}
	mine := core.ForNode(inbounds, node)
	var xin, sin []*model.Inbound
	for _, in := range mine {
		if in.Protocol.Core() == "xray" {
			xin = append(xin, in)
		} else {
			sin = append(sin, in)
		}
	}
	cascade, err := s.cascadeFor(ctx, id, inbounds)
	if err != nil {
		return nil, nil, nil, "", err
	}
	if len(xin) > 0 || cascade.Bridge != nil {
		if xray, err = core.Xray(xin, users, groups, cascade); err != nil {
			return nil, nil, nil, "", err
		}
		if core.XrayBefore(node.CoreVersion, 26, 6) {
			xray = core.LegacyKCPMasks(xray)
		}
	}
	if len(sin) > 0 {
		if singbox, err = core.SingBox(sin, users, groups); err != nil {
			return nil, nil, nil, "", err
		}
	}
	// certificates issued on the main server travel with the config; a
	// renewed certificate changes the version, so nodes pick it up
	paths := []string{}
	for _, in := range mine {
		for _, p := range []string{in.Settings.CertFile, in.Settings.KeyFile} {
			if core.Shared(p) && !slices.Contains(paths, p) {
				paths = append(paths, p)
			}
		}
	}
	slices.Sort(paths)
	h := sha256.New()
	h.Write(xray)
	h.Write([]byte{0})
	h.Write(singbox)
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, nil, nil, "", fmt.Errorf("certificate file %s is not on the main server: issue it in cloudrix-manager → «Сертификат домена»", p)
		}
		if files == nil {
			files = map[string]string{}
		}
		files[p] = string(b)
		h.Write([]byte(p))
		h.Write(b)
	}
	return xray, singbox, files, hex.EncodeToString(h.Sum(nil))[:16], nil
}

func (s *Server) handleNodeSync(w http.ResponseWriter, r *http.Request) {
	id, err1 := strconv.ParseInt(r.Header.Get(nodeproto.HeaderNode), 10, 64)
	ts, err2 := strconv.ParseInt(r.Header.Get(nodeproto.HeaderTime), 10, 64)
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad body")
		return
	}
	ctx := r.Context()
	node, err := s.store.GetNode(ctx, id)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	secret, err := s.store.NodeSecret(ctx, id)
	skew := time.Now().UnixMilli() - ts
	if err != nil || secret == "" || skew > nodeproto.MaxSkewMillis || skew < -nodeproto.MaxSkewMillis ||
		!nodeproto.Verify(secret, id, ts, body, r.Header.Get(nodeproto.HeaderSig)) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !s.nodes.accept(id, ts) {
		writeError(w, http.StatusConflict, "stale request")
		return
	}
	plain, err := nodeproto.Open(secret, "req", body)
	var req nodeproto.SyncRequest
	if err != nil || json.Unmarshal(plain, &req) != nil {
		writeError(w, http.StatusBadRequest, "bad body")
		return
	}

	up, down := s.applyNodeStats(ctx, node, req.Stats)
	if len(req.Stats) > 0 {
		s.checkAdmins(ctx) // traffic may have used up an admin's limit
	}
	if req.Hysteria != nil {
		s.ensureHysteria(ctx, node, *req.Hysteria)
	}
	if req.OnlineReported {
		s.devices.report(id, req.Online)
		var ids []int64
		for email, ips := range req.Online {
			if uid, ok := nodeproto.UserID(email); ok && len(ips) > 0 {
				ids = append(ids, uid)
			}
		}
		if err := s.store.TouchOnline(ctx, ids); err != nil {
			s.log.Warn("online users", "node", node.Name, "err", err)
		}
		s.checkDevices(ctx)
	}
	msg := req.Error
	if len(msg) > 500 {
		msg = msg[:500]
	}
	cores := req.Cores
	if len(cores) > 120 {
		cores = cores[:120]
	}
	cert := req.CertSHA256
	if !certHashRe.MatchString(cert) {
		cert = ""
	}
	if cert != "" && cert != node.CertSHA256 {
		defer s.usersChanged() // links pin the new certificate
	}
	if err := s.store.RecordNodeSync(ctx, id, cores, msg, cert, up, down); err != nil {
		s.writeStoreError(w, err)
		return
	}

	// Subscribe before building the config: a change that lands while the
	// config is being built still wakes this request.
	changed := s.nodes.changed()
	resp := s.nodeResponse(ctx, node, req.Version)
	resp.Kicks, resp.KicksVersion = s.devices.kickSet()
	if req.Wait && !resp.Changed && !resp.Disabled && resp.Problem == "" && resp.KicksVersion == req.Kicks {
		// Hold the request until something changes so edits reach nodes at once.
		select {
		case <-changed:
			if n, err := s.store.GetNode(ctx, id); err == nil {
				node = n
			}
			resp = s.nodeResponse(ctx, node, req.Version)
			resp.Kicks, resp.KicksVersion = s.devices.kickSet()
		case <-time.After(nodePollWait):
		case <-ctx.Done():
			return
		}
	}
	plainResp, _ := json.Marshal(resp)
	sealed, err := nodeproto.Seal(secret, "resp", plainResp)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "seal failed")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(sealed)
}

func (s *Server) nodeResponse(ctx context.Context, node *model.Node, have string) nodeproto.SyncResponse {
	if node.Status == model.NodeDisabled {
		return nodeproto.SyncResponse{Disabled: true, Changed: have != "", Version: ""}
	}
	xray, singbox, files, version, err := s.nodeConfigs(ctx, node.ID)
	if err != nil {
		s.log.Warn("node config", "node", node.Name, "err", err)
		return nodeproto.SyncResponse{Version: have, Problem: err.Error()}
	}
	resp := nodeproto.SyncResponse{Version: version, Changed: version != have}
	if resp.Changed {
		resp.Xray, resp.SingBox, resp.Files = xray, singbox, files
	}
	return resp
}

// applyNodeStats adds reported traffic to users (times the node's usage
// coefficient) and returns the raw totals for the node itself.
func (s *Server) applyNodeStats(ctx context.Context, node *model.Node, stats []nodeproto.Stat) (up, down int64) {
	perUser := map[int64]int64{}
	for _, st := range stats {
		if st.Up < 0 || st.Down < 0 {
			continue
		}
		up += st.Up
		down += st.Down
		if uid, ok := nodeproto.UserID(st.Email); ok {
			perUser[uid] += st.Up + st.Down
		}
	}
	coef := node.UsageCoefficient
	if coef <= 0 {
		coef = 1
	}
	for uid, b := range perUser {
		if billed := int64(float64(b) * coef); billed > 0 {
			if err := s.store.AddTraffic(ctx, uid, billed); err != nil && !errors.Is(err, store.ErrNotFound) {
				s.log.Warn("node traffic", "node", node.Name, "user", uid, "err", err)
			}
		}
	}
	// node statistics count what really went through the node
	if err := s.store.AddNodeTraffic(ctx, node.ID, up, down, perUser); err != nil {
		s.log.Warn("node statistics", "node", node.Name, "err", err)
	}
	return up, down
}

// watchNodes marks nodes offline when their agent stops calling in.
func (s *Server) watchNodes(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	var pruned, checkpointed time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if time.Since(pruned) > 6*time.Hour {
				pruned = time.Now()
				if err := s.store.PruneNodeTraffic(ctx, nodeUserStatsDays, nodeStatsDays); err != nil {
					s.log.Warn("prune node statistics", "err", err)
				}
			}
			if time.Since(checkpointed) > 10*time.Minute {
				checkpointed = time.Now()
				s.checkpoint(ctx)
			}
			if _, err := s.store.MarkStaleNodes(ctx, time.Now().Add(-nodeStaleAfter), "no contact from the node agent"); err != nil {
				s.log.Warn("node watch", "err", err)
			}
			s.checkAdmins(ctx) // admin terms end on time
		}
	}
}

// checkpoint keeps the database's write-ahead log from growing: SQLite only
// empties it when no other process holds an old read snapshot, and a stuck
// one once let it fill the whole disk.
func (s *Server) checkpoint(ctx context.Context) {
	if s.cfg.DatabasePath == "" {
		return
	}
	busy, size, err := s.store.Checkpoint(ctx, s.cfg.DatabasePath)
	switch {
	case err != nil:
		s.log.Warn("database checkpoint", "err", err)
	case busy && size > 256<<20:
		s.log.Warn("database log cannot be emptied: another process keeps the database open (check: fuser -v "+s.cfg.DatabasePath+")", "wal_mb", size>>20)
	}
}

// panelURL is the address nodes use to reach the panel.
func (s *Server) panelURL(r *http.Request) string {
	if s.cfg.SubPublicURL != "" {
		return s.cfg.SubPublicURL
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// handleNodeInstall returns the one-line command that installs the agent.
func (s *Server) handleNodeInstall(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	node, err := s.store.GetNode(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	secret, err := s.store.NodeSecret(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	if secret == "" {
		secret = nodeproto.NewSecret()
		if err := s.store.SetNodeSecret(r.Context(), id, secret); err != nil {
			s.writeStoreError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, s.installInfo(r, node, secret))
}

// handleNodeRotate issues a new agent secret; the node must be reinstalled.
func (s *Server) handleNodeRotate(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	node, err := s.store.GetNode(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	secret := nodeproto.NewSecret()
	if err := s.store.SetNodeSecret(r.Context(), id, secret); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "node.rotate", node.Name, "")
	writeJSON(w, http.StatusOK, s.installInfo(r, node, secret))
}

func (s *Server) installInfo(r *http.Request, node *model.Node, secret string) map[string]any {
	repo, branch := s.cfg.InstallRepo, s.cfg.InstallBranch
	script := "https://raw.githubusercontent.com/" + repo + "/" + url.PathEscape(branch) + "/install.sh"
	script = strings.ReplaceAll(script, "%2F", "/")
	panel := s.panelURL(r)
	env := "CLOUDRIX_BRANCH=" + branch
	if repo != defaultRepo {
		env = "CLOUDRIX_REPO=" + repo + " " + env
	}
	cmd := env + " bash <(curl -fsSL " + script + ") node --panel " + panel +
		" --node-id " + strconv.FormatInt(node.ID, 10) + " --secret " + secret
	return map[string]any{"command": cmd, "panel_url": panel, "node_id": node.ID, "secret": secret, "plain_http": strings.HasPrefix(panel, "http://")}
}

const defaultRepo = "k9032431-cmd/Cloudrix"
