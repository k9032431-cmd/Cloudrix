// Package core builds server-side configs for the proxy cores (Xray and sing-box)
// from the panel's inbounds and active users.
package core

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/sub"
)

// Email is the per-user identifier cores use for stats; the id prefix keeps it
// stable across username changes.
func Email(u *model.User) string {
	return fmt.Sprintf("%d.%s", u.ID, u.Username)
}

// usersFor returns active users allowed on the inbound.
func usersFor(in *model.Inbound, users []*model.User) []*model.User {
	var out []*model.User
	for _, u := range users {
		if !u.Active() {
			continue
		}
		if len(u.Inbounds) > 0 && !contains(u.Inbounds, in.Tag) {
			continue
		}
		out = append(out, u)
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// ForNode filters inbounds served by the given node (nil = local core).
func ForNode(inbounds []*model.Inbound, nodeID *int64) []*model.Inbound {
	var out []*model.Inbound
	for _, in := range inbounds {
		if !in.Enabled {
			continue
		}
		if (nodeID == nil && in.NodeID == nil) || (nodeID != nil && in.NodeID != nil && *nodeID == *in.NodeID) {
			out = append(out, in)
		}
	}
	return out
}

// Xray renders an Xray-core config for the vless/vmess/trojan/shadowsocks inbounds.
func Xray(inbounds []*model.Inbound, users []*model.User) ([]byte, error) {
	xin := []map[string]any{{
		"tag": "api", "listen": "127.0.0.1", "port": 10085, "protocol": "dokodemo-door",
		"settings": map[string]any{"address": "127.0.0.1"},
	}}
	for _, in := range inbounds {
		if in.Protocol.Core() != "xray" {
			continue
		}
		x, err := xrayInbound(in, usersFor(in, users))
		if err != nil {
			return nil, fmt.Errorf("inbound %s: %w", in.Tag, err)
		}
		xin = append(xin, x)
	}
	cfg := map[string]any{
		"log":      map[string]any{"loglevel": "warning"},
		"api":      map[string]any{"tag": "api", "services": []string{"HandlerService", "StatsService", "LoggerService"}},
		"stats":    map[string]any{},
		"policy":   map[string]any{"levels": map[string]any{"0": map[string]any{"statsUserUplink": true, "statsUserDownlink": true, "statsUserOnline": true}}, "system": map[string]any{"statsInboundUplink": true, "statsInboundDownlink": true}},
		"inbounds": xin,
		"outbounds": []map[string]any{
			{"tag": "direct", "protocol": "freedom"},
			{"tag": "block", "protocol": "blackhole"},
		},
		"routing": map[string]any{
			"rules": []map[string]any{
				{"inboundTag": []string{"api"}, "outboundTag": "api"},
				{"ip": []string{"geoip:private"}, "outboundTag": "block"},
				{"protocol": []string{"bittorrent"}, "outboundTag": "block"},
			},
		},
	}
	return json.MarshalIndent(cfg, "", "  ")
}

func xrayInbound(in *model.Inbound, users []*model.User) (map[string]any, error) {
	s := in.Settings
	clients := []map[string]any{}
	settings := map[string]any{}

	switch in.Protocol {
	case model.ProtoVLESS:
		for _, u := range users {
			c := map[string]any{"id": u.Credentials.UUID, "email": Email(u)}
			if s.Flow != "" && orDefault(s.Network, "tcp") == "tcp" {
				c["flow"] = s.Flow
			}
			clients = append(clients, c)
		}
		settings["clients"] = clients
		settings["decryption"] = "none"
	case model.ProtoVMess:
		for _, u := range users {
			clients = append(clients, map[string]any{"id": u.Credentials.UUID, "email": Email(u)})
		}
		settings["clients"] = clients
	case model.ProtoTrojan:
		for _, u := range users {
			clients = append(clients, map[string]any{"password": u.Credentials.Password, "email": Email(u)})
		}
		settings["clients"] = clients
	case model.ProtoShadowsocks:
		method := sub.SSMethod(s)
		for _, u := range users {
			c := map[string]any{"password": sub.SSUserKey(method, u.Credentials.Password), "email": Email(u)}
			if !strings.HasPrefix(method, "2022-") {
				c["method"] = method
			}
			clients = append(clients, c)
		}
		settings["clients"] = clients
		settings["network"] = "tcp,udp"
		if strings.HasPrefix(method, "2022-") {
			if s.SSServerKey == "" {
				return nil, fmt.Errorf("shadowsocks 2022 requires ss_server_key")
			}
			settings["method"] = method
			settings["password"] = s.SSServerKey
		}
	default:
		return nil, fmt.Errorf("protocol %s is not served by xray", in.Protocol)
	}

	return map[string]any{
		"tag":            in.Tag,
		"listen":         orDefault(in.Listen, "0.0.0.0"),
		"port":           in.Port,
		"protocol":       string(in.Protocol),
		"settings":       settings,
		"streamSettings": xrayStream(s),
		"sniffing":       map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}},
	}, nil
}

