// Package nodeproto is the wire protocol between the panel and node agents.
//
// The agent calls the panel (nodes need no open control port). Every request
// is signed with the node secret (HMAC-SHA256 over node id, a strictly
// increasing millisecond timestamp and the body hash), and both request and
// response bodies are sealed with AES-256-GCM under a key derived from the
// same secret. The channel therefore stays private and tamper-proof even when
// the panel itself is served over plain HTTP.
package nodeproto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

const (
	SyncPath   = "/api/node/v1/sync"
	HeaderNode = "X-Cloudrix-Node"
	HeaderTime = "X-Cloudrix-Time"
	HeaderSig  = "X-Cloudrix-Sig"
	// MaxSkewMillis is how far a request timestamp may drift from the panel clock.
	MaxSkewMillis = 5 * 60 * 1000
)

// Stat is traffic one user generated on the node since the last report.
type Stat struct {
	Email string `json:"email"`
	Up    int64  `json:"up"`
	Down  int64  `json:"down"`
}

// SyncRequest is what the agent reports on every call.
type SyncRequest struct {
	Version string `json:"version"` // config version the agent runs ("" = none)
	Stats   []Stat `json:"stats,omitempty"`
	Cores   string `json:"cores,omitempty"` // e.g. "xray 25.9.11, sing-box 1.12.8"
	Agent   string `json:"agent,omitempty"` // agent build
	Error   string `json:"error,omitempty"` // last core error, shown in the panel
	Wait    bool   `json:"wait,omitempty"`  // long-poll until the config changes
	// CertSHA256 is the hex SHA-256 of the agent's self-signed certificate.
	CertSHA256 string `json:"cert_sha256,omitempty"`
	// Online maps user email to the IPs it is connected from right now and
	// when each last opened a connection (unix seconds); OnlineReported tells
	// an empty map apart from an agent that cannot tell.
	Online         map[string]map[string]int64 `json:"online,omitempty"`
	OnlineReported bool                        `json:"online_reported,omitempty"`
	// Kicks is the version of the device cut-offs the agent has applied.
	Kicks string `json:"kicks,omitempty"`
	// Hysteria is set once the installer gave this node a domain with a
	// real certificate; the panel then creates the Hysteria2 inbound.
	Hysteria *Hysteria `json:"hysteria,omitempty"`
}

// Hysteria describes a node's Hysteria2 setup made by the installer.
type Hysteria struct {
	Domain string `json:"domain"`
	Port   int    `json:"port"`
	Cert   string `json:"cert"`
	Key    string `json:"key"`
}

// SyncResponse carries the core configs when they changed.
type SyncResponse struct {
	Version  string          `json:"version"`
	Changed  bool            `json:"changed"`
	Disabled bool            `json:"disabled,omitempty"`
	Xray     json.RawMessage `json:"xray,omitempty"`
	SingBox  json.RawMessage `json:"sing_box,omitempty"`
	Problem  string          `json:"problem,omitempty"` // config could not be built; keep the old one
	// Kicks lists devices over a user's device limit to cut off (core email
	// → IPs), applied as routing rules without restarting the core.
	Kicks        map[string][]string `json:"kicks,omitempty"`
	KicksVersion string              `json:"kicks_version,omitempty"`
	// Files are certificates issued on the main server that this node's
	// inbounds use (path → PEM); the node writes them to the same paths.
	Files map[string]string `json:"files,omitempty"`
}

// NewSecret returns a random node secret (32 bytes, base64url).
func NewSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func key(secret, purpose string) []byte {
	h := sha256.Sum256([]byte("cloudrix-node-v1|" + purpose + "|" + secret))
	return h[:]
}

// Sign returns the request signature.
func Sign(secret string, nodeID, ts int64, body []byte) string {
	sum := sha256.Sum256(body)
	m := hmac.New(sha256.New, key(secret, "sign"))
	fmt.Fprintf(m, "%d\n%d\n%x", nodeID, ts, sum)
	return hex.EncodeToString(m.Sum(nil))
}

// Verify checks a signature in constant time.
func Verify(secret string, nodeID, ts int64, body []byte, sig string) bool {
	want, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	got, _ := hex.DecodeString(Sign(secret, nodeID, ts, body))
	return hmac.Equal(want, got)
}

// Seal encrypts plaintext for the given direction ("req" or "resp").
func Seal(secret, dir string, plaintext []byte) ([]byte, error) {
	g, err := gcm(secret, dir)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return g.Seal(nonce, nonce, plaintext, nil), nil
}

// Open decrypts data produced by Seal for the same direction.
func Open(secret, dir string, data []byte) ([]byte, error) {
	g, err := gcm(secret, dir)
	if err != nil {
		return nil, err
	}
	if len(data) < g.NonceSize() {
		return nil, errors.New("sealed body too short")
	}
	return g.Open(nil, data[:g.NonceSize()], data[g.NonceSize():], nil)
}

func gcm(secret, dir string) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key(secret, "seal-"+dir))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// UserID extracts the panel user id from a core email ("<id>.<username>").
func UserID(email string) (int64, bool) {
	for i := 0; i < len(email); i++ {
		if email[i] == '.' {
			id, err := strconv.ParseInt(email[:i], 10, 64)
			return id, err == nil && id > 0
		}
	}
	return 0, false
}
