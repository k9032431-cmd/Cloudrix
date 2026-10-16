package api

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/auth"
	"github.com/k9032431-cmd/cloudrix/internal/core"
	"github.com/k9032431-cmd/cloudrix/internal/model"
)

var tagRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)

func validateInbound(in *model.Inbound) error {
	if !tagRe.MatchString(in.Tag) {
		return errors.New("tag must be 1-64 chars: letters, digits, _ . -")
	}
	if in.Tag == "api" {
		return errors.New(`tag "api" is reserved`)
	}
	if !in.Protocol.Valid() {
		return errors.New("unknown protocol")
	}
	if in.Port < 1 || in.Port > 65535 {
		return errors.New("port must be 1..65535")
	}
	// the public address goes into client links as is
	in.Address = strings.TrimSpace(in.Address)
	if in.Address != "" && !hostRe.MatchString(in.Address) {
		return errors.New("public address must be an IP or a domain name, without scheme, port or spaces")
	}
	if in.Listen == "" {
		in.Listen = "0.0.0.0"
	}
	s := &in.Settings
	switch s.Network {
	case "", "tcp", "ws", "grpc", "xhttp", "httpupgrade", "kcp":
	default:
		return errors.New("network must be tcp, ws, grpc, xhttp, httpupgrade or kcp")
	}
	if err := validateExtras(s); err != nil {
		return err
	}
	switch s.Security {
	case "", "none", "tls", "reality":
	default:
		return errors.New("security must be none, tls or reality")
	}
	if s.Security == "reality" {
		if in.Protocol != model.ProtoVLESS && in.Protocol != model.ProtoTrojan {
			return errors.New("reality is supported for vless and trojan only")
		}
		if s.RealityPrivateKey == "" || s.RealityPublicKey == "" || s.RealityDest == "" {
			return errors.New("reality requires reality_dest and a key pair")
		}
		if len(s.RealityServerNames) == 0 && s.SNI != "" {
			s.RealityServerNames = []string{s.SNI}
		}
		if len(s.RealityServerNames) == 0 {
			return errors.New("reality requires server names (the domains it poses as)")
		}
	}
	switch in.Protocol {
	case model.ProtoHysteria2, model.ProtoTUIC:
		if (s.CertFile == "") != (s.KeyFile == "") {
			return errors.New(string(in.Protocol) + " needs both cert_file and key_file, or neither")
		}
		if s.CertFile == "" {
			// the node uses a self-signed certificate, so clients must not verify it
			s.AllowInsecure = true
		}
	case model.ProtoWireGuard:
		if s.WGPrivateKey == "" || s.WGPublicKey == "" {
			return errors.New("wireguard requires a server key pair")
		}
	case model.ProtoShadowsocks:
		if strings.HasPrefix(s.SSMethod, "2022-") && s.SSServerKey == "" {
			return errors.New("shadowsocks 2022 requires ss_server_key")
		}
	}
	return nil
}

// inboundCore settles which core an inbound belongs to: the one sent, or
// the core of the node sent (older API clients), or the default core.
func (s *Server) inboundCore(w http.ResponseWriter, r *http.Request, in *model.Inbound) bool {
	if in.CoreID == 0 && in.NodeID != nil {
		node, err := s.store.GetNode(r.Context(), *in.NodeID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "node not found")
			return false
		}
		in.CoreID = node.CoreID
	}
	if in.CoreID == 0 {
		def, err := s.store.DefaultCore(r.Context())
		if err != nil {
			s.writeStoreError(w, err)
			return false
		}
		in.CoreID = def
	}
	if _, err := s.store.GetCore(r.Context(), in.CoreID); err != nil {
		writeError(w, http.StatusBadRequest, "core not found")
		return false
	}
	return true
}

