import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { Code2, Copy, Pencil, Plus, Trash2, Wand2 } from '../components/icons'
import { del, get, post, put } from '../lib/api'
import { useFetch } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import { PROTOCOLS, type Group, type Inbound, type InboundSettings, type Node, type Protocol } from '../lib/types'
import { Badge, Button, Card, Confirm, CopyButton, Empty, ErrorNote, Field, IconButton, Input, Modal, PageHeader, Select, Table, Td, Th, Toggle } from '../components/ui'

const XRAY: Protocol[] = ['vless', 'vmess', 'trojan', 'shadowsocks']
const coreOf = (p: Protocol) => (XRAY.includes(p) ? 'xray' : 'sing-box')

// Sensible starting points per protocol so a new inbound works with minimal edits.
const PRESETS: Record<Protocol, Partial<Inbound>> = {
  vless: { port: 443, settings: { network: 'tcp', security: 'reality', flow: 'xtls-rprx-vision', fingerprint: 'chrome', reality_dest: 'www.microsoft.com:443', reality_server_names: ['www.microsoft.com'] } },
  vmess: { port: 8080, settings: { network: 'ws', path: '/vm', security: 'none' } },
  trojan: { port: 2083, settings: { network: 'tcp', security: 'tls' } },
  shadowsocks: { port: 8388, settings: { ss_method: 'chacha20-ietf-poly1305' } },
  hysteria2: { port: 8443, settings: { obfs: '', alpn: ['h3'] } },
  tuic: { port: 8444, settings: { congestion_control: 'bbr', alpn: ['h3'] } },
  wireguard: { port: 51820, settings: { wg_network: '10.8.0.0/16', wg_dns: '1.1.1.1', mtu: 1420 } },
}

// Inbounds sharing a port on one node (same TCP or UDP socket) make the core
// refuse the node's whole config; the panel now rejects them, older ones are flagged.
const TCP: Protocol[] = ['vless', 'vmess', 'trojan', 'shadowsocks']
const sharesTransport = (a: Protocol, b: Protocol) => {
  const udp = (p: Protocol) => !TCP.includes(p) || p === 'shadowsocks'
  return (TCP.includes(a) && TCP.includes(b)) || (udp(a) && udp(b))
}
function portClashes(list: Inbound[]): Map<number, string> {
  const out = new Map<number, string>()
  for (const a of list)
    for (const b of list)
      if (a.id !== b.id && a.enabled && b.enabled && a.node_id != null && a.node_id === b.node_id && a.port === b.port && sharesTransport(a.protocol, b.protocol))
        out.set(a.id, b.tag)
  return out
}

// duplicate makes a new-inbound draft from an existing one: same settings,
// a free tag, and the next port not taken on that node.
function duplicate(ib: Inbound, all: Inbound[]): Inbound {
  const tags = new Set(all.map((x) => x.tag))
  let tag = `${ib.tag}-copy`
  for (let i = 2; tags.has(tag); i++) tag = `${ib.tag}-copy${i}`
  const ports = new Set(all.filter((x) => x.node_id === ib.node_id).map((x) => x.port))
  let port = ib.port
  while (ports.has(port) && port < 65535) port++
  return { ...ib, id: 0, tag, port, settings: { ...ib.settings } }
}

const blank = (p: Protocol): Inbound => ({
  id: 0,
  tag: `${p}-${Math.random().toString(36).slice(2, 6)}`,
  protocol: p,
  listen: '0.0.0.0',
  port: PRESETS[p].port ?? 443,
  address: '',
  remark: '{USERNAME} · ' + p.toUpperCase(),
  node_id: null,
  enabled: true,
  settings: { ...PRESETS[p].settings },
})

