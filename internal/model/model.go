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

	// The rest follows the PasarGuard inbound editor (Xray / sing-box
	// fields); empty values keep the core defaults.

	// sniffing (Xray); nil = on for http, tls, quic
	Sniffing *Sniffing `json:"sniffing,omitempty"`
	// socket options (Xray sockopt / sing-box listen fields)
	AcceptProxyProtocol  bool   `json:"accept_proxy_protocol,omitempty"`  // behind HAProxy / nginx with PROXY protocol
	TCPFastOpen          bool   `json:"tcp_fast_open,omitempty"`          //
	TCPCongestion        string `json:"tcp_congestion,omitempty"`         // bbr, cubic, reno
	TCPKeepAliveInterval int    `json:"tcp_keepalive_interval,omitempty"` // seconds
	UDPTimeout           string `json:"udp_timeout,omitempty"`            // sing-box, e.g. 5m

	// vless / trojan with TLS on tcp: where other traffic goes
	Fallbacks []Fallback `json:"fallbacks,omitempty"`
	// shadowsocks: tcp,udp (default), tcp or udp
	SSNetwork string `json:"ss_network,omitempty"`

	// transport extras
	WSHeartbeat          int    `json:"ws_heartbeat_period,omitempty"` // seconds
	XPaddingBytes        string `json:"xhttp_padding_bytes,omitempty"` // e.g. 100-1000
	XNoSSEHeader         bool   `json:"xhttp_no_sse_header,omitempty"`
	XNoGRPCHeader        bool   `json:"xhttp_no_grpc_header,omitempty"`
	XScMaxEachPostBytes  string `json:"xhttp_sc_max_each_post_bytes,omitempty"`
	XScMaxBufferedPosts  int    `json:"xhttp_sc_max_buffered_posts,omitempty"`
	XScStreamUpServerSec string `json:"xhttp_sc_stream_up_server_secs,omitempty"`
	KCPMTU               int    `json:"kcp_mtu,omitempty"`
	KCPTTI               int    `json:"kcp_tti,omitempty"`
	KCPUplink            int    `json:"kcp_uplink_capacity,omitempty"`
	KCPDownlink          int    `json:"kcp_downlink_capacity,omitempty"`
	KCPCongestion        bool   `json:"kcp_congestion,omitempty"`
	KCPReadBuffer        int    `json:"kcp_read_buffer_size,omitempty"`
	KCPWriteBuffer       int    `json:"kcp_write_buffer_size,omitempty"`

	// TLS extras
	TLSMinVersion       string `json:"tls_min_version,omitempty"` // 1.0 … 1.3
	TLSMaxVersion       string `json:"tls_max_version,omitempty"`
	TLSCipherSuites     string `json:"tls_cipher_suites,omitempty"`
	TLSRejectUnknownSNI bool   `json:"tls_reject_unknown_sni,omitempty"`

	// REALITY extras
	RealityShow          bool   `json:"reality_show,omitempty"`
	RealityXver          int    `json:"reality_xver,omitempty"`
	RealityMinClientVer  string `json:"reality_min_client_ver,omitempty"`
	RealityMaxClientVer  string `json:"reality_max_client_ver,omitempty"`
	RealityMaxTimeDiff   int    `json:"reality_max_time_diff,omitempty"` // ms
	RealityMldsa65Seed   string `json:"reality_mldsa65_seed,omitempty"`
	RealityMldsa65Verify string `json:"reality_mldsa65_verify,omitempty"`

	// hysteria2 / tuic extras (sing-box)
	Masquerade            string `json:"masquerade,omitempty"` // hysteria2: a site URL or file:///dir shown to probes
	IgnoreClientBandwidth bool   `json:"ignore_client_bandwidth,omitempty"`
	ZeroRTT               bool   `json:"zero_rtt_handshake,omitempty"` // tuic
	Heartbeat             string `json:"heartbeat,omitempty"`          // tuic, e.g. 10s
	AuthTimeout           string `json:"auth_timeout,omitempty"`       // tuic, e.g. 3s
}

