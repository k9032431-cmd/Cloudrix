package api

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/sub"
)

var fingerprints = []string{"", "chrome", "firefox", "safari", "ios", "android", "edge", "360", "qq", "random", "randomized"}

// cleanList trims the values, drops empty and repeated ones.
func cleanList(v []string) []string {
	out := []string{}
	for _, x := range v {
		if x = strings.TrimSpace(x); x != "" && !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}

func validateHost(h *model.Host) error {
	s := &h.HostSettings
	s.Remark = sub.CleanName(s.Remark)
	s.Address, s.SNI, s.Host = cleanList(s.Address), cleanList(s.SNI), cleanList(s.Host)
	s.Path = strings.TrimSpace(s.Path)
	switch {
	case h.InboundID <= 0:
		return errors.New("choose the inbound of this host")
	case s.Remark == "" || len(s.Remark) > 256:
		return errors.New("host name is required (up to 256 characters)")
	case s.Port < 0 || s.Port > 65535:
		return errors.New("port must be 0..65535 (0 = the inbound's port)")
	case s.Path != "" && !strings.HasPrefix(s.Path, "/"):
		return errors.New("path must start with /")
	case !slices.Contains(fingerprints, s.Fingerprint):
		return errors.New("unknown fingerprint")
	case !slices.Contains([]string{"", "tls", "none"}, s.Security):
		return errors.New("security must be the inbound's default, tls or none")
	}
	for _, a := range s.Address {
		if !hostRe.MatchString(a) {
			return errors.New("address " + a + " must be an IP or a domain name, without scheme, port or spaces")
		}
	}
	for _, v := range s.SNI {
		if !hostRe.MatchString(strings.TrimPrefix(v, "*.")) {
			return errors.New("SNI " + v + " must be a domain name")
		}
	}
	for _, v := range s.Host {
		if len(v) > 255 || strings.ContainsAny(v, " /") {
			return errors.New("host " + v + " must be a domain name")
		}
	}
	for _, a := range s.ALPN {
		if !slices.Contains([]string{"h3", "h2", "http/1.1"}, a) {
			return errors.New("ALPN may be h3, h2, http/1.1")
		}
	}
	for _, st := range s.Status {
		if !slices.Contains([]model.UserStatus{model.StatusActive, model.StatusDisabled, model.StatusLimited, model.StatusExpired, model.StatusOnHold}, st) {
			return errors.New("unknown user status " + string(st))
		}
	}
	return nil
}

func (s *Server) handleListHosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.store.ListHosts(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, hosts)
}

func (s *Server) handleCreateHost(w http.ResponseWriter, r *http.Request) {
	var h model.Host
	if !decode(w, r, &h) {
		return
	}
	h.ID, h.Priority = 0, 0
	if err := validateHost(&h); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.CreateHost(r.Context(), &h); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "host.create", h.Remark, strings.Join(h.Address, ", "))
	s.usersChanged()
	writeJSON(w, http.StatusCreated, h)
}

func (s *Server) handleUpdateHost(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var h model.Host
	if !decode(w, r, &h) {
		return
	}
	h.ID = id
	if err := validateHost(&h); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.UpdateHost(r.Context(), &h); err != nil {
		s.writeStoreError(w, err)
		return
	}
	got, err := s.store.GetHost(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "host.update", got.Remark, strings.Join(got.Address, ", "))
	s.usersChanged()
	writeJSON(w, http.StatusOK, got)
}

func (s *Server) handleDeleteHost(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	h, err := s.store.GetHost(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	if err := s.store.DeleteHost(r.Context(), id); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "host.delete", h.Remark, strings.Join(h.Address, ", "))
	s.usersChanged()
	w.WriteHeader(http.StatusNoContent)
}

// handleReorderHosts takes {"ids": [...]}: the hosts in subscription order.
func (s *Server) handleReorderHosts(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.store.ReorderHosts(r.Context(), in.IDs); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "host.reorder", "", "")
	s.usersChanged()
	w.WriteHeader(http.StatusNoContent)
}