export default function Inbounds() {
  const { t } = useI18n()
  const { data, error, reload } = useFetch<Inbound[]>('/api/inbounds')
  const clash = portClashes(data ?? [])
  const { data: nodes } = useFetch<Node[]>('/api/nodes')
  const { data: groups, reload: reloadGroups } = useFetch<Group[]>('/api/groups')
  const [editing, setEditing] = useState<Inbound | null>(null)
  const [deleting, setDeleting] = useState<Inbound | null>(null)
  const [preview, setPreview] = useState(false)

  return (
    <>
      <PageHeader
        title={t('nav.inbounds')}
        actions={
          <>
            <Button variant="secondary" onClick={() => setPreview(true)}>
              <Code2 className="h-4 w-4" />
              {t('inbounds.preview')}
            </Button>
            <Button onClick={() => setEditing({ ...blank('vless'), node_id: nodes?.[0]?.id ?? null })}>
              <Plus className="h-4 w-4" />
              {t('inbounds.new')}
            </Button>
          </>
        }
      />
      <ErrorNote error={error} />
      {nodes?.length === 0 && (
        <div className="mb-4 flex flex-wrap items-center gap-3 rounded-lg bg-warning/10 px-3 py-2.5 text-[13px] text-foreground ring-1 ring-inset ring-warning/20">
          <span className="flex-1">{t('inbounds.needNodes')}</span>
          <Link to="/nodes" className="btn btn-outline btn-sm">
            <Plus />
            {t('inbounds.addNode')}
          </Link>
        </div>
      )}
      <Card>
        {data?.length === 0 ? (
          <Empty />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>{t('inbounds.tag')}</Th>
                <Th className="hide-sm">{t('inbounds.protocol')}</Th>
                <Th className="hide-sm">{t('inbounds.port')}</Th>
                <Th className="hide-md">{t('inbounds.transport')}</Th>
                <Th className="hide-sm">{t('inbounds.node')}</Th>
                <Th className="hide-md">{t('inbounds.core')}</Th>
                <Th className="w-24" />
              </tr>
            </thead>
            <tbody>
              {data?.map((ib) => (
                <tr key={ib.id} className={ib.enabled ? '' : 'opacity-50'}>
                  <Td className="max-w-[200px] sm:max-w-none">
                    <div className="truncate font-medium text-foreground">{ib.tag}</div>
                    <div className="hide-sm truncate text-xs text-faint">{ib.remark}</div>
                    {/* phones: the hidden columns in one line */}
                    <div className="truncate text-xs text-muted-foreground show-sm">
                      {ib.protocol} · {ib.port} · {nodes?.find((n) => n.id === ib.node_id)?.name ?? '—'}
                      {ib.settings.exit_node_id != null && ` → ${nodes?.find((n) => n.id === ib.settings.exit_node_id)?.name ?? '?'}`}
                    </div>
                    {clash.get(ib.id) && (
                      <div className="mt-1 show-sm">
                        <Badge tone="red">{t('inbounds.portClash')}</Badge>
                      </div>
                    )}
                  </Td>
                  <Td className="hide-sm">
                    <Badge tone="blue">{ib.protocol}</Badge>
                  </Td>
                  <Td className="hide-sm tabular-nums">
                    {ib.port}
                    {clash.get(ib.id) && (
                      <span className="ml-1.5" title={t('inbounds.portClashHint', { tag: clash.get(ib.id)! })}>
                        <Badge tone="red">{t('inbounds.portClash')}</Badge>
                      </span>
                    )}
                  </Td>
                  <Td className="hide-md text-muted-foreground">
                    {XRAY.includes(ib.protocol) && ib.protocol !== 'shadowsocks'
                      ? `${ib.settings.network || 'tcp'} / ${ib.settings.security || 'none'}`
                      : ib.protocol === 'shadowsocks'
                        ? ib.settings.ss_method
                        : 'udp'}
                  </Td>
                  <Td className="hide-sm text-muted-foreground">
                    {nodes?.find((n) => n.id === ib.node_id)?.name ?? (
                      <span title={t('inbounds.noNodeHint')}>
                        <Badge tone="red">{t('inbounds.noNode')}</Badge>
                      </span>
                    )}
                    {ib.settings.exit_node_id != null && (
                      <span className="ml-1.5 whitespace-nowrap text-foreground" title={t('inbounds.exitHint')}>
                        → {nodes?.find((n) => n.id === ib.settings.exit_node_id)?.name ?? '?'}
                      </span>
                    )}
                  </Td>
                  <Td className="hide-md">
                    <Badge tone={coreOf(ib.protocol) === 'xray' ? 'gray' : 'violet'}>{coreOf(ib.protocol)}</Badge>
                  </Td>
                  <Td>
                    <div className="flex justify-end gap-0.5 sm:gap-1">
                      <IconButton onClick={() => setEditing(ib)} aria-label={t('common.edit')}>
                        <Pencil className="h-4 w-4" />
                      </IconButton>
                      <IconButton onClick={() => setEditing(duplicate(ib, data ?? []))} aria-label={t('inbounds.duplicate')} title={t('inbounds.duplicate')}>
                        <Copy className="h-4 w-4" />
                      </IconButton>
                      <IconButton onClick={() => setDeleting(ib)} aria-label={t('common.delete')}>
                        <Trash2 className="h-4 w-4" />
                      </IconButton>
                    </div>
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      {editing && (
        <InboundForm
          initial={editing}
          nodes={nodes ?? []}
          groups={groups ?? []}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null)
            reload()
            reloadGroups()
          }}
        />
      )}
      {preview && <CorePreview nodes={nodes ?? []} onClose={() => setPreview(false)} />}
      <Confirm
        open={!!deleting}
        onClose={() => setDeleting(null)}
        title={t('common.delete')}
        message={t('common.confirmDelete', { name: deleting?.tag ?? '' })}
        onConfirm={async () => {
          await del(`/api/inbounds/${deleting!.id}`)
          reload()
        }}
      />
    </>
  )
}

const list = (v?: string[]) => (v ?? []).join(', ')
const parseList = (s: string) =>
  s
    .split(',')
    .map((x) => x.trim())
    .filter(Boolean)

function InboundForm({ initial, nodes, groups, onClose, onSaved }: { initial: Inbound; nodes: Node[]; groups: Group[]; onClose: () => void; onSaved: () => void }) {
  const { t } = useI18n()
  const [ib, setIb] = useState<Inbound>(initial)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const s = ib.settings
  const set = <K extends keyof Inbound>(k: K, v: Inbound[K]) => setIb((x) => ({ ...x, [k]: v }))
  const setS = (patch: InboundSettings) => setIb((x) => ({ ...x, settings: { ...x.settings, ...patch } }))
  const isNew = ib.id === 0
  // New inbounds go into the first group (Default) so its users get them at once.
  const [groupIDs, setGroupIDs] = useState<number[]>(() =>
    isNew ? groups.slice(0, 1).map((g) => g.id) : groups.filter((g) => g.inbound_ids.includes(initial.id)).map((g) => g.id),
  )
  const streamProto = ['vless', 'vmess', 'trojan'].includes(ib.protocol)
  const needsCert = ib.protocol === 'hysteria2' || ib.protocol === 'tuic' || s.security === 'tls'

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const q = `?groups=${groupIDs.join(',')}`
      if (isNew) await post(`/api/inbounds${q}`, ib)
      else await put(`/api/inbounds/${ib.id}${q}`, ib)
      onSaved()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const genReality = async () => {
    const k = await post<{ private_key: string; public_key: string; short_ids: string[] }>('/api/tools/reality-keys')
    setS({ reality_private_key: k.private_key, reality_public_key: k.public_key, reality_short_ids: k.short_ids })
  }
  const genWG = async () => {
    const k = await post<{ private_key: string; public_key: string }>('/api/tools/wireguard-keys')
    setS({ wg_private_key: k.private_key, wg_public_key: k.public_key })
  }
  const genSSKey = async () => {
    const bytes = s.ss_method === '2022-blake3-aes-128-gcm' ? 16 : 32
    const r = await post<{ base64: string }>(`/api/tools/random?bytes=${bytes}`)
    setS({ ss_server_key: r.base64 })
  }
  const genObfs = async () => setS({ obfs_password: (await post<{ hex: string }>('/api/tools/random?bytes=16')).hex })

  return (
    <Modal
      open
      onClose={onClose}
      wide
      title={isNew ? t('inbounds.new') : `${t('common.edit')}: ${initial.tag}`}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="inbound-form" loading={busy}>
            {isNew ? t('common.create') : t('common.save')}
          </Button>
        </>
      }
    >
      <form id="inbound-form" onSubmit={submit} className="space-y-5">
        <ErrorNote error={error} />
        {isNew && (
          <div className="flex flex-wrap gap-1.5">
            {PROTOCOLS.map((p) => (
              <button
                type="button"
                key={p}
                onClick={() => setIb(blank(p))}
                className={
                  'rounded-lg px-3 py-1.5 text-xs font-medium ring-1 ring-inset transition-colors ' +
                  (ib.protocol === p
                    ? 'bg-primary text-primary-foreground ring-primary'
                    : 'text-muted-foreground ring-border hover:bg-muted/60')
                }
              >
                {p}
              </button>
            ))}
          </div>
        )}

        <div className="grid gap-4 sm:grid-cols-3">
          <Field label={t('inbounds.tag')}>
            <Input value={ib.tag} onChange={(e) => set('tag', e.target.value)} required />
          </Field>
          <Field label={t('inbounds.port')}>
            <Input type="number" min={1} max={65535} value={ib.port} onChange={(e) => set('port', Number(e.target.value))} required />
          </Field>
          <Field label={t('inbounds.node')}>
            <Select value={ib.node_id ?? ''} onChange={(e) => set('node_id', e.target.value ? Number(e.target.value) : null)} required aria-invalid={!ib.node_id}>
              <option value="" disabled>
                {t('inbounds.local')}
              </option>
              {nodes.map((n) => (
                <option key={n.id} value={n.id}>
                  {n.name}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={t('inbounds.remark')} hint={t('inbounds.remarkHint')} className="sm:col-span-2">
            <Input value={ib.remark} onChange={(e) => set('remark', e.target.value)} />
          </Field>
          <Field label={t('inbounds.address')} hint={t('inbounds.addressHint')}>
            <Input value={ib.address} onChange={(e) => set('address', e.target.value)} placeholder="vpn.example.com" />
          </Field>
          {coreOf(ib.protocol) === 'xray' && nodes.length > 1 && (
            <Field label={t('inbounds.exit')} hint={t('inbounds.exitHint')} className="sm:col-span-2">
              <Select
                value={ib.settings.exit_node_id ?? ''}
                onChange={(e) => setS({ exit_node_id: e.target.value ? Number(e.target.value) : undefined })}
              >
                <option value="">{t('inbounds.exitDirect')}</option>
                {nodes
                  .filter((n) => n.id !== ib.node_id)
                  .map((n) => (
                    <option key={n.id} value={n.id}>
                      {n.name} · {n.address}
                    </option>
                  ))}
              </Select>
            </Field>
          )}
        </div>

        {streamProto && (
          <Section title={t('inbounds.transport')}>
            <Field label="Network">
              <Select value={s.network || 'tcp'} onChange={(e) => setS({ network: e.target.value })}>
                {['tcp', 'ws', 'grpc', 'xhttp', 'httpupgrade'].map((n) => (
                  <option key={n}>{n}</option>
                ))}
              </Select>
            </Field>
            {['ws', 'xhttp', 'httpupgrade'].includes(s.network || '') && (
              <>
                <Field label="Path">
                  <Input value={s.path ?? ''} onChange={(e) => setS({ path: e.target.value })} placeholder="/" />
                </Field>
                <Field label="Host">
                  <Input value={s.host ?? ''} onChange={(e) => setS({ host: e.target.value })} />
                </Field>
              </>
            )}
            {s.network === 'xhttp' && (
              <Field label="Mode">
                <Select value={s.xhttp_mode || 'auto'} onChange={(e) => setS({ xhttp_mode: e.target.value })}>
                  {['auto', 'packet-up', 'stream-up', 'stream-one'].map((m) => (
                    <option key={m}>{m}</option>
                  ))}
                </Select>
              </Field>
            )}
            {s.network === 'grpc' && (
              <Field label="Service name">
                <Input value={s.service_name ?? ''} onChange={(e) => setS({ service_name: e.target.value })} />
              </Field>
            )}
            {ib.protocol === 'vless' && (s.network || 'tcp') === 'tcp' && (
              <Field label="Flow">
                <Select value={s.flow ?? ''} onChange={(e) => setS({ flow: e.target.value })}>
                  <option value="">none</option>
                  <option>xtls-rprx-vision</option>
                </Select>
              </Field>
            )}
          </Section>
        )}

        {streamProto && (
          <Section title={t('inbounds.security')}>
            <Field label="Security">
              <Select value={s.security || 'none'} onChange={(e) => setS({ security: e.target.value })}>
                <option>none</option>
                <option>tls</option>
                {ib.protocol !== 'vmess' && <option>reality</option>}
              </Select>
            </Field>
            {s.security && s.security !== 'none' && (
              <Field label="Fingerprint">
                <Select value={s.fingerprint ?? ''} onChange={(e) => setS({ fingerprint: e.target.value })}>
                  {['', 'chrome', 'firefox', 'safari', 'ios', 'android', 'edge', 'random', 'randomized'].map((f) => (
                    <option key={f} value={f}>
                      {f || '—'}
                    </option>
                  ))}
                </Select>
              </Field>
            )}
            {s.security === 'tls' && (
              <Field label="SNI">
                <Input value={s.sni ?? ''} onChange={(e) => setS({ sni: e.target.value })} />
              </Field>
            )}
            {s.security === 'reality' && (
              <>
                <Field label="Dest">
                  <Input value={s.reality_dest ?? ''} onChange={(e) => setS({ reality_dest: e.target.value })} placeholder="www.microsoft.com:443" />
                </Field>
                <Field label="Server names">
                  <Input value={list(s.reality_server_names)} onChange={(e) => setS({ reality_server_names: parseList(e.target.value) })} />
                </Field>
                <Field label="Short IDs">
                  <Input value={list(s.reality_short_ids)} onChange={(e) => setS({ reality_short_ids: parseList(e.target.value) })} />
                </Field>
                <Field label="Private key">
                  <Input value={s.reality_private_key ?? ''} onChange={(e) => setS({ reality_private_key: e.target.value })} className="font-mono text-xs" />
                </Field>
                <Field label="Public key">
                  <Input value={s.reality_public_key ?? ''} onChange={(e) => setS({ reality_public_key: e.target.value })} className="font-mono text-xs" />
                </Field>
                <div className="flex items-end">
                  <Button type="button" variant="secondary" onClick={genReality}>
                    <Wand2 className="h-4 w-4" />
                    {t('common.generate')}
                  </Button>
                </div>
              </>
            )}
          </Section>
        )}

        {ib.protocol === 'shadowsocks' && (
          <Section title="Shadowsocks">
            <Field label="Method">
              <Select value={s.ss_method || 'chacha20-ietf-poly1305'} onChange={(e) => setS({ ss_method: e.target.value })}>
                {['chacha20-ietf-poly1305', 'aes-256-gcm', 'aes-128-gcm', '2022-blake3-aes-128-gcm', '2022-blake3-aes-256-gcm', '2022-blake3-chacha20-poly1305'].map((m) => (
                  <option key={m}>{m}</option>
                ))}
              </Select>
            </Field>
            {s.ss_method?.startsWith('2022-') && (
              <>
                <Field label="Server key (base64)">
                  <Input value={s.ss_server_key ?? ''} onChange={(e) => setS({ ss_server_key: e.target.value })} className="font-mono text-xs" />
                </Field>
                <div className="flex items-end">
                  <Button type="button" variant="secondary" onClick={genSSKey}>
                    <Wand2 className="h-4 w-4" />
                    {t('common.generate')}
                  </Button>
                </div>
              </>
            )}
          </Section>
        )}

        {ib.protocol === 'hysteria2' && (
          <Section title="Hysteria2">
            <Field label="Obfs">
              <Select value={s.obfs ?? ''} onChange={(e) => setS({ obfs: e.target.value })}>
                <option value="">none</option>
                <option>salamander</option>
              </Select>
            </Field>
            {s.obfs && (
              <Field label="Obfs password">
                <div className="flex gap-2">
                  <Input value={s.obfs_password ?? ''} onChange={(e) => setS({ obfs_password: e.target.value })} />
                  <Button type="button" variant="secondary" onClick={genObfs} aria-label={t('common.generate')}>
                    <Wand2 className="h-4 w-4" />
                  </Button>
                </div>
              </Field>
            )}
            <Field label="Up / Down Mbps">
              <div className="flex gap-2">
                <Input type="number" min={0} value={s.up_mbps ?? 0} onChange={(e) => setS({ up_mbps: Number(e.target.value) })} />
                <Input type="number" min={0} value={s.down_mbps ?? 0} onChange={(e) => setS({ down_mbps: Number(e.target.value) })} />
              </div>
            </Field>
          </Section>
        )}

        {ib.protocol === 'tuic' && (
          <Section title="TUIC">
            <Field label="Congestion control">
              <Select value={s.congestion_control || 'bbr'} onChange={(e) => setS({ congestion_control: e.target.value })}>
                {['bbr', 'cubic', 'new_reno'].map((c) => (
                  <option key={c}>{c}</option>
                ))}
              </Select>
            </Field>
          </Section>
        )}

        {(ib.protocol === 'hysteria2' || ib.protocol === 'tuic') && (
          <Section title="TLS">
            <Field label="SNI">
              <Input value={s.sni ?? ''} onChange={(e) => setS({ sni: e.target.value })} />
            </Field>
            <Field label="ALPN">
              <Input value={list(s.alpn)} onChange={(e) => setS({ alpn: parseList(e.target.value) })} />
            </Field>
            <div className="flex items-end pb-2">
              <Toggle checked={!!s.allow_insecure} onChange={(v) => setS({ allow_insecure: v })} label="allow insecure" />
            </div>
          </Section>
        )}

        {needsCert && (
          <Section title="Certificate">
            <p className="hint sm:col-span-2">{t('inbounds.certHint')}</p>
            <Field label="Cert file">
              <Input value={s.cert_file ?? ''} onChange={(e) => setS({ cert_file: e.target.value })} placeholder="/etc/cloudrix/certs/fullchain.pem" />
            </Field>
            <Field label="Key file">
              <Input value={s.key_file ?? ''} onChange={(e) => setS({ key_file: e.target.value })} placeholder="/etc/cloudrix/certs/privkey.pem" />
            </Field>
          </Section>
        )}

        {ib.protocol === 'wireguard' && (
          <Section title="WireGuard">
            <Field label="Network">
              <Input value={s.wg_network ?? ''} onChange={(e) => setS({ wg_network: e.target.value })} placeholder="10.8.0.0/16" />
            </Field>
            <Field label="DNS">
              <Input value={s.wg_dns ?? ''} onChange={(e) => setS({ wg_dns: e.target.value })} />
            </Field>
            <Field label="MTU">
              <Input type="number" value={s.mtu ?? 0} onChange={(e) => setS({ mtu: Number(e.target.value) })} />
            </Field>
            <Field label="Private key">
              <Input value={s.wg_private_key ?? ''} onChange={(e) => setS({ wg_private_key: e.target.value })} className="font-mono text-xs" />
            </Field>
            <Field label="Public key">
              <Input value={s.wg_public_key ?? ''} onChange={(e) => setS({ wg_public_key: e.target.value })} className="font-mono text-xs" />
            </Field>
            <div className="flex items-end">
              <Button type="button" variant="secondary" onClick={genWG}>
                <Wand2 className="h-4 w-4" />
                {t('common.generate')}
              </Button>
            </div>
          </Section>
        )}

        <Toggle checked={ib.enabled} onChange={(v) => set('enabled', v)} label={ib.enabled ? t('common.enabled') : t('common.disabled')} />
        {groups.length > 0 && (
          <Field label={t('nav.groups')} hint={t('inbounds.groupsHint')}>
            <div className="flex flex-wrap gap-2">
              {groups.map((g) => {
                const on = groupIDs.includes(g.id)
                return (
                  <button
                    type="button"
                    key={g.id}
                    onClick={() => setGroupIDs((x) => (on ? x.filter((v) => v !== g.id) : [...x, g.id]))}
                    className={
                      'rounded-lg px-3 py-1.5 text-xs font-medium ring-1 ring-inset transition-colors ' +
                      (on ? 'bg-primary text-primary-foreground ring-primary' : 'text-muted-foreground ring-border hover:bg-muted/60')
                    }
                  >
                    {g.name}
                  </button>
                )
              })}
            </div>
          </Field>
        )}
      </form>
    </Modal>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <fieldset className="rounded-card border border-border p-4">
      <legend className="px-1 text-xs font-medium text-muted-foreground">{title}</legend>
      <div className="grid gap-4 sm:grid-cols-3">{children}</div>
    </fieldset>
  )
}

function CorePreview({ nodes, onClose }: { nodes: Node[]; onClose: () => void }) {
  const { t } = useI18n()
  const [core, setCore] = useState<'xray' | 'sing-box'>('xray')
  const [node, setNode] = useState(nodes[0] ? String(nodes[0].id) : '')
  const [text, setText] = useState('')
  const [error, setError] = useState<string | null>(null)
  const load = async (c = core, n = node) => {
    setError(null)
    try {
      setText(JSON.stringify(await get(`/api/core/config?core=${c}${n ? `&node=${n}` : ''}`), null, 2))
    } catch (e) {
      setText('')
      setError((e as Error).message)
    }
  }
  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  return (
    <Modal open onClose={onClose} wide title={t('inbounds.preview')}>
      <p className="mb-3 text-sm text-muted-foreground">{t('inbounds.previewHint')}</p>
      <div className="mb-3 flex flex-wrap gap-2">
        <Select
          className="w-auto"
          value={core}
          onChange={(e) => {
            const c = e.target.value as 'xray' | 'sing-box'
            setCore(c)
            load(c, node)
          }}
        >
          <option value="xray">Xray</option>
          <option value="sing-box">sing-box</option>
        </Select>
        <Select
          className="w-auto"
          value={node}
          onChange={(e) => {
            setNode(e.target.value)
            load(core, e.target.value)
          }}
        >
          {nodes.map((n) => (
            <option key={n.id} value={n.id}>
              {n.name}
            </option>
          ))}
        </Select>
        {text && <CopyButton text={text} />}
      </div>
      <ErrorNote error={error} />
      <pre className="max-h-[55vh] overflow-auto rounded-lg bg-muted p-4 text-xs leading-relaxed text-foreground ring-1 ring-inset ring-border">{text}</pre>
    </Modal>
  )
}
