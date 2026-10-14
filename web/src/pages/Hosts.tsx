import { useEffect, useState, type FormEvent } from 'react'
import { Reorder, useDragControls } from 'framer-motion'
import { Copy, Globe, GripVertical, Pencil, Plus, Trash2 } from '../components/icons'
import { del, post, put } from '../lib/api'
import { useFetch } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import type { Host, Inbound, Node } from '../lib/types'
import { Badge, Button, Card, Confirm, ErrorNote, Field, IconButton, Input, Modal, PageHeader, Select, Skeleton, Toggle, cx } from '../components/ui'
import { useToast } from '../components/toast'

const FINGERPRINTS = ['', 'chrome', 'firefox', 'safari', 'ios', 'android', 'edge', '360', 'qq', 'random', 'randomized']
const ALPNS = ['h3', 'h2', 'http/1.1']
const VARIABLES = ['{USERNAME}', '{DATA_LEFT}', '{DAYS_LEFT}', '{EXPIRE_DATE}', '{PROTOCOL}']

// Hosts are the links users get, as in PasarGuard: each points at an inbound
// (the server side) and sets its own address, name and TLS fields. Drag them
// to change the order of links in subscriptions.
export default function Hosts() {
  const { t } = useI18n()
  const toast = useToast()
  const { data, error, reload } = useFetch<Host[]>('/api/hosts')
  const { data: inbounds } = useFetch<Inbound[]>('/api/inbounds')
  const { data: nodes } = useFetch<Node[]>('/api/nodes')
  const [list, setList] = useState<Host[]>([])
  const [editing, setEditing] = useState<Host | null>(null)
  const [deleting, setDeleting] = useState<Host | null>(null)
  useEffect(() => setList(data ?? []), [data])

  const inboundOf = (h: Host) => inbounds?.find((ib) => ib.id === h.inbound_id)
  const nodeOf = (ib?: Inbound) => nodes?.find((n) => n.id === ib?.node_id)
  const blank = (): Host => ({ id: 0, inbound_id: inbounds?.[0]?.id ?? 0, priority: 0, enabled: true, remark: '{USERNAME} · ', address: '' })

  const saveOrder = async () => {
    const ids = list.map((h) => h.id)
    if (ids.join() === (data ?? []).map((h) => h.id).join()) return
    try {
      await put('/api/hosts/order', { ids })
      reload()
    } catch (e) {
      toast((e as Error).message, 'error')
      reload()
    }
  }
  const toggle = async (h: Host) => {
    setList((l) => l.map((x) => (x.id === h.id ? { ...x, enabled: !h.enabled } : x)))
    try {
      await put(`/api/hosts/${h.id}`, { ...h, enabled: !h.enabled })
    } catch (e) {
      toast((e as Error).message, 'error')
    }
    reload()
  }

  return (
    <>
      <PageHeader
        title={t('nav.hosts')}
        subtitle={t('hosts.subtitle')}
        actions={
          <Button onClick={() => setEditing(blank())} disabled={!inbounds?.length}>
            <Plus />
            {t('hosts.new')}
          </Button>
        }
      />
      <ErrorNote error={error} />
      {!data ? (
        <Card className="p-4">
          <Skeleton className="h-14 w-full" />
        </Card>
      ) : list.length === 0 ? (
        <Card>
          <div className="empty">
            <span className="empty-icon">
              <Globe />
            </span>
            <h3>{inbounds?.length ? t('hosts.empty') : t('hosts.noInbounds')}</h3>
            {!!inbounds?.length && (
              <Button className="mt-2" onClick={() => setEditing(blank())}>
                <Plus />
                {t('hosts.new')}
              </Button>
            )}
          </div>
        </Card>
      ) : (
        <Reorder.Group axis="y" values={list} onReorder={setList} className="grid gap-2">
          {list.map((h) => (
            <HostRow
              key={h.id}
              host={h}
              inbound={inboundOf(h)}
              node={nodeOf(inboundOf(h))}
              onDrop={saveOrder}
              onToggle={() => toggle(h)}
              onEdit={() => setEditing(h)}
              onCopy={() => setEditing({ ...h, id: 0, remark: `${h.remark} (copy)` })}
              onDelete={() => setDeleting(h)}
            />
          ))}
        </Reorder.Group>
      )}
      {editing && (
        <HostForm
          initial={editing}
          inbounds={inbounds ?? []}
          nodes={nodes ?? []}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null)
            reload()
          }}
        />
      )}
      <Confirm
        open={!!deleting}
        onClose={() => setDeleting(null)}
        title={t('common.delete')}
        message={t('common.confirmDelete', { name: deleting?.remark || deleting?.address || '' })}
        onConfirm={async () => {
          await del(`/api/hosts/${deleting!.id}`)
          reload()
        }}
      />
    </>
  )
}

