package api

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

func TestCascade(t *testing.T) {
	e := newEnv(t)
	root := e.login("root", "supersecret")
	var exit model.Node
	e.do("POST", "/api/nodes", root, nodeInput{Name: "fi", Address: "fi.example.com"}, &exit)
	entry := *e.node

	in := model.Inbound{Tag: "ru-in", Protocol: model.ProtoVLESS, Port: 443, NodeID: &entry, Enabled: true,
		Settings: model.InboundSettings{Security: "none", ExitNodeID: &exit.ID}}
	if r := e.do("POST", "/api/inbounds", root, in, nil); r.StatusCode != 201 {
		t.Fatalf("create cascade inbound: %d", r.StatusCode)
	}
	self := in
	self.Tag, self.Port, self.Settings.ExitNodeID = "loop", 8443, &entry
	if r := e.do("POST", "/api/inbounds", root, self, nil); r.StatusCode != 400 {
		t.Fatalf("exit through own node: %d", r.StatusCode)
	}
	e.do("POST", "/api/users", root, userInput{Username: "u1"}, nil)

	type xcfg struct {
		Inbounds []struct {
			Tag  string `json:"tag"`
			Port int    `json:"port"`
		} `json:"inbounds"`
		Outbounds []struct {
			Tag      string         `json:"tag"`
			Settings map[string]any `json:"settings"`
		} `json:"outbounds"`
		Routing struct {
			Rules []struct {
				InboundTag  []string `json:"inboundTag"`
				OutboundTag string   `json:"outboundTag"`
			} `json:"rules"`
		} `json:"routing"`
	}
	load := func(id int64) xcfg {
		x, _, _, _, err := e.api.nodeConfigs(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		var c xcfg
		if err := json.Unmarshal(x, &c); err != nil {
			t.Fatalf("node %d config: %v", id, err)
		}
		return c
	}

	// the exit node serves the bridge although it has no inbounds of its own
	ex := load(exit.ID)
	port := 0
	for _, in := range ex.Inbounds {
		if in.Tag == "cascade-in" {
			port = in.Port
		}
	}
	if port == 0 {
		t.Fatalf("exit node has no bridge inbound: %+v", ex.Inbounds)
	}

	en := load(entry)
	tag := fmt.Sprintf("exit-%d", exit.ID)
	found := false
	for _, o := range en.Outbounds {
		found = found || o.Tag == tag
	}
	routed := false
	for _, r := range en.Routing.Rules {
		routed = routed || (r.OutboundTag == tag && len(r.InboundTag) == 1 && r.InboundTag[0] == "ru-in")
	}
	if !found || !routed {
		t.Fatalf("entry node does not route through the exit: %+v", en)
	}

	// an inbound on the exit node cannot take the bridge port; the exit node
	// cannot be deleted while used
	clash := model.Inbound{Tag: "clash", Protocol: model.ProtoTrojan, Port: port, NodeID: &exit.ID, Enabled: true, Settings: model.InboundSettings{Security: "tls"}}
	if r := e.do("POST", "/api/inbounds", root, clash, nil); r.StatusCode != 400 {
		t.Fatalf("bridge port clash: %d", r.StatusCode)
	}
	if r := e.do("DELETE", fmt.Sprintf("/api/nodes/%d", exit.ID), root, nil, nil); r.StatusCode != 409 {
		t.Fatalf("delete exit node in use: %d", r.StatusCode)
	}

	// a disabled exit node blocks the traffic instead of leaking it
	e.do("PUT", fmt.Sprintf("/api/nodes/%d", exit.ID), root, nodeInput{Name: "fi", Address: "fi.example.com", Disabled: true, BridgePort: port}, nil)
	for _, r := range load(entry).Routing.Rules {
		if len(r.InboundTag) == 1 && r.InboundTag[0] == "ru-in" && r.OutboundTag != "block" {
			t.Fatalf("disabled exit: routed to %s", r.OutboundTag)
		}
	}
}
