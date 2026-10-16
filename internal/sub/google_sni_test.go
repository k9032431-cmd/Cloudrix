package sub

import (
	"strings"
	"testing"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

// Hysteria2 without a domain: the node's self-signed certificate, a host
// that sets SNI www.google.com; the link pins the certificate.
func TestHysteriaWithGoogleSNI(t *testing.T) {
	pin := strings.Repeat("cd", 32)
	in := &model.Inbound{ID: 1, Tag: "hy2", Protocol: model.ProtoHysteria2, Port: 855, CoreID: 1, Enabled: true,
		Settings: model.InboundSettings{Masquerade: "https://www.google.com"},
		Hosts: []model.Host{{ID: 1, Enabled: true, HostSettings: model.HostSettings{
			Remark: "DE", Address: []string{"5.75.180.135"}, SNI: []string{"www.google.com"}}}}}
	nodes := map[int64]*model.Node{1: {ID: 1, CoreID: 1, Address: "5.75.180.135", CertSHA256: pin}}
	eps := Resolve(testUser(), []*model.Inbound{in}, nil, nodes, "h")
	if len(eps) != 1 {
		t.Fatalf("endpoints: %d", len(eps))
	}
	l := Link(testUser(), eps[0])
	for _, want := range []string{"@5.75.180.135:855", "sni=www.google.com", "insecure=1", "pinSHA256=" + pin} {
		if !strings.Contains(l, want) {
			t.Errorf("link lacks %s: %s", want, l)
		}
	}
}
