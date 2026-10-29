// Package core builds server-side configs for the proxy cores (Xray and sing-box)
// from the panel's inbounds and active users.
package core

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/sub"
)

// Self-signed TLS pair the node agent generates for hysteria2/tuic inbounds
// that have no certificate of their own.
const (
	SelfSignedCert = "/var/lib/cloudrix-node/tls/cert.pem"
	SelfSignedKey  = "/var/lib/cloudrix-node/tls/key.pem"
)

// SharedTLSDir holds certificates issued on the main server (installer menu
// «Сертификат домена»). Inbounds may point at files under it: the panel sends
// them to the node, which stores them at the same path.
var SharedTLSDir = "/etc/cloudrix/tls/" // tests point it at a temp dir

// Shared reports whether path is a main-server certificate file.
func Shared(path string) bool {
	return strings.HasPrefix(path, SharedTLSDir) && !strings.Contains(path, "..")
}

// Email is the per-user identifier cores use for stats; the id prefix keeps it
// stable across username changes.
func Email(u *model.User) string {
	return fmt.Sprintf("%d.%s", u.ID, u.Username)
}

// usersFor returns active users allowed on the inbound.
func usersFor(in *model.Inbound, users []*model.User, groups model.Groups) []*model.User {
	var out []*model.User
	for _, u := range users {
		if u.Active() && u.Allows(in, groups) {
			out = append(out, u)
		}
	}
	return out
}

// ForNode filters the inbounds the node serves: the enabled inbounds of its
// core (those pinned to another node aside).
func ForNode(inbounds []*model.Inbound, node *model.Node) []*model.Inbound {
	var out []*model.Inbound
	for _, in := range inbounds {
		if node != nil && in.Runs(node) {
			out = append(out, in)
		}
	}
	return out
}

// Xray renders an Xray-core config for the vless/vmess/trojan/shadowsocks inbounds.
func Xray(inbounds []*model.Inbound, users []*model.User, groups model.Groups, cascade Cascade) ([]byte, error) {
	xin := []map[string]any{{
		"tag": "api", "listen": "127.0.0.1", "port": 10085, "protocol": "dokodemo-door",
		"settings": map[string]any{"address": "127.0.0.1"},
	}}
	listener := map[string]int{} // protocol, listen and port → index in xin
	alias := map[string]string{} // tag of a hosted inbound → listener tag
	// UDP ports sing-box takes (hysteria2, tuic, wireguard): shadowsocks on
	// such a port listens on TCP only, so both run on one port number
	udpTaken := map[int]bool{}
	for _, in := range inbounds {
		if UDP(in) {
			udpTaken[in.Port] = true
		}
	}
	for _, in := range inbounds {
		if in.Protocol.Core() != "xray" {
			continue
		}
		if in.Settings.Security == "reality" && !RealityTransport(in.Settings.Network) {
			// Xray would refuse the node's whole config for this one inbound
			continue
		}
		x, err := xrayInbound(in, usersFor(in, users, groups))
		if err != nil {
			return nil, fmt.Errorf("inbound %s: %w", in.Tag, err)
		}
		if in.Protocol == model.ProtoShadowsocks && udpTaken[in.Port] {
			x["settings"].(map[string]any)["network"] = "tcp"
		}
		// inbounds of one protocol on one port share a listener; the first
		// (lowest id) one's server side counts
		listen := in.Listen
		if wildcard(listen) {
			listen = "*"
		}
		key := fmt.Sprint(in.Protocol, "|", listen, "|", in.Port, "|", UDP(in))
		if i, ok := listener[key]; ok {
			mergeXray(xin[i], x)
			alias[in.Tag] = xin[i]["tag"].(string)
			continue
		}
		listener[key] = len(xin)
		xin = append(xin, x)
	}
	cascade = cascade.renamed(alias)
	outbounds := []map[string]any{
		{"tag": "direct", "protocol": "freedom"},
		{"tag": "block", "protocol": "blackhole"},
	}
	rules := []map[string]any{
		{"inboundTag": []string{"api"}, "outboundTag": "api"},
		{"ip": []string{"geoip:private"}, "outboundTag": "block"},
		{"protocol": []string{"bittorrent"}, "outboundTag": "block"},
		// mail servers: spam sent through a VPN gets the server's IP
		// blocked by its hosting
		{"network": "tcp", "port": "25", "outboundTag": "block"},
	}
	cascade.apply(&xin, &outbounds, &rules)
	// No per-inbound counters: with them Xray's hysteria inbound loses the
	// user of a connection and counts no per-user traffic or online.
	cfg := map[string]any{
		"log":       map[string]any{"loglevel": "warning", "access": "none"},
		"api":       map[string]any{"tag": "api", "services": []string{"HandlerService", "StatsService", "LoggerService", "RoutingService"}},
		"stats":     map[string]any{},
		"policy":    map[string]any{"levels": map[string]any{"0": map[string]any{"statsUserUplink": true, "statsUserDownlink": true, "statsUserOnline": true}}},
		"inbounds":  xin,
		"outbounds": outbounds,
		"routing":   map[string]any{"rules": rules},
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
		addFallbacks(settings, s)
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
		addFallbacks(settings, s)
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
		settings["network"] = orDefault(s.SSNetwork, "tcp,udp")
		if strings.HasPrefix(method, "2022-") {
			if s.SSServerKey == "" {
				return nil, fmt.Errorf("shadowsocks 2022 requires ss_server_key")
			}
			settings["method"] = method
			settings["password"] = s.SSServerKey
		}
	case model.ProtoHysteria2:
		for _, u := range users {
			clients = append(clients, map[string]any{"auth": u.Credentials.Password, "email": Email(u)})
		}
		settings["version"] = 2
		settings["clients"] = clients
	default:
		return nil, fmt.Errorf("protocol %s is not served by xray", in.Protocol)
	}

	stream := xrayStream(s)
	protocol := string(in.Protocol)
	if in.Protocol == model.ProtoHysteria2 {
		stream, protocol = hysteriaStream(s), "hysteria"
	}
	return map[string]any{
		"tag":            in.Tag,
		"listen":         orDefault(in.Listen, "0.0.0.0"),
		"port":           in.Port,
		"protocol":       protocol,
		"settings":       settings,
		"streamSettings": stream,
		"sniffing":       sniffing(s.Sniffing),
	}, nil
}

