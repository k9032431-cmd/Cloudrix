package core

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

// TestHysteriaCountsUsers runs the generated config on a real Xray
// (XRAY_BIN) and checks that hysteria2 traffic is counted per user.
func TestHysteriaCountsUsers(t *testing.T) {
	bin := os.Getenv("XRAY_BIN")
	if bin == "" {
		t.Skip("set XRAY_BIN to run a real xray")
	}
	if l, err := net.Listen("tcp", "127.0.0.1:10085"); err != nil {
		t.Skip("api port busy")
	} else {
		l.Close()
	}
	dir := t.TempDir()
	certFile, keyFile, pin := testCert(t, dir)
	port := freeUDP(t)
	hy := &model.Inbound{ID: 1, Tag: "hy", Protocol: model.ProtoHysteria2, Port: port, Listen: "127.0.0.1", Enabled: true,
		Settings: model.InboundSettings{CertFile: certFile, KeyFile: keyFile}}
	users := []*model.User{{ID: 7, Username: "arsi", Status: model.StatusActive, Credentials: model.Credentials{Password: "pw123"}}}
	srvCfg, err := Xray([]*model.Inbound{hy}, users, nil, Cascade{})
	if err != nil {
		t.Fatal(err)
	}
	// the test web server is on loopback, which the config sends to block
	srvCfg = reachLoopback(srvCfg)
	socks := freeTCP(t)
	cliCfg, _ := json.Marshal(map[string]any{
		"inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": socks, "protocol": "socks"}},
		"outbounds": []any{map[string]any{"protocol": "hysteria", "settings": map[string]any{"version": 2, "address": "127.0.0.1", "port": port},
			"streamSettings": map[string]any{"network": "hysteria", "hysteriaSettings": map[string]any{"version": 2, "auth": "pw123"},
				"security": "tls", "tlsSettings": map[string]any{"serverName": "www.google.com", "pinnedPeerCertSha256": pin, "alpn": []string{"h3"}}}}},
	})
	run := func(name string, cfg []byte) {
		p := filepath.Join(dir, name)
		_ = os.WriteFile(p, cfg, 0o600)
		cmd := exec.Command(bin, "run", "-c", p)
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
	var got int
	for i := 0; i < 20 && got == 0; i++ {
		time.Sleep(300 * time.Millisecond)
		if res, err := client.Get("http://" + l.Addr().String()); err == nil {
			b, _ := io.ReadAll(res.Body)
			res.Body.Close()
			got = len(b)
		}
	}
	if got != 100000 {
		t.Fatal("no traffic through hysteria2")
	}
	out, err := exec.Command(bin, "api", "statsquery", "--server=127.0.0.1:10085", "-pattern", "user>>>").CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !strings.Contains(string(out), "user>>>"+Email(users[0])+">>>traffic>>>downlink") {
		t.Fatalf("hysteria2 traffic is not counted per user:\n%s", out)
	}
}

func testCert(t *testing.T, dir string) (string, string, string) {
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
	sum := sha256.Sum256(der)
	return c, k, hex.EncodeToString(sum[:])
}

func freeTCP(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func freeUDP(t *testing.T) int {
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}
