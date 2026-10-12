package api

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/core"
	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/nodeproto"
)

func TestSharedCertificatesReachNodes(t *testing.T) {
	dir := t.TempDir() + "/"
	old := core.SharedTLSDir
	core.SharedTLSDir = dir
	t.Cleanup(func() { core.SharedTLSDir = old })
	cert, key := filepath.Join(dir, "hy.example.com", "fullchain.pem"), filepath.Join(dir, "hy.example.com", "privkey.pem")

	e := newEnv(t)
	root := e.login("root", "supersecret")
	var node model.Node
	e.do("POST", "/api/nodes", root, map[string]any{"name": "fi", "address": "77.42.11.42"}, &node)
	var inst struct {
		Secret string `json:"secret"`
	}
	e.do("GET", "/api/nodes/"+strconv.FormatInt(node.ID, 10)+"/install", root, nil, &inst)
	in := model.Inbound{Tag: "hy", Protocol: model.ProtoHysteria2, Port: 443, NodeID: &node.ID, Enabled: true, Address: "hy.example.com",
		Settings: model.InboundSettings{SNI: "hy.example.com", CertFile: cert, KeyFile: key}}

	// paths from the main-server menu must exist there
	if r := e.do("POST", "/api/inbounds", root, in, nil); r.StatusCode != 400 {
		t.Fatalf("missing certificate accepted: %d", r.StatusCode)
	}
	os.MkdirAll(filepath.Dir(cert), 0o700)
	os.WriteFile(cert, []byte("CERT-1"), 0o600)
	os.WriteFile(key, []byte("KEY-1"), 0o600)
	if r := e.do("POST", "/api/inbounds", root, in, nil); r.StatusCode != 201 {
		t.Fatalf("create: %d", r.StatusCode)
	}

	ts := time.Now().UnixMilli()
	sync := func(have string) *nodeproto.SyncResponse {
		ts++
		_, resp := nodeCall(t, e, node.ID, inst.Secret, ts, nodeproto.SyncRequest{Version: have})
		return resp
	}
	resp := sync("")
	if resp.Files[cert] != "CERT-1" || resp.Files[key] != "KEY-1" {
		t.Fatalf("files not sent: %v", resp.Files)
	}
	// a renewed certificate is a new version for the node
	os.WriteFile(cert, []byte("CERT-2"), 0o600)
	resp2 := sync(resp.Version)
	if !resp2.Changed || resp2.Files[cert] != "CERT-2" {
		t.Fatalf("renewal not delivered: changed=%v files=%v", resp2.Changed, resp2.Files)
	}
}
