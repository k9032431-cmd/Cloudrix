import { useEffect, useState, type FormEvent, type KeyboardEvent, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { Reorder, useDragControls } from 'framer-motion'
import { ChevronDown, Copy, Cpu, Globe, GripVertical, Pencil, Plus, Power, RefreshCw, Search, Trash2, X } from '../components/icons'
import { del, post, put } from '../lib/api'
import { useFetch } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import type { Core, Host, Inbound, Node, UserStatus } from '../lib/types'
import { Button, Card, Confirm, ErrorNote, Field, IconButton, Input, Modal, PageHeader, Select, Skeleton, Toggle, cx } from '../components/ui'
import { ActionsMenu } from '../components/ActionsMenu'
import { useToast } from '../components/toast'

const FINGERPRINTS = ['', 'chrome', 'firefox', 'safari', 'ios', 'android', 'edge', '360', 'qq', 'random', 'randomized']
const ALPNS = ['h3', 'h2', 'http/1.1']
const STATUSES: UserStatus[] = ['active', 'disabled', 'limited', 'expired', 'on_hold']
const VARIABLES = ['{USERNAME}', '{DATA_USED}', '{DATA_LEFT}', '{DATA_LIMIT}', '{DAYS_LEFT}', '{EXPIRE_DATE}', '{PROTOCOL}']

// Hosts are the links users get, as in PasarGuard: each points at an inbound
// of a core and sets its own name, addresses and TLS fields. Drag them to
// change the order of links in subscriptions.
export default function Hosts() {
  const { t } = useI18n()
  const toast = useToast()
  const { data, error, reload } = useFetch<Host[]>('/api/hosts')
  const { data: inbounds } = useFetch<Inbound[]>('/api/inbounds')
  const { data: cores } = useFetch<Core[]>('/api/cores')
  const { data: nodes } = useFetch<Node[]>('/api/nodes')
  const [list, setList] = useState<Host[]>([])
  const [editing, setEditing] = useState<Host | null>(null)
  const [deleting, setDeleting] = useState<Host | null>(null)
  const [q, setQ] = useState('')
  useEffect(() => setList(data ?? []), [data])

  const inboundOf = (h: Host) => inbounds?.find((ib) => ib.id === h.inbound_id)
  const blank = (): Host => ({ id: 0, inbound_id: inbounds?.[0]?.id ?? 0, priority: 0, enabled: true, remark: '', address: [] })
  const query = q.trim().toLowerCase()
  const shown = query
    ? list.filter((h) => [h.remark, ...h.address, inboundOf(h)?.tag ?? ''].some((v) => v.toLowerCase().includes(query)))
    : list

  const saveOrder = async () => {
    const ids = list.map((h) => h.id)
    if (ids.join() === (data ?? []).map((h) => h.id).join()) return
    try {
      await put('/api/hosts/order', { ids })
    } catch (e) {
      toast((e as Error).message, 'error')
    }
    reload()
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
      {!!list.length && (
        <div className="mb-3 flex gap-2">
          <div className="relative min-w-0 flex-1 sm:max-w-xs">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-faint" />
            <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('hosts.search')} className="pl-8" />
          </div>
          <IconButton onClick={() => reload()} aria-label={t('common.refresh')} title={t('common.refresh')}>
            <RefreshCw className="h-4 w-4" />
          </IconButton>
        </div>
      )}
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
            {inbounds?.length ? (
              <Button className="mt-2" onClick={() => setEditing(blank())}>
                <Plus />
                {t('hosts.new')}
              </Button>
            ) : (
              <Link to="/cores" className="btn btn-primary mt-2">
                <Cpu />
                {t('nav.cores')}
              </Link>
            )}
          </div>
        </Card>
      ) : (
        <Reorder.Group axis="y" values={list} onReorder={setList} className="grid gap-2">
          {shown.map((h) => (
            <HostRow
              key={h.id}
              host={h}
              inbound={inboundOf(h)}
              core={cores?.find((c) => c.id === inboundOf(h)?.core_id)}
              nodes={nodes ?? []}
              draggable={!query}
              onDrop={saveOrder}
              onEdit={() => setEditing(h)}
              onCopy={() => setEditing({ ...h, id: 0, remark: `${h.remark} (copy)` })}
              onToggle={() => toggle(h)}
              onDelete={() => setDeleting(h)}
            />
          ))}
        </Reorder.Group>
      )}
      {editing && (
        <HostForm
          initial={editing}
          inbounds={inbounds ?? []}
          cores={cores ?? []}
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
        message={t('common.confirmDelete', { name: deleting?.remark || deleting?.address[0] || '' })}
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
  core,
  nodes,
  draggable,
  onDrop,
  onEdit,
  onCopy,
  onToggle,
  onDelete,
}: {
  host: Host
  inbound?: Inbound
  core?: Core
  nodes: Node[]
  draggable: boolean
  onDrop: () => void
  onEdit: () => void
  onCopy: () => void
  onToggle: () => void
  onDelete: () => void
}) {
  const { t } = useI18n()
  const drag = useDragControls()
  // no address of its own: the addresses of the core's nodes
  const fallback = nodes.filter((n) => n.core_id === inbound?.core_id).map((n) => n.address)
  const address = h.address[0] ?? fallback[0] ?? '—'
  const more = (h.address.length || fallback.length) - 1
  const port = h.port || inbound?.port
  return (
    <Reorder.Item value={h} dragListener={false} dragControls={drag} onDragEnd={onDrop} className="card flex items-center gap-2 px-2 py-2.5 sm:gap-3 sm:px-3">
      <button
        type="button"
        className={cx('flex h-8 w-6 shrink-0 touch-none items-center justify-center rounded text-faint', draggable ? 'cursor-grab hover:text-foreground active:cursor-grabbing' : 'opacity-30')}
        onPointerDown={(e) => draggable && drag.start(e)}
        aria-label={t('hosts.drag')}
        title={t('hosts.drag')}
      >
        <GripVertical className="h-4 w-4" />
      </button>
      <span className={cx('h-2.5 w-2.5 shrink-0 rounded-full', h.enabled ? 'bg-success' : 'bg-destructive')} title={h.enabled ? t('common.enabled') : t('common.disabled')} />
      <button type="button" onClick={onEdit} className={cx('min-w-0 flex-1 text-left', !h.enabled && 'opacity-60')}>
        <div className="truncate font-medium text-foreground">{h.remark || inbound?.tag || '—'}</div>
        <div className="mt-0.5 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
          <span className="truncate font-mono">
            {address}
            {port ? `:${port}` : ''}
            {more > 0 && ` +${more}`}
          </span>
          <span className="truncate">
            {t('hosts.inboundOf')}: <span className="text-foreground">{inbound?.tag ?? '—'}</span>
            {core && <span className="hide-sm"> · {core.name}</span>}
          </span>
        </div>
      </button>
      <ActionsMenu
        label={t('common.actions')}
        actions={[
          { label: t('common.edit'), icon: Pencil, onClick: onEdit },
          { label: t('hosts.copy'), icon: Copy, onClick: onCopy },
          { label: h.enabled ? t('hosts.disable') : t('hosts.enable'), icon: Power, onClick: onToggle },
          { label: t('common.delete'), icon: Trash2, danger: true, onClick: onDelete },
        ]}
      />
    </Reorder.Item>
  )
}

