export type Role = 'sudo' | 'admin' | 'reseller'

export interface Admin {
  id: number
  username: string
  role: Role
  disabled: boolean
  user_limit: number
  created_at: string
  expire_at: string | null
  data_limit: number
  used_traffic: number
  max_user_data: number
  max_user_devices: number
}

export type UserStatus = 'active' | 'disabled' | 'limited' | 'expired' | 'on_hold'
export type ResetStrategy = 'no_reset' | 'day' | 'week' | 'month'

export interface User {
  id: number
  username: string
  sub_token: string
  status: UserStatus
  data_limit: number
  used_traffic: number
  lifetime_used: number
  expire_at: string | null
  on_hold_duration: number
  reset_strategy: ResetStrategy
  last_reset_at: string
  device_limit: number
  inbounds: string[]
  group_ids: number[]
  note: string
  admin_id: number
  created_at: string
  online_at: string | null
  sub_updated_at: string | null
  sub_user_agent: string
  online_ips?: string[]
  kicked_ips?: string[]
}

export interface UserInput {
  username: string
  status: 'active' | 'disabled' | 'on_hold'
  data_limit: number
  expire_at: string | null
  on_hold_duration: number
  reset_strategy: ResetStrategy
  device_limit: number
  inbounds: string[]
  group_ids: number[]
  note: string
  admin_id?: number
}

export interface Group {
  id: number
  name: string
  inbound_ids: number[]
  disabled: boolean
  users: number
  created_at: string
}

export type Protocol = 'vless' | 'vmess' | 'trojan' | 'shadowsocks' | 'hysteria2' | 'tuic' | 'wireguard'
export const PROTOCOLS: Protocol[] = ['vless', 'vmess', 'trojan', 'shadowsocks', 'hysteria2', 'tuic', 'wireguard']

export interface InboundSettings {
  network?: string
  path?: string
  host?: string
  service_name?: string
  header_type?: string
  xhttp_mode?: string
  security?: string
  sni?: string
  alpn?: string[]
  fingerprint?: string
  allow_insecure?: boolean
  exit_node_id?: number
  cert_file?: string
  key_file?: string
  reality_dest?: string
  reality_server_names?: string[]
  reality_private_key?: string
  reality_public_key?: string
  reality_short_ids?: string[]
  reality_spider_x?: string
  flow?: string
  ss_method?: string
  ss_server_key?: string
  obfs?: string
  obfs_password?: string
  up_mbps?: number
  down_mbps?: number
  congestion_control?: string
  wg_private_key?: string
  wg_public_key?: string
  wg_network?: string
  wg_dns?: string
  mtu?: number

  // advanced editor fields (as in PasarGuard); empty = core defaults
  sniffing?: Sniffing
  accept_proxy_protocol?: boolean
  tcp_fast_open?: boolean
  tcp_congestion?: string
  tcp_keepalive_interval?: number
  udp_timeout?: string
  fallbacks?: Fallback[]
  ss_network?: string
  ws_heartbeat_period?: number
  xhttp_padding_bytes?: string
  xhttp_no_sse_header?: boolean
  xhttp_no_grpc_header?: boolean
  xhttp_sc_max_each_post_bytes?: string
  xhttp_sc_max_buffered_posts?: number
  xhttp_sc_stream_up_server_secs?: string
  kcp_mtu?: number
  kcp_tti?: number
  kcp_uplink_capacity?: number
  kcp_downlink_capacity?: number
  kcp_congestion?: boolean
  kcp_read_buffer_size?: number
  kcp_write_buffer_size?: number
  tls_min_version?: string
  tls_max_version?: string
  tls_cipher_suites?: string
  tls_reject_unknown_sni?: boolean
  reality_show?: boolean
  reality_xver?: number
  reality_min_client_ver?: string
  reality_max_client_ver?: string
  reality_max_time_diff?: number
  reality_mldsa65_seed?: string
  reality_mldsa65_verify?: string
  masquerade?: string
  ignore_client_bandwidth?: boolean
  zero_rtt_handshake?: boolean
  heartbeat?: string
  auth_timeout?: string
}

export interface Sniffing {
  enabled: boolean
  dest_override?: string[]
  metadata_only?: boolean
  route_only?: boolean
}

export interface Fallback {
  name?: string
  alpn?: string
  path?: string
  dest: string
  xver?: number
}

export interface Inbound {
  id: number
  tag: string
  protocol: Protocol
  listen: string
  port: number
  address: string
  remark: string
  core_id: number
  node_id: number | null // only on this node of the core (installer-made Hysteria2)
  enabled: boolean
  settings: InboundSettings
  hosts?: Host[]
}

// Core is a core configuration, as in PasarGuard: a set of inbounds that
// every node running it serves.
export interface Core {
  id: number
  name: string
  created_at: string
  inbounds: number
  nodes: number
}

// Host is one client link of an inbound (address, name, SNI…), as in
// Marzban and PasarGuard; the inbound keeps the server side.
export interface Host {
  id: number
  inbound_id: number
  priority: number
  enabled: boolean
  remark: string
  address: string[]
  port?: number
  status?: UserStatus[]
  host?: string[]
  path?: string
  security?: '' | 'tls' | 'none'
  sni?: string[]
  alpn?: string[]
  fingerprint?: string
  allow_insecure?: boolean
  use_sni_as_host?: boolean
}

export type NodeStatus = 'connected' | 'connecting' | 'error' | 'disabled'

export interface Node {
  id: number
  name: string
  address: string
  bridge_port: number
  api_port: number
  usage_coefficient: number
  core_id: number
  status: NodeStatus
  message: string
  core_version: string
  last_seen: string | null
  upload_bytes: number
  download_bytes: number
  created_at: string
}

export interface Device {
  hwid: string
  platform: string
  os_version: string
  device_model: string
  user_agent: string
  first_seen: string
  last_seen: string
}

export interface AuditEntry {
  id: number
  admin: string
  action: string
  target: string
  details: string
  ip: string
  created_at: string
}

export interface DailyTraffic {
  day: string
  bytes: number
}

export interface Stats {
  users: {
    total: number
    active: number
    disabled: number
    limited: number
    expired: number
    on_hold: number
    online: number
    traffic: number
    expiring_soon: number
  }
  days: number
  traffic: DailyTraffic[]
  system: { version: string; uptime: number; goroutines: number; mem_alloc: number; cpus: number }
  nodes?: { total: number; connected: number }
}

export interface UserDrive {
  available: boolean
  url?: string
  synced_at?: string
  error?: string
}

export interface DriveSettings {
  enabled: boolean
  api_key: string
  client_id: string
  has_client_secret: boolean
  connected: boolean
  account: string
  format: 'base64' | 'plain'
  interval_minutes: number
  connect: { pending: boolean; user_code?: string; verification_url?: string; expires_at: string; error?: string }
  files: number
  errors: number
  last_run: { at?: string; files: number; uploaded: number; failed: number; seconds: number }
}

export interface Branding {
  title: string
  announce: string
  update_hours: number
  support_url: string
  web_page_url: string
  happ_only: boolean
  provider_id: string
  hide_settings: boolean
  always_hwid: boolean
}

export interface SubscriptionInfo {
  username: string
  status: UserStatus
  data_limit: number
  used_traffic: number
  expire_at: string | null
  reset_strategy: ResetStrategy
  url?: string
  happ_link?: string
  protected: boolean
  gdrive_url?: string
  title: string
  announce?: string
  support_url?: string
  links: string[]
  wireguard: string[]
}