// checkInboundNode checks an inbound against the nodes that will run it:
// free port, cascade link, exit node, certificate files.
func (s *Server) checkInboundNode(w http.ResponseWriter, r *http.Request, in *model.Inbound) bool {
	nodes, err := s.store.ListNodes(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return false
	}
	on := *in
	on.Enabled = true
	var runners []*model.Node
	for _, n := range nodes {
		if on.Runs(n) {
			runners = append(runners, n)
		}
	}
	if in.NodeID != nil && len(runners) == 0 {
		writeError(w, http.StatusBadRequest, "that node does not run this core")
		return false
	}
	others, err := s.store.ListInbounds(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return false
	}
	for _, o := range others {
		if o.ID != in.ID && core.Together(o, in) && core.PortClash(o, in) {
			// one busy port makes the core refuse the node's whole config
			writeError(w, http.StatusBadRequest, fmt.Sprintf("port %d in this core is already used by %s inbound %s: "+
				"only another %s inbound can share it, or pick another port", in.Port, o.Protocol, o.Tag, o.Protocol))
			return false
		}
	}
	for _, p := range []string{in.Settings.CertFile, in.Settings.KeyFile} {
		if core.Shared(p) {
			if _, err := os.Stat(p); err != nil {
				writeError(w, http.StatusBadRequest, "file "+p+" is not on the main server: issue the certificate in cloudrix-manager → «Сертификат домена» and copy the paths it shows")
				return false
			}
		}
	}
	for _, n := range runners {
		if n.Bridge.UUID != "" && n.Bridge.Port == in.Port {
			writeError(w, http.StatusBadRequest, "port "+fmt.Sprint(in.Port)+" carries the cascade link of node "+n.Name+": pick another port")
			return false
		}
	}
	if ex := in.Settings.ExitNodeID; ex != nil {
		if in.Protocol.Core() != "xray" {
			writeError(w, http.StatusBadRequest, "cascades work for vless, vmess, trojan and shadowsocks inbounds")
			return false
		}
		exit, err := s.store.GetNode(r.Context(), *ex)
		if err != nil {
			writeError(w, http.StatusBadRequest, "exit node not found")
			return false
		}
		if on.Runs(exit) {
			writeError(w, http.StatusBadRequest, "the exit node runs this inbound itself: pick a node with another core")
			return false
		}
	}
	return true
}

