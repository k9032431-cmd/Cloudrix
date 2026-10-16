// Package sub renders user subscriptions: share links, Clash/mihomo and sing-box profiles.
package sub

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math"
	"math/rand/v2"
	"net/netip"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

// Endpoint is an inbound resolved to the address clients should dial.
type Endpoint struct {
	Inbound *model.Inbound
	Host    string
	Remark  string
	// CertPin is the hex SHA-256 of the node's self-signed certificate when
	// the inbound uses it; clients pin it instead of verifying a CA chain.
	CertPin string
}

// Resolve picks the inbounds the user can use and gives one endpoint per
// enabled host of each, in host order. An inbound is served by the nodes
// running its core; nodes maps node id to the node (its address, core and
// certificate).
func Resolve(u *model.User, inbounds []*model.Inbound, groups model.Groups, nodes map[int64]*model.Node, defaultHost string) []Endpoint {
	type ranked struct {
		ep       Endpoint
		priority int
		id       int64
	}
	var runners []*model.Node
	for _, n := range nodes {
		runners = append(runners, n)
	}
	sort.Slice(runners, func(i, j int) bool { return runners[i].ID < runners[j].ID })
	var all []ranked
	for _, in := range inbounds {
		if !in.Enabled || !u.Allows(in, groups) {
			continue
		}
		// only inbounds some node serves: no dead links
		var serving []*model.Node
		for _, n := range runners {
			if in.Runs(n) {
				serving = append(serving, n)
			}
		}
		if len(serving) == 0 {
			continue
		}
		hosts := in.Hosts
		if hosts == nil {
			// not loaded from the store: the inbound's own fields are its host
			h := model.Host{Enabled: true, HostSettings: model.HostSettings{Remark: in.Remark, Fingerprint: in.Settings.Fingerprint}}
			if in.Address != "" {
				h.Address = []string{in.Address}
			}
			if in.Settings.SNI != "" {
				h.SNI = []string{in.Settings.SNI}
			}
			hosts = []model.Host{h}
		}
		for _, h := range hosts {
			if !h.Enabled || (len(h.Status) > 0 && !slices.Contains(h.Status, u.Status)) {
				continue
			}
			// several addresses: one picked per subscription; none: every
			// node of the core
			var addrs []string
			if len(h.Address) > 0 {
				addrs = []string{pick(h.Address)}
			} else {
				for _, n := range serving {
					addrs = append(addrs, n.Address)
				}
			}
			for _, addr := range addrs {
				if addr == "" {
					addr = defaultHost
				}
				c := withHost(in, h.HostSettings)
				ep := Endpoint{Inbound: c, Host: addr, Remark: RenderRemark(c, u)}
				if in.SelfSigned() && h.Security != "none" {
					// self-signed: clients cannot verify the chain, so they skip
					// it (older apps) or pin the certificate hash (Xray 26+)
					c.Settings.AllowInsecure = true
					if n := nodeAt(serving, addr); n != nil {
						ep.CertPin = n.CertSHA256
					}
				}
				all = append(all, ranked{ep, h.Priority, h.ID})
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].priority != all[j].priority {
			return all[i].priority < all[j].priority
		}
		return all[i].id < all[j].id
	})
	out := make([]Endpoint, 0, len(all))
	for _, r := range all {
		out = append(out, r.ep)
	}
	return out
}

// nodeAt finds the serving node a link points at: by address, or the only one.
func nodeAt(serving []*model.Node, addr string) *model.Node {
	for _, n := range serving {
		if strings.EqualFold(n.Address, addr) {
			return n
		}
	}
	if len(serving) == 1 {
		return serving[0]
	}
	return nil
}

func pick(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[rand.IntN(len(v))]
}

// withHost returns a copy of the inbound as one host's link sees it: the
// host's remark and client fields; the inbound's server side.
func withHost(in *model.Inbound, h model.HostSettings) *model.Inbound {
	c := *in
	c.Hosts = nil
	c.Settings.ALPN = slices.Clone(in.Settings.ALPN)
	c.Remark = h.Remark
	c.Settings.SNI, c.Settings.Fingerprint = pick(h.SNI), h.Fingerprint
	c.Settings.AllowInsecure = in.Settings.AllowInsecure || h.AllowInsecure
	switch h.Security {
	case "tls", "none":
		// a middle server (CDN, relay) talks to clients differently
		c.Settings.Security = h.Security
	}
	if h.Port > 0 {
		c.Port = h.Port
	}
	if host := pick(h.Host); host != "" {
		c.Settings.Host = host
	}
	if h.UseSNIAsHost && c.Settings.SNI != "" {
		c.Settings.Host = c.Settings.SNI
	}
	if h.Path != "" {
		c.Settings.Path = h.Path
	}
	if len(h.ALPN) > 0 {
		c.Settings.ALPN = slices.Clone(h.ALPN)
	}
	return &c
}

// RenderRemark expands template variables in an inbound remark (see RenderTemplate).
func RenderRemark(in *model.Inbound, u *model.User) string {
	remark := CleanName(in.Remark)
	if remark == "" {
		remark = in.Tag
	}
	return strings.ReplaceAll(RenderTemplate(remark, u), "{PROTOCOL}", string(in.Protocol))
}

