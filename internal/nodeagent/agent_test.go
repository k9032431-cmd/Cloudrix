package nodeagent

import "testing"

func TestCoreError(t *testing.T) {
	out := []byte(`Xray 26.3.27 (Xray, Penetrates Everything.) d2758a0 (go1.26.1 linux/amd64)
A unified platform for anti-censorship.
2026/10/11 15:53:10 [Info] infra/conf/serial: Reading config: &{Name:/x.json Format:json}
Failed to start: main: failed to load config files: [/x.json] > infra/conf: failed to build inbound config with tag vless > infra/conf: Failed to build TLS config.`)
	want := "infra/conf: failed to build inbound config with tag vless > infra/conf: Failed to build TLS config."
	if got := coreError(out); got != want {
		t.Fatalf("got %q", got)
	}
}