func (s *Server) handleListInbounds(w http.ResponseWriter, r *http.Request) {
	inbounds, err := s.store.ListInbounds(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	// Non-sudo admins only need tags to assign users; hide server secrets.
	if adminFrom(r.Context()).Role != model.RoleSudo {
		type brief struct {
			Tag      string         `json:"tag"`
			Protocol model.Protocol `json:"protocol"`
			Remark   string         `json:"remark"`
			Enabled  bool           `json:"enabled"`
		}
		out := make([]brief, 0, len(inbounds))
		for _, in := range inbounds {
			out = append(out, brief{in.Tag, in.Protocol, in.Remark, in.Enabled})
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	writeJSON(w, http.StatusOK, inbounds)
}

func (s *Server) handleCreateInbound(w http.ResponseWriter, r *http.Request) {
	var in model.Inbound
	if !decode(w, r, &in) {
		return
	}
	if err := validateInbound(&in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.inboundCore(w, r, &in) || !s.joinListener(w, r, &in, nil) || !s.checkInboundNode(w, r, &in) {
		return
	}
	if err := s.store.CreateInbound(r.Context(), &in); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "inbound.create", in.Tag, string(in.Protocol))
	s.applyInboundGroups(r, in.ID)
	s.usersChanged()
	writeJSON(w, http.StatusCreated, in)
}

func (s *Server) handleUpdateInbound(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in model.Inbound
	if !decode(w, r, &in) {
		return
	}
	in.ID = id
	if err := validateInbound(&in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	old, err := s.store.GetInbound(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	if !s.inboundCore(w, r, &in) || !s.joinListener(w, r, &in, old) || !s.checkInboundNode(w, r, &in) {
		return
	}
	if err := s.store.UpdateInbound(r.Context(), &in); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.syncHosts(r, &in)
	s.applyInboundGroups(r, in.ID)
	if old.Tag != in.Tag { // keep users that picked this inbound on it
		s.editUserInbounds(r, func(tags []string) ([]string, bool) {
			i := slices.Index(tags, old.Tag)
			if i < 0 {
				return nil, false
			}
			tags[i] = in.Tag
			return slices.Compact(tags), true
		})
	}
	s.audit(r, "inbound.update", in.Tag, "")
	s.usersChanged()
	writeJSON(w, http.StatusOK, in)
}

// joinListener makes an inbound that moves onto a port already served by
// an inbound of its protocol on the node a host of that listener: it takes
// the owner's server side. An inbound already on the listener keeps what was
// sent, and syncHosts passes it on to the others.
func (s *Server) joinListener(w http.ResponseWriter, r *http.Request, in, old *model.Inbound) bool {
	if in.Protocol.Core() != "xray" {
		return true
	}
	if old != nil && core.Together(old, in) && core.Hosted(old, in) {
		return true
	}
	all, err := s.store.ListInbounds(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return false
	}
	if owner := core.Owner(in, all); owner != nil {
		core.AdoptServer(in, owner)
	}
	return true
}

// syncHosts gives the server side of a saved inbound to the others sharing
// its listener, so the keys, transport and exit stay one for all of them.
func (s *Server) syncHosts(r *http.Request, in *model.Inbound) {
	all, err := s.store.ListInbounds(r.Context())
	if err != nil {
		return
	}
	for _, o := range core.Peers(in, all) {
		if core.AdoptServer(o, in) {
			if err := s.store.UpdateInbound(r.Context(), o); err != nil {
				s.log.Warn("sync shared-port inbound", "inbound", o.Tag, "err", err)
			}
		}
	}
}

// applyInboundGroups puts the inbound into the groups named by ?groups=1,2
// (an empty value takes it out of all groups; no parameter leaves them as is).
func (s *Server) applyInboundGroups(r *http.Request, id int64) {
	q := r.URL.Query()
	if !q.Has("groups") {
		return
	}
	var ids []int64
	for _, v := range strings.Split(q.Get("groups"), ",") {
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil && n > 0 {
			ids = append(ids, n)
		}
	}
	if err := s.store.SetInboundGroups(r.Context(), id, ids); err != nil {
		s.log.Warn("set inbound groups", "err", err)
	}
}

func (s *Server) editUserInbounds(r *http.Request, edit func([]string) ([]string, bool)) {
	if _, err := s.store.EditUserInbounds(r.Context(), edit); err != nil {
		s.log.Warn("update user inbounds", "err", err)
	}
}

func (s *Server) handleDeleteInbound(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	in, err := s.store.GetInbound(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	if err := s.store.DeleteInbound(r.Context(), id); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "inbound.delete", in.Tag, "")
	// Drop the tag from hand-picked lists, unless it was the only one:
	// an empty list would mean "all inbounds".
	s.editUserInbounds(r, func(tags []string) ([]string, bool) {
		i := slices.Index(tags, in.Tag)
		if i < 0 || len(tags) == 1 {
			return nil, false
		}
		return slices.Delete(tags, i, i+1), true
	})
	s.usersChanged()
	w.WriteHeader(http.StatusNoContent)
}

// handleCoreConfig previews the generated server config: ?core=xray|sing-box&node=<id>.
func (s *Server) handleCoreConfig(w http.ResponseWriter, r *http.Request) {
	inbounds, err := s.store.ListInbounds(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	users, err := s.store.AllUsers(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	groups, err := s.store.GroupMap(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	var nodeID *int64
	var node *model.Node
	if n := queryInt(r, "node", 0); n > 0 {
		id := int64(n)
		nodeID = &id
		if node, err = s.store.GetNode(r.Context(), id); err != nil {
			s.writeStoreError(w, err)
			return
		}
	}
	selected := core.ForNode(inbounds, node)
	var cascade core.Cascade
	if nodeID != nil {
		if cascade, err = s.cascadeFor(r.Context(), *nodeID, inbounds); err != nil {
			s.writeStoreError(w, err)
			return
		}
	}
	var body []byte
	switch r.URL.Query().Get("core") {
	case "", "xray":
		body, err = core.Xray(selected, users, groups, cascade)
	case "sing-box":
		body, err = core.SingBox(selected, users, groups)
	default:
		writeError(w, http.StatusBadRequest, "core must be xray or sing-box")
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(body)
}

func (s *Server) handleRealityKeys(w http.ResponseWriter, _ *http.Request) {
	priv, pub := auth.X25519KeyPair(base64.RawURLEncoding)
	writeJSON(w, http.StatusOK, map[string]any{
		"private_key": priv, "public_key": pub, "short_ids": []string{auth.RandomHex(8), auth.RandomHex(4)},
	})
}

func (s *Server) handleWireGuardKeys(w http.ResponseWriter, _ *http.Request) {
	priv, pub := auth.X25519KeyPair(base64.StdEncoding)
	writeJSON(w, http.StatusOK, map[string]string{"private_key": priv, "public_key": pub})
}

// handleRandom returns random secrets: ?bytes=32 as base64 (fits SS-2022 keys) plus a hex variant.
func (s *Server) handleRandom(w http.ResponseWriter, r *http.Request) {
	n := min(max(queryInt(r, "bytes", 32), 8), 64)
	b := auth.RandomBytes(n)
	writeJSON(w, http.StatusOK, map[string]string{
		"base64": base64.StdEncoding.EncodeToString(b),
		"hex":    auth.RandomHex(n),
		"uuid":   auth.NewUUID(),
	})
}

// validateExtras checks the advanced fields of the inbound editor.
func validateExtras(s *model.InboundSettings) error {
	oneOf := func(v string, allowed ...string) bool { return v == "" || slices.Contains(allowed, v) }
	switch {
	case !oneOf(s.TLSMinVersion, "1.0", "1.1", "1.2", "1.3") || !oneOf(s.TLSMaxVersion, "1.0", "1.1", "1.2", "1.3"):
		return errors.New("TLS versions are 1.0, 1.1, 1.2 or 1.3")
	case !oneOf(s.KCPHeader, "wechat", "dtls", "srtp", "utp", "wireguard", "dns"):
		return errors.New("mKCP header is wechat, dtls, srtp, utp, wireguard or dns")
	case s.KCPHeaderDomain != "" && !hostRe.MatchString(s.KCPHeaderDomain):
		return errors.New("mKCP dns header needs a domain name")
	case !oneOf(s.SSNetwork, "tcp,udp", "tcp", "udp"):
		return errors.New("shadowsocks network is tcp,udp, tcp or udp")
	case !oneOf(s.TCPCongestion, "bbr", "cubic", "reno"):
		return errors.New("TCP congestion is bbr, cubic or reno")
	case s.RealityXver < 0 || s.RealityXver > 2:
		return errors.New("REALITY xver is 0, 1 or 2")
	case s.RealityMaxTimeDiff < 0 || s.WSHeartbeat < 0 || s.TCPKeepAliveInterval < 0 || s.XScMaxBufferedPosts < 0 ||
		s.KCPMTU < 0 || s.KCPTTI < 0 || s.KCPUplink < 0 || s.KCPDownlink < 0 || s.KCPReadBuffer < 0 || s.KCPWriteBuffer < 0:
		return errors.New("numbers must not be negative")
	case s.KCPMTU != 0 && (s.KCPMTU < 576 || s.KCPMTU > 1460):
		return errors.New("mKCP MTU is 576..1460")
	case s.KCPTTI != 0 && (s.KCPTTI < 10 || s.KCPTTI > 100):
		return errors.New("mKCP TTI is 10..100 ms")
	}
	if s.Sniffing != nil {
		for _, d := range s.Sniffing.DestOverride {
			if !oneOf(d, "http", "tls", "quic", "fakedns") {
				return errors.New("sniffing covers http, tls, quic, fakedns")
			}
		}
	}
	for _, f := range s.Fallbacks {
		if strings.TrimSpace(f.Dest) == "" {
			return errors.New("every fallback needs a destination (port or host:port)")
		}
		if f.Xver < 0 || f.Xver > 2 {
			return errors.New("fallback xver is 0, 1 or 2")
		}
		if f.Path != "" && !strings.HasPrefix(f.Path, "/") {
			return errors.New("fallback path must start with /")
		}
	}
	for _, d := range []string{s.UDPTimeout, s.Heartbeat, s.AuthTimeout} {
		if d != "" {
			if _, err := time.ParseDuration(d); err != nil {
				return errors.New("durations look like 10s or 5m: " + d)
			}
		}
	}
	return nil
}
