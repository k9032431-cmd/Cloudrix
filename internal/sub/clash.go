package sub

import (
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

type clashProfile struct {
	MixedPort   int              `yaml:"mixed-port"`
	AllowLan    bool             `yaml:"allow-lan"`
	Mode        string           `yaml:"mode"`
	LogLevel    string           `yaml:"log-level"`
	IPv6        bool             `yaml:"ipv6"`
	Proxies     []map[string]any `yaml:"proxies"`
	ProxyGroups []map[string]any `yaml:"proxy-groups"`
	Rules       []string         `yaml:"rules"`
}

// Clash renders a Clash Meta (mihomo) profile.
func Clash(u *model.User, eps []Endpoint) ([]byte, error) {
	p := clashProfile{MixedPort: 7890, Mode: "rule", LogLevel: "warning", IPv6: true, Rules: []string{"MATCH,Proxy"}}
	names := []string{}
	seen := map[string]int{}
	for _, ep := range eps {
		px := clashProxy(u, ep)
		if px == nil {
			continue
		}
		name := uniqueName(ep.Remark, seen)
		px["name"] = name
		p.Proxies = append(p.Proxies, px)
		names = append(names, name)
	}
	p.ProxyGroups = []map[string]any{
		{"name": "Proxy", "type": "select", "proxies": append([]string{"Auto"}, names...)},
		{"name": "Auto", "type": "url-test", "url": "https://www.gstatic.com/generate_204", "interval": 300, "tolerance": 50, "proxies": names},
	}
	if len(names) == 0 {
		// mihomo rejects url-test groups without members
		p.ProxyGroups = []map[string]any{{"name": "Proxy", "type": "select", "proxies": []string{"DIRECT"}}}
	}
	return yaml.Marshal(p)
}

func clashProxy(u *model.User, ep Endpoint) map[string]any {
	in, s, c := ep.Inbound, ep.Inbound.Settings, u.Credentials
	px := map[string]any{"server": ep.Host, "port": in.Port, "udp": true}

	switch in.Protocol {
	case model.ProtoVLESS:
		px["type"] = "vless"
		px["uuid"] = c.UUID
		if s.Flow != "" && network(s) == "tcp" && (s.Security == "tls" || s.Security == "reality") {
			px["flow"] = s.Flow
		}
		clashTransport(px, s)
	case model.ProtoVMess:
		px["type"] = "vmess"
		px["uuid"] = c.UUID
		px["alterId"] = 0
		px["cipher"] = "auto"
		clashTransport(px, s)
	case model.ProtoTrojan:
		px["type"] = "trojan"
		px["password"] = c.Password
		clashTransport(px, s)
	case model.ProtoShadowsocks:
		px["type"] = "ss"
		px["cipher"] = SSMethod(s)
		px["password"] = SSClientPassword(s, c.Password)
	case model.ProtoHysteria2:
		px["type"] = "hysteria2"
		px["password"] = c.Password
		setIf(px, "sni", s.SNI)
		if s.Obfs != "" {
			px["obfs"] = s.Obfs
			px["obfs-password"] = s.ObfsPassword
		}
		if s.AllowInsecure {
			px["skip-cert-verify"] = true
		}
	case model.ProtoTUIC:
		px["type"] = "tuic"
		px["uuid"] = c.UUID
		px["password"] = c.Password
		px["congestion-controller"] = orDefault(s.CongestionCtrl, "bbr")
		px["udp-relay-mode"] = "native"
		px["alpn"] = orDefaultSlice(s.ALPN, []string{"h3"})
		setIf(px, "sni", s.SNI)
		if s.AllowInsecure {
			px["skip-cert-verify"] = true
		}
	case model.ProtoWireGuard:
		addr, err := WGAddress(s.WGNetwork, u.ID)
		if err != nil {
			return nil
		}
		px["type"] = "wireguard"
		px["private-key"] = c.WGPrivateKey
		px["public-key"] = s.WGPublicKey
		px["ip"] = strings.TrimSuffix(addr, "/32")
		if s.MTU > 0 {
			px["mtu"] = s.MTU
		}
	default:
		return nil
	}
	return px
}

func clashTransport(px map[string]any, s model.InboundSettings) {
	switch network(s) {
	case "ws", "httpupgrade":
		px["network"] = "ws"
		opts := map[string]any{"path": orDefault(s.Path, "/")}
		if s.Host != "" {
			opts["headers"] = map[string]string{"Host": s.Host}
		}
		if network(s) == "httpupgrade" {
			opts["v2ray-http-upgrade"] = true
		}
		px["ws-opts"] = opts
	case "grpc":
		px["network"] = "grpc"
		px["grpc-opts"] = map[string]any{"grpc-service-name": s.ServiceName}
	case "xhttp":
		// mihomo has partial xhttp support; clients without it skip this proxy.
		px["network"] = "xhttp"
		px["xhttp-opts"] = map[string]any{"path": orDefault(s.Path, "/"), "host": s.Host, "mode": orDefault(s.XHTTPMode, "auto")}
	default:
		px["network"] = "tcp"
	}
	switch s.Security {
	case "tls":
		px["tls"] = true
		setIf(px, "servername", s.SNI)
		setIf(px, "client-fingerprint", s.Fingerprint)
		if len(s.ALPN) > 0 {
			px["alpn"] = s.ALPN
		}
		if s.AllowInsecure {
			px["skip-cert-verify"] = true
		}
	case "reality":
		px["tls"] = true
		sni := s.SNI
		if sni == "" && len(s.RealityServerNames) > 0 {
			sni = s.RealityServerNames[0]
		}
		setIf(px, "servername", sni)
		px["client-fingerprint"] = orDefault(s.Fingerprint, "chrome")
		opts := map[string]any{"public-key": s.RealityPublicKey}
		if len(s.RealityShortIDs) > 0 {
			opts["short-id"] = s.RealityShortIDs[0]
		}
		px["reality-opts"] = opts
	}
}

func setIf(m map[string]any, k, v string) {
	if v != "" {
		m[k] = v
	}
}

func uniqueName(name string, seen map[string]int) string {
	seen[name]++
	if n := seen[name]; n > 1 {
		return name + " #" + strconv.Itoa(n)
	}
	return name
}
