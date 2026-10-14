import { useEffect, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ChevronLeft, Code2, Copy, Pencil, Plus, Server, Shuffle, Trash2, Wand2 } from '../components/icons'
import { del, get, post, put } from '../lib/api'
import { useFetch } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import { PROTOCOLS, type Core, type Group, type Inbound, type InboundSettings, type Node, type Protocol } from '../lib/types'
import { Badge, Button, Card, Confirm, CopyButton, ErrorNote, Field, IconButton, Input, Modal, PageHeader, Select, Skeleton, Table, Td, Th, Toggle } from '../components/ui'
import { CoreNameDialog } from './Cores'

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

// Inbounds on one port of a node clash (the core would refuse the node's
// whole config) unless they listen on different IPs or are of one Xray
// protocol: then, like hosts in Marzban/PasarGuard, they share a listener
// whose server side (transport, keys, exit) is the first one's, and each
// keeps its own address, remark and SNI. Mirrors core.PortClash.
const TCP: Protocol[] = ['vless', 'vmess', 'trojan', 'shadowsocks']
// Xray protocols listen on TCP, sing-box ones on UDP; shadowsocks leaves
// UDP to a sing-box inbound on its port, so they never clash
const sharesTransport = (a: Protocol, b: Protocol) => TCP.includes(a) === TCP.includes(b)
const wildcard = (l: string) => !l || l === '0.0.0.0' || l === '::'
const hosted = (a: Inbound, b: Inbound) =>
  a.port === b.port && a.protocol === b.protocol && TCP.includes(a.protocol) && (a.listen === b.listen || (wildcard(a.listen) && wildcard(b.listen)))
function clashes(a: Inbound, b: Inbound): boolean {
  if (a.port !== b.port || !sharesTransport(a.protocol, b.protocol)) return false
  if (a.listen !== b.listen && !wildcard(a.listen) && !wildcard(b.listen)) return false
  return !hosted(a, b)
}
// two inbounds run side by side: one core, no pins to different nodes
const together = (a: Inbound, b: Inbound) => a.core_id === b.core_id && (a.node_id == null || b.node_id == null || a.node_id === b.node_id)
function portClashes(list: Inbound[]): Map<number, string> {
  const out = new Map<number, string>()
  for (const a of list)
    for (const b of list)
      if (a.id !== b.id && a.enabled && b.enabled && together(a, b) && clashes(a, b))
        out.set(a.id, b.tag)
  return out
}

// duplicate makes a new-inbound draft from an existing one: same settings
// and a free tag. Xray inbounds keep the port (the copy becomes a host of the
// same listener: change its address, remark or SNI); sing-box ones move to
// the next free port.
function duplicate(ib: Inbound, all: Inbound[]): Inbound {
  const tags = new Set(all.map((x) => x.tag))
  let tag = `${ib.tag}-copy`
  for (let i = 2; tags.has(tag); i++) tag = `${ib.tag}-copy${i}`
  let port = ib.port
  if (!TCP.includes(ib.protocol)) {
    const ports = new Set(all.filter((x) => together(x, ib)).map((x) => x.port))
    while (ports.has(port) && port < 65535) port++
  }
  return { ...ib, id: 0, tag, port, settings: { ...ib.settings } }
}

const randomPort = () => 10000 + Math.floor(Math.random() * 50000)

// newInbound starts a protocol's preset in the core, on a port the core does
// not use yet (the preset's when free, else a random one)
const newInbound = (p: Protocol, core: number, all: Inbound[]): Inbound => {
  const ib = blank(p, core)
  const used = new Set(all.filter((x) => x.core_id === core).map((x) => x.port))
  while (used.has(ib.port)) ib.port = randomPort()
  return ib
}

const blank = (p: Protocol, core: number): Inbound => ({
  id: 0,
  tag: `${p}-${Math.random().toString(36).slice(2, 6)}`,
  protocol: p,
  listen: '0.0.0.0',
  port: PRESETS[p].port ?? 443,
  address: '',
  remark: '{USERNAME} · ' + p.toUpperCase(),
  core_id: core,
  node_id: null,
  enabled: true,
  settings: { ...PRESETS[p].settings },
})

