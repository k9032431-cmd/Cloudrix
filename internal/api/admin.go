package api

import (
	"net/http"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/auth"
	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/nodeproto"
)

type adminInput struct {
	Username       string     `json:"username"`
	Password       string     `json:"password"`
	Role           model.Role `json:"role"`
	Disabled       bool       `json:"disabled"`
	UserLimit      int        `json:"user_limit"`
	ExpireAt       *time.Time `json:"expire_at"`
	DataLimit      int64      `json:"data_limit"`
	MaxUserData    int64      `json:"max_user_data"`
	MaxUserDevices int        `json:"max_user_devices"`
	ResetTraffic   bool       `json:"reset_traffic"` // start the traffic count over
}

func (in *adminInput) limitsValid() bool {
	return in.UserLimit >= 0 && in.DataLimit >= 0 && in.MaxUserData >= 0 && in.MaxUserDevices >= 0
}

// applyLimits copies the limits; sudo admins have none.
func (in *adminInput) applyLimits(a *model.Admin) {
	a.UserLimit = in.UserLimit
	if in.Role == model.RoleSudo {
		a.ExpireAt, a.DataLimit, a.MaxUserData, a.MaxUserDevices = nil, 0, 0, 0
		return
	}
	a.ExpireAt, a.DataLimit, a.MaxUserData, a.MaxUserDevices = in.ExpireAt, in.DataLimit, in.MaxUserData, in.MaxUserDevices
	if in.ResetTraffic {
		a.UsedTraffic = 0
	}
}

func (s *Server) handleListAdmins(w http.ResponseWriter, r *http.Request) {
	admins, err := s.store.ListAdmins(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, admins)
}

func (s *Server) handleCreateAdmin(w http.ResponseWriter, r *http.Request) {
	var in adminInput
	if !decode(w, r, &in) {
		return
	}
	if !usernameRe.MatchString(in.Username) || !in.Role.Valid() || !in.limitsValid() {
		writeError(w, http.StatusBadRequest, "invalid username, role or limits")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a := &model.Admin{Username: in.Username, PasswordHash: hash, Role: in.Role, Disabled: in.Disabled}
	in.applyLimits(a)
	if err := s.store.CreateAdmin(r.Context(), a); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "admin.create", a.Username, string(a.Role))
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) handleUpdateAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in adminInput
	if !decode(w, r, &in) {
		return
	}
	a, err := s.store.GetAdmin(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	if !in.Role.Valid() || !in.limitsValid() {
		writeError(w, http.StatusBadRequest, "invalid role or limits")
		return
	}
	if me := adminFrom(r.Context()); me.ID == a.ID && (in.Disabled || in.Role != model.RoleSudo) {
		writeError(w, http.StatusBadRequest, "you cannot disable or demote yourself")
		return
	}
	if in.Password != "" {
		if a.PasswordHash, err = auth.HashPassword(in.Password); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	a.Role, a.Disabled = in.Role, in.Disabled
	in.applyLimits(a)
	if err := s.store.UpdateAdmin(r.Context(), a); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "admin.update", a.Username, "")
	s.usersChanged() // a raised or lowered limit turns the admin's users on or off
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) handleDeleteAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if adminFrom(r.Context()).ID == id {
		writeError(w, http.StatusBadRequest, "you cannot delete yourself")
		return
	}
	a, err := s.store.GetAdmin(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	if err := s.store.DeleteAdmin(r.Context(), id); err != nil {
		s.writeStoreError(w, err) // conflict: admin still owns users
		return
	}
	s.audit(r, "admin.delete", a.Username, "")
	w.WriteHeader(http.StatusNoContent)
}

type nodeInput struct {
	Name             string  `json:"name"`
	Address          string  `json:"address"`
	APIPort          int     `json:"api_port"`
	UsageCoefficient float64 `json:"usage_coefficient"`
	Disabled         bool    `json:"disabled"`
	BridgePort       int     `json:"bridge_port"` // cascade port when other nodes exit here, 0 = auto
	CoreID           int64   `json:"core_id"`     // core configuration it runs, 0 = the default one
}

// nodeCore checks the chosen core; 0 picks the default one.
func (s *Server) nodeCore(w http.ResponseWriter, r *http.Request, id int64) (int64, bool) {
	if id == 0 {
		def, err := s.store.DefaultCore(r.Context())
		if err != nil {
			s.writeStoreError(w, err)
			return 0, false
		}
		return def, true
	}
	if _, err := s.store.GetCore(r.Context(), id); err != nil {
		writeError(w, http.StatusBadRequest, "core not found")
		return 0, false
	}
	return id, true
}