// TagsInput holds several values: Enter, comma or leaving the field adds one.
function TagsInput({ value, onChange, placeholder }: { value: string[]; onChange: (v: string[]) => void; placeholder?: string }) {
  const [draft, setDraft] = useState('')
  const add = () => {
    const parts = draft
      .split(',')
      .map((x) => x.trim())
      .filter((x) => x && !value.includes(x))
    if (parts.length) onChange([...value, ...parts])
    setDraft('')
  }
  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault()
      add()
    } else if (e.key === 'Backspace' && !draft && value.length) {
      onChange(value.slice(0, -1))
    }
  }
  return (
    <div className="input flex !h-auto min-h-[34px] flex-wrap items-center gap-1.5 !py-1 focus-within:border-ring" onClick={(e) => (e.currentTarget.querySelector('input') as HTMLInputElement)?.focus()}>
      {value.map((v) => (
        <span key={v} className="inline-flex max-w-full items-center gap-1 rounded-md bg-muted px-2 py-0.5 text-xs text-foreground ring-1 ring-inset ring-border">
          <span className="truncate font-mono">{v}</span>
          <button type="button" onClick={() => onChange(value.filter((x) => x !== v))} className="text-faint hover:text-foreground" aria-label={`× ${v}`}>
            <X className="h-3 w-3" />
          </button>
        </span>
      ))}
      <input
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={onKey}
        onBlur={add}
        placeholder={value.length ? '' : placeholder}
        className="min-w-[120px] flex-1 bg-transparent text-[13.5px] outline-none placeholder:text-faint"
      />
    </div>
  )
}

function Accordion({ title, open, onToggle, children }: { title: string; open: boolean; onToggle: () => void; children: ReactNode }) {
  return (
    <div className="rounded-card border border-border">
      <button type="button" onClick={onToggle} aria-expanded={open} className="flex w-full items-center justify-between px-4 py-3 text-left text-sm font-medium text-foreground">
        {title}
        <ChevronDown className={cx('h-4 w-4 text-muted-foreground transition-transform', open && 'rotate-180')} />
      </button>
      {open && <div className="grid gap-4 border-t border-border p-4 sm:grid-cols-2">{children}</div>}
    </div>
  )
}

