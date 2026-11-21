import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Reorder, useDragControls } from 'framer-motion'
import { ChartUp, CircleAlert, GripVertical, Info, Pencil, Plus, RefreshCw, Terminal, Trash2 } from '../components/icons'
import { del, post, put } from '../lib/api'
import { useFetch } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import { formatBytes, relativeTime } from '../lib/format'
import type { Core, Node } from '../lib/types'
import { useToast } from '../components/toast'
import { ActionsMenu } from '../components/ActionsMenu'
import { Button, Card, Confirm, CopyButton, ErrorNote, Field, Input, Modal, PageHeader, Select, Skeleton, Toggle } from '../components/ui'
import { NodeStatusBadge } from '../components/StatusBadge'
import { NodesTabs } from '../components/NodesTabs'
import { NodeStatsDialog, NodesOverview, PeriodSwitch, type StatsPeriod } from './nodes/NodeStats'

interface InstallInfo {
  command: string
  panel_url: string
  plain_http: boolean
}

export default function Nodes() {
  const { t } = useI18n()
  const { data, error, reload } = useFetch<Node[]>('/api/nodes')
  const { data: cores } = useFetch<Core[]>('/api/cores')
  const [editing, setEditing] = useState<Node | 'new' | null>(null)
  const [deleting, setDeleting] = useState<Node | null>(null)
  const [installing, setInstalling] = useState<Node | null>(null)
  const [period, setPeriod] = useState<StatsPeriod>('30')
  const [stats, setStats] = useState<{ id: number; name: string } | null>(null)
  // the order the admin drags the nodes to; a poll must not undo a drag
  const [list, setList] = useState<Node[]>([])
  const dragging = useRef(false)
  const toast = useToast()
  useEffect(() => {
    if (!dragging.current) setList(data ?? [])
  }, [data])
  const saveOrder = async () => {
    dragging.current = false
    const ids = list.map((n) => n.id)
    if (ids.join() === (data ?? []).map((n) => n.id).join()) return
    try {
      await put('/api/nodes/order', { ids })
    } catch (e) {
      toast((e as Error).message, 'error')
    }
    reload()
  }

  const message = (n: Node) => {
    if (n.message === 'no contact from the node agent') return t('nodes.offline')
    // SIGKILL from outside the agent: the system ran out of memory
    const killed = n.message.match(/^(xray|sing-box) exited \(signal: killed\)/)
    if (killed) return t('nodes.killed', { core: killed[1] === 'xray' ? 'Xray' : 'sing-box' })
    // a port another program on the server already holds stops the core
    const busy = n.message.match(/listen (tcp|udp)\d? [^ ]*:(\d+): bind: address already in use/)
    if (busy) return t('nodes.portBusy', { port: busy[2], proto: busy[1].toUpperCase() })
    return n.message
  }

  return (
    <>
      <NodesTabs value="nodes" />
      <PageHeader
        title={t('nav.nodes')}
        subtitle={t('nodes.agentNote')}
        actions={
          <>
            {!!data?.length && <PeriodSwitch value={period} onChange={setPeriod} />}
            <Button onClick={() => setEditing('new')}>
              <Plus />
              {t('nodes.new')}
            </Button>
          </>
        }
      />
      <ErrorNote error={error} />
      {!!data?.length && <NodesOverview period={period} onOpen={setStats} />}
      {data?.length === 0 ? (
        <Card>
          <div className="empty">
            <span className="empty-icon">
              <Info />
            </span>
            <h3>{t('nodes.empty')}</h3>
            <Button className="mt-2" onClick={() => setEditing('new')}>
              <Plus />
              {t('nodes.new')}
            </Button>
          </div>
        </Card>
      ) : !data ? (
        <Card className="p-4">
          <Skeleton className="h-14 w-full" />
        </Card>
      ) : (
        <Reorder.Group axis="y" values={list} onReorder={setList} className="grid gap-2">
          {list.map((n) => (
            <NodeRow
              key={n.id}
              node={n}
              coreName={cores?.find((c) => c.id === n.core_id)?.name ?? '—'}
              message={message(n)}
              onDragStart={() => (dragging.current = true)}
              onDrop={saveOrder}
              onInstall={() => setInstalling(n)}
              onStats={() => setStats(n)}
              onEdit={() => setEditing(n)}
              onDelete={() => setDeleting(n)}
            />
          ))}
        </Reorder.Group>
      )}
      {editing && (
        <NodeForm
          node={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onDelete={(n) => {
            setEditing(null)
            setDeleting(n)
          }}
          onSaved={(saved, created) => {
            setEditing(null)
            reload()
            if (created) setInstalling(saved)
          }}
        />
      )}
      {stats && <NodeStatsDialog node={stats} initial={period} onClose={() => setStats(null)} />}
      {installing && <InstallDialog node={installing} onClose={() => setInstalling(null)} />}
      <Confirm
        open={!!deleting}
        onClose={() => setDeleting(null)}
        title={t('common.delete')}
        message={t('common.confirmDelete', { name: deleting?.name ?? '' })}
        onConfirm={async () => {
          await del(`/api/nodes/${deleting!.id}`)
          reload()
        }}
      />
    </>
  )
}

// statusDot colours a node the way hosts show enabled / disabled.
const statusDot: Record<Node['status'], string> = {
  connected: 'bg-success',
  connecting: 'bg-warning',
  error: 'bg-destructive',
  disabled: 'bg-muted-foreground/40',
}

// NodeRow is one node in the list, a card like a host; the grip on the left
// drags it to a new place.
function NodeRow({
  node: n,
  coreName,
  message,
  onDragStart,
  onDrop,
  onInstall,
  onStats,
  onEdit,
  onDelete,
}: {
  node: Node
  coreName: string
  message: string
  onDragStart: () => void
  onDrop: () => void
  onInstall: () => void
  onStats: () => void
  onEdit: () => void
  onDelete: () => void
}) {
  const { t, lang } = useI18n()
  const drag = useDragControls()
  return (
    <Reorder.Item value={n} dragListener={false} dragControls={drag} onDragEnd={onDrop} className="card flex items-center gap-2 px-2 py-2.5 sm:gap-3 sm:px-3">
      <button
        type="button"
        className="flex h-8 w-6 shrink-0 cursor-grab touch-none items-center justify-center rounded text-faint hover:text-foreground active:cursor-grabbing"
        onPointerDown={(e) => {
          onDragStart()
          drag.start(e)
        }}
        aria-label={t('nodes.drag')}
        title={t('nodes.drag')}
      >
        <GripVertical className="h-4 w-4" />
      </button>
      <span className={`h-2.5 w-2.5 shrink-0 rounded-full ${statusDot[n.status]}`} title={t(`nodes.status.${n.status}`)} />
      <button type="button" onClick={onStats} className="min-w-0 flex-1 text-left">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate font-medium text-foreground">{n.name}</span>
          <span className="hide-sm shrink-0">
            <NodeStatusBadge status={n.status} />
          </span>
        </div>
        <div className="mt-0.5 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
          <span className="truncate font-mono">{n.address}</span>
          <span className="truncate">
            {t('nodes.core')}: <span className="text-foreground">{coreName}</span>
          </span>
          <span className="tabular-nums">
            {formatBytes(n.upload_bytes + n.download_bytes)}
            {n.usage_coefficient !== 1 && <span className="ml-1 text-faint">×{n.usage_coefficient}</span>}
          </span>
          <span className="hide-sm">{relativeTime(n.last_seen, lang)}</span>
          {n.core_version && <span className="hide-md text-faint">{n.core_version}</span>}
        </div>
        {n.message && <NodeError text={message} raw={n.message} />}
      </button>
      <Button
        variant={n.status === 'connecting' ? 'primary' : 'secondary'}
        size="sm"
        className="shrink-0 max-sm:w-8 max-sm:px-0"
        onClick={onInstall}
        aria-label={t('nodes.install')}
        title={t('nodes.install')}
      >
        <Terminal />
        <span className="hide-sm">{t('nodes.install')}</span>
      </Button>
      <ActionsMenu
        label={t('common.actions')}
        actions={[
          { label: t('nodes.stats.open'), icon: ChartUp, onClick: onStats },
          { label: t('common.edit'), icon: Pencil, onClick: onEdit },
          { label: t('common.delete'), icon: Trash2, danger: true, onClick: onDelete },
        ]}
      />
    </Reorder.Item>
  )
}

// InstallDialog shows the one-line command that sets the node server up.
function InstallDialog({ node, onClose }: { node: Node; onClose: () => void }) {
  const { t } = useI18n()
  const toast = useToast()
  const { data, error, setData } = useFetch<InstallInfo>(`/api/nodes/${node.id}/install`)
  const [rotating, setRotating] = useState(false)
  return (
    <>
      <Modal open onClose={onClose} wide title={t('nodes.installTitle', { name: node.name })}>
        <ErrorNote error={error} />
        <ol className="grid gap-4 text-[13.5px] text-foreground">
          <Step n={1}>{t('nodes.installStep1')}</Step>
          <Step n={2}>
            {t('nodes.installStep2')}
            <div className="mt-2.5 rounded-lg bg-muted p-3 ring-1 ring-inset ring-border">
              {data ? <code className="block break-all font-mono text-[12.5px] leading-relaxed text-foreground">{data.command}</code> : <Skeleton className="h-10 w-full" />}
            </div>
            <div className="mt-2 flex flex-wrap items-center gap-2">
              {data && <CopyButton text={data.command} />}
              <Button variant="ghost" size="sm" onClick={() => setRotating(true)}>
                <RefreshCw />
                {t('nodes.rotate')}
              </Button>
              <span className="hint">{t('nodes.installSecret')}</span>
            </div>
          </Step>
          <Step n={3}>{t('nodes.installStep3')}</Step>
        </ol>
        {data?.plain_http && (
          <p className="mt-4 flex gap-2 rounded-lg bg-warning/10 px-3 py-2.5 text-[13px] text-foreground ring-1 ring-inset ring-warning/20">
            <Info className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
            {t('nodes.plainHttp')}
          </p>
        )}
      </Modal>
      <Confirm
        open={rotating}
        onClose={() => setRotating(false)}
        title={t('nodes.rotate')}
        message={t('nodes.rotateConfirm')}
        onConfirm={async () => {
          setData(await post<InstallInfo>(`/api/nodes/${node.id}/rotate`))
          toast(t('toast.saved'))
        }}
      />
    </>
  )
}

function Step({ n, children }: { n: number; children: React.ReactNode }) {
  return (
    <li className="grid grid-cols-[24px_1fr] gap-3">
      <span className="flex h-6 w-6 items-center justify-center rounded-full bg-secondary text-xs font-semibold text-secondary-foreground ring-1 ring-inset ring-border">{n}</span>
      <div className="min-w-0 pt-0.5">{children}</div>
    </li>
  )
}

function NodeForm({
  node,
  onClose,
  onDelete,
  onSaved,
}: {
  node: Node | null
  onClose: () => void
  onDelete: (n: Node) => void
  onSaved: (n: Node, created: boolean) => void
}) {
  const { t } = useI18n()
  const { data: cores } = useFetch<Core[]>('/api/cores')
  const [form, setForm] = useState({
    name: node?.name ?? '',
    address: node?.address ?? '',
    core_id: node?.core_id ?? 0,
    usage_coefficient: node?.usage_coefficient ?? 1,
    bridge_port: node?.bridge_port ?? 0,
    disabled: node?.status === 'disabled',
  })
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const body = { ...form, address: form.address.trim().replace(/^https?:\/\//, '').replace(/[/:].*$/, '') }
      if (node) onSaved(await put<Node>(`/api/nodes/${node.id}`, body), false)
      else onSaved(await post<Node>('/api/nodes', body), true)
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
      title={node ? `${t('common.edit')}: ${node.name}` : t('nodes.new')}
      footer={
        <>
          {node && (
            <Button variant="danger" className="mr-auto" onClick={() => onDelete(node)}>
              <Trash2 />
              {t('common.delete')}
            </Button>
          )}
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="node-form" loading={busy}>
            {node ? t('common.save') : t('common.create')}
          </Button>
        </>
      }
    >
      <form id="node-form" onSubmit={submit} className="grid gap-4 sm:grid-cols-2">
        <div className="sm:col-span-2">
          <ErrorNote error={error} />
        </div>
        <Field label={t('nodes.name')}>
          <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Netherlands" required autoFocus />
        </Field>
        <Field label={t('nodes.address')} hint={t('nodes.addressHint')}>
          <Input value={form.address} onChange={(e) => setForm({ ...form, address: e.target.value })} placeholder="46.8.103.95" required />
        </Field>
        <Field label={t('nodes.coreConfig')} hint={t('nodes.coreConfigHint')}>
          <Select value={form.core_id || cores?.[0]?.id || ''} onChange={(e) => setForm({ ...form, core_id: Number(e.target.value) })} required>
            {(cores ?? []).map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </Select>
        </Field>
        <Field label={t('nodes.coefficient')}>
          <Input type="number" min={0.1} max={100} step={0.1} value={form.usage_coefficient} onChange={(e) => setForm({ ...form, usage_coefficient: Number(e.target.value) })} />
        </Field>
        <Field label={t('nodes.bridgePort')} hint={t('nodes.bridgePortHint')}>
          <Input type="number" min={0} max={65535} value={form.bridge_port} onChange={(e) => setForm({ ...form, bridge_port: Number(e.target.value) })} />
        </Field>
        <div className="flex items-end pb-1.5">
          <Toggle checked={form.disabled} onChange={(v) => setForm({ ...form, disabled: v })} label={t('nodes.disabled')} />
        </div>
      </form>
    </Modal>
  )
}

// NodeError shows a node's problem in three lines; a tap shows all of it.
function NodeError({ text, raw }: { text: string; raw: string }) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  return (
    <button
      type="button"
      onClick={() => setOpen(!open)}
      className="mt-1 flex max-w-[170px] items-start gap-1 text-left text-xs text-destructive sm:max-w-[340px]"
      title={open ? t('nodes.errorLess') : t('nodes.errorMore')}
    >
      <CircleAlert className="mt-px h-3.5 w-3.5 shrink-0" />
      <span className={(open ? '' : 'line-clamp-3 ') + 'whitespace-normal break-words'}>
        {text}
        {open && text !== raw && <span className="mt-1 block font-mono text-[11px] opacity-80">{raw}</span>}
      </span>
    </button>
  )
}
