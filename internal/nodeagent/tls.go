package nodeagent

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
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/core"
	"github.com/k9032431-cmd/cloudrix/internal/nodeproto"
)

// ensureSelfSigned creates the certificate hysteria2/tuic inbounds use when
// the admin gave them none (clients connect with verification off).
func ensureSelfSigned() error {
	if fileExists(core.SelfSignedCert) && fileExists(core.SelfSignedKey) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(core.SelfSignedCert), 0o700); err != nil {
		return err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "cloudrix-node"},
		DNSNames:     []string{"cloudrix-node"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	kb, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(core.SelfSignedKey, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600); err != nil {
		return err
	}
	return os.WriteFile(core.SelfSignedCert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}

// certSHA256 returns the hex SHA-256 of the PEM certificate's DER bytes,
// the form Xray's pinnedPeerCertSha256 expects.
func certSHA256(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		return "", errors.New("no PEM block")
	}
	sum := sha256.Sum256(blk.Bytes)
	return hex.EncodeToString(sum[:]), nil
}

// hysteria reads the Hysteria2 setup the installer wrote (domain, UDP port
// and the Let's Encrypt certificate), or nil when there is none.
func (a *Agent) hysteria() *nodeproto.Hysteria {
	raw, err := os.ReadFile(a.path("hysteria.json"))
	if err != nil {
		return nil
	}
	var h nodeproto.Hysteria
	if json.Unmarshal(raw, &h) != nil || h.Domain == "" || h.Port <= 0 || !fileExists(h.Cert) || !fileExists(h.Key) {
		return nil
	}
	return &h
}

// writeShared stores the main-server certificates the panel sent, at the
// same paths (only under core.SharedTLSDir). It reports whether any changed.
func writeShared(files map[string]string) (bool, error) {
	changed := false
	for p, content := range files {
		if !core.Shared(p) || filepath.Clean(p) != p {
			return changed, fmt.Errorf("refusing to write %s", p)
		}
		if old, err := os.ReadFile(p); err == nil && string(old) == content {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return changed, err
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			return changed, err
		}
		changed = true
	}
	return changed, nil
}
