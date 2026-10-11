package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/nodeproto"
)

// nodeCall performs one signed, sealed sync call like the agent does.
func nodeCall(t *testing.T, e *env, id int64, secret string, ts int64, req nodeproto.SyncRequest) (int, *nodeproto.SyncResponse) {
	t.Helper()
	plain, _ := json.Marshal(req)
	body, err := nodeproto.Seal(secret, "req", plain)
	if err != nil {
		t.Fatal(err)
	}
	hr, _ := http.NewRequest("POST", e.srv.URL+nodeproto.SyncPath, bytes.NewReader(body))
	hr.Header.Set(nodeproto.HeaderNode, strconv.FormatInt(id, 10))
	hr.Header.Set(nodeproto.HeaderTime, strconv.FormatInt(ts, 10))
	hr.Header.Set(nodeproto.HeaderSig, nodeproto.Sign(secret, id, ts, body))
	res, err := http.DefaultClient.Do(hr)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		return res.StatusCode, nil
	}
	out, err := nodeproto.Open(secret, "resp", data)
	if err != nil {
		t.Fatalf("open response: %v", err)
	}
	var resp nodeproto.SyncResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatal(err)
	}
	return 200, &resp
}

func TestNodeSync(t *testing.T) {
	e := newEnv(t)
	root := e.login("root", "supersecret")

	var node model.Node
	if r := e.do("POST", "/api/nodes", root, map[string]any{"name": "nl", "address": "46.8.103.95"}, &node); r.StatusCode != 201 {
		t.Fatalf("create node: %d", r.StatusCode)
	}
	var inst struct {
		Command string `json:"command"`
		Secret  string `json:"secret"`
	}
	e.do("GET", "/api/nodes/"+strconv.FormatInt(node.ID, 10)+"/install", root, nil, &inst)
	if inst.Secret == "" || !strings.Contains(inst.Command, "--secret "+inst.Secret) || !strings.Contains(inst.Command, "node --panel ") {
		t.Fatalf("install info: %+v", inst)
	}

	// an inbound without a node is refused: the panel serves no traffic itself
	if r := e.do("POST", "/api/inbounds", root, model.Inbound{Tag: "x", Protocol: model.ProtoTrojan, Port: 443, Enabled: true}, nil); r.StatusCode != 400 {
		t.Fatalf("nodeless inbound: %d", r.StatusCode)
	}
	if r := e.do("POST", "/api/inbounds", root, model.Inbound{Tag: "tr", Protocol: model.ProtoTrojan, Port: 443, NodeID: &node.ID, Enabled: true}, nil); r.StatusCode != 201 {
		t.Fatalf("inbound: %d", r.StatusCode)
	}
	var alice model.User
	e.do("POST", "/api/users", root, map[string]any{"username": "alice"}, &alice)

	ts := time.Now().UnixMilli()
	code, resp := nodeCall(t, e, node.ID, inst.Secret, ts, nodeproto.SyncRequest{Cores: "xray 25.9.11"})
	if code != 200 || !resp.Changed || !strings.Contains(string(resp.Xray), alice.Credentials.Password) || resp.SingBox != nil {
		t.Fatalf("first sync: %d %+v", code, resp)
	}

	// replay and forgery are rejected
	if c, _ := nodeCall(t, e, node.ID, inst.Secret, ts, nodeproto.SyncRequest{}); c != 409 {
		t.Fatalf("replay: %d", c)
	}
	if c, _ := nodeCall(t, e, node.ID, "wrong-secret", ts+1, nodeproto.SyncRequest{}); c != 401 {
		t.Fatalf("forged: %d", c)
	}

	// traffic is billed to the user; the node shows as connected
	email := strconv.FormatInt(alice.ID, 10) + ".alice"
	code, resp = nodeCall(t, e, node.ID, inst.Secret, ts+2, nodeproto.SyncRequest{Version: resp.Version, Stats: []nodeproto.Stat{{Email: email, Up: 1000, Down: 4000}}})
	if code != 200 || resp.Changed {
		t.Fatalf("stats sync: %d %+v", code, resp)
	}
	var got model.User
	e.do("GET", "/api/users/"+strconv.FormatInt(alice.ID, 10), root, nil, &got)
	if got.UsedTraffic != 5000 {
		t.Fatalf("used traffic = %d", got.UsedTraffic)
	}
	var nodes []model.Node
	e.do("GET", "/api/nodes", root, nil, &nodes)
	if nodes[len(nodes)-1].Status != model.NodeConnected || nodes[len(nodes)-1].CoreVersion != "" && nodes[len(nodes)-1].DownloadBytes != 4000 {
		t.Fatalf("node after sync: %+v", nodes[len(nodes)-1])
	}

	// a waiting agent hears about a new user right away (well before the 25s poll window ends)
	type result struct {
		code int
		resp *nodeproto.SyncResponse
	}
	done := make(chan result, 1)
	start := time.Now()
	go func() {
		c, r := nodeCall(t, e, node.ID, inst.Secret, ts+3, nodeproto.SyncRequest{Version: resp.Version, Wait: true})
		done <- result{c, r}
	}()
	time.Sleep(300 * time.Millisecond)
	var bob model.User
	e.do("POST", "/api/users", root, map[string]any{"username": "bob"}, &bob)
	r := <-done
	if r.code != 200 || !r.resp.Changed || !strings.Contains(string(r.resp.Xray), bob.Credentials.Password) || time.Since(start) > 15*time.Second {
		t.Fatalf("long poll: %d %+v after %s", r.code, r.resp, time.Since(start))
	}

	// disabling the node tells the agent to stop its cores
	e.do("PUT", "/api/nodes/"+strconv.FormatInt(node.ID, 10), root, map[string]any{"name": "nl", "address": "46.8.103.95", "disabled": true}, nil)
	if c, r := nodeCall(t, e, node.ID, inst.Secret, ts+4, nodeproto.SyncRequest{Version: r.resp.Version}); c != 200 || !r.Disabled {
		t.Fatalf("disabled: %d %+v", c, r)
	}
}
