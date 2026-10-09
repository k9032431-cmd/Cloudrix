package sub

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

// Links renders one share link per endpoint.
func Links(u *model.User, eps []Endpoint) []string {
	var out []string
	for _, ep := range eps {
		if l := Link(u, ep); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// Base64Links is the classic v2ray subscription body.
func Base64Links(u *model.User, eps []Endpoint) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(Links(u, eps), "\n")))
}

func Link(u *model.User, ep Endpoint) string {
	in, s := ep.Inbound, ep.Inbound.Settings
	hostPort := net.JoinHostPort(ep.Host, strconv.Itoa(in.Port))
	frag := "#" + url.PathEscape(ep.Remark)
	c := u.Credentials

	switch in.Protocol {
	case model.ProtoVLESS:
		q := transportQuery(s)
		q.Set("encryption", "none")
		if s.Flow != "" && network(s) == "tcp" && s.Security != "none" && s.Security != "" {
			q.Set("flow", s.Flow)
		}
		return "vless://" + c.UUID + "@" + hostPort + "?" + q.Encode() + frag

	case model.ProtoTrojan:
		q := transportQuery(s)
		return "trojan://" + url.PathEscape(c.Password) + "@" + hostPort + "?" + q.Encode() + frag

	case model.ProtoVMess:
		v := map[string]string{
			"v": "2", "ps": ep.Remark, "add": ep.Host, "port": strconv.Itoa(in.Port), "id": c.UUID,
			"aid": "0", "scy": "auto", "net": network(s), "type": headerType(s), "host": s.Host,
			"path": pathOrService(s), "tls": tlsField(s), "sni": s.SNI, "alpn": strings.Join(s.ALPN, ","), "fp": s.Fingerprint,
		}
		b, _ := json.Marshal(v)
		return "vmess://" + base64.StdEncoding.EncodeToString(b)

	case model.ProtoShadowsocks:
		userinfo := base64.RawURLEncoding.EncodeToString([]byte(SSMethod(s) + ":" + SSClientPassword(s, c.Password)))
		return "ss://" + userinfo + "@" + hostPort + frag

	case model.ProtoHysteria2:
		q := url.Values{}
		setNonEmpty(q, "sni", s.SNI)
		if s.Obfs != "" {
			q.Set("obfs", s.Obfs)
			q.Set("obfs-password", s.ObfsPassword)
		}
		if s.AllowInsecure {
			q.Set("insecure", "1")
		}
		return "hysteria2://" + url.PathEscape(c.Password) + "@" + hostPort + "/?" + q.Encode() + frag

	case model.ProtoTUIC:
		q := url.Values{}
		q.Set("congestion_control", orDefault(s.CongestionCtrl, "bbr"))
		q.Set("udp_relay_mode", "native")
		q.Set("alpn", strings.Join(orDefaultSlice(s.ALPN, []string{"h3"}), ","))
		setNonEmpty(q, "sni", s.SNI)
		if s.AllowInsecure {
			q.Set("allow_insecure", "1")
		}
		return "tuic://" + c.UUID + ":" + url.PathEscape(c.Password) + "@" + hostPort + "?" + q.Encode() + frag

	case model.ProtoWireGuard:
		addr, err := WGAddress(s.WGNetwork, u.ID)
		if err != nil {
			return ""
		}
		q := url.Values{}
		q.Set("publickey", s.WGPublicKey)
		q.Set("address", addr)
		if s.MTU > 0 {
			q.Set("mtu", strconv.Itoa(s.MTU))
		}
		return "wireguard://" + url.PathEscape(c.WGPrivateKey) + "@" + hostPort + "?" + q.Encode() + frag
	}
	return ""
}

// WireGuardConf renders a classic wg-quick config for one endpoint.
func WireGuardConf(u *model.User, ep Endpoint) (string, error) {
	s := ep.Inbound.Settings
	addr, err := WGAddress(s.WGNetwork, u.ID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("[Interface]\n")
	b.WriteString("PrivateKey = " + u.Credentials.WGPrivateKey + "\n")
	b.WriteString("Address = " + addr + "\n")
	b.WriteString("DNS = " + orDefault(s.WGDNS, "1.1.1.1") + "\n")
	if s.MTU > 0 {
		b.WriteString("MTU = " + strconv.Itoa(s.MTU) + "\n")
	}
	b.WriteString("\n[Peer]\n")
	b.WriteString("PublicKey = " + s.WGPublicKey + "\n")
	b.WriteString("AllowedIPs = 0.0.0.0/0, ::/0\n")
	b.WriteString("Endpoint = " + net.JoinHostPort(ep.Host, strconv.Itoa(ep.Inbound.Port)) + "\n")
	b.WriteString("PersistentKeepalive = 25\n")
	return b.String(), nil
}

func network(s model.InboundSettings) string {
	return orDefault(s.Network, "tcp")
}

func headerType(s model.InboundSettings) string {
	return orDefault(s.HeaderType, "none")
}

func pathOrService(s model.InboundSettings) string {
	if network(s) == "grpc" {
		return s.ServiceName
	}
	return s.Path
}

func tlsField(s model.InboundSettings) string {
	if s.Security == "tls" {
		return "tls"
	}
	return ""
}

// transportQuery renders the shared v2ray URI scheme parameters.
func transportQuery(s model.InboundSettings) url.Values {
	q := url.Values{}
	q.Set("type", network(s))
	switch network(s) {
	case "ws", "httpupgrade":
		setNonEmpty(q, "path", s.Path)
		setNonEmpty(q, "host", s.Host)
	case "xhttp":
		setNonEmpty(q, "path", s.Path)
		setNonEmpty(q, "host", s.Host)
		setNonEmpty(q, "mode", s.XHTTPMode)
	case "grpc":
		setNonEmpty(q, "serviceName", s.ServiceName)
		q.Set("mode", "gun")
	case "tcp":
		if s.HeaderType == "http" {
			q.Set("headerType", "http")
			setNonEmpty(q, "path", s.Path)
			setNonEmpty(q, "host", s.Host)
		}
	}
	security := orDefault(s.Security, "none")
	q.Set("security", security)
	switch security {
	case "tls":
		setNonEmpty(q, "sni", s.SNI)
		setNonEmpty(q, "fp", s.Fingerprint)
		if len(s.ALPN) > 0 {
			q.Set("alpn", strings.Join(s.ALPN, ","))
		}
		if s.AllowInsecure {
			q.Set("allowInsecure", "1")
		}
	case "reality":
		sni := s.SNI
		if sni == "" && len(s.RealityServerNames) > 0 {
			sni = s.RealityServerNames[0]
		}
		setNonEmpty(q, "sni", sni)
		q.Set("fp", orDefault(s.Fingerprint, "chrome"))
		setNonEmpty(q, "pbk", s.RealityPublicKey)
		if len(s.RealityShortIDs) > 0 {
			q.Set("sid", s.RealityShortIDs[0])
		}
		setNonEmpty(q, "spx", s.RealitySpiderX)
	}
	return q
}

func setNonEmpty(q url.Values, k, v string) {
	if v != "" {
		q.Set(k, v)
	}
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
