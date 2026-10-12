package api

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/core"
	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/nodeproto"
)

func TestDeviceLimitKicksOldest(t *testing.T) {
	e := newEnv(t)
	root := e.login("root", "supersecret")
	var node model.Node
	e.do("POST", "/api/nodes", root, map[string]any{"name": "fi", "address": "fi.example.com"}, &node)
	var inst struct {
		Secret string `json:"secret"`
	}
	e.do("GET", "/api/nodes/"+strconv.FormatInt(node.ID, 10)+"/install", root, nil, &inst)
	e.do("POST", "/api/inbounds", root, model.Inbound{Tag: "tr", Protocol: model.ProtoTrojan, Port: 443, NodeID: &node.ID, Enabled: true, Settings: model.InboundSettings{Security: "tls"}}, nil)
	var u, other model.User
	e.do("POST", "/api/users", root, userInput{Username: "family", DeviceLimit: 2}, &u)
	e.do("POST", "/api/users", root, userInput{Username: "solo"}, &other)
	email := core.Email(&u)

	ts := time.Now().UnixMilli()
	sync := func(online map[string]map[string]int64, kicks string) *nodeproto.SyncResponse {
		ts++
		_, resp := nodeCall(t, e, node.ID, inst.Secret, ts, nodeproto.SyncRequest{OnlineReported: true, Online: online, Kicks: kicks})
		return resp
	}
	devices := func() (online, kicked []string) {
		var got model.User
		e.do("GET", "/api/users/"+strconv.FormatInt(u.ID, 10), root, nil, &got)
		return got.OnlineIPs, got.KickedIPs
	}

	// two devices on a limit of two: nothing happens
	resp := sync(map[string]map[string]int64{email: {"10.0.0.1": 100, "10.0.0.2": 200}}, "")
	if len(resp.Kicks) != 0 {
		t.Fatalf("kicked within the limit: %v", resp.Kicks)
	}

	// a third device connects: it works, the oldest (10.0.0.1) is cut off,
	// and the unlimited user sharing a carrier IP is not touched
	resp = sync(map[string]map[string]int64{
		email:              {"10.0.0.1": 100, "10.0.0.2": 200, "10.0.0.3": 300},
		core.Email(&other): {"10.0.0.1": 50, "10.0.0.9": 60, "10.0.0.8": 70},
	}, "")
	if !slices.Equal(resp.Kicks[email], []string{"10.0.0.1"}) || len(resp.Kicks) != 1 || resp.KicksVersion == "" {
		t.Fatalf("kicks: %v", resp.Kicks)
	}
	if on, kicked := devices(); !slices.Equal(on, []string{"10.0.0.2", "10.0.0.3"}) || !slices.Equal(kicked, []string{"10.0.0.1"}) {
		t.Fatalf("devices: online %v kicked %v", on, kicked)
	}

	// the cut-off device keeps retrying while blocked: its retries do not
	// push anyone else out
	resp = sync(map[string]map[string]int64{email: {"10.0.0.1": 400, "10.0.0.2": 200, "10.0.0.3": 300}}, resp.KicksVersion)
	if !slices.Equal(resp.Kicks[email], []string{"10.0.0.1"}) {
		t.Fatalf("retries changed the kicks: %v", resp.Kicks)
	}

	// the block ends and the device reconnects (VPN off and on): now it is
	// the newest, so it works and the next oldest (10.0.0.2) goes
	e.api.devices.mu.Lock()
	e.api.devices.kicks[u.ID]["10.0.0.1"] = time.Now().Add(-time.Second)
	e.api.devices.mu.Unlock()
	resp = sync(map[string]map[string]int64{email: {"10.0.0.1": 500, "10.0.0.2": 200, "10.0.0.3": 300}}, resp.KicksVersion)
	if !slices.Equal(resp.Kicks[email], []string{"10.0.0.2"}) {
		t.Fatalf("after reconnect: %v", resp.Kicks)
	}
}
