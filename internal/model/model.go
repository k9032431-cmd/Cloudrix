// Package model holds the domain types shared by the store, API and core layers.
package model

import "time"

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
	Inbounds       []string      `json:"inbounds"`     // inbound tags, empty = all enabled
	Note           string        `json:"note"`
	AdminID        int64         `json:"admin_id"`
	Credentials    Credentials   `json:"credentials"`
	CreatedAt      time.Time     `json:"created_at"`
	OnlineAt       *time.Time    `json:"online_at"`
	SubUpdatedAt   *time.Time    `json:"sub_updated_at"`
	SubUserAgent   string        `json:"sub_user_agent"`
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
	CertFile      string   `json:"cert_file,omitempty"`
	KeyFile       string   `json:"key_file,omitempty"`

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
