package nodeagent

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// probeExits checks that this node can reach the exit nodes of its cascades
// (outbounds tagged exit-N in the Xray config). A cloud firewall in front of
// the exit node is the usual culprit, and it is invisible otherwise: the
// entry node accepts clients but their traffic goes nowhere.
func (a *Agent) probeExits() string {
	raw, err := os.ReadFile(a.path("xray.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		Outbounds []struct {
			Tag      string `json:"tag"`
			Settings struct {
				Vnext []struct {
					Address string `json:"address"`
					Port    int    `json:"port"`
				} `json:"vnext"`
			} `json:"settings"`
		} `json:"outbounds"`
	}
	if json.Unmarshal(raw, &cfg) != nil {
		return ""
	}
	var bad []string
	for _, o := range cfg.Outbounds {
		if !strings.HasPrefix(o.Tag, "exit-") {
			continue
		}
		for _, v := range o.Settings.Vnext {
			addr := net.JoinHostPort(v.Address, strconv.Itoa(v.Port))
			c, err := net.DialTimeout("tcp", addr, 4*time.Second)
			if err != nil {
				bad = append(bad, fmt.Sprintf("cascade: exit node %s is unreachable (open TCP %d on it and in its hosting firewall)", addr, v.Port))
				continue
			}
			c.Close()
		}
	}
	return strings.Join(bad, "; ")
}
