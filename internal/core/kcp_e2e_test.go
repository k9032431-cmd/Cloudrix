package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/sub"
)

// TestKCPConnects runs a vless mKCP inbound from the generated config on a
// real Xray (XRAY_BIN) and connects with the masking the share link carries.
func TestKCPConnects(t *testing.T) {
	bin := os.Getenv("XRAY_BIN")
	if bin == "" {
		t.Skip("set XRAY_BIN to run a real xray")
	}
	for _, tc := range []struct{ name, header, seed string }{{"plain", "", ""}, {"seed", "", "s3cret"}, {"wechat+seed", "wechat", "s3cret"}, {"dns", "dns", ""}} {
		t.Run(tc.name, func(t *testing.T) {
			if l, err := net.Listen("tcp", "127.0.0.1:10085"); err != nil {
				t.Skip("api port busy")
			} else {
				l.Close()
			}
			dir := t.TempDir()
			port := freeUDP(t)
			s := model.InboundSettings{Network: "kcp", Security: "none", KCPHeader: tc.header, KCPSeed: tc.seed}
			in := &model.Inbound{ID: 1, Tag: "kcp", Protocol: model.ProtoVLESS, Port: port, Listen: "127.0.0.1", Enabled: true, Settings: s}
			users := []*model.User{{ID: 7, Username: "arsi", Status: model.StatusActive, Credentials: model.Credentials{UUID: "11111111-1111-1111-1111-111111111111"}}}
			srvCfg, err := Xray([]*model.Inbound{in}, users, nil, Cascade{})
			if err != nil {
				t.Fatal(err)
			}
			var cfg map[string]any
			_ = json.Unmarshal(srvCfg, &cfg)
			routing := cfg["routing"].(map[string]any)
			var rules []any
			for _, r := range routing["rules"].([]any) {
				if _, ok := r.(map[string]any)["ip"]; !ok {
					rules = append(rules, r)
				}
			}
			routing["rules"] = rules
			srvCfg, _ = json.Marshal(cfg)

			socks := freeTCP(t)
			cliCfg, _ := json.Marshal(map[string]any{
				"inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": socks, "protocol": "socks"}},
				"outbounds": []any{map[string]any{"protocol": "vless", "settings": map[string]any{"vnext": []any{map[string]any{"address": "127.0.0.1", "port": port,
					"users": []any{map[string]any{"id": "11111111-1111-1111-1111-111111111111", "encryption": "none"}}}}},
					"streamSettings": map[string]any{"network": "kcp", "finalmask": sub.KCPMask(s)}}},
			})
			run := func(name string, c []byte) {
				p := filepath.Join(dir, name)
				_ = os.WriteFile(p, c, 0o600)
				cmd := exec.Command(bin, "run", "-c", p)
				out, _ := os.Create(p + ".log")
				cmd.Stdout, cmd.Stderr = out, out
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
			}
			run("server.json", srvCfg)
			run("client.json", cliCfg)

			web := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(make([]byte, 100000)) })}
			l, _ := net.Listen("tcp", "127.0.0.1:0")
			go func() { _ = web.Serve(l) }()
			defer web.Close()
			proxy, _ := url.Parse(fmt.Sprintf("socks5://127.0.0.1:%d", socks))
			client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy)}, Timeout: 5 * time.Second}
			got := 0
			for i := 0; i < 15 && got == 0; i++ {
				time.Sleep(300 * time.Millisecond)
				if res, err := client.Get("http://" + l.Addr().String()); err == nil {
					b, _ := io.ReadAll(res.Body)
					res.Body.Close()
					got = len(b)
				}
			}
			if got != 100000 {
				sl, _ := os.ReadFile(filepath.Join(dir, "server.json.log"))
				cl, _ := os.ReadFile(filepath.Join(dir, "client.json.log"))
				t.Fatalf("no traffic over mKCP\nserver: %s\nclient: %s", sl, cl)
			}
		})
	}
}