// invisible marks that sneak in with copied text or phone keyboards; before
// a flag they stop apps (Happ, v2rayNG…) from showing it as the server icon
var invisible = strings.NewReplacer(
	"\u200b", "", "\u200c", "", "\u200e", "", "\u200f", "", "\u202a", "", "\u202b", "", "\u202c", "",
	"\u202d", "", "\u202e", "", "\u2060", "", "\u2066", "", "\u2067", "", "\u2068", "", "\u2069", "",
	"\ufeff", "", "\u00ad", "",
)

// flagTwins maps territory flags that look like a country's flag to that
// country's: phone keyboards often offer 🇨🇵 (Clipperton) for France, and
// apps do not know those codes, so they show no flag icon.
var flagTwins = func() *strings.Replacer {
	flag := func(code string) string {
		return string([]rune{0x1F1E6 + rune(code[0]-'A'), 0x1F1E6 + rune(code[1]-'A')})
	}
	var pairs []string
	for twin, real := range map[string]string{"CP": "FR", "MF": "FR", "BV": "NO", "SJ": "NO", "UM": "US", "EA": "ES", "HM": "AU"} {
		pairs = append(pairs, flag(twin), flag(real))
	}
	return strings.NewReplacer(pairs...)
}()

// CleanName drops invisible characters and outer spaces from a link name,
// so a leading flag emoji is the very first character, and turns look-alike
// territory flags into the country's flag.
func CleanName(s string) string {
	s = flagTwins.Replace(invisible.Replace(s))
	// a stray variation selector or joiner before the first visible character
	s = strings.TrimLeftFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == '\ufe0f' || r == '\ufe0e' || r == '\u200d' })
	return strings.TrimRightFunc(s, unicode.IsSpace)
}

// RenderTemplate fills user variables into admin-written text:
// {USERNAME} {NOTE} {STATUS} {DATA_USED} {DATA_LIMIT} {DATA_LEFT} {DAYS_LEFT} {EXPIRE_DATE}.
func RenderTemplate(text string, u *model.User) string {
	if !strings.Contains(text, "{") {
		return text
	}
	dataLeft, dataLimit, daysLeft, expire := "∞", "∞", "∞", "∞"
	if u.DataLimit > 0 {
		dataLeft = FormatBytes(max(u.DataLimit-u.UsedTraffic, 0))
		dataLimit = FormatBytes(u.DataLimit)
	}
	if u.ExpireAt != nil {
		days := math.Ceil(time.Until(*u.ExpireAt).Hours() / 24)
		daysLeft = fmt.Sprint(max(int(days), 0))
		expire = u.ExpireAt.Format("02.01.2006")
	}
	return strings.NewReplacer(
		"{USERNAME}", u.Username,
		"{NOTE}", u.Note,
		"{STATUS}", string(u.Status),
		"{DATA_USED}", FormatBytes(u.UsedTraffic),
		"{DATA_LIMIT}", dataLimit,
		"{DATA_LEFT}", dataLeft,
		"{DAYS_LEFT}", daysLeft,
		"{EXPIRE_DATE}", expire,
	).Replace(text)
}

func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// SSUserKey derives the per-user key for a Shadowsocks cipher. Shadowsocks 2022
// ciphers need a base64 key of exact length, classic AEAD ciphers take any string.
func SSUserKey(method, password string) string {
	n := ss2022KeyLen(method)
	if n == 0 {
		return password
	}
	sum := sha256.Sum256([]byte(password))
	return base64.StdEncoding.EncodeToString(sum[:n])
}

// SSClientPassword is the password a client must use for the given inbound.
func SSClientPassword(s model.InboundSettings, password string) string {
	key := SSUserKey(s.SSMethod, password)
	if ss2022KeyLen(s.SSMethod) > 0 && s.SSServerKey != "" {
		return s.SSServerKey + ":" + key
	}
	return key
}

func ss2022KeyLen(method string) int {
	switch method {
	case "2022-blake3-aes-128-gcm":
		return 16
	case "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
		return 32
	}
	return 0
}

func SSMethod(s model.InboundSettings) string {
	if s.SSMethod == "" {
		return "chacha20-ietf-poly1305"
	}
	return s.SSMethod
}

// WGAddress assigns the user a stable address inside the inbound network,
// skipping the network address and the server's .1.
func WGAddress(network string, userID int64) (string, error) {
	if network == "" {
		network = "10.8.0.0/16"
	}
	prefix, err := netip.ParsePrefix(network)
	if err != nil {
		return "", err
	}
	prefix = prefix.Masked()
	if !prefix.Addr().Is4() {
		return "", fmt.Errorf("only IPv4 wireguard networks are supported")
	}
	hostBits := 32 - prefix.Bits()
	capacity := int64(1)<<hostBits - 3 // network, server, broadcast
	if hostBits >= 62 || userID > capacity {
		return "", fmt.Errorf("wireguard network %s is full", network)
	}
	base := prefix.Addr().As4()
	v := uint32(base[0])<<24 | uint32(base[1])<<16 | uint32(base[2])<<8 | uint32(base[3])
	v += uint32(userID) + 1
	addr := netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
	return addr.String() + "/32", nil
}

// WGServerAddress is the server-side interface address (.1) of the network.
func WGServerAddress(network string) (string, error) {
	if network == "" {
		network = "10.8.0.0/16"
	}
	prefix, err := netip.ParsePrefix(network)
	if err != nil {
		return "", err
	}
	prefix = prefix.Masked()
	return netip.PrefixFrom(prefix.Addr().Next(), prefix.Bits()).String(), nil
}
