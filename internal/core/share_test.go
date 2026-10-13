package core

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

func TestSharedPort(t *testing.T) {
	reality := model.InboundSettings{
		Network: "tcp", Security: "reality", Flow: "xtls-rprx-vision",
		RealityDest: "www.google.com:443", RealityPrivateKey: "SMR3hYd5-3QCzkUUkrHqHmBHO6ZWuQuDH7LlBybV8EU",
	}
	lux, nl := reality, reality
	lux.SNI, lux.RealityShortIDs = "www.google.com", []string{"aa"}
	nl.SNI, nl.RealityShortIDs = "www.microsoft.com", []string{"bb"}
	exit := int64(9)
	ws := reality
	ws.ExitNodeID, ws.SNI = &exit, "www.google.com"
	inbounds := []*model.Inbound{
		{ID: 1, Tag: "lux", Protocol: model.ProtoVLESS, Port: 443, Address: "1.1.1.1", Settings: lux},
		{ID: 2, Tag: "nl", Protocol: model.ProtoVLESS, Port: 443, Address: "2.2.2.2", Settings: nl},
		{ID: 3, Tag: "other-exit", Protocol: model.ProtoVLESS, Port: 8443, Settings: ws},
	}
	if PortClash(inbounds[0], inbounds[1]) {
		t.Fatal("hosts of one server side clash")
	}
	if !PortClash(inbounds[0], &model.Inbound{Protocol: model.ProtoVLESS, Port: 443, Settings: ws}) {
		t.Fatal("other exit on the same port does not clash")
	}
	users := []*model.User{
		{ID: 1, Username: "a", Status: model.StatusActive, Inbounds: []string{"lux"}, Credentials: model.Credentials{UUID: "11111111-1111-1111-1111-111111111111"}},
		{ID: 2, Username: "b", Status: model.StatusActive, Inbounds: []string{"nl"}, Credentials: model.Credentials{UUID: "22222222-2222-2222-2222-222222222222"}},
		{ID: 3, Username: "c", Status: model.StatusActive, Credentials: model.Credentials{UUID: "33333333-3333-3333-3333-333333333333"}},
	}
	b, err := Xray(inbounds, users, nil, Cascade{Exits: []Exit{{NodeID: 9, Inbounds: []string{"other-exit"}}}})
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Inbounds []struct {
			Tag      string `json:"tag"`
			Port     int    `json:"port"`
			Settings struct {
				Clients []map[string]any `json:"clients"`
			} `json:"settings"`
			Stream struct {
				Reality struct {
					ServerNames []string `json:"serverNames"`
					ShortIDs    []string `json:"shortIds"`
				} `json:"realitySettings"`
			} `json:"streamSettings"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	// api, the shared 443 listener, 8443
	if len(cfg.Inbounds) != 3 {
		t.Fatalf("inbounds: %d", len(cfg.Inbounds))
	}
	l := cfg.Inbounds[1]
	if l.Tag != "lux" || len(l.Settings.Clients) != 3 || len(l.Stream.Reality.ServerNames) != 2 || len(l.Stream.Reality.ShortIDs) != 2 {
		t.Fatalf("shared listener: %+v", l)
	}

	// the real core accepts it, when one is around
	xray := os.Getenv("XRAY_BIN")
	if xray == "" {
		t.Skip("set XRAY_BIN to check with xray run -test")
	}
	p := filepath.Join(t.TempDir(), "x.json")
	_ = os.WriteFile(p, b, 0o600)
	if out, err := exec.Command(xray, "run", "-test", "-c", p).CombinedOutput(); err != nil {
		t.Fatalf("xray: %v\n%s", err, out)
	}
}
