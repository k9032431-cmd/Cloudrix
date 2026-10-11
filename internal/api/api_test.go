package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/auth"
	"github.com/k9032431-cmd/cloudrix/internal/config"
	"github.com/k9032431-cmd/cloudrix/internal/happ"
	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/store"
)

type env struct {
	t    *testing.T
	srv  *httptest.Server
	st   *store.Store
	api  *Server
	node *int64 // every inbound in the tests is served by this node
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
	happ.APIURL = "http://127.0.0.1:1/" // tests never call Happ's service: crypt4 fallback
	cfg := config.Config{SubPath: "/sub", DefaultHost: "vpn.example.com", SubTitle: "Test", SubUpdateHours: 12}
	s := New(cfg, st, auth.NewIssuer("test-secret", time.Hour), slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	n := &model.Node{Name: "test", Address: "vpn.example.com", UsageCoefficient: 1, Status: model.NodeConnecting}
	if err := st.CreateNode(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	return &env{t: t, srv: srv, st: st, api: s, node: &n.ID}
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
	in := model.Inbound{Tag: "vless-reality", Protocol: model.ProtoVLESS, Port: 443, NodeID: e.node, Enabled: true, Settings: model.InboundSettings{
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
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/core/config?core=xray&node="+strconv.FormatInt(*e.node, 10), nil)
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
	e.do("POST", "/api/inbounds", root, model.Inbound{Tag: "tr", Protocol: model.ProtoTrojan, Port: 443, NodeID: e.node, Enabled: true, Settings: model.InboundSettings{Security: "tls"}}, nil)
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

func TestSubscriptionBranding(t *testing.T) {
	e := newEnv(t)
	root := e.login("root", "supersecret")
	e.do("POST", "/api/inbounds", root, model.Inbound{Tag: "tr", Protocol: model.ProtoTrojan, Port: 443, NodeID: e.node, Enabled: true, Settings: model.InboundSettings{Security: "tls"}}, nil)
	exp := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	var u model.User
	e.do("POST", "/api/users", root, userInput{Username: "vip", Note: "VIP User", DataLimit: 250 << 30, ExpireAt: &exp}, &u)

	bad := branding{Title: "", UpdateHours: 1}
	if resp := e.do("PUT", "/api/settings/subscription", root, bad, nil); resp.StatusCode != 400 {
		t.Fatalf("empty title accepted: %d", resp.StatusCode)
	}
	bad = branding{Title: "x", UpdateHours: 1, SupportURL: "javascript:alert(1)"}
	if resp := e.do("PUT", "/api/settings/subscription", root, bad, nil); resp.StatusCode != 400 {
		t.Fatalf("bad link accepted: %d", resp.StatusCode)
	}

	want := branding{
		Title:       "VIP COREX",
		Announce:    "✦ VIP COREX ✦\n⚡ Your Exclusive Connection\n👤 Username: {NOTE}\n📅 Expires: {EXPIRE_DATE}\nLeft: {DATA_LEFT}",
		UpdateHours: 1,
		SupportURL:  "https://t.me/corex_support",
		WebPageURL:  "https://corex.example.com",
	}
	if resp := e.do("PUT", "/api/settings/subscription", root, want, nil); resp.StatusCode != 200 {
		t.Fatalf("save branding: %d", resp.StatusCode)
	}
	var got branding
	e.do("GET", "/api/settings/subscription", root, nil, &got)
	if got != want {
		t.Fatalf("branding not persisted: %+v", got)
	}

	resp, err := http.Get(e.srv.URL + "/sub/" + u.SubToken)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	decodeHdr := func(name string) string {
		v := strings.TrimPrefix(resp.Header.Get(name), "base64:")
		b, _ := base64.StdEncoding.DecodeString(v)
		return string(b)
	}
	if decodeHdr("Profile-Title") != "VIP COREX" || resp.Header.Get("Profile-Update-Interval") != "1" {
		t.Fatalf("title/interval headers: %q %q", decodeHdr("Profile-Title"), resp.Header.Get("Profile-Update-Interval"))
	}
	announce := decodeHdr("Announce")
	if !strings.Contains(announce, "Username: VIP User") || !strings.Contains(announce, "Expires: 29.10.2026") || !strings.Contains(announce, "Left: 250.0 GB") {
		t.Fatalf("announce not rendered for user: %q", announce)
	}
	if resp.Header.Get("Support-Url") != want.SupportURL || resp.Header.Get("Profile-Web-Page-Url") != want.WebPageURL {
		t.Fatalf("link headers: %v", resp.Header)
	}

	var info subscriptionInfo
	e.do("GET", "/sub/"+u.SubToken+"/info", "", nil, &info)
	if info.Title != "VIP COREX" || info.Announce != announce || info.SupportURL != want.SupportURL {
		t.Fatalf("info: %+v", info)
	}

	// Non-sudo admins cannot change branding.
	e.do("POST", "/api/admins", root, adminInput{Username: "helper", Password: "helperpass", Role: model.RoleAdmin}, nil)
	helper := e.login("helper", "helperpass")
	if resp := e.do("PUT", "/api/settings/subscription", helper, want, nil); resp.StatusCode != 403 {
		t.Fatalf("admin changed branding: %d", resp.StatusCode)
	}
}

func TestSubscriptionProtection(t *testing.T) {
	e := newEnv(t)
	root := e.login("root", "supersecret")
	e.do("POST", "/api/inbounds", root, model.Inbound{Tag: "tr", Protocol: model.ProtoTrojan, Port: 443, NodeID: e.node, Enabled: true, Settings: model.InboundSettings{Security: "tls"}}, nil)
	var u model.User
	e.do("POST", "/api/users", root, userInput{Username: "alice"}, &u)

	// provider features need a provider ID
	if r := e.do("PUT", "/api/settings/subscription", root, branding{Title: "X", UpdateHours: 1, HideSettings: true}, nil); r.StatusCode != 400 {
		t.Fatalf("hide settings without provider id: %d", r.StatusCode)
	}
	b := branding{Title: "X", UpdateHours: 1, HappOnly: true, ProviderID: "abc123", HideSettings: true, AlwaysHWID: true}
	if r := e.do("PUT", "/api/settings/subscription", root, b, nil); r.StatusCode != 200 {
		t.Fatalf("save: %d", r.StatusCode)
	}

	fetch := func(ua string) (*http.Response, string) {
		req, _ := http.NewRequest("GET", e.srv.URL+"/sub/"+u.SubToken, nil)
		req.Header.Set("User-Agent", ua)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		dec, _ := base64.StdEncoding.DecodeString(string(body))
		return res, string(dec)
	}
	// another client gets no keys
	if _, body := fetch("v2rayNG/1.9"); strings.Contains(body, u.Credentials.Password) {
		t.Fatalf("non-Happ client got keys: %q", body)
	}
	// Happ gets the keys and is told to hide them
	res, body := fetch("Happ/3.8.1")
	if !strings.Contains(body, u.Credentials.Password) || res.Header.Get("hide-settings") != "1" ||
		res.Header.Get("providerid") != "abc123" || res.Header.Get("subscription-always-hwid-enable") != "1" {
		t.Fatalf("happ response: %v %q", res.Header, body)
	}

	// the public page shows only the encrypted link
	var info subscriptionInfo
	e.do("GET", "/sub/"+u.SubToken+"/info", "", nil, &info)
	if !info.Protected || info.URL != "" || len(info.Links) != 0 || !strings.HasPrefix(info.HappLink, "happ://crypt4/") {
		t.Fatalf("protected info: %+v", info)
	}
	var link map[string]string
	if r := e.do("GET", "/api/users/"+itoa(u.ID)+"/happ-link", root, nil, &link); r.StatusCode != 200 || !strings.HasPrefix(link["link"], "happ://crypt") {
		t.Fatalf("happ link: %d %v", r.StatusCode, link)
	}
}

func TestInboundSyncsUserLists(t *testing.T) {
	e := newEnv(t)
	root := e.login("root", "supersecret")
	ib := func(tag string) model.Inbound {
		return model.Inbound{Tag: tag, Protocol: model.ProtoTrojan, Port: 443, NodeID: e.node, Enabled: true, Settings: model.InboundSettings{Security: "tls"}}
	}
	var a model.Inbound
	e.do("POST", "/api/inbounds", root, ib("a"), &a)
	e.do("POST", "/api/inbounds", root, ib("b"), nil)
	var picked model.User
	e.do("POST", "/api/users", root, userInput{Username: "picked", Inbounds: []string{"a", "b"}}, &picked)
	tags := func(id int64) []string {
		var u model.User
		e.do("GET", fmt.Sprintf("/api/users/%d", id), root, nil, &u)
		return u.Inbounds
	}

	a.Tag = "a2"
	e.do("PUT", fmt.Sprintf("/api/inbounds/%d", a.ID), root, a, nil)
	if got := tags(picked.ID); !slices.Equal(got, []string{"a2", "b"}) {
		t.Fatalf("after rename: %v", got)
	}
	e.do("DELETE", fmt.Sprintf("/api/inbounds/%d", a.ID), root, nil, nil)
	if got := tags(picked.ID); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("after delete: %v", got)
	}
}

func TestGroups(t *testing.T) {
	e := newEnv(t)
	root := e.login("root", "supersecret")
	var groups []model.Group
	e.do("GET", "/api/groups", root, nil, &groups)
	if len(groups) != 1 || groups[0].Name != "Default" {
		t.Fatalf("default group: %+v", groups)
	}
	def := groups[0].ID
	ib := func(tag string) model.Inbound {
		return model.Inbound{Tag: tag, Protocol: model.ProtoTrojan, Port: 443, NodeID: e.node, Enabled: true, Settings: model.InboundSettings{Security: "tls"}}
	}
	e.do("POST", fmt.Sprintf("/api/inbounds?groups=%d", def), root, ib("one"), nil)
	var vip model.Group
	if r := e.do("POST", "/api/groups", root, groupInput{Name: "VIP"}, &vip); r.StatusCode != 201 {
		t.Fatalf("create group: %d", r.StatusCode)
	}
	var u model.User
	e.do("POST", "/api/users", root, userInput{Username: "grp", GroupIDs: []int64{def}}, &u)
	links := func() string {
		res, err := http.Get(e.srv.URL + "/sub/" + u.SubToken + "?format=links")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return string(b)
	}
	if got := links(); !strings.Contains(got, "#one") {
		t.Fatalf("group inbound missing: %q", got)
	}

	// A new inbound added to the group reaches its users; one outside does not.
	e.do("POST", fmt.Sprintf("/api/inbounds?groups=%d", def), root, ib("two"), nil)
	e.do("POST", fmt.Sprintf("/api/inbounds?groups=%d", vip.ID), root, ib("vip"), nil)
	got := links()
	if !strings.Contains(got, "#two") || strings.Contains(got, "#vip") {
		t.Fatalf("after adding inbounds: %q", got)
	}

	// Disabled group: no keys. Groups with users cannot be deleted.
	e.do("PUT", fmt.Sprintf("/api/groups/%d", def), root, groupInput{Name: "Default", Disabled: true}, nil)
	if got := links(); strings.Contains(got, "#one") {
		t.Fatalf("disabled group still served: %q", got)
	}
	if r := e.do("DELETE", fmt.Sprintf("/api/groups/%d", def), root, nil, nil); r.StatusCode != 409 {
		t.Fatalf("delete group with users: %d", r.StatusCode)
	}
	if r := e.do("DELETE", fmt.Sprintf("/api/groups/%d", vip.ID), root, nil, nil); r.StatusCode != 204 {
		t.Fatalf("delete empty group: %d", r.StatusCode)
	}
}