// hysteriaStream is the Xray stream of a Hysteria2 inbound: QUIC with TLS
// (the node's self-signed certificate unless one is set), optional Brutal
// speeds, masquerade and salamander obfuscation.
func hysteriaStream(s model.InboundSettings) map[string]any {
	hy := map[string]any{"version": 2}
	if s.UpMbps > 0 {
		hy["up"] = fmt.Sprintf("%d mbps", s.UpMbps)
	}
	if s.DownMbps > 0 {
		hy["down"] = fmt.Sprintf("%d mbps", s.DownMbps)
	}
	if d, err := time.ParseDuration(s.UDPTimeout); err == nil && d > 0 {
		hy["udpIdleTimeout"] = int(d.Seconds())
	}
	if m := s.Masquerade; m != "" {
		if dir, ok := strings.CutPrefix(m, "file://"); ok {
			hy["masquerade"] = map[string]any{"type": "file", "dir": dir}
		} else {
			hy["masquerade"] = map[string]any{"type": "proxy", "url": m, "rewriteHost": true}
		}
	}
	certFile, keyFile := s.CertFile, s.KeyFile
	if certFile == "" || keyFile == "" {
		certFile, keyFile = SelfSignedCert, SelfSignedKey
	}
	st := map[string]any{
		"network": "hysteria", "hysteriaSettings": hy, "security": "tls",
		"tlsSettings": map[string]any{
			"certificates": []map[string]any{{"certificateFile": certFile, "keyFile": keyFile}},
			"alpn":         orDefaultSlice(s.ALPN, []string{"h3"}),
		},
	}
	if s.Obfs == "salamander" && s.ObfsPassword != "" {
		st["finalmask"] = map[string]any{"udp": []map[string]any{{"type": "salamander", "settings": map[string]any{"password": s.ObfsPassword}}}}
	}
	return st
}

