// Package model holds the domain types shared by the store, API and core layers.
package model

import (
	"slices"
	"time"
)

type Role string

const (
	RoleSudo     Role = "sudo"     // full access, manages admins, inbounds and nodes
	RoleAdmin    Role = "admin"    // manages own users
	RoleReseller Role = "reseller" // manages own users within quota
)

func (r Role) Valid() bool {
	return r == RoleSudo || r == RoleAdmin || r == RoleReseller
}

type Admin struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	Disabled     bool      `json:"disabled"`
	UserLimit    int       `json:"user_limit"` // 0 = unlimited
	CreatedAt    time.Time `json:"created_at"`

	// Limits for admins and resellers (sudo admins ignore them). Past
	// ExpireAt or over DataLimit the admin cannot sign in and the admin's
	// users get no keys until the limit is raised.
	ExpireAt       *time.Time `json:"expire_at"`
	DataLimit      int64      `json:"data_limit"`       // bytes all its users may use, 0 = unlimited
	UsedTraffic    int64      `json:"used_traffic"`     // bytes its users used (kept when users are deleted)
	MaxUserData    int64      `json:"max_user_data"`    // cap on each user's data limit, 0 = none
	MaxUserDevices int        `json:"max_user_devices"` // cap on each user's device limit, 0 = none
}

// Suspended reports why a non-sudo admin's access and users are on hold.
func (a *Admin) Suspended(now time.Time) string {
	if a.Role == RoleSudo {
		return ""
	}
	if a.ExpireAt != nil && !now.Before(*a.ExpireAt) {
		return "expired"
	}
	if a.DataLimit > 0 && a.UsedTraffic >= a.DataLimit {
		return "data limit reached"
	}
	return ""
}

type UserStatus string

const (
	StatusActive   UserStatus = "active"
	StatusDisabled UserStatus = "disabled"
	StatusLimited  UserStatus = "limited"
	StatusExpired  UserStatus = "expired"
	StatusOnHold   UserStatus = "on_hold" // timer starts on first connection
)

type ResetStrategy string

const (
	ResetNone  ResetStrategy = "no_reset"
	ResetDay   ResetStrategy = "day"
	ResetWeek  ResetStrategy = "week"
	ResetMonth ResetStrategy = "month"
)

func (r ResetStrategy) Valid() bool {
	switch r {
	case ResetNone, ResetDay, ResetWeek, ResetMonth:
		return true
	}
	return false
}

// Credentials are generated once per user and reused by every protocol.
type Credentials struct {
	UUID         string `json:"uuid"`           // vless / vmess / tuic
	Password     string `json:"password"`       // trojan / shadowsocks / hysteria2 / tuic
	WGPrivateKey string `json:"wg_private_key"` // wireguard
	WGPublicKey  string `json:"wg_public_key"`
}

type User struct {
	ID             int64         `json:"id"`
	Username       string        `json:"username"`
	SubToken       string        `json:"sub_token"`
	Status         UserStatus    `json:"status"`
	DataLimit      int64         `json:"data_limit"` // bytes, 0 = unlimited
	UsedTraffic    int64         `json:"used_traffic"`
	LifetimeUsed   int64         `json:"lifetime_used"`
	ExpireAt       *time.Time    `json:"expire_at"`
	OnHoldDuration int64         `json:"on_hold_duration"` // seconds, used while status = on_hold
	ResetStrategy  ResetStrategy `json:"reset_strategy"`
	LastResetAt    time.Time     `json:"last_reset_at"`
	DeviceLimit    int           `json:"device_limit"` // 0 = unlimited
	GroupIDs       []int64       `json:"group_ids"`    // groups whose inbounds the user gets
	Inbounds       []string      `json:"inbounds"`     // legacy, without groups: inbound tags, empty = all enabled
	Note           string        `json:"note"`
	AdminID        int64         `json:"admin_id"`
	Credentials    Credentials   `json:"credentials"`
	CreatedAt      time.Time     `json:"created_at"`
	OnlineAt       *time.Time    `json:"online_at"`
	SubUpdatedAt   *time.Time    `json:"sub_updated_at"`
	SubUserAgent   string        `json:"sub_user_agent"`

	// Live data from the nodes, not stored: the IPs the user is connected
	// from, and the oldest devices cut off for going over the device limit.
	OnlineIPs []string `json:"online_ips,omitempty"`
	KickedIPs []string `json:"kicked_ips,omitempty"`
}

