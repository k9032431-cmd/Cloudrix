package api

import (
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/auth"
	"github.com/k9032431-cmd/cloudrix/internal/model"
)

type adminInput struct {
	Username  string     `json:"username"`
	Password  string     `json:"password"`
	Role      model.Role `json:"role"`
	Disabled  bool       `json:"disabled"`
	UserLimit int        `json:"user_limit"`
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
	if !usernameRe.MatchString(in.Username) || !in.Role.Valid() || in.UserLimit < 0 {
		writeError(w, http.StatusBadRequest, "invalid username, role or user_limit")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a := &model.Admin{Username: in.Username, PasswordHash: hash, Role: in.Role, Disabled: in.Disabled, UserLimit: in.UserLimit}
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
	if !in.Role.Valid() || in.UserLimit < 0 {
		writeError(w, http.StatusBadRequest, "invalid role or user_limit")
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
	a.Role, a.Disabled, a.UserLimit = in.Role, in.Disabled, in.UserLimit
	if err := s.store.UpdateAdmin(r.Context(), a); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "admin.update", a.Username, "")
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
}

func (in *nodeInput) valid() bool {
	in.Name, in.Address = strings.TrimSpace(in.Name), strings.TrimSpace(in.Address)
	if in.UsageCoefficient == 0 {
		in.UsageCoefficient = 1
	}
	return in.Name != "" && len(in.Name) <= 64 && in.Address != "" && in.APIPort > 0 && in.APIPort < 65536 && in.UsageCoefficient > 0
}

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
		writeError(w, http.StatusBadRequest, "name, address and api_port are required")
		return
	}
	n := &model.Node{Name: in.Name, Address: in.Address, APIPort: in.APIPort, UsageCoefficient: in.UsageCoefficient, Status: model.NodeConnecting}
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
		writeError(w, http.StatusBadRequest, "name, address and api_port are required")
		return
	}
	n, err := s.store.GetNode(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	n.Name, n.Address, n.APIPort, n.UsageCoefficient = in.Name, in.Address, in.APIPort, in.UsageCoefficient
	if in.Disabled {
		n.Status = model.NodeDisabled
	} else if n.Status == model.NodeDisabled {
		n.Status = model.NodeConnecting
	}
	if err := s.store.UpdateNode(r.Context(), n); err != nil {
		s.writeStoreError(w, err)
		return
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
	history, err := s.store.TrafficHistory(r.Context(), nil, owner, 30)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	out := map[string]any{
		"users":   users,
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
