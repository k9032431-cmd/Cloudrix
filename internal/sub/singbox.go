package sub

import (
	"encoding/json"
	"strings"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

// SingBox renders a sing-box (1.11+) client profile.
func SingBox(u *model.User, eps []Endpoint) ([]byte, error) {
	var outbounds, endpoints []map[string]any
	tags := []string{}
	seen := map[string]int{}
	for _, ep := range eps {
		ob := singboxOutbound(u, ep)
		if ob == nil {
			continue
		}
		tag := uniqueName(ep.Remark, seen)
		ob["tag"] = tag
		tags = append(tags, tag)
		if ob["type"] == "wireguard" {
			endpoints = append(endpoints, ob)
		} else {
			outbounds = append(outbounds, ob)
		}
	}

	groups := []map[string]any{}
	if len(tags) > 0 {
		groups = append(groups,
			map[string]any{"type": "selector", "tag": "proxy", "outbounds": append([]string{"auto"}, tags...), "default": "auto"},
			map[string]any{"type": "urltest", "tag": "auto", "outbounds": tags, "url": "https://www.gstatic.com/generate_204", "interval": "5m"},
		)
	} else {
		groups = append(groups, map[string]any{"type": "selector", "tag": "proxy", "outbounds": []string{"direct"}})
	}
	all := append(groups, outbounds...)
	all = append(all, map[string]any{"type": "direct", "tag": "direct"})

	profile := map[string]any{
		"log": map[string]any{"level": "warn"},
		"inbounds": []map[string]any{
			{"type": "tun", "tag": "tun-in", "address": []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"}, "auto_route": true, "strict_route": true, "stack": "mixed"},
			{"type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1", "listen_port": 2080},
		},
		"outbounds": all,
		"route": map[string]any{
			"auto_detect_interface": true,
			"final":                 "proxy",
			"rules": []map[string]any{
				{"action": "sniff"},
				{"protocol": "dns", "action": "hijack-dns"},
				{"ip_is_private": true, "outbound": "direct"},
			},
		},
	}
	if len(endpoints) > 0 {
		profile["endpoints"] = endpoints
	}
	return json.MarshalIndent(profile, "", "  ")
}

func singboxOutbound(u *model.User, ep Endpoint) map[string]any {
	in, s, c := ep.Inbound, ep.Inbound.Settings, u.Credentials
	ob := map[string]any{"server": ep.Host, "server_port": in.Port}

	switch in.Protocol {
	case model.ProtoVLESS:
		if network(s) == "xhttp" {
			return nil // sing-box has no xhttp transport
		}
		ob["type"] = "vless"
		ob["uuid"] = c.UUID
		if s.Flow != "" && network(s) == "tcp" && (s.Security == "tls" || s.Security == "reality") {
			ob["flow"] = s.Flow
		}
		singboxTransport(ob, s)
		singboxTLS(ob, s)
	case model.ProtoVMess:
		if network(s) == "xhttp" {
			return nil
		}
		ob["type"] = "vmess"
		ob["uuid"] = c.UUID
		ob["security"] = "auto"
		singboxTransport(ob, s)
		singboxTLS(ob, s)
	case model.ProtoTrojan:
		if network(s) == "xhttp" {
			return nil
		}
		ob["type"] = "trojan"
		ob["password"] = c.Password
		singboxTransport(ob, s)
		singboxTLS(ob, s)
	case model.ProtoShadowsocks:
		ob["type"] = "shadowsocks"
		ob["method"] = SSMethod(s)
		ob["password"] = SSClientPassword(s, c.Password)
	case model.ProtoHysteria2:
		ob["type"] = "hysteria2"
		ob["password"] = c.Password
		if s.Obfs != "" {
			ob["obfs"] = map[string]any{"type": s.Obfs, "password": s.ObfsPassword}
		}
		ob["tls"] = quicTLS(s, nil)
	case model.ProtoTUIC:
		ob["type"] = "tuic"
		ob["uuid"] = c.UUID
		ob["password"] = c.Password
		ob["congestion_control"] = orDefault(s.CongestionCtrl, "bbr")
		ob["udp_relay_mode"] = "native"
		ob["tls"] = quicTLS(s, []string{"h3"})
	case model.ProtoWireGuard:
		addr, err := WGAddress(s.WGNetwork, u.ID)
		if err != nil {
			return nil
		}
		ep := map[string]any{
			"type":        "wireguard",
			"address":     []string{addr},
			"private_key": c.WGPrivateKey,
			"peers": []map[string]any{{
				"address": ep.Host, "port": in.Port, "public_key": s.WGPublicKey,
				"allowed_ips": []string{"0.0.0.0/0", "::/0"}, "persistent_keepalive_interval": 25,
			}},
		}
		if s.MTU > 0 {
			ep["mtu"] = s.MTU
		}
		return ep
	default:
		return nil
	}
	return ob
}

func singboxTransport(ob map[string]any, s model.InboundSettings) {
	switch network(s) {
	case "ws":
		t := map[string]any{"type": "ws", "path": orDefault(s.Path, "/")}
		if s.Host != "" {
			t["headers"] = map[string]string{"Host": s.Host}
		}
		ob["transport"] = t
	case "httpupgrade":
		ob["transport"] = map[string]any{"type": "httpupgrade", "path": orDefault(s.Path, "/"), "host": s.Host}
	case "grpc":
		ob["transport"] = map[string]any{"type": "grpc", "service_name": s.ServiceName}
	case "tcp":
		if s.HeaderType == "http" {
			t := map[string]any{"type": "http", "path": orDefault(s.Path, "/")}
			if s.Host != "" {
				t["host"] = strings.Split(s.Host, ",")
			}
			ob["transport"] = t
		}
	}
}

func singboxTLS(ob map[string]any, s model.InboundSettings) {
	switch s.Security {
	case "tls":
		t := map[string]any{"enabled": true, "insecure": s.AllowInsecure}
		if s.SNI != "" {
			t["server_name"] = s.SNI
		}
		if len(s.ALPN) > 0 {
			t["alpn"] = s.ALPN
		}
		if s.Fingerprint != "" {
			t["utls"] = map[string]any{"enabled": true, "fingerprint": s.Fingerprint}
		}
		ob["tls"] = t
	case "reality":
		sni := s.SNI
		if sni == "" && len(s.RealityServerNames) > 0 {
			sni = s.RealityServerNames[0]
		}
		shortID := ""
		if len(s.RealityShortIDs) > 0 {
			shortID = s.RealityShortIDs[0]
		}
		ob["tls"] = map[string]any{
			"enabled":     true,
			"server_name": sni,
			"utls":        map[string]any{"enabled": true, "fingerprint": orDefault(s.Fingerprint, "chrome")},
			"reality":     map[string]any{"enabled": true, "public_key": s.RealityPublicKey, "short_id": shortID},
		}
	}
}

func quicTLS(s model.InboundSettings, defaultALPN []string) map[string]any {
	t := map[string]any{"enabled": true, "insecure": s.AllowInsecure}
	if s.SNI != "" {
		t["server_name"] = s.SNI
	}
	if alpn := orDefaultSlice(s.ALPN, defaultALPN); len(alpn) > 0 {
		t["alpn"] = alpn
	}
	return t
}
