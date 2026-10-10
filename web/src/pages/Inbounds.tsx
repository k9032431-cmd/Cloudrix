import { useEffect, useState, type FormEvent } from 'react'
import { Code2, Pencil, Plus, Trash2, Wand2 } from '../components/icons'
import { del, get, post, put } from '../lib/api'
import { useFetch } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import { PROTOCOLS, type Inbound, type InboundSettings, type Node, type Protocol } from '../lib/types'
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
  const { data: nodes } = useFetch<Node[]>('/api/nodes')
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
            <Button onClick={() => setEditing(blank('vless'))}>
              <Plus className="h-4 w-4" />
              {t('inbounds.new')}
            </Button>
          </>
        }
      />
      <ErrorNote error={error} />
      <Card>
        {data?.length === 0 ? (
          <Empty />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>{t('inbounds.tag')}</Th>
                <Th>{t('inbounds.protocol')}</Th>
                <Th>{t('inbounds.port')}</Th>
                <Th>{t('inbounds.transport')}</Th>
                <Th>{t('inbounds.node')}</Th>
                <Th>{t('inbounds.core')}</Th>
                <Th className="w-24" />
              </tr>
            </thead>
            <tbody>
              {data?.map((ib) => (
                <tr key={ib.id} className={ib.enabled ? '' : 'opacity-50'}>
                  <Td>
                    <div className="font-medium text-slate-900 dark:text-white">{ib.tag}</div>
                    <div className="text-xs text-slate-400">{ib.remark}</div>
                  </Td>
                  <Td>
                    <Badge tone="blue">{ib.protocol}</Badge>
                  </Td>
                  <Td className="tabular-nums">{ib.port}</Td>
                  <Td className="text-slate-500">
                    {XRAY.includes(ib.protocol) && ib.protocol !== 'shadowsocks'
                      ? `${ib.settings.network || 'tcp'} / ${ib.settings.security || 'none'}`
                      : ib.protocol === 'shadowsocks'
                        ? ib.settings.ss_method
                        : 'udp'}
                  </Td>
                  <Td className="text-slate-500">{nodes?.find((n) => n.id === ib.node_id)?.name ?? t('inbounds.local')}</Td>
                  <Td>
                    <Badge tone={coreOf(ib.protocol) === 'xray' ? 'gray' : 'violet'}>{coreOf(ib.protocol)}</Badge>
                  </Td>
                  <Td>
                    <div className="flex justify-end gap-1">
                      <IconButton onClick={() => setEditing(ib)} aria-label={t('common.edit')}>
                        <Pencil className="h-4 w-4" />
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
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null)
            reload()
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

function InboundForm({ initial, nodes, onClose, onSaved }: { initial: Inbound; nodes: Node[]; onClose: () => void; onSaved: () => void }) {
  const { t } = useI18n()
  const [ib, setIb] = useState<Inbound>(initial)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const s = ib.settings
  const set = <K extends keyof Inbound>(k: K, v: Inbound[K]) => setIb((x) => ({ ...x, [k]: v }))
  const setS = (patch: InboundSettings) => setIb((x) => ({ ...x, settings: { ...x.settings, ...patch } }))
  const isNew = ib.id === 0
  const streamProto = ['vless', 'vmess', 'trojan'].includes(ib.protocol)
  const needsCert = ib.protocol === 'hysteria2' || ib.protocol === 'tuic' || s.security === 'tls'

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      if (isNew) await post('/api/inbounds', ib)
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
                onClick={() => setIb(blank(p))}
                className={
                  'rounded-lg px-3 py-1.5 text-xs font-medium ring-1 ring-inset transition-colors ' +
                  (ib.protocol === p
                    ? 'bg-brand-600 text-white ring-brand-600'
                    : 'text-slate-600 ring-slate-200 hover:bg-slate-50 dark:text-slate-300 dark:ring-slate-700 dark:hover:bg-slate-800')
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
            <Select value={ib.node_id ?? ''} onChange={(e) => set('node_id', e.target.value ? Number(e.target.value) : null)}>
              <option value="">{t('inbounds.local')}</option>
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
      </form>
    </Modal>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <fieldset className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
      <legend className="px-1 text-xs font-semibold uppercase tracking-wide text-slate-500">{title}</legend>
      <div className="grid gap-4 sm:grid-cols-3">{children}</div>
    </fieldset>
  )
}

function CorePreview({ nodes, onClose }: { nodes: Node[]; onClose: () => void }) {
  const { t } = useI18n()
  const [core, setCore] = useState<'xray' | 'sing-box'>('xray')
  const [node, setNode] = useState('')
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
      <p className="mb-3 text-sm text-slate-500">{t('inbounds.previewHint')}</p>
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
          <option value="">{t('inbounds.local')}</option>
          {nodes.map((n) => (
            <option key={n.id} value={n.id}>
              {n.name}
            </option>
          ))}
        </Select>
        {text && <CopyButton text={text} />}
      </div>
      <ErrorNote error={error} />
      <pre className="max-h-[55vh] overflow-auto rounded-lg bg-slate-950 p-4 text-xs leading-relaxed text-slate-200">{text}</pre>
    </Modal>
  )
}
