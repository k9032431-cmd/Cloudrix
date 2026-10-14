package api

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

var fingerprints = []string{"", "chrome", "firefox", "safari", "ios", "android", "edge", "360", "qq", "random", "randomized"}

func validateHost(h *model.Host) error {
	s := &h.HostSettings
	s.Remark = strings.TrimSpace(s.Remark)
	s.Address = strings.TrimSpace(s.Address)
	s.SNI = strings.TrimSpace(s.SNI)
	s.Host = strings.TrimSpace(s.Host)
	s.Path = strings.TrimSpace(s.Path)
	switch {
	case h.InboundID <= 0:
		return errors.New("choose the inbound of this host")
	case len(s.Remark) > 256:
		return errors.New("remark is too long")
	case s.Address != "" && !hostRe.MatchString(s.Address):
		return errors.New("address must be an IP or a domain name, without scheme, port or spaces")
	case s.Port < 0 || s.Port > 65535:
		return errors.New("port must be 0..65535 (0 = the inbound's port)")
	case s.SNI != "" && !hostRe.MatchString(s.SNI):
		return errors.New("SNI must be a domain name")
	case len(s.Host) > 255 || strings.ContainsAny(s.Host, " /"):
		return errors.New("host must be a domain name")
	case s.Path != "" && !strings.HasPrefix(s.Path, "/"):
		return errors.New("path must start with /")
	case !slices.Contains(fingerprints, s.Fingerprint):
		return errors.New("unknown fingerprint")
	}
	for _, a := range s.ALPN {
		if !slices.Contains([]string{"h3", "h2", "http/1.1"}, a) {
			return errors.New("ALPN may be h3, h2, http/1.1")
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
	s.audit(r, "host.create", h.Remark, h.Address)
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
	s.audit(r, "host.update", got.Remark, got.Address)
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
	s.audit(r, "host.delete", h.Remark, h.Address)
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