func sniffing(sn *model.Sniffing) map[string]any {
	if sn == nil {
		return map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}}
	}
	out := map[string]any{"enabled": sn.Enabled}
	if sn.Enabled {
		out["destOverride"] = orDefaultSlice(sn.DestOverride, []string{"http", "tls", "quic"})
		if sn.MetadataOnly {
			out["metadataOnly"] = true
		}
		if sn.RouteOnly {
			out["routeOnly"] = true
		}
	}
	return out
}

// addFallbacks adds the fallbacks of a vless/trojan inbound on tcp.
func addFallbacks(settings map[string]any, s model.InboundSettings) {
	if len(s.Fallbacks) == 0 || orDefault(s.Network, "tcp") != "tcp" {
		return
	}
	var list []map[string]any
	for _, f := range s.Fallbacks {
		fb := map[string]any{"dest": f.Dest}
		if n, err := strconv.Atoi(f.Dest); err == nil {
			fb["dest"] = n
		}
		for k, v := range map[string]string{"name": f.Name, "alpn": f.ALPN, "path": f.Path} {
			if v != "" {
				fb[k] = v
			}
		}
		if f.Xver > 0 {
			fb["xver"] = f.Xver
		}
		list = append(list, fb)
	}
	settings["fallbacks"] = list
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
		if s.WSHeartbeat > 0 {
			ws["heartbeatPeriod"] = s.WSHeartbeat
		}
		st["wsSettings"] = ws
	case "kcp":
		// Xray 26 moved the mKCP header and seed to finalmask
		kcp := map[string]any{"congestion": s.KCPCongestion}
		for k, v := range map[string]int{"mtu": s.KCPMTU, "tti": s.KCPTTI, "uplinkCapacity": s.KCPUplink, "downlinkCapacity": s.KCPDownlink,
			"readBufferSize": s.KCPReadBuffer, "writeBufferSize": s.KCPWriteBuffer} {
			if v > 0 {
				kcp[k] = v
			}
		}
		st["kcpSettings"] = kcp
		st["finalmask"] = sub.KCPMask(s)
	case "httpupgrade":
		st["httpupgradeSettings"] = map[string]any{"path": orDefault(s.Path, "/"), "host": s.Host}
	case "xhttp":
		x := map[string]any{"path": orDefault(s.Path, "/"), "host": s.Host, "mode": orDefault(s.XHTTPMode, "auto")}
		extra := map[string]any{}
		if s.XPaddingBytes != "" {
			extra["xPaddingBytes"] = s.XPaddingBytes
		}
		if s.XNoSSEHeader {
			extra["noSSEHeader"] = true
		}
		if s.XNoGRPCHeader {
			extra["noGRPCHeader"] = true
		}
		if s.XScMaxEachPostBytes != "" {
			extra["scMaxEachPostBytes"] = s.XScMaxEachPostBytes
		}
		if s.XScMaxBufferedPosts > 0 {
			extra["scMaxBufferedPosts"] = s.XScMaxBufferedPosts
		}
		if s.XScStreamUpServerSec != "" {
			extra["scStreamUpServerSecs"] = s.XScStreamUpServerSec
		}
		if len(extra) > 0 {
			x["extra"] = extra
		}
		st["xhttpSettings"] = x
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
		certFile, keyFile := s.CertFile, s.KeyFile
		if certFile == "" || keyFile == "" {
			// the node agent creates this self-signed pair; clients pin its hash
			certFile, keyFile = SelfSignedCert, SelfSignedKey
		}
		tls := map[string]any{"certificates": []map[string]any{{"certificateFile": certFile, "keyFile": keyFile}}}
		if s.SNI != "" {
			tls["serverName"] = s.SNI
		}
		if len(s.ALPN) > 0 {
			tls["alpn"] = s.ALPN
		}
		for k, v := range map[string]string{"minVersion": s.TLSMinVersion, "maxVersion": s.TLSMaxVersion, "cipherSuites": s.TLSCipherSuites} {
			if v != "" {
				tls[k] = v
			}
		}
		if s.TLSRejectUnknownSNI {
			tls["rejectUnknownSni"] = true
		}
		st["tlsSettings"] = tls
	case "reality":
		st["security"] = "reality"
		names := s.RealityServerNames
		if len(names) == 0 && s.SNI != "" {
			names = []string{s.SNI}
		}
		r := map[string]any{
			"show": s.RealityShow, "dest": s.RealityDest, "xver": s.RealityXver, "serverNames": names,
			"privateKey": s.RealityPrivateKey, "shortIds": orDefaultSlice(s.RealityShortIDs, []string{""}),
		}
		for k, v := range map[string]string{"minClientVer": s.RealityMinClientVer, "maxClientVer": s.RealityMaxClientVer, "mldsa65Seed": s.RealityMldsa65Seed} {
			if v != "" {
				r[k] = v
			}
		}
		if s.RealityMaxTimeDiff > 0 {
			r["maxTimeDiff"] = s.RealityMaxTimeDiff
		}
		st["realitySettings"] = r
	default:
		st["security"] = "none"
	}
	sockopt := map[string]any{}
	if s.AcceptProxyProtocol {
		sockopt["acceptProxyProtocol"] = true
	}
	if s.TCPFastOpen {
		sockopt["tcpFastOpen"] = true
	}
	if s.TCPCongestion != "" {
		sockopt["tcpcongestion"] = s.TCPCongestion
	}
	if s.TCPKeepAliveInterval > 0 {
		sockopt["tcpKeepAliveInterval"] = s.TCPKeepAliveInterval
	}
	if behindCDN(s) {
		// the connection comes from the CDN; the client's own IP, which
		// the device limit counts, is in the CDN's header
		sockopt["trustedXForwardedFor"] = cdnClientIPHeaders
	}
	if len(sockopt) > 0 {
		st["sockopt"] = sockopt
	}
	return st
}

