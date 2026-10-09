package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/auth"
	"github.com/k9032431-cmd/cloudrix/internal/config"
	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/store"
)

type env struct {
	t   *testing.T
	srv *httptest.Server
	st  *store.Store
}

func newEnv(t *testing.T) *env {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	hash, _ := auth.HashPassword("supersecret")
	if err := st.CreateAdmin(context.Background(), &model.Admin{Username: "root", PasswordHash: hash, Role: model.RoleSudo}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{SubPath: "/sub", DefaultHost: "vpn.example.com", SubTitle: "Test", SubUpdateHours: 12}
	s := New(cfg, st, auth.NewIssuer("test-secret", time.Hour), slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return &env{t: t, srv: srv, st: st}
}

func (e *env) do(method, path, token string, body any, out any) *http.Response {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, r)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			e.t.Fatalf("%s %s: decode: %v", method, path, err)
		}
	}
	return resp
}

func (e *env) login(user, pass string) string {
	var res loginResponse
	resp := e.do("POST", "/api/auth/login", "", loginRequest{user, pass}, &res)
	if resp.StatusCode != 200 {
		e.t.Fatalf("login %s: %d", user, resp.StatusCode)
	}
	return res.Token
}

func TestEndToEnd(t *testing.T) {
	e := newEnv(t)

	if resp := e.do("POST", "/api/auth/login", "", loginRequest{"root", "wrong-password"}, nil); resp.StatusCode != 401 {
		t.Fatalf("bad password: %d", resp.StatusCode)
	}
	root := e.login("root", "supersecret")

	// Inbound with REALITY keys from the tools endpoint.
	var keys map[string]any
	e.do("POST", "/api/tools/reality-keys", root, nil, &keys)
	in := model.Inbound{Tag: "vless-reality", Protocol: model.ProtoVLESS, Port: 443, Enabled: true, Settings: model.InboundSettings{
		Security: "reality", Flow: "xtls-rprx-vision", RealityDest: "www.example.com:443", RealityServerNames: []string{"www.example.com"},
		RealityPrivateKey: keys["private_key"].(string), RealityPublicKey: keys["public_key"].(string), RealityShortIDs: []string{"ab12"},
	}}
	if resp := e.do("POST", "/api/inbounds", root, in, nil); resp.StatusCode != 201 {
		t.Fatalf("create inbound: %d", resp.StatusCode)
	}

	// Reseller with a quota of one user.
	resp := e.do("POST", "/api/admins", root, adminInput{Username: "reseller", Password: "resellerpass", Role: model.RoleReseller, UserLimit: 1}, nil)
	if resp.StatusCode != 201 {
		t.Fatalf("create reseller: %d", resp.StatusCode)
	}
	reseller := e.login("reseller", "resellerpass")
	if resp := e.do("GET", "/api/admins", reseller, nil, nil); resp.StatusCode != 403 {
		t.Fatalf("reseller must not list admins: %d", resp.StatusCode)
	}

	var u model.User
	if resp := e.do("POST", "/api/users", reseller, userInput{Username: "alice", DataLimit: 1 << 30}, &u); resp.StatusCode != 201 {
		t.Fatalf("create user: %d", resp.StatusCode)
	}
	if u.Status != model.StatusActive || u.SubToken == "" || u.Credentials.UUID == "" {
		t.Fatalf("unexpected user: %+v", u)
	}
	if resp := e.do("POST", "/api/users", reseller, userInput{Username: "bob"}, nil); resp.StatusCode != 403 {
		t.Fatalf("reseller quota not enforced: %d", resp.StatusCode)
	}
	if resp := e.do("POST", "/api/users", root, userInput{Username: "carol"}, nil); resp.StatusCode != 201 {
		t.Fatalf("root create: %d", resp.StatusCode)
	}

	// Reseller sees only own users.
	var list struct {
		Users []model.User `json:"users"`
		Total int          `json:"total"`
	}
	e.do("GET", "/api/users", reseller, nil, &list)
	if list.Total != 1 || list.Users[0].Username != "alice" {
		t.Fatalf("reseller list: %+v", list)
	}
	e.do("GET", "/api/users?search=car", root, nil, &list)
	if list.Total != 1 || list.Users[0].Username != "carol" {
		t.Fatalf("root search: %+v", list)
	}

	// Subscription formats.
	subURL := e.srv.URL + "/sub/" + u.SubToken
	get := func(url, ua string) (*http.Response, string) {
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("User-Agent", ua)
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		return r, string(b)
	}
	r, body := get(subURL, "v2rayNG/1.9")
	decoded, err := base64.StdEncoding.DecodeString(body)
	if r.StatusCode != 200 || err != nil || !strings.HasPrefix(string(decoded), "vless://"+u.Credentials.UUID+"@vpn.example.com:443") {
		t.Fatalf("base64 sub: %d %q", r.StatusCode, decoded)
	}
	if !strings.Contains(r.Header.Get("Subscription-Userinfo"), "total=1073741824") {
		t.Fatalf("userinfo header: %q", r.Header.Get("Subscription-Userinfo"))
	}
	if _, body := get(subURL, "clash.meta/1.18"); !strings.Contains(body, "reality-opts") {
		t.Fatalf("clash sub: %s", body)
	}
	if _, body := get(subURL, "SFA/1.11"); !strings.Contains(body, `"reality"`) {
		t.Fatalf("sing-box sub: %s", body)
	}
	if r, _ := get(e.srv.URL+"/sub/doesnotexist-token-1234", "x"); r.StatusCode != 404 {
		t.Fatalf("unknown token: %d", r.StatusCode)
	}

	// Disabling the user empties the subscription.
	upd := userInput{Username: "alice", Status: model.StatusDisabled, DataLimit: 1 << 30}
	if resp := e.do("PUT", "/api/users/"+itoa(u.ID), reseller, upd, nil); resp.StatusCode != 200 {
		t.Fatalf("update: %d", resp.StatusCode)
	}
	if _, body := get(subURL+"?format=links", "x"); body != "" {
		t.Fatalf("disabled user must get no links, got %q", body)
	}

	// Revoke rotates the token.
	var revoked model.User
	e.do("POST", "/api/users/"+itoa(u.ID)+"/revoke", reseller, nil, &revoked)
	if revoked.SubToken == u.SubToken {
		t.Fatal("token not rotated")
	}
	if r, _ := get(subURL, "x"); r.StatusCode != 404 {
		t.Fatalf("old token still works: %d", r.StatusCode)
	}

	// Core config preview contains the user's credentials.
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/core/config?core=xray", nil)
	req.Header.Set("Authorization", "Bearer "+root)
	cr, _ := http.DefaultClient.Do(req)
	cb, _ := io.ReadAll(cr.Body)
	cr.Body.Close()
	if cr.StatusCode != 200 || !strings.Contains(string(cb), "carol") || strings.Contains(string(cb), revoked.Credentials.UUID) {
		t.Fatalf("core config: %d (disabled alice must be absent)\n%s", cr.StatusCode, cb)
	}

	// Bulk delete by the reseller cannot touch root's users.
	var bulk map[string]int
	e.do("POST", "/api/users/bulk", reseller, bulkRequest{IDs: []int64{1, 2}, Action: "delete"}, &bulk)
	if bulk["affected"] != 1 {
		t.Fatalf("bulk affected = %d", bulk["affected"])
	}
}

func TestDeviceLimit(t *testing.T) {
	e := newEnv(t)
	root := e.login("root", "supersecret")
	e.do("POST", "/api/inbounds", root, model.Inbound{Tag: "tr", Protocol: model.ProtoTrojan, Port: 443, Enabled: true, Settings: model.InboundSettings{Security: "tls"}}, nil)
	var u model.User
	e.do("POST", "/api/users", root, userInput{Username: "dev", DeviceLimit: 1}, &u)

	fetch := func(hwid string) string {
		req, _ := http.NewRequest("GET", e.srv.URL+"/sub/"+u.SubToken+"?format=links", nil)
		if hwid != "" {
			req.Header.Set("x-hwid", hwid)
		}
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		return string(b)
	}
	if !strings.HasPrefix(fetch("phone"), "trojan://") {
		t.Fatal("first device must get links")
	}
	if !strings.HasPrefix(fetch("phone"), "trojan://") {
		t.Fatal("known device must keep access")
	}
	if fetch("laptop") != "" {
		t.Fatal("second device must be refused")
	}
	if fetch("") != "" {
		t.Fatal("client without hwid must be refused when a limit is set")
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
