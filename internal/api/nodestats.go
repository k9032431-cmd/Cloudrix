package api

import (
	"net/http"
	"strconv"

	"github.com/k9032431-cmd/cloudrix/internal/store"
)

const (
	nodeStatsDays     = 400 // node totals per day are kept this long
	nodeUserStatsDays = 120 // users per node and day are kept this long
)

// statsDays reads ?days= (1..365, default 30).
func statsDays(r *http.Request) int {
	d, err := strconv.Atoi(r.URL.Query().Get("days"))
	if err != nil || d < 1 {
		return 30
	}
	return min(d, 365)
}

type nodeUsage struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Up     int64  `json:"up"`
	Down   int64  `json:"down"`
	Users  int    `json:"users"`  // users who went through the node in the period
	Online int    `json:"online"` // users online on it now
}

// handleNodesStats compares the nodes over the period and gives their
// combined daily traffic (twice the period, for the previous-period line).
func (s *Server) handleNodesStats(w http.ResponseWriter, r *http.Request) {
	days := statsDays(r)
	nodes, err := s.store.ListNodes(r.Context())
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	totals, err := s.store.NodeTotals(r.Context(), days)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	history, err := s.store.NodeTrafficHistory(r.Context(), nil, days*2)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	online := s.devices.onlineByNode()
	out := make([]nodeUsage, 0, len(nodes))
	for _, n := range nodes {
		t := totals[n.ID]
		out = append(out, nodeUsage{ID: n.ID, Name: n.Name, Status: string(n.Status), Up: t.Up, Down: t.Down, Users: t.Users, Online: online[n.ID]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": days, "nodes": out, "history": history})
}

// handleNodeStats details one node: daily traffic (twice the period), totals,
// users online now and the users who used it most.
func (s *Server) handleNodeStats(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	days := statsDays(r)
	n, err := s.store.GetNode(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	history, err := s.store.NodeTrafficHistory(r.Context(), &id, days*2)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	totals, err := s.store.NodeTotals(r.Context(), days)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	top, err := s.store.NodeTopUsers(r.Context(), id, days, 10)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	t := totals[id]
	writeJSON(w, http.StatusOK, struct {
		Days     int                   `json:"days"`
		Node     nodeUsage             `json:"node"`
		History  []store.NodeDaily     `json:"history"`
		TopUsers []store.NodeUserUsage `json:"top_users"`
	}{days, nodeUsage{ID: n.ID, Name: n.Name, Status: string(n.Status), Up: t.Up, Down: t.Down, Users: t.Users, Online: s.devices.onlineByNode()[id]}, history, top})
}
