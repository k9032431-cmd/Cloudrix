package nodeagent

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/k9032431-cmd/cloudrix/internal/core"
)

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

func TestProbeExits(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	open := ln.Addr().(*net.TCPAddr).Port
	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	shut := closed.Addr().(*net.TCPAddr).Port
	closed.Close()

	dir := t.TempDir()
	a := &Agent{cfg: Config{Dir: dir}}
	cfg := fmt.Sprintf(`{"outbounds":[{"tag":"direct"},
		{"tag":"exit-2","settings":{"vnext":[{"address":"127.0.0.1","port":%d}]}},
		{"tag":"exit-3","settings":{"vnext":[{"address":"127.0.0.1","port":%d}]}}]}`, open, shut)
	if err := os.WriteFile(filepath.Join(dir, "xray.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	got := a.probeExits()
	if !strings.Contains(got, fmt.Sprintf("127.0.0.1:%d is unreachable", shut)) || strings.Contains(got, fmt.Sprintf(":%d ", open)) {
		t.Fatalf("probe: %q", got)
	}
}

func TestWriteShared(t *testing.T) {
	dir := t.TempDir() + "/"
	old := core.SharedTLSDir
	core.SharedTLSDir = dir
	t.Cleanup(func() { core.SharedTLSDir = old })
	p := filepath.Join(dir, "d", "fullchain.pem")
	if changed, err := writeShared(map[string]string{p: "A"}); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if changed, _ := writeShared(map[string]string{p: "A"}); changed {
		t.Fatal("unchanged file reported as changed")
	}
	if _, err := writeShared(map[string]string{"/etc/passwd": "x"}); err == nil {
		t.Fatal("wrote outside the shared dir")
	}
	if _, err := writeShared(map[string]string{dir + "../escape": "x"}); err == nil {
		t.Fatal("path traversal accepted")
	}
}