// Sniffing makes Xray read the destination domain from the traffic.
type Sniffing struct {
	Enabled      bool     `json:"enabled"`
	DestOverride []string `json:"dest_override,omitempty"` // http, tls, quic, fakedns
	MetadataOnly bool     `json:"metadata_only,omitempty"`
	RouteOnly    bool     `json:"route_only,omitempty"`
}

// Fallback sends non-proxy traffic of a vless/trojan TLS inbound elsewhere.
type Fallback struct {
	Name string `json:"name,omitempty"` // SNI to match
	ALPN string `json:"alpn,omitempty"`
	Path string `json:"path,omitempty"`
	Dest string `json:"dest"` // port or host:port or unix socket
	Xver int    `json:"xver,omitempty"`
}

type Inbound struct {
	ID       int64           `json:"id"`
	Tag      string          `json:"tag"`
	Protocol Protocol        `json:"protocol"`
	Listen   string          `json:"listen"`
	Port     int             `json:"port"`
	Address  string          `json:"address"` // seeds the first host only
	Remark   string          `json:"remark"`  // seeds the first host only
	CoreID   int64           `json:"core_id"` // the core config it belongs to; runs on every node of that core
	NodeID   *int64          `json:"node_id"` // optional: only on this node of the core (installer-made Hysteria2)
	Enabled  bool            `json:"enabled"`
	Settings InboundSettings `json:"settings"`
	// Hosts are the links clients get for this inbound, as in Marzban and
	// PasarGuard (read only here, edited on their own). Nil when not loaded.
	Hosts []Host `json:"hosts,omitempty"`
}

// Host is one client link of an inbound: where to connect and how it is
// named. The inbound holds the server side (protocol, keys, transport).
type Host struct {
	ID        int64 `json:"id"`
	InboundID int64 `json:"inbound_id"`
	Priority  int   `json:"priority"` // order in subscriptions, lowest first
	Enabled   bool  `json:"enabled"`
	HostSettings
}

// HostSettings are a host's own fields, as in PasarGuard; empty ones fall
// back to the inbound. Where a field takes several values, each
// subscription picks one at random.
type HostSettings struct {
	Remark        string       `json:"remark"`                    // link name, with {USERNAME} and other variables
	Address       []string     `json:"address"`                   // empty = the addresses of the core's nodes
	Port          int          `json:"port,omitempty"`            // 0 = the inbound's port
	Status        []UserStatus `json:"status,omitempty"`          // only users in these states get it; empty = all
	Host          []string     `json:"host,omitempty"`            // HTTP Host header (ws, xhttp, httpupgrade)
	Path          string       `json:"path,omitempty"`            // overrides the inbound's path
	Security      string       `json:"security,omitempty"`        // "" = inbound's default, tls, none
	SNI           []string     `json:"sni,omitempty"`             // TLS / Reality server name
	ALPN          []string     `json:"alpn,omitempty"`            // overrides the inbound's ALPN
	Fingerprint   string       `json:"fingerprint,omitempty"`     // uTLS fingerprint
	AllowInsecure bool         `json:"allow_insecure,omitempty"`  // skip certificate checks
	UseSNIAsHost  bool         `json:"use_sni_as_host,omitempty"` // Host header = the SNI picked
}

// Core is a core configuration, as in PasarGuard: a named set of inbounds.
// Each node runs one core; the first core is the default.
type Core struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	Inbounds  int       `json:"inbounds"` // how many inbounds it has
	Nodes     int       `json:"nodes"`    // how many nodes run it
}

// Runs reports whether the inbound is served by the node.
func (in *Inbound) Runs(n *Node) bool {
	return in.Enabled && in.CoreID == n.CoreID && (in.NodeID == nil || *in.NodeID == n.ID)
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
	CoreID           int64      `json:"core_id"` // the core config it runs (the default one when unset)
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
