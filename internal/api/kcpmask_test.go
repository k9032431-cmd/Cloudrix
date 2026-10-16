package api

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/nodeproto"
)

// A node names the mKCP masks the way its own Xray does.
func TestKCPMasksFollowNodeXray(t *testing.T) {
	e := newEnv(t)
	root := e.login("root", "supersecret")
	var node model.Node
	e.do("POST", "/api/nodes", root, map[string]any{"name": "fi", "address": "fi.example.com"}, &node)
	var inst struct {
		Secret string `json:"secret"`
	}
	e.do("GET", "/api/nodes/"+strconv.FormatInt(node.ID, 10)+"/install", root, nil, &inst)
	e.do("POST", "/api/inbounds", root, model.Inbound{Tag: "kcp", Protocol: model.ProtoVLESS, Port: 2083, NodeID: &node.ID, Enabled: true,
		Settings: model.InboundSettings{Network: "kcp", KCPSeed: "pw"}}, nil)

	ts := time.Now().UnixMilli()
	for cores, want := range map[string]string{
		"xray 26.3.27, sing-box 1.14.3":  `"mkcp-aes128gcm"`,
		"xray 26.10.10, sing-box 1.14.3": `"mkcp-legacy"`,
	} {
		ts++
		_, resp := nodeCall(t, e, node.ID, inst.Secret, ts, nodeproto.SyncRequest{Cores: cores})
		if resp == nil || !strings.Contains(string(resp.Xray), want) {
			t.Fatalf("%s: want %s in\n%s", cores, want, resp.Xray)
		}
	}
}
