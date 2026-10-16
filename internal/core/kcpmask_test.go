package core

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/sub"
)

// forXray adapts a config to the Xray binary the test runs, as the panel
// does for each node by its reported version.
func forXray(t *testing.T, bin string, cfg []byte) []byte {
	out, err := exec.Command(bin, "version").Output()
	if err != nil {
		t.Fatal(err)
	}
	f := strings.Fields(string(out))
	if len(f) > 1 && XrayBefore(f[1], 26, 6) {
		return LegacyKCPMasks(cfg)
	}
	return cfg
}

func TestXrayBefore(t *testing.T) {
	for v, want := range map[string]bool{
		"xray 26.3.27, sing-box 1.14.3": true, "26.5.9": true, "v26.6.22": false, "xray 26.10.10": false,
		"25.12.8": true, "": false, "sing-box 1.14.3": false,
	} {
		if got := XrayBefore(v, 26, 6); got != want {
			t.Errorf("XrayBefore(%q) = %v", v, got)
		}
	}
}

func TestLegacyKCPMasks(t *testing.T) {
	in := &model.Inbound{ID: 1, Tag: "k", Protocol: model.ProtoVLESS, Port: 9000, Enabled: true,
		Settings: model.InboundSettings{Network: "kcp", KCPHeader: "dns", KCPHeaderDomain: "a.example", KCPSeed: "pw"}}
	b, err := Xray([]*model.Inbound{in}, nil, nil, Cascade{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"mkcp-legacy"`) {
		t.Fatalf("new cores get mkcp-legacy:\n%s", b)
	}
	var c struct {
		Inbounds []struct {
			StreamSettings struct {
				Finalmask struct {
					UDP []map[string]any `json:"udp"`
				} `json:"finalmask"`
			} `json:"streamSettings"`
		} `json:"inbounds"`
	}
	_ = json.Unmarshal(LegacyKCPMasks(b), &c)
	var got []string
	for _, in := range c.Inbounds {
		for _, m := range in.StreamSettings.Finalmask.UDP {
			s, _ := json.Marshal(m)
			got = append(got, string(s))
		}
	}
	want := `{"settings":{"password":"pw"},"type":"mkcp-aes128gcm"} {"settings":{"domain":"a.example"},"type":"header-dns"}`
	if strings.Join(got, " ") != want {
		t.Fatalf("legacy masks: %s", strings.Join(got, " "))
	}
	if m := sub.KCPMask(model.InboundSettings{}); len(m["udp"].([]map[string]any)) != 1 {
		t.Fatalf("plain mKCP: %v", m)
	}
}

// reachLoopback lets a test server reach the test web server on 127.0.0.1:
// the config blocks private addresses, and so does Xray 26.10+ freedom.
func reachLoopback(cfg []byte) []byte {
	var c map[string]any
	_ = json.Unmarshal(cfg, &c)
	routing := c["routing"].(map[string]any)
	var rules []any
	for _, r := range routing["rules"].([]any) {
		if _, ok := r.(map[string]any)["ip"]; !ok {
			rules = append(rules, r)
		}
	}
	routing["rules"] = rules
	for _, o := range c["outbounds"].([]any) {
		if o := o.(map[string]any); o["protocol"] == "freedom" {
			o["settings"] = map[string]any{"finalRules": []any{map[string]any{"action": "allow", "ip": []string{"127.0.0.0/8"}}}}
		}
	}
	out, _ := json.Marshal(c)
	return out
}
