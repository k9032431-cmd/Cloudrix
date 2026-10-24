package core

import (
	"testing"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

// Inbounds behind a CDN take the client's IP from the CDN's headers, so the
// device limit sees users, not CDN servers.
func TestBehindCDNTrustsForwardedIP(t *testing.T) {
	for _, c := range []struct {
		s    model.InboundSettings
		want bool
	}{
		{model.InboundSettings{Network: "xhttp"}, true},
		{model.InboundSettings{Network: "ws", Security: "none"}, true},
		{model.InboundSettings{Network: "httpupgrade"}, true},
		{model.InboundSettings{Network: "xhttp", Security: "tls"}, false},
		{model.InboundSettings{Network: "xhttp", Security: "reality"}, false},
		{model.InboundSettings{Network: "tcp"}, false},
	} {
		so, _ := xrayStream(c.s)["sockopt"].(map[string]any)
		_, got := so["trustedXForwardedFor"]
		if got != c.want {
			t.Errorf("%+v: trustedXForwardedFor %v, want %v", c.s, got, c.want)
		}
	}
}
