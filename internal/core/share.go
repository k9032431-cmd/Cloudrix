package core

import (
	"fmt"
	"slices"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

// Several panel inbounds of one protocol may sit on one port of a node, like
// hosts in Marzban/PasarGuard. The first of them (lowest id) owns the server
// side: transport, TLS/Reality keys, flow, exit. The others take it over and
// keep their own address, remark, SNI and fingerprint, so each gives its own
// link while the core runs one listener for all their users.

func wildcard(listen string) bool {
	return listen == "" || listen == "0.0.0.0" || listen == "::"
}

func sameListen(a, b string) bool {
	return a == b || (wildcard(a) && wildcard(b))
}

// Hosted reports whether two inbounds of one node share a listener.
func Hosted(a, b *model.Inbound) bool {
	return a.Port == b.Port && a.Protocol == b.Protocol && a.Protocol.Core() == "xray" && UDP(a) == UDP(b) && sameListen(a.Listen, b.Listen)
}

// UDP reports whether the inbound's main socket is UDP: sing-box protocols
// (hysteria2, tuic, wireguard) and Xray mKCP. Shadowsocks also takes UDP but
// leaves it to such an inbound on the same port (see Xray), so it counts as
// TCP here.
func UDP(in *model.Inbound) bool {
	return in.Protocol.Core() != "xray" || in.Protocol == model.ProtoHysteria2 ||
		(in.Protocol != model.ProtoShadowsocks && in.Settings.Network == "kcp")
}

// sharesTransport reports whether two inbounds would listen on the same kind
// of socket (TCP or UDP).
func sharesTransport(a, b *model.Inbound) bool {
	return UDP(a) == UDP(b)
}

// PortClash reports whether two inbounds of one node cannot both run: the
// same port and kind of socket on overlapping addresses, not as hosts of one
// listener.
func PortClash(a, b *model.Inbound) bool {
	if a.Port != b.Port || !sharesTransport(a, b) {
		return false
	}
	if a.Listen != b.Listen && !wildcard(a.Listen) && !wildcard(b.Listen) {
		return false // different IPs of the server
	}
	return !Hosted(a, b)
}

// AdoptServer gives dst the server side of src; it reports whether anything
// changed.
func AdoptServer(dst, src *model.Inbound) bool {
	d, s := &dst.Settings, src.Settings
	before := fmt.Sprint(dst.Listen, *d, deref(d.ExitNodeID))
	dst.Listen = src.Listen
	d.Network, d.Path, d.Host, d.ServiceName, d.HeaderType, d.XHTTPMode = s.Network, s.Path, s.Host, s.ServiceName, s.HeaderType, s.XHTTPMode
	d.Security, d.ALPN, d.CertFile, d.KeyFile, d.AllowInsecure = s.Security, slices.Clone(s.ALPN), s.CertFile, s.KeyFile, s.AllowInsecure
	d.RealityDest, d.RealityPrivateKey, d.RealityPublicKey = s.RealityDest, s.RealityPrivateKey, s.RealityPublicKey
	d.Flow, d.SSMethod, d.SSServerKey = s.Flow, s.SSMethod, s.SSServerKey
	d.Obfs, d.ObfsPassword, d.UpMbps, d.DownMbps, d.Masquerade, d.UDPTimeout = s.Obfs, s.ObfsPassword, s.UpMbps, s.DownMbps, s.Masquerade, s.UDPTimeout
	d.ExitNodeID = nil
	if s.ExitNodeID != nil {
		v := *s.ExitNodeID
		d.ExitNodeID = &v
	}
	if d.Security == "reality" && len(d.RealityShortIDs) == 0 {
		d.RealityShortIDs = slices.Clone(s.RealityShortIDs)
	}
	return fmt.Sprint(dst.Listen, *d, deref(d.ExitNodeID)) != before
}

func deref(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

// Together reports whether two inbounds can run on the same node: one core,
// and no pins to two different nodes.
func Together(a, b *model.Inbound) bool {
	return a.CoreID == b.CoreID && (a.NodeID == nil || b.NodeID == nil || *a.NodeID == *b.NodeID)
}

// Owner returns the inbound owning the listener in would join (the lowest
// id among the others hosted with it), or nil.
func Owner(in *model.Inbound, all []*model.Inbound) *model.Inbound {
	var owner *model.Inbound
	for _, o := range all {
		if o.ID != in.ID && Together(o, in) && Hosted(o, in) && (owner == nil || o.ID < owner.ID) {
			owner = o
		}
	}
	return owner
}

// Peers returns the other inbounds sharing in's listener.
func Peers(in *model.Inbound, all []*model.Inbound) []*model.Inbound {
	var out []*model.Inbound
	for _, o := range all {
		if o.ID != in.ID && Together(o, in) && Hosted(o, in) {
			out = append(out, o)
		}
	}
	return out
}

// mergeXray folds inbound x into the listener m serving the same port: its
// users, and the Reality names and short ids its links use.
func mergeXray(m, x map[string]any) {
	ms, xs := m["settings"].(map[string]any), x["settings"].(map[string]any)
	clients, _ := ms["clients"].([]map[string]any)
	have := map[any]bool{}
	for _, c := range clients {
		have[c["email"]] = true
	}
	more, _ := xs["clients"].([]map[string]any)
	for _, c := range more {
		if !have[c["email"]] {
			have[c["email"]] = true
			clients = append(clients, c)
		}
	}
	if clients != nil {
		ms["clients"] = clients
	}
	mst, xst := m["streamSettings"].(map[string]any), x["streamSettings"].(map[string]any)
	if t, ok := mst["tlsSettings"].(map[string]any); ok {
		if xt, _ := xst["tlsSettings"].(map[string]any); fmt.Sprint(t["serverName"]) != fmt.Sprint(xt["serverName"]) {
			delete(t, "serverName")
		}
	}
	if r, ok := mst["realitySettings"].(map[string]any); ok {
		xr, _ := xst["realitySettings"].(map[string]any)
		for _, k := range []string{"serverNames", "shortIds"} {
			a, _ := r[k].([]string)
			b, _ := xr[k].([]string)
			for _, v := range b {
				if !slices.Contains(a, v) {
					a = append(a, v)
				}
			}
			r[k] = a
		}
	}
}