// valid normalises the input. Address is the node's public IP or domain that
// goes into client links; the agent dials the panel, so no API port is needed.
func (in *nodeInput) valid() bool {
	in.Name, in.Address = strings.TrimSpace(in.Name), strings.TrimSpace(in.Address)
	if in.UsageCoefficient == 0 {
		in.UsageCoefficient = 1
	}
	return in.Name != "" && len(in.Name) <= 64 && hostRe.MatchString(in.Address) &&
		in.APIPort >= 0 && in.APIPort < 65536 && in.UsageCoefficient > 0 && in.UsageCoefficient <= 100 &&
		in.BridgePort >= 0 && in.BridgePort < 65536
}

// hostRe accepts an IPv4/IPv6 address or a domain name, without scheme or port.
var hostRe = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$|^[0-9a-fA-F:]+$`)

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.store.ListNodes(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (s *Server) handleCreateNode(w http.ResponseWriter, r *http.Request) {
	var in nodeInput
	if !decode(w, r, &in) {
		return
	}
	if !in.valid() {
		writeError(w, http.StatusBadRequest, "name and a public IP or domain are required")
		return
	}
	coreID, ok := s.nodeCore(w, r, in.CoreID)
	if !ok {
		return
	}
	n := &model.Node{Name: in.Name, Address: in.Address, APIPort: in.APIPort, UsageCoefficient: in.UsageCoefficient, CoreID: coreID, Status: model.NodeConnecting, Secret: nodeproto.NewSecret()}
	if in.Disabled {
		n.Status = model.NodeDisabled
	}
	if err := s.store.CreateNode(r.Context(), n); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "node.create", n.Name, n.Address)
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) handleUpdateNode(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in nodeInput
	if !decode(w, r, &in) {
		return
	}
	if !in.valid() {
		writeError(w, http.StatusBadRequest, "name and a public IP or domain are required")
		return
	}
	n, err := s.store.GetNode(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	n.Name, n.Address, n.APIPort, n.UsageCoefficient = in.Name, in.Address, in.APIPort, in.UsageCoefficient
	if in.CoreID != 0 {
		if n.CoreID, ok = s.nodeCore(w, r, in.CoreID); !ok {
			return
		}
	}
	if in.Disabled {
		n.Status = model.NodeDisabled
	} else if n.Status == model.NodeDisabled {
		n.Status = model.NodeConnecting
	}
	if err := s.store.UpdateNode(r.Context(), n); err != nil {
		s.writeStoreError(w, err)
		return
	}
	if in.BridgePort != n.Bridge.Port {
		n.Bridge.Port = in.BridgePort
		if err := s.store.SetNodeBridge(r.Context(), n.ID, n.Bridge); err != nil {
			s.writeStoreError(w, err)
			return
		}
		n.BridgePort = in.BridgePort
	}
	s.audit(r, "node.update", n.Name, "")
	s.usersChanged()
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	n, err := s.store.GetNode(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	inbounds, err := s.store.ListInbounds(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	for _, in := range inbounds {
		if ex := in.Settings.ExitNodeID; ex != nil && *ex == id {
			writeError(w, http.StatusConflict, "inbound "+in.Tag+" exits through this node: change it first")
			return
		}
	}
	if err := s.store.DeleteNode(r.Context(), id); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "node.delete", n.Name, "")
	s.usersChanged()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	limit := min(max(queryInt(r, "limit", 100), 1), 500)
	entries, err := s.store.ListAudit(r.Context(), limit, max(queryInt(r, "offset", 0), 0))
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	a := adminFrom(r.Context())
	owner := ownerFilter(a)
	users, err := s.store.UserStats(r.Context(), owner, time.Now().Add(-2*time.Minute))
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	// ?days= selects the period; twice that is returned so the UI can compare
	// the current period with the previous one.
	days := min(max(queryInt(r, "days", 30), 1), 366)
	history, err := s.store.TrafficHistory(r.Context(), nil, owner, days*2)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	out := map[string]any{
		"users":   users,
		"days":    days,
		"traffic": history,
		"system": map[string]any{
			"version":    Version,
			"uptime":     int64(time.Since(s.started).Seconds()),
			"goroutines": runtime.NumGoroutine(),
			"mem_alloc":  mem.Alloc,
			"cpus":       runtime.NumCPU(),
		},
	}
	if a.Role == model.RoleSudo {
		nodes, err := s.store.ListNodes(r.Context())
		if err != nil {
			s.writeStoreError(w, err)
			return
		}
		online := 0
		for _, n := range nodes {
			if n.Status == model.NodeConnected {
				online++
			}
		}
		out["nodes"] = map[string]int{"total": len(nodes), "connected": online}
	}
	writeJSON(w, http.StatusOK, out)
}

// Version is set at build time via -ldflags.
var Version = "dev"