func xrayStream(s model.InboundSettings) map[string]any {
	network := orDefault(s.Network, "tcp")
	st := map[string]any{"network": network}
	switch network {
	case "ws":
		ws := map[string]any{"path": orDefault(s.Path, "/")}
		if s.Host != "" {
			ws["host"] = s.Host
		}
		st["wsSettings"] = ws
	case "httpupgrade":
		st["httpupgradeSettings"] = map[string]any{"path": orDefault(s.Path, "/"), "host": s.Host}
	case "xhttp":
		st["xhttpSettings"] = map[string]any{"path": orDefault(s.Path, "/"), "host": s.Host, "mode": orDefault(s.XHTTPMode, "auto")}
	case "grpc":
		st["grpcSettings"] = map[string]any{"serviceName": s.ServiceName}
	case "tcp":
		if s.HeaderType == "http" {
			st["tcpSettings"] = map[string]any{"header": map[string]any{"type": "http", "request": map[string]any{
				"path": []string{orDefault(s.Path, "/")}, "headers": map[string]any{"Host": strings.Split(orDefault(s.Host, ""), ",")},
			}}}
		}
	}
	switch s.Security {
	case "tls":
		st["security"] = "tls"
		tls := map[string]any{"certificates": []map[string]any{{"certificateFile": s.CertFile, "keyFile": s.KeyFile}}}
		if s.SNI != "" {
			tls["serverName"] = s.SNI
		}
		if len(s.ALPN) > 0 {
			tls["alpn"] = s.ALPN
		}
		st["tlsSettings"] = tls
	case "reality":
		st["security"] = "reality"
		names := s.RealityServerNames
		if len(names) == 0 && s.SNI != "" {
			names = []string{s.SNI}
		}
		st["realitySettings"] = map[string]any{
			"show": false, "dest": s.RealityDest, "xver": 0, "serverNames": names,
			"privateKey": s.RealityPrivateKey, "shortIds": orDefaultSlice(s.RealityShortIDs, []string{""}),
		}
	default:
		st["security"] = "none"
	}
	return st
}

// SingBox renders a sing-box server config for hysteria2/tuic/wireguard inbounds.
func SingBox(inbounds []*model.Inbound, users []*model.User) ([]byte, error) {
	var sin, endpoints []map[string]any
	for _, in := range inbounds {
		if in.Protocol.Core() != "sing-box" {
			continue
		}
		s := in.Settings
		us := usersFor(in, users)
		base := map[string]any{"tag": in.Tag, "listen": orDefault(in.Listen, "::"), "listen_port": in.Port}
		tls := map[string]any{"enabled": true, "certificate_path": s.CertFile, "key_path": s.KeyFile}
		if s.SNI != "" {
			tls["server_name"] = s.SNI
		}

		switch in.Protocol {
		case model.ProtoHysteria2:
			list := []map[string]any{}
			for _, u := range us {
				list = append(list, map[string]any{"name": Email(u), "password": u.Credentials.Password})
			}
			base["type"] = "hysteria2"
			base["users"] = list
			base["tls"] = withALPN(tls, s.ALPN, []string{"h3"})
			if s.Obfs != "" {
				base["obfs"] = map[string]any{"type": s.Obfs, "password": s.ObfsPassword}
			}
			if s.UpMbps > 0 {
				base["up_mbps"] = s.UpMbps
			}
			if s.DownMbps > 0 {
				base["down_mbps"] = s.DownMbps
			}
			sin = append(sin, base)
		case model.ProtoTUIC:
			list := []map[string]any{}
			for _, u := range us {
				list = append(list, map[string]any{"name": Email(u), "uuid": u.Credentials.UUID, "password": u.Credentials.Password})
			}
			base["type"] = "tuic"
			base["users"] = list
			base["congestion_control"] = orDefault(s.CongestionCtrl, "bbr")
			base["tls"] = withALPN(tls, s.ALPN, []string{"h3"})
			sin = append(sin, base)
		case model.ProtoWireGuard:
			addr, err := sub.WGServerAddress(s.WGNetwork)
			if err != nil {
				return nil, fmt.Errorf("inbound %s: %w", in.Tag, err)
			}
			peers := []map[string]any{}
			for _, u := range us {
				ip, err := sub.WGAddress(s.WGNetwork, u.ID)
				if err != nil {
					return nil, fmt.Errorf("inbound %s: %w", in.Tag, err)
				}
				peers = append(peers, map[string]any{"public_key": u.Credentials.WGPublicKey, "allowed_ips": []string{ip}})
			}
			ep := map[string]any{
				"type": "wireguard", "tag": in.Tag, "address": []string{addr}, "listen_port": in.Port,
				"private_key": s.WGPrivateKey, "peers": peers,
			}
			if s.MTU > 0 {
				ep["mtu"] = s.MTU
			}
			endpoints = append(endpoints, ep)
		}
	}
	cfg := map[string]any{
		"log":       map[string]any{"level": "warn"},
		"inbounds":  orEmpty(sin),
		"outbounds": []map[string]any{{"type": "direct", "tag": "direct"}},
		"route": map[string]any{"rules": []map[string]any{
			{"ip_is_private": true, "action": "reject"},
		}},
	}
	if len(endpoints) > 0 {
		cfg["endpoints"] = endpoints
	}
	return json.MarshalIndent(cfg, "", "  ")
}

func withALPN(tls map[string]any, alpn, def []string) map[string]any {
	tls["alpn"] = orDefaultSlice(alpn, def)
	return tls
}

func orEmpty(v []map[string]any) []map[string]any {
	if v == nil {
		return []map[string]any{}
	}
	return v
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func orDefaultSlice(v, def []string) []string {
	if len(v) == 0 {
		return def
	}
	return v
}
