// Package happ builds encrypted subscription links for the Happ client.
//
// An encrypted link (happ://crypt5/… or happ://crypt4/…) hides the
// subscription address: once added, Happ does not let the user view, edit,
// copy or share the server configurations inside it. See
// https://www.happ.su/main/dev-docs/crypto-link.
package happ

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// APIURL is Happ's encryption service; it returns crypt5 links.
var APIURL = "https://crypto.happ.su/api-v2.php"

// publicKey is Happ's RSA-4096 key for crypt4 links (from the Happ docs).
// It is used when the encryption service cannot be reached.
const publicKey = `-----BEGIN PUBLIC KEY-----
MIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEAlBetA0wjbaj+h7oJ/d/h
pNrXvAcuhOdFGEFcfCxSWyLzWk4SAQ05gtaEGZyetTax2uqagi9HT6lapUSUe2S8
nMLJf5K+LEs9TYrhhBdx/B0BGahA+lPJa7nUwp7WfUmSF4hir+xka5ApHjzkAQn6
cdG6FKtSPgq1rYRPd1jRf2maEHwiP/e/jqdXLPP0SFBjWTMt/joUDgE7v/IGGB0L
Q7mGPAlgmxwUHVqP4bJnZ//5sNLxWMjtYHOYjaV+lixNSfhFM3MdBndjpkmgSfmg
D5uYQYDL29TDk6Eu+xetUEqry8ySPjUbNWdDXCglQWMxDGjaqYXMWgxBA1UKjUBW
wbgr5yKTJ7mTqhlYEC9D5V/LOnKd6pTSvaMxkHXwk8hBWvUNWAxzAf5JZ7EVE3jt
0j682+/hnmL/hymUE44yMG1gCcWvSpB3BTlKoMnl4yrTakmdkbASeFRkN3iMRewa
IenvMhzJh1fq7xwX94otdd5eLB2vRFavrnhOcN2JJAkKTnx9dwQwFpGEkg+8U613
+Tfm/f82l56fFeoFN98dD2mUFLFZoeJ5CG81ZeXrH83niI0joX7rtoAZIPWzq3Y1
Zb/Zq+kK2hSIhphY172Uvs8X2Qp2ac9UoTPM71tURsA9IvPNvUwSIo/aKlX5KE3I
VE0tje7twWXL5Gb1sfcXRzsCAwEAAQ==
-----END PUBLIC KEY-----`

// Crypt4 encrypts a URL locally with Happ's RSA-4096 key (PKCS#1 v1.5).
func Crypt4(url string) (string, error) {
	block, _ := pem.Decode([]byte(publicKey))
	if block == nil {
		return "", errors.New("bad happ public key")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", err
	}
	pub, ok := key.(*rsa.PublicKey)
	if !ok {
		return "", errors.New("happ key is not RSA")
	}
	ct, err := rsa.EncryptPKCS1v15(rand.Reader, pub, []byte(url))
	if err != nil {
		return "", err
	}
	return "happ://crypt4/" + base64.StdEncoding.EncodeToString(ct), nil
}

// Encryptor turns subscription URLs into encrypted Happ links, caching the
// result so each URL is sent to the service once.
type Encryptor struct {
	Client *http.Client
	mu     sync.Mutex
	cache  map[string]string
}

func NewEncryptor() *Encryptor {
	return &Encryptor{Client: &http.Client{Timeout: 10 * time.Second}, cache: map[string]string{}}
}

// Link returns a crypt5 link from Happ's service, or a locally made crypt4
// link when the service is unavailable.
func (e *Encryptor) Link(ctx context.Context, url string) (string, error) {
	e.mu.Lock()
	if l, ok := e.cache[url]; ok {
		e.mu.Unlock()
		return l, nil
	}
	e.mu.Unlock()
	link, err := e.crypt5(ctx, url)
	if err != nil {
		if link, err = Crypt4(url); err != nil {
			return "", err
		}
		return link, nil // not cached: try crypt5 again next time
	}
	e.mu.Lock()
	if len(e.cache) > 50000 {
		e.cache = map[string]string{}
	}
	e.cache[url] = link
	e.mu.Unlock()
	return link, nil
}

func (e *Encryptor) crypt5(ctx context.Context, url string) (string, error) {
	body, _ := json.Marshal(map[string]string{"url": url})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, APIURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := e.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("happ crypto service: HTTP %d", res.StatusCode)
	}
	var out struct {
		Link string `json:"encrypted_link"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if !strings.HasPrefix(out.Link, "happ://crypt") {
		return "", errors.New("happ crypto service returned no link")
	}
	return out.Link, nil
}