// Group bundles inbounds; users get every inbound of their groups, so an
// inbound added to a group reaches all its users on the next refresh.
type Group struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	InboundIDs []int64   `json:"inbound_ids"`
	Disabled   bool      `json:"disabled"`
	Users      int       `json:"users"`
	CreatedAt  time.Time `json:"created_at"`
}

type Groups map[int64]*Group

// Allows reports whether the user may use the inbound: through an enabled
// group, or for users without groups through the legacy tag list.
func (u *User) Allows(in *Inbound, groups Groups) bool {
	if len(u.GroupIDs) > 0 {
		for _, id := range u.GroupIDs {
			if g := groups[id]; g != nil && !g.Disabled && slices.Contains(g.InboundIDs, in.ID) {
				return true
			}
		}
		return false
	}
	return len(u.Inbounds) == 0 || slices.Contains(u.Inbounds, in.Tag)
}

// NodeBridge holds the VLESS + Reality inbound an exit node serves for
// cascades. Keys are made on first use; Port 0 picks a free one.
type NodeBridge struct {
	Port       int    `json:"port"`
	UUID       string `json:"uuid"`
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
	ShortID    string `json:"short_id"`
}

// SelfSigned reports whether the inbound serves TLS with the node's
// self-signed certificate (no certificate of its own).
func (in *Inbound) SelfSigned() bool {
	s := in.Settings
	if s.CertFile != "" && s.KeyFile != "" {
		return false
	}
	switch in.Protocol {
	case ProtoHysteria2, ProtoTUIC:
		return true
	case ProtoVLESS, ProtoVMess, ProtoTrojan:
		return s.Security == "tls"
	}
	return false
}

// Active reports whether the user should currently be allowed to connect.
func (u *User) Active() bool {
	return u.Status == StatusActive || u.Status == StatusOnHold
}

type Protocol string

const (
	ProtoVLESS       Protocol = "vless"
	ProtoVMess       Protocol = "vmess"
	ProtoTrojan      Protocol = "trojan"
	ProtoShadowsocks Protocol = "shadowsocks"
	ProtoHysteria2   Protocol = "hysteria2"
	ProtoTUIC        Protocol = "tuic"
	ProtoWireGuard   Protocol = "wireguard"
)

var AllProtocols = []Protocol{ProtoVLESS, ProtoVMess, ProtoTrojan, ProtoShadowsocks, ProtoHysteria2, ProtoTUIC, ProtoWireGuard}

func (p Protocol) Valid() bool {
	for _, x := range AllProtocols {
		if x == p {
			return true
		}
	}
	return false
}

// Core returns which proxy core serves the protocol.
func (p Protocol) Core() string {
	switch p {
	case ProtoHysteria2, ProtoTUIC, ProtoWireGuard:
		return "sing-box"
	default:
		return "xray"
	}
}

