package api

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/k9032431-cmd/cloudrix/internal/core"
	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/nodeproto"
)

// hysteriaTag names the inbound the panel keeps for a node's installer-made
// Hysteria2 setup; running the installer again updates this inbound.
func hysteriaTag(nodeID int64) string { return fmt.Sprintf("hysteria2-node%d", nodeID) }

// ensureHysteria creates or updates the node's Hysteria2 inbound from what
// the installer set up there: a domain pointed at the node and a real
// Let's Encrypt certificate, so clients verify it (no "insecure").
func (s *Server) ensureHysteria(ctx context.Context, node *model.Node, h nodeproto.Hysteria) {
	domain := strings.ToLower(strings.TrimSpace(h.Domain))
	if !hostRe.MatchString(domain) || h.Port < 1 || h.Port > 65535 || !strings.HasPrefix(h.Cert, "/") || !strings.HasPrefix(h.Key, "/") {
		return
	}
	inbounds, err := s.store.ListInbounds(ctx)
	if err != nil {
		return
	}
	tag := hysteriaTag(node.ID)
	var cur *model.Inbound
	for _, in := range inbounds {
		if in.Tag == tag {
			cur = in
			continue
		}
		// a port taken by another UDP inbound would stop the node's sing-box
		if in.Runs(node) && core.PortClash(in, &model.Inbound{Protocol: model.ProtoHysteria2, Listen: "0.0.0.0", Port: h.Port}) {
			s.log.Warn("hysteria2 setup: port in use", "node", node.Name, "port", h.Port, "inbound", in.Tag)
			return
		}
	}
	want := model.Inbound{
		// in the node's core, pinned to the node: the certificate is only there
		Tag: tag, Protocol: model.ProtoHysteria2, Listen: "0.0.0.0", Port: h.Port, CoreID: node.CoreID, NodeID: &node.ID, Enabled: true,
		Address: domain, Remark: "{USERNAME} · Hysteria2",
		Settings: model.InboundSettings{SNI: domain, CertFile: h.Cert, KeyFile: h.Key, ALPN: []string{"h3"}},
	}
	if cur != nil {
		s2 := cur.Settings
		if cur.Port == want.Port && cur.CoreID == node.CoreID && cur.Address == domain && s2.SNI == domain && s2.CertFile == h.Cert && s2.KeyFile == h.Key {
			return // up to date
		}
		// keep what the admin changed (name, obfs, enabled); refresh the rest
		want.ID, want.Remark, want.Enabled = cur.ID, cur.Remark, cur.Enabled
		want.Settings.Obfs, want.Settings.ObfsPassword = s2.Obfs, s2.ObfsPassword
		want.Settings.ExitNodeID = s2.ExitNodeID
	}
	if err := validateInbound(&want); err != nil {
		s.log.Warn("hysteria2 setup", "node", node.Name, "err", err)
		return
	}
	action := "inbound.update"
	if cur != nil {
		if err = s.store.UpdateInbound(ctx, &want); err == nil {
			// hosts that pointed at the old domain follow it
			for _, h := range cur.Hosts {
				if slices.Contains(h.Address, cur.Address) || slices.Contains(h.SNI, cur.Settings.SNI) {
					h.Address, h.SNI = []string{domain}, []string{domain}
					if herr := s.store.UpdateHost(ctx, &h); herr != nil {
						s.log.Warn("hysteria2 host", "node", node.Name, "err", herr)
					}
				}
			}
		}
	} else {
		action = "inbound.create"
		if err = s.store.CreateInbound(ctx, &want); err == nil {
			// new inbounds go to the first group (Default), like the form does
			if groups, gerr := s.store.ListGroups(ctx); gerr == nil && len(groups) > 0 {
				err = s.store.SetInboundGroups(ctx, want.ID, []int64{groups[0].ID})
			}
		}
	}
	if err != nil {
		s.log.Warn("hysteria2 setup", "node", node.Name, "err", err)
		return
	}
	e := &model.AuditEntry{Admin: "system", Action: action, Target: tag,
		Details: fmt.Sprintf("Hysteria2 on %s:%d (node %s, set up by the installer)", domain, h.Port, node.Name)}
	if err := s.store.AddAudit(ctx, e); err != nil {
		s.log.Warn("audit write failed", "err", err)
	}
	s.usersChanged()
}