function Chips<T extends string>({ options, value, onChange, label }: { options: T[]; value: T[]; onChange: (v: T[]) => void; label: (v: T) => string }) {
  return (
    <div className="flex flex-wrap gap-2">
      {options.map((o) => {
        const on = value.includes(o)
        return (
          <button
            type="button"
            key={o}
            aria-pressed={on}
            onClick={() => onChange(on ? value.filter((x) => x !== o) : [...value, o])}
            className={cx(
              'rounded-lg px-3 py-1.5 text-xs font-medium ring-1 ring-inset transition-colors',
              on ? 'bg-primary text-primary-foreground ring-primary' : 'text-muted-foreground ring-border hover:bg-muted/60',
            )}
          >
            {label(o)}
          </button>
        )
      })}
    </div>
  )
}

function HostForm({ initial, inbounds, cores, onClose, onSaved }: { initial: Host; inbounds: Inbound[]; cores: Core[]; onClose: () => void; onSaved: () => void }) {
  const { t } = useI18n()
  const [h, setH] = useState<Host>(initial)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [section, setSection] = useState<'' | 'network' | 'security'>('')
  const set = (patch: Partial<Host>) => setH((x) => ({ ...x, ...patch }))
  const isNew = h.id === 0
  const ib = inbounds.find((x) => x.id === h.inbound_id)
  const coreName = (id?: number) => cores.find((c) => c.id === id)?.name ?? '—'

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
      title={isNew ? t('hosts.new') : t('hosts.edit')}
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
      <form id="host-form" onSubmit={submit} className="space-y-4">
        <ErrorNote error={error} />
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t('hosts.inbound')}>
            <Select value={h.inbound_id || ''} onChange={(e) => set({ inbound_id: Number(e.target.value) })} required>
              {inbounds.map((x) => (
                <option key={x.id} value={x.id}>
                  {x.tag} · {x.protocol} · {coreName(x.core_id)}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={t('hosts.status')} hint={t('hosts.statusHint')}>
            <Chips<UserStatus> options={STATUSES} value={h.status ?? []} onChange={(v) => set({ status: v })} label={(v) => t(`status.${v}`)} />
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
            <Input value={h.remark} onChange={(e) => set({ remark: e.target.value })} placeholder="🇳🇱 Netherlands · {USERNAME}" required />
          </Field>
          <Field label={t('hosts.address')} hint={t('hosts.addressHint')}>
            <TagsInput value={h.address} onChange={(v) => set({ address: v })} placeholder="nl.example.com" />
          </Field>
          <Field label={t('hosts.port')} hint={t('hosts.portHint')}>
            <Input type="number" min={0} max={65535} value={h.port || ''} onChange={(e) => set({ port: Number(e.target.value) || undefined })} placeholder={String(ib?.port ?? '')} />
          </Field>
        </div>

        <Accordion title={t('hosts.network')} open={section === 'network'} onToggle={() => setSection(section === 'network' ? '' : 'network')}>
          <Field label="Host" hint={t('hosts.hostHint')}>
            <TagsInput value={h.host ?? []} onChange={(v) => set({ host: v })} placeholder={ib?.settings.host || 'cdn.example.com'} />
          </Field>
          <Field label="Path" hint={t('hosts.pathHint')}>
            <Input value={h.path ?? ''} onChange={(e) => set({ path: e.target.value })} placeholder={ib?.settings.path || '/'} />
          </Field>
        </Accordion>

        <Accordion title={t('hosts.securitySection')} open={section === 'security'} onToggle={() => setSection(section === 'security' ? '' : 'security')}>
          <Field label={t('hosts.security')} hint={t('hosts.securityHint')}>
            <Select value={h.security ?? ''} onChange={(e) => set({ security: e.target.value as Host['security'] })}>
              <option value="">{t('hosts.securityDefault')}</option>
              <option value="tls">TLS</option>
              <option value="none">None</option>
            </Select>
          </Field>
          <Field label="SNI" hint={ib?.settings.security === 'reality' ? t('hosts.sniReality') : t('hosts.sniHint')}>
            <TagsInput value={h.sni ?? []} onChange={(v) => set({ sni: v })} placeholder={ib?.settings.reality_server_names?.[0] ?? 'example.com'} />
          </Field>
          <Field label="ALPN" hint={t('hosts.alpnHint')}>
            <Chips<string> options={ALPNS} value={h.alpn ?? []} onChange={(v) => set({ alpn: v })} label={(v) => v} />
          </Field>
          <Field label={t('hosts.fingerprint')}>
            <Select value={h.fingerprint ?? ''} onChange={(e) => set({ fingerprint: e.target.value })}>
              {FINGERPRINTS.map((f) => (
                <option key={f} value={f}>
                  {f || t('hosts.default')}
                </option>
              ))}
            </Select>
          </Field>
          <Toggle checked={!!h.allow_insecure} onChange={(v) => set({ allow_insecure: v })} label={t('hosts.insecure')} />
          <Toggle checked={!!h.use_sni_as_host} onChange={(v) => set({ use_sni_as_host: v })} label={t('hosts.sniAsHost')} />
        </Accordion>
      </form>
    </Modal>
  )
}
