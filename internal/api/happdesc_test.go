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
