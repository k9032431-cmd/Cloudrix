// Package sub renders user subscriptions: share links, Clash/mihomo and sing-box profiles.
package sub

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math"
	"net/netip"
	"strings"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

// Endpoint is an inbound resolved to the address clients should dial.
type Endpoint struct {
	Inbound *model.Inbound
	Host    string
	Remark  string
}

// Resolve picks the inbounds the user can use and fills in host and remark.
// nodeHosts maps node id to its public address.
func Resolve(u *model.User, inbounds []*model.Inbound, nodeHosts map[int64]string, defaultHost string) []Endpoint {
	allowed := map[string]bool{}
	for _, t := range u.Inbounds {
		allowed[t] = true
	}
	var out []Endpoint
	for _, in := range inbounds {
		if !in.Enabled || (len(allowed) > 0 && !allowed[in.Tag]) {
			continue
		}
		host := in.Address
		if host == "" && in.NodeID != nil {
			host = nodeHosts[*in.NodeID]
		}
		if host == "" {
			host = defaultHost
		}
		out = append(out, Endpoint{Inbound: in, Host: host, Remark: RenderRemark(in, u)})
	}
	return out
}

// RenderRemark expands template variables in an inbound remark (see RenderTemplate).
func RenderRemark(in *model.Inbound, u *model.User) string {
	remark := in.Remark
	if remark == "" {
		remark = in.Tag
	}
	return strings.ReplaceAll(RenderTemplate(remark, u), "{PROTOCOL}", string(in.Protocol))
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
