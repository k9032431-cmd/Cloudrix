package core

import (
	"fmt"
	"slices"
)

// BridgeSNI is the site the node-to-node Reality link imitates.
const BridgeSNI = "www.microsoft.com"

// Bridge is the VLESS + Reality inbound an exit node serves for entry nodes.
type Bridge struct {
	Port                        int
	UUID, PrivateKey, PublicKey string
	ShortID, SNI                string
}

// Exit sends the listed inbounds of an entry node out through another node.
// An empty Address (the exit node is gone or disabled) blocks the traffic.
type Exit struct {
	NodeID   int64
	Address  string
	Bridge   Bridge
	Inbounds []string
}

// Cascade is the node-to-node part of one node's Xray config.
type Cascade struct {
	Bridge *Bridge // set when other nodes exit through this one
	Exits  []Exit  // set when this node's inbounds exit through others
}

const bridgeTag = "cascade-in"

func (b Bridge) inbound() map[string]any {
	return map[string]any{
		"tag": bridgeTag, "listen": "0.0.0.0", "port": b.Port, "protocol": "vless",
		"settings": map[string]any{
			"clients":    []map[string]any{{"id": b.UUID, "flow": "xtls-rprx-vision", "email": "cascade"}},
			"decryption": "none",
		},
		"streamSettings": map[string]any{
			"network": "tcp", "security": "reality",
			"realitySettings": map[string]any{
				"dest": b.SNI + ":443", "serverNames": []string{b.SNI},
				"privateKey": b.PrivateKey, "shortIds": []string{b.ShortID},
			},
		},
		"sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}},
	}
}

func (e Exit) tag() string { return fmt.Sprintf("exit-%d", e.NodeID) }

func (e Exit) outbound() map[string]any {
	return map[string]any{
		"tag": e.tag(), "protocol": "vless",
		"settings": map[string]any{"vnext": []map[string]any{{
			"address": e.Address, "port": e.Bridge.Port,
			"users": []map[string]any{{"id": e.Bridge.UUID, "flow": "xtls-rprx-vision", "encryption": "none"}},
		}}},
		"streamSettings": map[string]any{
			"network": "tcp", "security": "reality",
			"realitySettings": map[string]any{
				"serverName": e.Bridge.SNI, "fingerprint": "chrome",
				"publicKey": e.Bridge.PublicKey, "shortId": e.Bridge.ShortID,
			},
		},
	}
}

// apply adds the bridge inbound, exit outbounds and their routing rules.
func (c Cascade) apply(inbounds *[]map[string]any, outbounds *[]map[string]any, rules *[]map[string]any) {
	if c.Bridge != nil && c.Bridge.Port > 0 {
		*inbounds = append(*inbounds, c.Bridge.inbound())
	}
	for _, e := range c.Exits {
		if len(e.Inbounds) == 0 {
			continue
		}
		to := "block"
		if e.Address != "" && e.Bridge.Port > 0 {
			*outbounds = append(*outbounds, e.outbound())
			to = e.tag()
		}
		*rules = append(*rules, map[string]any{"inboundTag": e.Inbounds, "outboundTag": to})
	}
}

// renamed points the exits at the listeners that serve folded inbounds.
func (c Cascade) renamed(alias map[string]string) Cascade {
	if len(alias) == 0 {
		return c
	}
	exits := make([]Exit, len(c.Exits))
	for i, e := range c.Exits {
		var tags []string
		for _, t := range e.Inbounds {
			if a, ok := alias[t]; ok {
				t = a
			}
			if !slices.Contains(tags, t) {
				tags = append(tags, t)
			}
		}
		e.Inbounds = tags
		exits[i] = e
	}
	c.Exits = exits
	return c
}
