package api

import (
	"context"
	"encoding/base64"
	"slices"

	"github.com/k9032431-cmd/cloudrix/internal/auth"
	"github.com/k9032431-cmd/cloudrix/internal/core"
	"github.com/k9032431-cmd/cloudrix/internal/model"
)

// Cascades: an inbound on an entry node can send its traffic out through
// another (exit) node. The entry node's Xray dials the exit node over
// VLESS + Reality (the "bridge"), so the exit node's IP is what sites see.

// bridgePorts are tried in order when an exit node needs a bridge port.
var bridgePorts = []int{4443, 4444, 4445, 8444, 9443, 10443, 20443}

// ensureBridge gives the exit node its bridge keys and a free port once.
func (s *Server) ensureBridge(ctx context.Context, n *model.Node, inbounds []*model.Inbound) error {
	if n.Bridge.UUID != "" && n.Bridge.Port > 0 {
		return nil
	}
	b := n.Bridge
	if b.UUID == "" {
		b.UUID = auth.NewUUID()
		b.PrivateKey, b.PublicKey = auth.X25519KeyPair(base64.RawURLEncoding)
		b.ShortID = auth.RandomHex(8)
	}
	if b.Port == 0 {
		used := map[int]bool{}
		for _, in := range inbounds {
			if in.NodeID != nil && *in.NodeID == n.ID {
				used[in.Port] = true
			}
		}
		for _, p := range bridgePorts {
			if !used[p] {
				b.Port = p
				break
			}
		}
	}
	if err := s.store.SetNodeBridge(ctx, n.ID, b); err != nil {
		return err
	}
	n.Bridge = b
	return nil
}

// cascadeFor builds the bridge (if other nodes exit through this one) and
// the exits (if this node's inbounds leave through others) for node id.
func (s *Server) cascadeFor(ctx context.Context, id int64, inbounds []*model.Inbound) (core.Cascade, error) {
	var c core.Cascade
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return c, err
	}
	byID := map[int64]*model.Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	exits := map[int64]*core.Exit{}
	servesBridge := false
	for _, in := range inbounds {
		ex := in.Settings.ExitNodeID
		if !in.Enabled || in.NodeID == nil || ex == nil || in.Protocol.Core() != "xray" {
			continue
		}
		if *ex == id && *in.NodeID != id {
			servesBridge = true
		}
		if *in.NodeID != id {
			continue
		}
		e := exits[*ex]
		if e == nil {
			e = &core.Exit{NodeID: *ex}
			exits[*ex] = e
			// a missing or disabled exit node blocks the traffic instead of
			// letting it out through the entry node's IP
			if n := byID[*ex]; n != nil && n.Status != model.NodeDisabled {
				if err := s.ensureBridge(ctx, n, inbounds); err != nil {
					return c, err
				}
				e.Address, e.Bridge = n.Address, toCoreBridge(n.Bridge)
			}
		}
		e.Inbounds = append(e.Inbounds, in.Tag)
	}
	if servesBridge {
		if n := byID[id]; n != nil {
			if err := s.ensureBridge(ctx, n, inbounds); err != nil {
				return c, err
			}
			b := toCoreBridge(n.Bridge)
			c.Bridge = &b
		}
	}
	for _, e := range exits {
		c.Exits = append(c.Exits, *e)
	}
	slices.SortFunc(c.Exits, func(a, b core.Exit) int { return int(a.NodeID - b.NodeID) })
	return c, nil
}

func toCoreBridge(b model.NodeBridge) core.Bridge {
	return core.Bridge{Port: b.Port, UUID: b.UUID, PrivateKey: b.PrivateKey, PublicKey: b.PublicKey, ShortID: b.ShortID, SNI: core.BridgeSNI}
}
