package api

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/nodeproto"
)

func TestHysteriaFromInstaller(t *testing.T) {
	e := newEnv(t)
	root := e.login("root", "supersecret")
	var node model.Node
	e.do("POST", "/api/nodes", root, map[string]any{"name": "fi", "address": "77.42.11.42"}, &node)
	var inst struct {
		Secret string `json:"secret"`
	}
	e.do("GET", "/api/nodes/"+strconv.FormatInt(node.ID, 10)+"/install", root, nil, &inst)
	var groups []model.Group
	e.do("GET", "/api/groups", root, nil, &groups)
	var u model.User
	e.do("POST", "/api/users", root, userInput{Username: "hyuser", GroupIDs: []int64{groups[0].ID}}, &u)

	ts := time.Now().UnixMilli()
	setup := func(domain string, port int) *nodeproto.SyncResponse {
		ts++
		_, resp := nodeCall(t, e, node.ID, inst.Secret, ts, nodeproto.SyncRequest{Hysteria: &nodeproto.Hysteria{
			Domain: domain, Port: port, Cert: "/var/lib/cloudrix-node/tls/hysteria/fullchain.pem", Key: "/var/lib/cloudrix-node/tls/hysteria/key.pem",
		}})
		return resp
	}
	link := func() string {
		res, err := http.Get(e.srv.URL + "/sub/" + u.SubToken + "?format=links")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return string(b)
	}

	resp := setup("hy.example.com", 443)
	if resp == nil || !resp.Changed || !strings.Contains(string(resp.Xray), "/var/lib/cloudrix-node/tls/hysteria/fullchain.pem") {
		t.Fatalf("node did not get the hysteria2 config")
	}
	got := link()
	if !strings.Contains(got, "@hy.example.com:443") || !strings.Contains(got, "sni=hy.example.com") || strings.Contains(got, "insecure") {
		t.Fatalf("hysteria2 link: %q", got)
	}

	// running the installer again with a new domain replaces the setup
	setup("hy2.example.com", 8443)
	var list []model.Inbound
	e.do("GET", "/api/inbounds", root, nil, &list)
	n := 0
	for _, in := range list {
		if in.Protocol == model.ProtoHysteria2 {
			n++
			if in.Address != "hy2.example.com" || in.Port != 8443 {
				t.Fatalf("not updated: %+v", in)
			}
		}
	}
	if n != 1 {
		t.Fatalf("%d hysteria2 inbounds, want 1", n)
	}
}
