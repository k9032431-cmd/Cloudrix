package nodeagent

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAddedUsers(t *testing.T) {
	if n := addedUsers([]byte("processing inbound: hy\nunsupported inbound type\nAdded 0 user(s) in total.\n")); n != 0 {
		t.Fatalf("got %d", n)
	}
	if n := addedUsers([]byte("Added 2 user(s) in total.")); n != 2 {
		t.Fatalf("got %d", n)
	}
	if n := addedUsers([]byte("failed to dial")); n != -1 {
		t.Fatalf("got %d", n)
	}
}

// TestHotUpdateHysteria needs a real Xray (XRAY_BIN): its adu cannot add
// hysteria users, so such a change must restart Xray instead.
func TestHotUpdateHysteria(t *testing.T) {
	bin := os.Getenv("XRAY_BIN")
	if bin == "" {
		t.Skip("XRAY_BIN not set")
	}
	if l, err := net.Listen("tcp", xrayAPI); err != nil {
		t.Skip("api port busy")
	} else {
		l.Close()
	}
	dir := t.TempDir()
	certFile, keyFile := writeTestCert(t, dir)
	cfg := func(hyUsers, vlUsers []any) []byte {
		b, _ := json.Marshal(map[string]any{
			"api":    map[string]any{"tag": "api", "services": []string{"HandlerService"}},
			"policy": map[string]any{},
			"inbounds": []any{
				map[string]any{"tag": "api", "listen": "127.0.0.1", "port": 10085, "protocol": "dokodemo-door", "settings": map[string]any{"address": "127.0.0.1"}},
				map[string]any{"tag": "hy", "listen": "127.0.0.1", "port": 19543, "protocol": "hysteria",
					"settings": map[string]any{"version": 2, "clients": hyUsers},
					"streamSettings": map[string]any{"network": "hysteria", "hysteriaSettings": map[string]any{"version": 2}, "security": "tls",
						"tlsSettings": map[string]any{"certificates": []any{map[string]any{"certificateFile": certFile, "keyFile": keyFile}}, "alpn": []string{"h3"}}}},
				map[string]any{"tag": "vl", "listen": "127.0.0.1", "port": 19544, "protocol": "vless",
					"settings": map[string]any{"decryption": "none", "clients": vlUsers}},
			},
			"outbounds": []any{map[string]any{"protocol": "freedom"}, map[string]any{"tag": "api", "protocol": "blackhole"}},
			"routing":   map[string]any{"rules": []any{map[string]any{"inboundTag": []string{"api"}, "outboundTag": "api"}}},
		})
		return b
	}
	hy1 := []any{map[string]any{"auth": "a", "email": "1.a"}}
	vl1 := []any{map[string]any{"id": "11111111-1111-1111-1111-111111111111", "email": "2.b"}}
	start := cfg(hy1, vl1)
	file := filepath.Join(dir, "x.json")
	if err := os.WriteFile(file, start, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "run", "-c", file)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	time.Sleep(time.Second)

	a := &Agent{cfg: Config{XrayBin: bin, Dir: dir}, log: slog.New(slog.DiscardHandler), xray: &proc{env: os.Environ()}}
	ctx := context.Background()
	vl2 := append(vl1, map[string]any{"id": "22222222-2222-2222-2222-222222222222", "email": "3.c"})
	if !a.hotUpdate(ctx, start, cfg(hy1, vl2)) {
		t.Fatal("a new vless user should be added without a restart")
	}
	// Xray before 26.6 cannot add hysteria users through the API (and still
	// exits 0): then the update must ask for a restart, never claim success
	hy2 := append(hy1, map[string]any{"auth": "d", "email": "4.d"})
	hot := a.hotUpdate(ctx, cfg(hy1, vl2), cfg(hy2, vl2))
	out, err := a.xrayAPI(ctx, "inbounduser", "-tag=hy")
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if added := strings.Contains(string(out), "4.d"); hot != added {
		t.Fatalf("hot update said %v, user added: %v\n%s", hot, added, out)
	}
}

func writeTestCert(t *testing.T, dir string) (string, string) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "x"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := x509.MarshalECPrivateKey(key)
	c, k := filepath.Join(dir, "c.pem"), filepath.Join(dir, "k.pem")
	_ = os.WriteFile(c, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)
	_ = os.WriteFile(k, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600)
	return c, k
}
