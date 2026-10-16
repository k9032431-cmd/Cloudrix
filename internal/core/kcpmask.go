package core

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
)

// XrayBefore reports whether an Xray version ("26.3.27", or the agent's
// "xray 26.3.27, sing-box 1.14.3") is older than year.month. An unknown
// version counts as new.
func XrayBefore(version string, year, month int) bool {
	v := version
	if i := strings.Index(v, "xray "); i >= 0 {
		v = v[i+len("xray "):]
	}
	v, _, _ = strings.Cut(v, ",")
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".")
	if len(parts) < 2 {
		return false
	}
	y, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return y < year || (y == year && m < month)
}

// LegacyKCPMasks rewrites the mKCP masks of an Xray config for cores older
// than 26.6, which name them mkcp-original, mkcp-aes128gcm and header-*
// instead of mkcp-legacy, and stack them in the opposite order: a header and
// a password listed as [header, password] for a new core must be
// [password, header] for an old one to talk to it.
func LegacyKCPMasks(cfg []byte) []byte {
	var c map[string]any
	if json.Unmarshal(cfg, &c) != nil {
		return cfg
	}
	changed := false
	ins, _ := c["inbounds"].([]any)
	for _, raw := range ins {
		in, _ := raw.(map[string]any)
		st, _ := in["streamSettings"].(map[string]any)
		fm, _ := st["finalmask"].(map[string]any)
		udp, _ := fm["udp"].([]any)
		for i, m := range udp {
			mask, _ := m.(map[string]any)
			if mask["type"] != "mkcp-legacy" {
				continue
			}
			set, _ := mask["settings"].(map[string]any)
			header, _ := set["header"].(string)
			value, _ := set["value"].(string)
			switch {
			case header == "dns":
				udp[i] = map[string]any{"type": "header-dns", "settings": map[string]any{"domain": value}}
			case header != "":
				udp[i] = map[string]any{"type": "header-" + header}
			case value != "":
				udp[i] = map[string]any{"type": "mkcp-aes128gcm", "settings": map[string]any{"password": value}}
			default:
				udp[i] = map[string]any{"type": "mkcp-original"}
			}
			changed = true
		}
		slices.Reverse(udp)
	}
	if !changed {
		return cfg
	}
	out, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return cfg
	}
	return out
}
