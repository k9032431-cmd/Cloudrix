package sub

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

func testUser() *model.User {
	exp := time.Now().Add(48 * time.Hour)
	return &model.User{
		ID: 7, Username: "alice", Status: model.StatusActive, DataLimit: 10 << 30, UsedTraffic: 1 << 30, ExpireAt: &exp,
		Credentials: model.Credentials{UUID: "11111111-2222-4333-8444-555555555555", Password: "s3cret", WGPrivateKey: "cHJpdg==", WGPublicKey: "cHVi"},
	}
}

func allInbounds() []*model.Inbound {
	return []*model.Inbound{
		{Tag: "vless-reality", Protocol: model.ProtoVLESS, Port: 443, Enabled: true, Remark: "{USERNAME} {DATA_LEFT}", Settings: model.InboundSettings{
			Security: "reality", Flow: "xtls-rprx-vision", RealityPublicKey: "PBK", RealityShortIDs: []string{"abcd"}, RealityServerNames: []string{"www.example.com"}, RealityDest: "www.example.com:443", RealityPrivateKey: "PK"}},
		{Tag: "vmess-ws", Protocol: model.ProtoVMess, Port: 8080, Enabled: true, Settings: model.InboundSettings{Network: "ws", Path: "/ws", Host: "cdn.example.com", Security: "tls", SNI: "cdn.example.com"}},
		{Tag: "trojan-grpc", Protocol: model.ProtoTrojan, Port: 2083, Enabled: true, Settings: model.InboundSettings{Network: "grpc", ServiceName: "tg", Security: "tls"}},
		{Tag: "ss", Protocol: model.ProtoShadowsocks, Port: 8388, Enabled: true, Settings: model.InboundSettings{SSMethod: "2022-blake3-aes-128-gcm", SSServerKey: "c2VydmVya2V5c2VydmVyaw=="}},
		{Tag: "hy2", Protocol: model.ProtoHysteria2, Port: 8443, Enabled: true, Settings: model.InboundSettings{SNI: "hy.example.com", Obfs: "salamander", ObfsPassword: "ob"}},
		{Tag: "tuic", Protocol: model.ProtoTUIC, Port: 8444, Enabled: true, Settings: model.InboundSettings{SNI: "tuic.example.com"}},
		{Tag: "wg", Protocol: model.ProtoWireGuard, Port: 51820, Enabled: true, Settings: model.InboundSettings{WGPublicKey: "U0VSVkVS", WGNetwork: "10.9.0.0/24"}},
		{Tag: "disabled", Protocol: model.ProtoVLESS, Port: 1, Enabled: false},
	}
}

func TestLinksCoverEveryProtocol(t *testing.T) {
	u := testUser()
	eps := Resolve(u, allInbounds(), nil, "vpn.example.com")
	if len(eps) != 7 {
		t.Fatalf("expected 7 endpoints (disabled skipped), got %d", len(eps))
	}
	links := Links(u, eps)
	if len(links) != 7 {
		t.Fatalf("expected 7 links, got %d: %v", len(links), links)
	}
	want := []string{"vless://", "vmess://", "trojan://", "ss://", "hysteria2://", "tuic://", "wireguard://"}
	for i, prefix := range want {
		if !strings.HasPrefix(links[i], prefix) {
			t.Errorf("link %d: want prefix %s, got %s", i, prefix, links[i])
		}
	}

	vless, _ := url.Parse(links[0])
	q := vless.Query()
	if q.Get("security") != "reality" || q.Get("pbk") != "PBK" || q.Get("sid") != "abcd" || q.Get("flow") != "xtls-rprx-vision" || q.Get("sni") != "www.example.com" {
		t.Errorf("unexpected vless query: %s", vless.RawQuery)
	}
	if vless.Fragment != "alice 9.0 GB" {
		t.Errorf("remark template not rendered: %q", vless.Fragment)
	}

	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(links[1], "vmess://"))
	if err != nil {
		t.Fatal(err)
	}
	var vm map[string]string
	if err := json.Unmarshal(raw, &vm); err != nil {
		t.Fatal(err)
	}
	if vm["net"] != "ws" || vm["path"] != "/ws" || vm["tls"] != "tls" || vm["add"] != "vpn.example.com" {
		t.Errorf("unexpected vmess json: %v", vm)
	}

	ss, _ := url.Parse(links[3])
	userinfo, _ := base64.RawURLEncoding.DecodeString(ss.User.Username())
	if !strings.HasPrefix(string(userinfo), "2022-blake3-aes-128-gcm:c2VydmVya2V5c2VydmVyaw==:") {
		t.Errorf("ss 2022 password must be server:user key, got %s", userinfo)
	}

	wg, _ := url.Parse(links[6])
	if wg.Query().Get("address") != "10.9.0.8/32" {
		t.Errorf("wireguard address = %s, want 10.9.0.8/32", wg.Query().Get("address"))
	}
}

func TestUserInboundFilter(t *testing.T) {
	u := testUser()
	u.Inbounds = []string{"hy2"}
	eps := Resolve(u, allInbounds(), nil, "h")
	if len(eps) != 1 || eps[0].Inbound.Tag != "hy2" {
		t.Fatalf("expected only hy2, got %v", eps)
	}
}

func TestClashAndSingBoxRender(t *testing.T) {
	u := testUser()
	eps := Resolve(u, allInbounds(), nil, "vpn.example.com")

	body, err := Clash(u, eps)
	if err != nil {
		t.Fatal(err)
	}
	var clash struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(body, &clash); err != nil {
		t.Fatal(err)
	}
	if len(clash.Proxies) != 7 {
		t.Errorf("clash: expected 7 proxies, got %d", len(clash.Proxies))
	}

	body, err = SingBox(u, eps)
	if err != nil {
		t.Fatal(err)
	}
	var sb struct {
		Outbounds []map[string]any `json:"outbounds"`
		Endpoints []map[string]any `json:"endpoints"`
	}
	if err := json.Unmarshal(body, &sb); err != nil {
		t.Fatal(err)
	}
	// 6 proxies + selector + urltest + direct; wireguard goes to endpoints
	if len(sb.Outbounds) != 9 || len(sb.Endpoints) != 1 {
		t.Errorf("sing-box: got %d outbounds, %d endpoints", len(sb.Outbounds), len(sb.Endpoints))
	}
}

func TestWGAddress(t *testing.T) {
	cases := []struct {
		network string
		id      int64
		want    string
		err     bool
	}{
		{"10.8.0.0/24", 1, "10.8.0.2/32", false},
		{"10.8.0.0/24", 253, "10.8.0.254/32", false},
		{"10.8.0.0/24", 254, "", true},
		{"10.8.0.0/16", 300, "10.8.1.45/32", false},
	}
	for _, c := range cases {
		got, err := WGAddress(c.network, c.id)
		if (err != nil) != c.err || got != c.want {
			t.Errorf("WGAddress(%s, %d) = %q, %v", c.network, c.id, got, err)
		}
	}
}