function HostRow({
  host: h,
  inbound,
  node,
  onDrop,
  onToggle,
  onEdit,
  onCopy,
  onDelete,
}: {
  host: Host
  inbound?: Inbound
  node?: Node
  onDrop: () => void
  onToggle: () => void
  onEdit: () => void
  onCopy: () => void
  onDelete: () => void
}) {
  const { t } = useI18n()
  const drag = useDragControls()
  const address = h.address || node?.address || '—'
  const port = h.port || inbound?.port
  return (
    <Reorder.Item
      value={h}
      dragListener={false}
      dragControls={drag}
      onDragEnd={onDrop}
      className={cx('card flex items-center gap-2 px-2 py-2.5 sm:gap-3 sm:px-3', !h.enabled && 'opacity-60')}
    >
      <button
        type="button"
        className="flex h-8 w-6 shrink-0 cursor-grab touch-none items-center justify-center rounded text-faint hover:text-foreground active:cursor-grabbing"
        onPointerDown={(e) => drag.start(e)}
        aria-label={t('hosts.drag')}
        title={t('hosts.drag')}
      >
        <GripVertical className="h-4 w-4" />
      </button>
      <button type="button" role="switch" aria-checked={h.enabled} aria-label={h.enabled ? t('common.enabled') : t('common.disabled')} onClick={onToggle} className="switch shrink-0" />
      <button type="button" onClick={onEdit} className="min-w-0 flex-1 text-left">
        <div className="truncate font-medium text-foreground">{h.remark || inbound?.tag || '—'}</div>
        <div className="mt-0.5 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
          <span className="truncate font-mono">
            {address}
            {port ? `:${port}` : ''}
          </span>
          {inbound && (
            <Badge tone="blue">
              {inbound.tag} · {inbound.protocol}
            </Badge>
          )}
          {node && <span className="hide-sm">{node.name}</span>}
          {h.sni && <span className="hide-sm truncate">SNI {h.sni}</span>}
        </div>
      </button>
      <div className="flex shrink-0 gap-0.5 sm:gap-1">
        <IconButton onClick={onCopy} aria-label={t('hosts.copy')} title={t('hosts.copy')}>
          <Copy />
        </IconButton>
        <IconButton onClick={onEdit} aria-label={t('common.edit')}>
          <Pencil />
        </IconButton>
        <IconButton onClick={onDelete} aria-label={t('common.delete')}>
          <Trash2 />
        </IconButton>
      </div>
    </Reorder.Item>
  )
}

