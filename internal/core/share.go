package core

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

// Several panel inbounds may sit on one port of a node, like hosts in
// Marzban/PasarGuard: when their server side is the same (protocol,
// transport, TLS/Reality keys, exit) the core gets one inbound serving all
// their users, and each panel inbound still gives its own link with its
// own address, remark and SNI.

// ShareKey identifies the server side of an Xray inbound; inbounds with the
// same key share one listener. Empty for protocols Xray does not serve.
func ShareKey(in *model.Inbound) string {
	if in.Protocol.Core() != "xray" {
		return ""
	}
	x, err := xrayInbound(in, nil)
	if err != nil {
		return ""
	}
	delete(x, "tag")
	delete(x["settings"].(map[string]any), "clients")
	// names and short ids only pick clients in; the merged inbound accepts all
	st := x["streamSettings"].(map[string]any)
	if t, ok := st["tlsSettings"].(map[string]any); ok {
		delete(t, "serverName")
	}
	if r, ok := st["realitySettings"].(map[string]any); ok {
		delete(r, "serverNames")
		delete(r, "shortIds")
	}
	if ex := in.Settings.ExitNodeID; ex != nil {
		x["exit"] = *ex // traffic is routed by inbound, so the exit must match
	}
	b, _ := json.Marshal(x)
	return string(b)
}

func wildcard(listen string) bool {
	return listen == "" || listen == "0.0.0.0" || listen == "::"
}

// sharesTransport reports whether two protocols would listen on the same
// kind of socket (TCP or UDP); shadowsocks takes both.
func sharesTransport(a, b model.Protocol) bool {
	tcp := func(p model.Protocol) bool { return p.Core() == "xray" }
	udp := func(p model.Protocol) bool { return p.Core() != "xray" || p == model.ProtoShadowsocks }
	return (tcp(a) && tcp(b)) || (udp(a) && udp(b))
}

// PortClash reports whether two inbounds of one node cannot both run: the
// same port and kind of socket on overlapping addresses, without a server
// side they could share.
func PortClash(a, b *model.Inbound) bool {
	if a.Port != b.Port || !sharesTransport(a.Protocol, b.Protocol) {
		return false
	}
	if a.Listen != b.Listen && !wildcard(a.Listen) && !wildcard(b.Listen) {
		return false // different IPs of the server
	}
	if k := ShareKey(a); k != "" && k == ShareKey(b) {
		return false
	}
	return true
}

// mergeXray folds inbound x into the listener m serving the same port.
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