// InboundSettings is a flat bag of transport/security options. Fields that do
// not apply to a protocol are ignored.
type InboundSettings struct {
	// transport: tcp | ws | grpc | xhttp | httpupgrade
	Network     string `json:"network,omitempty"`
	Path        string `json:"path,omitempty"`
	Host        string `json:"host,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
	HeaderType  string `json:"header_type,omitempty"`
	XHTTPMode   string `json:"xhttp_mode,omitempty"`

	// security: none | tls | reality
	Security      string   `json:"security,omitempty"`
	SNI           string   `json:"sni,omitempty"`
	ALPN          []string `json:"alpn,omitempty"`
	Fingerprint   string   `json:"fingerprint,omitempty"`
	AllowInsecure bool     `json:"allow_insecure,omitempty"`

	// cascade: send this inbound's traffic out through another node
	ExitNodeID *int64 `json:"exit_node_id,omitempty"`
	CertFile   string `json:"cert_file,omitempty"`
	KeyFile    string `json:"key_file,omitempty"`

	// reality
	RealityDest        string   `json:"reality_dest,omitempty"`
	RealityServerNames []string `json:"reality_server_names,omitempty"`
	RealityPrivateKey  string   `json:"reality_private_key,omitempty"`
	RealityPublicKey   string   `json:"reality_public_key,omitempty"`
	RealityShortIDs    []string `json:"reality_short_ids,omitempty"`
	RealitySpiderX     string   `json:"reality_spider_x,omitempty"`

	// protocol specific
	Flow           string `json:"flow,omitempty"`               // vless: xtls-rprx-vision
	SSMethod       string `json:"ss_method,omitempty"`          // shadowsocks
	SSServerKey    string `json:"ss_server_key,omitempty"`      // shadowsocks 2022 server PSK (base64)
	Obfs           string `json:"obfs,omitempty"`               // hysteria2: salamander
	ObfsPassword   string `json:"obfs_password,omitempty"`      // hysteria2
	UpMbps         int    `json:"up_mbps,omitempty"`            // hysteria2
	DownMbps       int    `json:"down_mbps,omitempty"`          // hysteria2
	CongestionCtrl string `json:"congestion_control,omitempty"` // tuic: bbr | cubic | new_reno
	WGPrivateKey   string `json:"wg_private_key,omitempty"`     // wireguard server key
	WGPublicKey    string `json:"wg_public_key,omitempty"`
	WGNetwork      string `json:"wg_network,omitempty"` // e.g. 10.8.0.0/24
	WGDNS          string `json:"wg_dns,omitempty"`
	MTU            int    `json:"mtu,omitempty"`
}

type Inbound struct {
	ID       int64           `json:"id"`
	Tag      string          `json:"tag"`
	Protocol Protocol        `json:"protocol"`
	Listen   string          `json:"listen"`
	Port     int             `json:"port"`
	Address  string          `json:"address"` // public address put into client links; empty = node/panel host
	Remark   string          `json:"remark"`
	NodeID   *int64          `json:"node_id"` // nil = local core
	Enabled  bool            `json:"enabled"`
	Settings InboundSettings `json:"settings"`
}

type NodeStatus string

const (
	NodeConnected  NodeStatus = "connected"
	NodeConnecting NodeStatus = "connecting"
	NodeError      NodeStatus = "error"
	NodeDisabled   NodeStatus = "disabled"
)

type Node struct {
	ID               int64      `json:"id"`
	Name             string     `json:"name"`
	Address          string     `json:"address"`
	APIPort          int        `json:"api_port"`
	UsageCoefficient float64    `json:"usage_coefficient"`
	Status           NodeStatus `json:"status"`
	Message          string     `json:"message"`
	CoreVersion      string     `json:"core_version"`
	LastSeen         *time.Time `json:"last_seen"`
	UploadBytes      int64      `json:"upload_bytes"`
	DownloadBytes    int64      `json:"download_bytes"`
	CreatedAt        time.Time  `json:"created_at"`
	// Bridge is the node-to-node link other nodes use to exit through this
	// one (cascade); BridgePort mirrors its port for the admin UI.
	Bridge     NodeBridge `json:"-"`
	BridgePort int        `json:"bridge_port"`
	// CertSHA256 is the hex SHA-256 of the node's self-signed certificate,
	// pinned by clients of TLS inbounds that have no certificate of their own.
	CertSHA256 string `json:"cert_sha256"`
	// Secret signs and encrypts agent traffic; never sent to the browser.
	Secret string `json:"-"`
}

type AuditEntry struct {
	ID        int64     `json:"id"`
	AdminID   int64     `json:"admin_id"`
	Admin     string    `json:"admin"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	Details   string    `json:"details"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"created_at"`
}
