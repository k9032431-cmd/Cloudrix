// Package auth handles admin passwords, session tokens and secret generation.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/curve25519"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

func HashPassword(pw string) (string, error) {
	if len(pw) < 8 {
		return "", errors.New("password must be at least 8 characters")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// dummyHash lets failed lookups spend the same time as a real comparison.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("cloudrix-timing-guard"), bcrypt.DefaultCost)

func SpendCompareTime(pw string) {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(pw))
}

type Claims struct {
	Role model.Role `json:"role"`
	jwt.RegisteredClaims
}

type Issuer struct {
	secret []byte
	ttl    time.Duration
}

func NewIssuer(secret string, ttl time.Duration) *Issuer {
	return &Issuer{secret: []byte(secret), ttl: ttl}
}

func (i *Issuer) Issue(a *model.Admin) (string, time.Time, error) {
	exp := time.Now().Add(i.ttl)
	claims := Claims{
		Role: a.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(a.ID, 10),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(exp),
			Issuer:    "cloudrix",
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
	return tok, exp, err
}

// Parse validates a token and returns the admin id it was issued for.
func (i *Issuer) Parse(token string) (int64, *Claims, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(token, &c, func(t *jwt.Token) (any, error) {
		return i.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer("cloudrix"))
	if err != nil {
		return 0, nil, err
	}
	id, err := strconv.ParseInt(c.Subject, 10, 64)
	if err != nil {
		return 0, nil, fmt.Errorf("bad subject: %w", err)
	}
	return id, &c, nil
}

func RandomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return b
}

// RandomToken returns a URL-safe token with n bytes of entropy.
func RandomToken(n int) string {
	return base64.RawURLEncoding.EncodeToString(RandomBytes(n))
}

func RandomHex(n int) string {
	return hex.EncodeToString(RandomBytes(n))
}

func NewUUID() string {
	b := RandomBytes(16)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// X25519KeyPair returns a base64 private/public key pair. WireGuard uses
// standard base64, Xray REALITY uses raw URL-safe base64.
func X25519KeyPair(enc *base64.Encoding) (priv, pub string) {
	k := RandomBytes(32)
	k[0] &= 248
	k[31] = (k[31] & 127) | 64
	p, err := curve25519.X25519(k, curve25519.Basepoint)
	if err != nil {
		panic(err)
	}
	return enc.EncodeToString(k), enc.EncodeToString(p)
}

func NewCredentials() model.Credentials {
	priv, pub := X25519KeyPair(base64.StdEncoding)
	return model.Credentials{
		UUID:         NewUUID(),
		Password:     RandomToken(18),
		WGPrivateKey: priv,
		WGPublicKey:  pub,
	}
}
