package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

func TestCores(t *testing.T) {
	e := newEnv(t)
	root := e.login("root", "supersecret")
	var cores []model.Core
	e.do("GET", "/api/cores", root, nil, &cores)
	if len(cores) != 1 || cores[0].Name != "Default" || cores[0].Nodes != 1 {
		t.Fatalf("default core: %+v", cores)
	}
	def := cores[0].ID

	// two nodes on one core both serve its inbound: a host without an
	// address gives one link per node
	var second model.Node
	e.do("POST", "/api/nodes", root, nodeInput{Name: "de", Address: "de.example.com"}, &second)
	var in model.Inbound
	if r := e.do("POST", "/api/inbounds", root, model.Inbound{Tag: "tr", Protocol: model.ProtoTrojan, Port: 443, CoreID: def, Enabled: true, Settings: model.InboundSettings{Security: "tls"}}, &in); r.StatusCode != 201 {
		t.Fatalf("inbound: %d", r.StatusCode)
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
	if l := links(); len(l) != 2 || !strings.Contains(strings.Join(l, " "), "@de.example.com:443") {
		t.Fatalf("links of a two-node core: %v", l)
	}
	cfg := func(node int64) string {
		x, _, _, _, err := e.api.nodeConfigs(t.Context(), node)
		if err != nil {
			t.Fatal(err)
		}
		return string(x)
	}
	if !strings.Contains(cfg(second.ID), `"tag": "tr"`) {
		t.Fatal("second node does not run the core's inbound")
	}

	// moving the node to another core takes the inbound off it
	var other model.Core
	if r := e.do("POST", "/api/cores", root, map[string]string{"name": "Germany"}, &other); r.StatusCode != 201 {
		t.Fatalf("create core: %d", r.StatusCode)
	}
	if r := e.do("PUT", fmt.Sprintf("/api/nodes/%d", second.ID), root, nodeInput{Name: "de", Address: "de.example.com", CoreID: other.ID}, nil); r.StatusCode != 200 {
		t.Fatalf("switch core: %d", r.StatusCode)
	}
	if strings.Contains(cfg(second.ID), `"tag": "tr"`) {
		t.Fatal("node still runs its old core")
	}
	if l := links(); len(l) != 1 {
		t.Fatalf("links after the move: %v", l)
	}

	// a host only for some statuses
	h := in.Hosts[0]
	h.Remark, h.Status = "only limited", []model.UserStatus{model.StatusLimited}
	if r := e.do("PUT", fmt.Sprintf("/api/hosts/%d", h.ID), root, h, nil); r.StatusCode != 200 {
		t.Fatalf("status filter: %d", r.StatusCode)
	}
	if l := links(); len(l) != 0 {
		t.Fatalf("active user got a limited-only host: %v", l)
	}

	// duplicate copies inbounds and hosts; the default core stays
	var dup model.Core
	if r := e.do("POST", fmt.Sprintf("/api/cores/%d/duplicate", def), root, nil, &dup); r.StatusCode != 201 || dup.Inbounds != 1 {
		t.Fatalf("duplicate: %d %+v", r.StatusCode, dup)
	}
	if r := e.do("DELETE", fmt.Sprintf("/api/cores/%d", def), root, nil, nil); r.StatusCode != 400 {
		t.Fatalf("delete default: %d", r.StatusCode)
	}
	if r := e.do("DELETE", fmt.Sprintf("/api/cores/%d", dup.ID), root, nil, nil); r.StatusCode != 204 {
		t.Fatalf("delete copy: %d", r.StatusCode)
	}
	var list []model.Inbound
	if e.do("GET", "/api/inbounds", root, nil, &list); len(list) != 1 {
		t.Fatalf("copied inbounds left: %d", len(list))
	}
}
