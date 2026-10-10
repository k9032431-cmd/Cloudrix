export type Role = 'sudo' | 'admin' | 'reseller'

export interface Admin {
  id: number
  username: string
  role: Role
  disabled: boolean
  user_limit: number
  created_at: string
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
  note: string
  admin_id: number
  created_at: string
  online_at: string | null
  sub_updated_at: string | null
  sub_user_agent: string
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
  note: string
  admin_id?: number
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
}

export interface Inbound {
  id: number
  tag: string
  protocol: Protocol
  listen: string
  port: number
  address: string
  remark: string
  node_id: number | null
  enabled: boolean
  settings: InboundSettings
}

export type NodeStatus = 'connected' | 'connecting' | 'error' | 'disabled'

export interface Node {
  id: number
  name: string
  address: string
  api_port: number
  usage_coefficient: number
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
}

export interface SubscriptionInfo {
  username: string
  status: UserStatus
  data_limit: number
  used_traffic: number
  expire_at: string | null
  reset_strategy: ResetStrategy
  url: string
  gdrive_url?: string
  title: string
  announce?: string
  support_url?: string
  links: string[]
  wireguard: string[]
}
