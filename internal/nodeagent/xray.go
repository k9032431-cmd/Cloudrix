package nodeagent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/nodeproto"
)

const xrayAPI = "127.0.0.1:10085" // the api inbound core.Xray adds

func (a *Agent) xrayAPI(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, a.cfg.XrayBin, append([]string{"api", args[0], "--server=" + xrayAPI}, args[1:]...)...)
	cmd.Env = a.xray.env
	return cmd.CombinedOutput()
}

// collectStats reads and resets the per-user counters of Xray and adds them
// to the traffic waiting to be reported (kept until the panel accepts it).
func (a *Agent) collectStats(ctx context.Context) {
	if !a.xray.running() {
		return
	}
	out, err := a.xrayAPI(ctx, "statsquery", "-pattern", "user>>>", "-reset")
	if err != nil {
		a.log.Debug("xray stats", "err", err, "out", string(out))
		return
	}
	var res struct {
		Stat []struct {
			Name  string          `json:"name"`
			Value json.RawMessage `json:"value"`
		} `json:"stat"`
	}
	if json.Unmarshal(out, &res) != nil {
		return
	}
	for _, s := range res.Stat {
		// user>>>{email}>>>traffic>>>uplink|downlink
		parts := strings.Split(s.Name, ">>>")
		if len(parts) != 4 || parts[0] != "user" || parts[2] != "traffic" {
			continue
		}
		v, err := strconv.ParseInt(strings.Trim(string(s.Value), `"`), 10, 64)
		if err != nil || v <= 0 {
			continue
		}
		st := a.pending[parts[1]]
		if st == nil {
			st = &nodeproto.Stat{Email: parts[1]}
			a.pending[parts[1]] = st
		}
		if parts[3] == "uplink" {
			st.Up += v
		} else {
			st.Down += v
		}
	}
}

// hotUpdate applies a config that differs from the running one only in its
// users, through the Xray API. It returns false when a restart is needed.
func (a *Agent) hotUpdate(ctx context.Context, oldCfg, newCfg []byte) bool {
	var o, n map[string]any
	if json.Unmarshal(oldCfg, &o) != nil || json.Unmarshal(newCfg, &n) != nil {
		return false
	}
	oldClients, oldRest := splitClients(o)
	newClients, newRest := splitClients(n)
	if !reflect.DeepEqual(oldRest, newRest) {
		return false
	}
	inbounds := map[string]map[string]any{}
	for _, in := range asList(n["inbounds"]) {
		if m, ok := in.(map[string]any); ok {
			inbounds[str(m["tag"])] = m
		}
	}
	for tag, olds := range oldClients {
		news := newClients[tag]
		var gone []string
		for email, c := range olds {
			if nc, ok := news[email]; !ok || !reflect.DeepEqual(nc, c) {
				gone = append(gone, email)
			}
		}
		if len(gone) > 0 {
			if out, err := a.xrayAPI(ctx, append([]string{"rmu", "-tag=" + tag}, gone...)...); err != nil {
				a.log.Warn("xray rmu failed, restarting instead", "err", err, "out", string(out))
				return false
			}
		}
	}
	for tag, news := range newClients {
		olds := oldClients[tag]
		var add []any
		for email, c := range news {
			if oc, ok := olds[email]; !ok || !reflect.DeepEqual(oc, c) {
				add = append(add, c)
			}
		}
		if len(add) == 0 {
			continue
		}
		in := map[string]any{}
		for k, v := range inbounds[tag] {
			in[k] = v
		}
		settings := map[string]any{}
		if s, ok := in["settings"].(map[string]any); ok {
			for k, v := range s {
				settings[k] = v
			}
		}
		settings["clients"] = add
		in["settings"] = settings
		file := a.path("xray.adu.json")
		b, _ := json.Marshal(map[string]any{"inbounds": []any{in}})
		if err := os.WriteFile(file, b, 0o600); err != nil {
			return false
		}
		out, err := a.xrayAPI(ctx, "adu", file)
		_ = os.Remove(file)
		if err != nil {
			a.log.Warn("xray adu failed, restarting instead", "err", err, "out", string(out))
			return false
		}
	}
	a.log.Info("xray users updated without restart")
	return true
}

// splitClients returns clients by inbound tag and email, plus the config
// with every client list emptied (to compare everything else).
func splitClients(cfg map[string]any) (map[string]map[string]any, map[string]any) {
	clients := map[string]map[string]any{}
	rest := map[string]any{}
	for k, v := range cfg {
		rest[k] = v
	}
	var ins []any
	for _, raw := range asList(cfg["inbounds"]) {
		in, ok := raw.(map[string]any)
		if !ok {
			ins = append(ins, raw)
			continue
		}
		cp := map[string]any{}
		for k, v := range in {
			cp[k] = v
		}
		if s, ok := in["settings"].(map[string]any); ok {
			sc := map[string]any{}
			for k, v := range s {
				sc[k] = v
			}
			if list, ok := s["clients"].([]any); ok {
				byEmail := map[string]any{}
				for _, c := range list {
					if cm, ok := c.(map[string]any); ok {
						byEmail[str(cm["email"])] = c
					}
				}
				clients[str(in["tag"])] = byEmail
				sc["clients"] = nil
			}
			cp["settings"] = sc
		}
		ins = append(ins, cp)
	}
	rest["inbounds"] = ins
	return clients, rest
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
