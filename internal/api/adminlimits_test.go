package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

func TestAdminLimits(t *testing.T) {
	e := newEnv(t)
	root := e.login("root", "supersecret")
	e.do("POST", "/api/inbounds", root, model.Inbound{Tag: "tr", Protocol: model.ProtoTrojan, Port: 443, NodeID: e.node, Enabled: true, Settings: model.InboundSettings{Security: "tls"}}, nil)
	const gb = int64(1 << 30)
	var res model.Admin
	if r := e.do("POST", "/api/admins", root, adminInput{Username: "shop", Password: "shop-secret", Role: model.RoleReseller,
		UserLimit: 5, DataLimit: 100 * gb, MaxUserData: 10 * gb, MaxUserDevices: 3}, &res); r.StatusCode != 201 {
		t.Fatalf("create reseller: %d", r.StatusCode)
	}
	shop := e.login("shop", "shop-secret")

	// per-user caps: a data limit and a device limit within them are required
	if r := e.do("POST", "/api/users", shop, userInput{Username: "nolimit"}, nil); r.StatusCode != 400 {
		t.Fatalf("unlimited user by capped reseller: %d", r.StatusCode)
	}
	if r := e.do("POST", "/api/users", shop, userInput{Username: "big", DataLimit: 20 * gb, DeviceLimit: 1}, nil); r.StatusCode != 400 {
		t.Fatalf("over traffic cap: %d", r.StatusCode)
	}
	if r := e.do("POST", "/api/users", shop, userInput{Username: "many", DataLimit: 5 * gb, DeviceLimit: 4}, nil); r.StatusCode != 400 {
		t.Fatalf("over device cap: %d", r.StatusCode)
	}
	var u model.User
	if r := e.do("POST", "/api/users", shop, userInput{Username: "client", DataLimit: 5 * gb, DeviceLimit: 2}, &u); r.StatusCode != 201 {
		t.Fatalf("user within caps: %d", r.StatusCode)
	}
	// the main admin sees how many users it made; the reseller sees its own plan
	var list []model.Admin
	e.do("GET", "/api/admins", root, nil, &list)
	for _, a := range list {
		if a.Username == "shop" && a.UsersCount != 1 {
			t.Fatalf("users of the reseller in the list: %d", a.UsersCount)
		}
	}
	var me model.Admin
	e.do("GET", "/api/auth/me", shop, nil, &me)
	if me.UsersCount != 1 || me.UserLimit != 5 || me.DataLimit != 100*gb {
		t.Fatalf("reseller's own plan: %+v", me)
	}

	links := func() string {
		req, _ := http.NewRequest("GET", e.srv.URL+"/sub/"+u.SubToken+"?format=links", nil)
		req.Header.Set("x-hwid", "phone-1") // the user has a device limit
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return string(b)
	}
	if !strings.Contains(links(), "trojan://") {
		t.Fatalf("user of an active reseller gets no keys: %q", links())
	}

	// the reseller's users used up its traffic: no keys, no sign-in
	if err := e.st.AddTraffic(context.Background(), u.ID, 100*gb); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(links(), "trojan://") {
		t.Fatal("keys served past the reseller's traffic limit")
	}
	body, _ := json.Marshal(map[string]string{"username": "shop", "password": "shop-secret"})
	if r, _ := http.Post(e.srv.URL+"/api/auth/login", "application/json", strings.NewReader(string(body))); r.StatusCode != 403 {
		t.Fatalf("over-limit reseller signed in: %d", r.StatusCode)
	}

	// the main admin resets the count: the keys come back; then the term ends
	in := adminInput{Username: "shop", Role: model.RoleReseller, UserLimit: 5, DataLimit: 100 * gb, MaxUserData: 10 * gb, MaxUserDevices: 3, ResetTraffic: true}
	e.do("PUT", fmt.Sprintf("/api/admins/%d", res.ID), root, in, nil)
	if !strings.Contains(links(), "trojan://") {
		t.Fatal("keys not back after the reset")
	}
	past := time.Now().Add(-time.Hour)
	in.ResetTraffic, in.ExpireAt = false, &past
	e.do("PUT", fmt.Sprintf("/api/admins/%d", res.ID), root, in, nil)
	if strings.Contains(links(), "trojan://") {
		t.Fatal("keys served after the reseller's term ended")
	}
	// a session that is already open ends with the term, saying why
	var e2 struct {
		Error string `json:"error"`
	}
	if r := e.do("GET", "/api/auth/me", shop, nil, &e2); r.StatusCode != 401 || !strings.Contains(e2.Error, "expired") {
		t.Fatalf("open session after the term: %d %q", r.StatusCode, e2.Error)
	}
}