function HostForm({ initial, inbounds, nodes, onClose, onSaved }: { initial: Host; inbounds: Inbound[]; nodes: Node[]; onClose: () => void; onSaved: () => void }) {
  const { t } = useI18n()
  const [h, setH] = useState<Host>(initial)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const set = (patch: Partial<Host>) => setH((x) => ({ ...x, ...patch }))
  const isNew = h.id === 0
  const ib = inbounds.find((x) => x.id === h.inbound_id)
  const node = nodes.find((n) => n.id === ib?.node_id)
  const net = ib?.settings.network || 'tcp'
  const security = ib?.settings.security || 'none'
  const httpish = ['ws', 'xhttp', 'httpupgrade'].includes(net) || (net === 'tcp' && ib?.settings.header_type === 'http')
  const tlsish = security !== 'none' || ['hysteria2', 'tuic'].includes(ib?.protocol ?? '')

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      if (isNew) await post('/api/hosts', h)
      else await put(`/api/hosts/${h.id}`, h)
      onSaved()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      wide
      title={isNew ? t('hosts.new') : `${t('common.edit')}: ${initial.remark || initial.address}`}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="host-form" loading={busy}>
            {isNew ? t('common.create') : t('common.save')}
          </Button>
        </>
      }
    >
      <form id="host-form" onSubmit={submit} className="space-y-5">
        <ErrorNote error={error} />
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t('hosts.inbound')} className="sm:col-span-2">
            <Select value={h.inbound_id || ''} onChange={(e) => set({ inbound_id: Number(e.target.value) })} required>
              {inbounds.map((x) => (
                <option key={x.id} value={x.id}>
                  {x.tag} · {x.protocol} · {nodes.find((n) => n.id === x.node_id)?.name ?? '—'} · {x.port}
                </option>
              ))}
            </Select>
          </Field>
          <Field
            label={t('hosts.remark')}
            className="sm:col-span-2"
            hint={
              <span className="flex flex-wrap gap-1">
                {VARIABLES.map((v) => (
                  <button
                    type="button"
                    key={v}
                    onClick={() => set({ remark: h.remark + v })}
                    className="rounded-md px-1.5 py-0.5 font-mono text-[11px] ring-1 ring-inset ring-border hover:bg-muted/60"
                  >
                    {v}
                  </button>
                ))}
              </span>
            }
          >
            <Input value={h.remark} onChange={(e) => set({ remark: e.target.value })} placeholder="🇳🇱 Netherlands · {USERNAME}" />
          </Field>
          <Field label={t('hosts.address')} hint={t('hosts.addressHint')}>
            <Input value={h.address} onChange={(e) => set({ address: e.target.value })} placeholder={node?.address ?? 'vpn.example.com'} />
          </Field>
          <Field label={t('hosts.port')} hint={t('hosts.portHint')}>
            <Input type="number" min={0} max={65535} value={h.port || ''} onChange={(e) => set({ port: Number(e.target.value) || undefined })} placeholder={String(ib?.port ?? '')} />
          </Field>
        </div>

        {(tlsish || httpish) && (
          <fieldset className="rounded-card border border-border p-4">
            <legend className="px-1 text-xs font-medium text-muted-foreground">{t('hosts.advanced')}</legend>
            <div className="grid gap-4 sm:grid-cols-2">
              {tlsish && (
                <Field label="SNI" hint={security === 'reality' ? t('hosts.sniReality') : t('hosts.sniHint')}>
                  <Input value={h.sni ?? ''} onChange={(e) => set({ sni: e.target.value })} placeholder={ib?.settings.reality_server_names?.[0] ?? ''} />
                </Field>
              )}
              {httpish && (
                <>
                  <Field label="Host" hint={t('hosts.hostHint')}>
                    <Input value={h.host ?? ''} onChange={(e) => set({ host: e.target.value })} placeholder={ib?.settings.host ?? ''} />
                  </Field>
                  <Field label="Path" hint={t('hosts.pathHint')}>
                    <Input value={h.path ?? ''} onChange={(e) => set({ path: e.target.value })} placeholder={ib?.settings.path || '/'} />
                  </Field>
                </>
              )}
              {tlsish && (
                <>
                  <Field label="Fingerprint">
                    <Select value={h.fingerprint ?? ''} onChange={(e) => set({ fingerprint: e.target.value })}>
                      {FINGERPRINTS.map((f) => (
                        <option key={f} value={f}>
                          {f || t('hosts.default')}
                        </option>
                      ))}
                    </Select>
                  </Field>
                  <Field label="ALPN" hint={t('hosts.alpnHint')}>
                    <div className="flex flex-wrap gap-2">
                      {ALPNS.map((a) => {
                        const on = h.alpn?.includes(a) ?? false
                        return (
                          <button
                            type="button"
                            key={a}
                            onClick={() => set({ alpn: on ? h.alpn!.filter((x) => x !== a) : [...(h.alpn ?? []), a] })}
                            className={cx(
                              'rounded-lg px-3 py-1.5 text-xs font-medium ring-1 ring-inset transition-colors',
                              on ? 'bg-primary text-primary-foreground ring-primary' : 'text-muted-foreground ring-border hover:bg-muted/60',
                            )}
                          >
                            {a}
                          </button>
                        )
                      })}
                    </div>
                  </Field>
                  {security !== 'reality' && (
                    <div className="flex items-end pb-2 sm:col-span-2">
                      <Toggle checked={!!h.allow_insecure} onChange={(v) => set({ allow_insecure: v })} label={t('hosts.insecure')} />
                    </div>
                  )}
                </>
              )}
            </div>
          </fieldset>
        )}

        <Toggle checked={h.enabled} onChange={(v) => set({ enabled: v })} label={h.enabled ? t('common.enabled') : t('common.disabled')} />
      </form>
    </Modal>
  )
}
