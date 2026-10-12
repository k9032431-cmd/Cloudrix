package api

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/core"
	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/nodeproto"
)

func TestDeviceLimitOnNodes(t *testing.T) {
	old := deviceGrace
	deviceGrace = 0
	t.Cleanup(func() { deviceGrace = old })

	e := newEnv(t)
	root := e.login("root", "supersecret")
	var node model.Node
	e.do("POST", "/api/nodes", root, map[string]any{"name": "fi", "address": "fi.example.com"}, &node)
	var inst struct {
		Secret string `json:"secret"`
	}
	e.do("GET", "/api/nodes/"+strconv.FormatInt(node.ID, 10)+"/install", root, nil, &inst)
	e.do("POST", "/api/inbounds", root, model.Inbound{Tag: "tr", Protocol: model.ProtoTrojan, Port: 443, NodeID: &node.ID, Enabled: true, Settings: model.InboundSettings{Security: "tls"}}, nil)
	var u model.User
	e.do("POST", "/api/users", root, userInput{Username: "phone", DeviceLimit: 1}, &u)

	ts := time.Now().UnixMilli()
	sync := func(ips ...string) *nodeproto.SyncResponse {
		ts++
		_, resp := nodeCall(t, e, node.ID, inst.Secret, ts, nodeproto.SyncRequest{
			OnlineReported: true, Online: map[string][]string{core.Email(&u): ips},
		})
		return resp
	}
	// one device: fine
	sync("10.0.0.1")
	sync("10.0.0.1")
	var got model.User
	e.do("GET", "/api/users/"+strconv.FormatInt(u.ID, 10), root, nil, &got)
	if got.DeviceBlockedUntil != nil || len(got.OnlineIPs) != 1 {
		t.Fatalf("one device: %+v", got)
	}

	// two devices on a limit of one: first report starts the clock, the next blocks
	sync("10.0.0.1", "10.0.0.2")
	resp := sync("10.0.0.1", "10.0.0.2")
	e.do("GET", "/api/users/"+strconv.FormatInt(u.ID, 10), root, nil, &got)
	if got.DeviceBlockedUntil == nil || time.Until(*got.DeviceBlockedUntil) < 9*time.Minute {
		t.Fatalf("not blocked: %+v", got)
	}
	if resp == nil || !resp.Changed || strings.Contains(string(resp.Xray), u.Credentials.Password) {
		t.Fatalf("blocked user still in the node config")
	}

	// once the block ends the keys come back
	e.api.devices.mu.Lock()
	e.api.devices.blocked[u.ID] = time.Now().Add(-time.Second)
	e.api.devices.mu.Unlock()
	resp = sync("10.0.0.1")
	if resp == nil || !strings.Contains(string(resp.Xray), u.Credentials.Password) {
		t.Fatalf("user not restored after the block")
	}
}
