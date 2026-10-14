package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/store"
)

// Core configurations, as in PasarGuard: each is a named set of inbounds,
// and every node runs one of them.

func (s *Server) handleListCores(w http.ResponseWriter, r *http.Request) {
	cores, err := s.store.ListCores(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cores)
}

func coreName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return "", false
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 64 {
		writeError(w, http.StatusBadRequest, "core name is required (up to 64 characters)")
		return "", false
	}
	return name, true
}

func (s *Server) handleCreateCore(w http.ResponseWriter, r *http.Request) {
	name, ok := coreName(w, r)
	if !ok {
		return
	}
	c := &model.Core{Name: name}
	if err := s.store.CreateCore(r.Context(), c); err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "core.create", c.Name, "")
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleUpdateCore(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	name, ok := coreName(w, r)
	if !ok {
		return
	}
	if err := s.store.RenameCore(r.Context(), id, name); err != nil {
		s.writeStoreError(w, err)
		return
	}
	c, err := s.store.GetCore(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "core.update", c.Name, "")
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteCore(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	c, err := s.store.GetCore(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	if err := s.store.DeleteCore(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrDefaultCore) {
			writeError(w, http.StatusBadRequest, "the default (first) core cannot be deleted")
			return
		}
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "core.delete", c.Name, "")
	s.usersChanged()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDuplicateCore(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	c, err := s.store.DuplicateCore(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.audit(r, "core.create", c.Name, "copy")
	writeJSON(w, http.StatusCreated, c)
}