// CoreEditor edits one core configuration, as in PasarGuard: its name and
// the inbounds (protocols and ports) every node of the core runs. The links
// users get are set up under Hosts.
export default function CoreEditor() {
  const { t } = useI18n()
  const id = Number(useParams().id)
  const { data: cores, reload: reloadCores } = useFetch<Core[]>('/api/cores')
  const { data: all, error, reload } = useFetch<Inbound[]>('/api/inbounds')
  const { data: nodes } = useFetch<Node[]>('/api/nodes')
  const { data: groups, reload: reloadGroups } = useFetch<Group[]>('/api/groups')
  const [editing, setEditing] = useState<Inbound | null>(null)
  const [deleting, setDeleting] = useState<Inbound | null>(null)
  const [preview, setPreview] = useState(false)
  const [renaming, setRenaming] = useState(false)
  const core = cores?.find((c) => c.id === id)
  const data = all?.filter((ib) => ib.core_id === id)
  const clash = portClashes(all ?? [])
  const running = (nodes ?? []).filter((n) => n.core_id === id)

  return (
    <>
      <Link to="/cores" className="mb-3 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
        <ChevronLeft className="h-4 w-4" />
        {t('nav.cores')}
      </Link>
      <PageHeader
        title={core ? core.name : '…'}
        subtitle={t('core.editorSubtitle')}
        actions={
          <>
            <Button variant="secondary" onClick={() => setRenaming(true)} disabled={!core}>
              <Pencil className="h-4 w-4" />
              <span className="hide-sm">{t('core.rename')}</span>
            </Button>
            <Button variant="secondary" onClick={() => setPreview(true)} disabled={!running.length}>
              <Code2 className="h-4 w-4" />
              <span className="hide-sm">{t('inbounds.preview')}</span>
            </Button>
            <Button onClick={() => setEditing(newInbound('vless', id, all ?? []))} disabled={!core}>
              <Plus className="h-4 w-4" />
              {t('core.addInbound')}
            </Button>
          </>
        }
      />
      <ErrorNote error={error} />
      <div className="mb-4 flex flex-wrap items-center gap-2 text-[13px] text-muted-foreground">
        <Server className="h-4 w-4" />
        {running.length ? (
          <>
            <span>{t('core.runsOn')}</span>
            {running.map((n) => (
              <Badge key={n.id} tone="gray">
                {n.name}
              </Badge>
            ))}
          </>
        ) : (
          <span>
            {t('core.noNodes')}{' '}
            <Link to="/nodes" className="text-foreground underline underline-offset-2">
              {t('nav.nodes')}
            </Link>
          </span>
        )}
      </div>
      <Card>
        <div className="card-head">
          <div>
            <h2 className="card-title">{t('core.inbounds')}</h2>
            <p className="card-desc">{t('core.inboundsDesc')}</p>
          </div>
        </div>
        {!data ? (
          <div className="p-4">
            <Skeleton className="h-10 w-full" />
          </div>
        ) : data.length === 0 ? (
          <div className="empty">
            <h3>{t('core.noInbounds')}</h3>
            <Button className="mt-2" onClick={() => setEditing(newInbound('vless', id, all ?? []))}>
              <Plus className="h-4 w-4" />
              {t('core.addInbound')}
            </Button>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th className="w-10">#</Th>
                <Th>{t('inbounds.tag')}</Th>
                <Th className="hide-sm">{t('inbounds.protocol')}</Th>
                <Th className="hide-sm">{t('inbounds.port')}</Th>
                <Th className="hide-md">{t('inbounds.transport')}</Th>
                <Th className="w-24" />
              </tr>
            </thead>
            <tbody>
              {data.map((ib, i) => (
                <tr key={ib.id} className={ib.enabled ? 'cursor-pointer' : 'cursor-pointer opacity-50'} onClick={() => setEditing(ib)}>
                  <Td className="tabular-nums text-faint">{i + 1}</Td>
                  <Td className="max-w-[200px] sm:max-w-none">
                    <div className="truncate font-medium text-foreground">{ib.tag}</div>
                    <div className="truncate text-xs text-muted-foreground show-sm">
                      {ib.protocol} · {ib.port}
                    </div>
                    <div className="flex flex-wrap gap-1 text-xs text-faint">
                      <Link to="/hosts" onClick={(e) => e.stopPropagation()} className="hover:text-foreground hover:underline">
                        {t('inbounds.hostsCount', { n: ib.hosts?.filter((h) => h.enabled).length ?? 0 })}
                      </Link>
                      {ib.node_id != null && <span>· {t('core.pinned', { node: nodes?.find((n) => n.id === ib.node_id)?.name ?? '?' })}</span>}
                      {ib.settings.exit_node_id != null && <span>· → {nodes?.find((n) => n.id === ib.settings.exit_node_id)?.name ?? '?'}</span>}
                    </div>
                    {clash.get(ib.id) && (
                      <div className="mt-1 show-sm">
                        <Badge tone="red">{t('inbounds.portClash')}</Badge>
                      </div>
                    )}
                  </Td>
                  <Td className="hide-sm">
                    <Badge tone={coreOf(ib.protocol) === 'xray' ? 'blue' : 'violet'}>{ib.protocol}</Badge>
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
                  <Td>
                    <div className="flex justify-end gap-0.5 sm:gap-1" onClick={(e) => e.stopPropagation()}>
                      <IconButton onClick={() => setEditing(ib)} aria-label={t('common.edit')}>
                        <Pencil className="h-4 w-4" />
                      </IconButton>
                      <IconButton onClick={() => setEditing(duplicate(ib, all ?? []))} aria-label={t('inbounds.duplicate')} title={t('inbounds.duplicate')}>
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
          all={all ?? []}
          nodes={nodes ?? []}
          groups={groups ?? []}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null)
            reload()
            reloadGroups()
            reloadCores()
          }}
        />
      )}
      {preview && <CorePreview nodes={running} onClose={() => setPreview(false)} />}
      {renaming && core && (
        <CoreNameDialog
          core={core}
          onClose={() => setRenaming(false)}
          onSaved={() => {
            setRenaming(false)
            reloadCores()
          }}
        />
      )}
      <Confirm
        open={!!deleting}
        onClose={() => setDeleting(null)}
        title={t('common.delete')}
        message={t('core.deleteInbound', { name: deleting?.tag ?? '' })}
        onConfirm={async () => {
          await del(`/api/inbounds/${deleting!.id}`)
          reload()
          reloadCores()
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

function InboundForm({
  initial,
  all,
  nodes,
  groups,
  onClose,
  onSaved,
}: {
  initial: Inbound
  all: Inbound[]
  nodes: Node[]
  groups: Group[]
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useI18n()
  const [ib, setIb] = useState<Inbound>(initial)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const s = ib.settings
  const set = <K extends keyof Inbound>(k: K, v: Inbound[K]) => setIb((x) => ({ ...x, [k]: v }))
  const setS = (patch: InboundSettings) => setIb((x) => ({ ...x, settings: { ...x.settings, ...patch } }))
  const isNew = ib.id === 0
  // New inbounds go into the first group (Default) so its users get them
  // at once; groups are managed on their own page, as in PasarGuard.
  const groupIDs = groups.slice(0, 1).map((g) => g.id)
  const streamProto = ['vless', 'vmess', 'trojan'].includes(ib.protocol)
  // other inbounds on this listener; joining takes the first one's server side
  const peers = all.filter((o) => o.id !== ib.id && together(o, ib) && hosted(o, ib))
  const joining = peers.length > 0 && !(initial.id !== 0 && together(initial, ib) && hosted(initial, ib))
  const owner = peers.reduce<Inbound | undefined>((a, o) => (!a || o.id < a.id ? o : a), undefined)
  const needsCert = ib.protocol === 'hysteria2' || ib.protocol === 'tuic' || s.security === 'tls'

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      if (isNew) await post(`/api/inbounds?groups=${groupIDs.join(',')}`, ib)
      else await put(`/api/inbounds/${ib.id}`, ib)
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
                onClick={() => setIb(newInbound(p, ib.core_id, all))}
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
            <div className="flex gap-2">
              <Input type="number" min={1} max={65535} value={ib.port} onChange={(e) => set('port', Number(e.target.value))} required />
              <Button type="button" variant="secondary" onClick={() => set('port', randomPort())} aria-label={t('core.randomPort')} title={t('core.randomPort')}>
                <Shuffle className="h-4 w-4" />
              </Button>
            </div>
          </Field>
          <Field label="Listen" hint={t('inbounds.listenHint')}>
            <Input value={ib.listen} onChange={(e) => set('listen', e.target.value)} placeholder="0.0.0.0" />
          </Field>
          {coreOf(ib.protocol) === 'xray' && nodes.length > 1 && (
            <Field label={t('inbounds.exit')} hint={t('inbounds.exitHint')} className="sm:col-span-2">
              <Select
                value={ib.settings.exit_node_id ?? ''}
                onChange={(e) => setS({ exit_node_id: e.target.value ? Number(e.target.value) : undefined })}
              >
                <option value="">{t('inbounds.exitDirect')}</option>
                {nodes
                  .filter((n) => n.core_id !== ib.core_id)
                  .map((n) => (
                    <option key={n.id} value={n.id}>
                      {n.name} · {n.address}
                    </option>
                  ))}
              </Select>
            </Field>
          )}
        </div>

        {peers.length > 0 && (
          <div className="flex gap-2 rounded-lg bg-primary/5 px-3 py-2.5 text-[13px] text-muted-foreground ring-1 ring-inset ring-primary/15">
            <Copy className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
            <span>
              {joining
                ? t('inbounds.hostJoin', { port: ib.port, tag: owner!.tag })
                : t('inbounds.hostShared', { tags: peers.map((o) => o.tag).join(', ') })}
            </span>
          </div>
        )}

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
            {s.security === 'tls' && (
              <Field label="ALPN">
                <Input value={list(s.alpn)} onChange={(e) => setS({ alpn: parseList(e.target.value) })} placeholder="h2, http/1.1" />
              </Field>
            )}
            {s.security === 'reality' && (
              <>
                <Field label="Dest">
                  <Input value={s.reality_dest ?? ''} onChange={(e) => setS({ reality_dest: e.target.value })} placeholder="www.microsoft.com:443" />
                </Field>
                <Field label="Server names" hint={t('inbounds.serverNamesHint')}>
                  <Input
                    value={list(s.reality_server_names)}
                    onChange={(e) => setS({ reality_server_names: parseList(e.target.value) })}
                    placeholder="www.microsoft.com"
                    required
                  />
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
            <Field label="ALPN">
              <Input value={list(s.alpn)} onChange={(e) => setS({ alpn: parseList(e.target.value) })} placeholder="h3" />
            </Field>
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
        {ib.node_id != null && <p className="hint">{t('core.pinnedHint', { node: nodes.find((n) => n.id === ib.node_id)?.name ?? '?' })}</p>}
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
