package api

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

type groupInput struct {
	Name       string  `json:"name"`
	InboundIDs []int64 `json:"inbound_ids"`
	Disabled   bool    `json:"disabled"`
}

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.store.ListGroups(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

// groupFromInput validates the input; unknown inbound ids are rejected.
func (s *Server) groupFromInput(w http.ResponseWriter, r *http.Request) (*model.Group, bool) {
	var in groupInput
	if !decode(w, r, &in) {
		return nil, false
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 64 {
		writeError(w, http.StatusBadRequest, "name must be 1-64 characters")
		return nil, false
	}
	inbounds, err := s.store.ListInbounds(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return nil, false
	}
	known := map[int64]bool{}
	for _, ib := range inbounds {
		known[ib.ID] = true
	}
	for _, id := range in.InboundIDs {
		if !known[id] {
			writeError(w, http.StatusBadRequest, "unknown inbound")
			return nil, false
		}
	}
	if in.InboundIDs == nil {
		in.InboundIDs = []int64{}
	}
	return &model.Group{Name: in.Name, InboundIDs: in.InboundIDs, Disabled: in.Disabled}, true
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	g, ok := s.groupFromInput(w, r)
	if !ok {
		return
	}
	if err := s.store.CreateGroup(r.Context(), g); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "group.create", g.Name, "")
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	g, ok := s.groupFromInput(w, r)
	if !ok {
		return
	}
	g.ID = id
	if err := s.store.UpdateGroup(r.Context(), g); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "group.update", g.Name, "")
	s.usersChanged()
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	g, err := s.store.GetGroup(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	// A user left without groups would fall back to every inbound.
	if g.Users > 0 {
		writeError(w, http.StatusConflict, "the group has users: move them to another group first")
		return
	}
	if err := s.store.DeleteGroup(r.Context(), id); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "group.delete", g.Name, "")
	w.WriteHeader(http.StatusNoContent)
}
