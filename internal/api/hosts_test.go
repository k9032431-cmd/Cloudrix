package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

func TestHosts(t *testing.T) {
	e := newEnv(t)
	root := e.login("root", "supersecret")
	var in model.Inbound
	e.do("POST", "/api/inbounds", root, model.Inbound{Tag: "tr", Protocol: model.ProtoTrojan, Port: 443, NodeID: e.node, Enabled: true,
		Address: "a.example.com", Remark: "A {USERNAME}", Settings: model.InboundSettings{Security: "tls"}}, &in)
	if len(in.Hosts) != 1 || len(in.Hosts[0].Address) != 1 || in.Hosts[0].Address[0] != "a.example.com" {
		t.Fatalf("first host: %+v", in.Hosts)
	}
	var u model.User
	e.do("POST", "/api/users", root, userInput{Username: "bob"}, &u)
	links := func() []string {
		r, err := http.Get(e.srv.URL + "/sub/" + u.SubToken + "?format=links")
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		return strings.Fields(string(b))
	}

	// a second host: own address, port, remark and SNI
	var b model.Host
	nh := model.Host{InboundID: in.ID, Enabled: true, HostSettings: model.HostSettings{Remark: "B", Address: []string{"b.example.com"}, Port: 8443, SNI: []string{"cdn.example.com"}}}
	if r := e.do("POST", "/api/hosts", root, nh, &b); r.StatusCode != 201 || b.Priority <= in.Hosts[0].Priority {
		t.Fatalf("create host: %d %+v", r.StatusCode, b)
	}
	l := links()
	if len(l) != 2 || !strings.Contains(l[0], "@a.example.com:443") || !strings.Contains(l[0], "#A%20bob") ||
		!strings.Contains(l[1], "@b.example.com:8443") || !strings.Contains(l[1], "sni=cdn.example.com") {
		t.Fatalf("links: %v", l)
	}

	// order follows the hosts list
	if r := e.do("PUT", "/api/hosts/order", root, map[string]any{"ids": []int64{b.ID, in.Hosts[0].ID}}, nil); r.StatusCode != 204 {
		t.Fatalf("reorder: %d", r.StatusCode)
	}
	if l := links(); !strings.Contains(l[0], "b.example.com") {
		t.Fatalf("after reorder: %v", l)
	}

	// a disabled host gives no link; a bad one is refused
	b.Enabled = false
	if r := e.do("PUT", fmt.Sprintf("/api/hosts/%d", b.ID), root, b, nil); r.StatusCode != 200 {
		t.Fatalf("disable: %d", r.StatusCode)
	}
	if l := links(); len(l) != 1 {
		t.Fatalf("disabled host still listed: %v", l)
	}
	bad := nh
	bad.Address = []string{"http://x"}
	if r := e.do("POST", "/api/hosts", root, bad, nil); r.StatusCode != 400 {
		t.Fatalf("bad address: %d", r.StatusCode)
	}
	bad = nh
	bad.InboundID = 999
	if r := e.do("POST", "/api/hosts", root, bad, nil); r.StatusCode != 404 {
		t.Fatalf("unknown inbound: %d", r.StatusCode)
	}

	// hosts go with their inbound
	e.do("DELETE", fmt.Sprintf("/api/inbounds/%d", in.ID), root, nil, nil)
	var hosts []model.Host
	if e.do("GET", "/api/hosts", root, nil, &hosts); len(hosts) != 0 {
		t.Fatalf("hosts left: %+v", hosts)
	}
}
