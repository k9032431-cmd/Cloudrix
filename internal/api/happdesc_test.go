package api

import (
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

func TestHappDescriptionInSubscription(t *testing.T) {
	e := newEnv(t)
	root := e.login("root", "supersecret")
	var ib model.Inbound
	e.do("POST", "/api/inbounds", root, model.Inbound{Tag: "hy", Protocol: model.ProtoHysteria2, Port: 2053, NodeID: e.node, Enabled: true}, &ib)
	var u model.User
	e.do("POST", "/api/users", root, userInput{Username: "alice"}, &u)
	var hosts []model.Host
	e.do("GET", "/api/hosts", root, nil, &hosts)
	var h model.Host
	for _, x := range hosts {
		if x.InboundID == ib.ID {
			h = x
		}
	}
	h.Remark, h.Description = "HK", "YouTube Speed"
	if r := e.do("PUT", "/api/hosts/"+itoa(h.ID), root, h, nil); r.StatusCode != 200 {
		t.Fatalf("update host: %d", r.StatusCode)
	}
	fetch := func(ua string) string {
		req, _ := http.NewRequest("GET", e.srv.URL+"/sub/"+u.SubToken, nil)
		req.Header.Set("User-Agent", ua)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		dec, _ := base64.StdEncoding.DecodeString(string(body))
		return string(dec)
	}
	want := "#HK?serverDescription=" + base64.StdEncoding.EncodeToString([]byte("YouTube Speed"))
	if b := fetch("Happ/3.8.1"); !strings.Contains(b, want) {
		t.Fatalf("happ: %q", b)
	}
	if b := fetch("v2rayNG/1.9"); strings.Contains(b, "serverDescription") {
		t.Fatalf("other app: %q", b)
	}
}

// Happ ping settings reach the app as subscription headers and need a
// provider ID, like the other app settings.
func TestHappPingHeaders(t *testing.T) {
	e := newEnv(t)
	root := e.login("root", "supersecret")
	var u model.User
	e.do("POST", "/api/users", root, userInput{Username: "alice"}, &u)
	b := branding{Title: "X", UpdateHours: 1, PingType: "proxy-head", PingMode: "keepalive"}
	if r := e.do("PUT", "/api/settings/subscription", root, b, nil); r.StatusCode != 400 {
		t.Fatalf("ping settings without provider id: %d", r.StatusCode)
	}
	b.ProviderID, b.PingURL = "abc123", "https://cp.cloudflare.com/generate_204"
	if r := e.do("PUT", "/api/settings/subscription", root, b, nil); r.StatusCode != 200 {
		t.Fatalf("save: %d", r.StatusCode)
	}
	req, _ := http.NewRequest("GET", e.srv.URL+"/sub/"+u.SubToken, nil)
	req.Header.Set("User-Agent", "Happ/4.8.0")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.Header.Get("ping-type") != "proxy-head" || res.Header.Get("proxy-ping-mode") != "keepalive" ||
		res.Header.Get("check-url-via-proxy") != "https://cp.cloudflare.com/generate_204" {
		t.Fatalf("headers: %v", res.Header)
	}
}
