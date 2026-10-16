package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

// TestEditorExtras renders every advanced editor field and, when the real
// cores are around (XRAY_BIN, SINGBOX_BIN), checks they accept the config.
func TestEditorExtras(t *testing.T) {
	sniff := &model.Sniffing{Enabled: true, DestOverride: []string{"http", "tls"}, RouteOnly: true}
	inbounds := []*model.Inbound{
		{ID: 1, Tag: "vless-tls", Protocol: model.ProtoVLESS, Port: 443, Enabled: true, Settings: model.InboundSettings{
			Security: "tls", Flow: "xtls-rprx-vision", TLSMinVersion: "1.2", TLSMaxVersion: "1.3", TLSRejectUnknownSNI: true, SNI: "a.example.com",
			Fallbacks: []model.Fallback{{Dest: "8080"}, {Path: "/ws", Dest: "127.0.0.1:2000", Xver: 1}},
			Sniffing:  sniff, AcceptProxyProtocol: true, TCPFastOpen: true, TCPCongestion: "bbr", TCPKeepAliveInterval: 30}},
		{ID: 2, Tag: "reality", Protocol: model.ProtoVLESS, Port: 8443, Enabled: true, Settings: model.InboundSettings{
			Security: "reality", RealityDest: "www.microsoft.com:443", RealityServerNames: []string{"www.microsoft.com"},
			RealityPrivateKey: "SMR3hYd5-3QCzkUUkrHqHmBHO6ZWuQuDH7LlBybV8EU", RealityXver: 1, RealityMinClientVer: "25.1.1", RealityMaxTimeDiff: 60000, RealityShow: true}},
		{ID: 3, Tag: "kcp", Protocol: model.ProtoVMess, Port: 2000, Enabled: true, Settings: model.InboundSettings{
			Network: "kcp", KCPMTU: 1350, KCPTTI: 20, KCPUplink: 5, KCPDownlink: 20, KCPCongestion: true, KCPHeader: "wechat", KCPSeed: "s3cret"}},
		{ID: 4, Tag: "xhttp", Protocol: model.ProtoVLESS, Port: 2001, Enabled: true, Settings: model.InboundSettings{
			Network: "xhttp", Path: "/x", XPaddingBytes: "100-1000", XNoSSEHeader: true, XScMaxEachPostBytes: "1000000", XScMaxBufferedPosts: 30, XScStreamUpServerSec: "20-80"}},
		{ID: 5, Tag: "ws", Protocol: model.ProtoTrojan, Port: 2002, Enabled: true, Settings: model.InboundSettings{Network: "ws", Path: "/ws", WSHeartbeat: 30}},
		{ID: 6, Tag: "ss", Protocol: model.ProtoShadowsocks, Port: 2003, Enabled: true, Settings: model.InboundSettings{SSMethod: "aes-256-gcm", SSNetwork: "tcp", Sniffing: &model.Sniffing{}}},
		{ID: 7, Tag: "hy2", Protocol: model.ProtoHysteria2, Port: 2004, Enabled: true, Settings: model.InboundSettings{
			Obfs: "salamander", ObfsPassword: "p", Masquerade: "https://www.bing.com", IgnoreClientBandwidth: true, UDPTimeout: "5m"}},
		{ID: 8, Tag: "tuic", Protocol: model.ProtoTUIC, Port: 2005, Enabled: true, Settings: model.InboundSettings{ZeroRTT: true, Heartbeat: "10s", AuthTimeout: "3s", UDPTimeout: "2m"}},
	}
	users := []*model.User{{ID: 1, Username: "a", Status: model.StatusActive, Credentials: model.Credentials{UUID: "11111111-1111-1111-1111-111111111111", Password: "p"}}}
	x, err := Xray(inbounds, users, nil, Cascade{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"fallbacks"`, `"rejectUnknownSni": true`, `"minClientVer"`, `"kcpSettings"`, `"mkcp-legacy"`, `"header": "wechat"`, `"xPaddingBytes"`, `"heartbeatPeriod": 30`, `"acceptProxyProtocol": true`, `"routeOnly": true`, `"network": "tcp"`} {
		if !strings.Contains(string(x), want) {
			t.Errorf("xray config lacks %s", want)
		}
	}
	sb, err := SingBox(inbounds, users, nil)
	if err != nil {
		t.Fatal(err)
	}
	// hysteria2 runs on Xray now
	for _, want := range []string{`"protocol": "hysteria"`, `"masquerade"`, `"udpIdleTimeout": 300`, `"salamander"`} {
		if !strings.Contains(string(x), want) {
			t.Errorf("xray config lacks %s", want)
		}
	}
	for _, want := range []string{`"zero_rtt_handshake"`, `"udp_timeout"`} {
		if !strings.Contains(string(sb), want) {
			t.Errorf("sing-box config lacks %s", want)
		}
	}
	check := func(env string, args func(path string) []string, body []byte) {
		bin := os.Getenv(env)
		if bin == "" {
			return
		}
		p := filepath.Join(t.TempDir(), "c.json")
		_ = os.WriteFile(p, body, 0o600)
		if out, err := exec.Command(bin, args(p)...).CombinedOutput(); err != nil {
			t.Errorf("%s rejects the config: %v\n%s", env, err, out)
		}
	}
	if bin := os.Getenv("XRAY_BIN"); bin != "" {
		x = forXray(t, bin, x)
	}
	check("XRAY_BIN", func(p string) []string { return []string{"run", "-test", "-c", p} }, x)
	check("SINGBOX_BIN", func(p string) []string { return []string{"check", "-c", p} }, sb)
}