// cdnClientIPHeaders mark a request that came through Cloudflare, Fastly,
// CloudFront (with its viewer headers forwarded) or an nginx-style proxy;
// Xray then takes the client's IP from the first X-Forwarded-For entry
// these add.
var cdnClientIPHeaders = []string{"CF-Connecting-IP", "Fastly-Client-IP", "CloudFront-Viewer-Address", "X-Real-IP"}

// behindCDN reports an inbound made to sit behind a CDN or reverse proxy:
// an HTTP transport without TLS of its own.
func behindCDN(s model.InboundSettings) bool {
	switch s.Network {
	case "ws", "xhttp", "httpupgrade":
		return s.Security == "" || s.Security == "none"
	}
	return false
}

// SingBox renders a sing-box server config for hysteria2/tuic/wireguard inbounds.
func SingBox(inbounds []*model.Inbound, users []*model.User, groups model.Groups) ([]byte, error) {
	var sin, endpoints []map[string]any
	for _, in := range inbounds {
		if in.Protocol.Core() != "sing-box" {
			continue
		}
		s := in.Settings
		us := usersFor(in, users, groups)
		base := map[string]any{"tag": in.Tag, "listen": orDefault(in.Listen, "::"), "listen_port": in.Port}
		if s.TCPFastOpen {
			base["tcp_fast_open"] = true
		}
		if s.UDPTimeout != "" {
			base["udp_timeout"] = s.UDPTimeout
		}
		certFile, keyFile := s.CertFile, s.KeyFile
		if certFile == "" || keyFile == "" {
			// the node agent creates this self-signed pair; clients skip verification
			certFile, keyFile = SelfSignedCert, SelfSignedKey
		}
		tls := map[string]any{"enabled": true, "certificate_path": certFile, "key_path": keyFile}
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
			if s.IgnoreClientBandwidth {
				base["ignore_client_bandwidth"] = true
			}
			if s.Masquerade != "" {
				base["masquerade"] = s.Masquerade
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
			if s.ZeroRTT {
				base["zero_rtt_handshake"] = true
			}
			if s.Heartbeat != "" {
				base["heartbeat"] = s.Heartbeat
			}
			if s.AuthTimeout != "" {
				base["auth_timeout"] = s.AuthTimeout
			}
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
			{"network": "tcp", "port": []int{25}, "action": "reject"},
			{"action": "sniff"},
			{"protocol": []string{"bittorrent"}, "action": "reject"},
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

// RealityTransport reports whether Xray accepts REALITY over the transport.
func RealityTransport(network string) bool {
	switch network {
	case "", "tcp", "xhttp", "grpc":
		return true
	}
	return false
}
