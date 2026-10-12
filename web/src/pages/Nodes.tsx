import { useEffect, useState, type FormEvent } from 'react'
import { CircleAlert, Info, Pencil, Plus, RefreshCw, Terminal, Trash2 } from '../components/icons'
import { del, post, put } from '../lib/api'
import { useFetch } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import { formatBytes, relativeTime } from '../lib/format'
import type { Node } from '../lib/types'
import { useToast } from '../components/toast'
import { Button, Card, Confirm, CopyButton, ErrorNote, Field, IconButton, Input, Modal, PageHeader, Skeleton, Table, Td, Th, Toggle } from '../components/ui'
import { NodeStatusBadge } from '../components/StatusBadge'

interface InstallInfo {
  command: string
  panel_url: string
  plain_http: boolean
}

export default function Nodes() {
  const { t, lang } = useI18n()
  const { data, error, reload } = useFetch<Node[]>('/api/nodes')
  const [editing, setEditing] = useState<Node | 'new' | null>(null)
  const [deleting, setDeleting] = useState<Node | null>(null)
  const [installing, setInstalling] = useState<Node | null>(null)

  // the agent reports in every ~25s; keep the list fresh while the page is open
  useEffect(() => {
    const id = window.setInterval(() => reload(), 15_000)
    return () => window.clearInterval(id)
  }, [reload])

  const message = (n: Node) => (n.message === 'no contact from the node agent' ? t('nodes.offline') : n.message)

  return (
    <>
      <PageHeader
        title={t('nav.nodes')}
        subtitle={t('nodes.agentNote')}
        actions={
          <Button onClick={() => setEditing('new')}>
            <Plus />
            {t('nodes.new')}
          </Button>
        }
      />
      <ErrorNote error={error} />
      <Card>
        {data?.length === 0 ? (
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
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>{t('nodes.name')}</Th>
                <Th>{t('nodes.status')}</Th>
                <Th className="hide-sm">{t('nodes.cores')}</Th>
                <Th className="hide-md r">{t('nodes.traffic')}</Th>
                <Th className="hide-sm">{t('nodes.lastSeen')}</Th>
                <Th className="sm:w-36" />
              </tr>
            </thead>
            <tbody>
              {!data && (
                <tr>
                  <td colSpan={6}>
                    <Skeleton className="h-5 w-full" />
                  </td>
                </tr>
              )}
              {data?.map((n) => (
                <tr key={n.id}>
                  <Td>
                    <div className="ent-name">{n.name}</div>
                    <div className="ent-sub font-mono">{n.address}</div>
                  </Td>
                  <Td className="max-sm:[&_.badge]:whitespace-normal max-sm:[&_.badge]:text-left">
                    <NodeStatusBadge status={n.status} />
                    {n.message && (
                      <div className="mt-1 flex max-w-[130px] items-center gap-1 truncate text-xs text-destructive sm:max-w-[260px]" title={n.message}>
                        <CircleAlert className="h-3.5 w-3.5 shrink-0" />
                        <span className="truncate">{message(n)}</span>
                      </div>
                    )}
                  </Td>
                  <Td className="hide-sm text-xs text-muted-foreground">{n.core_version || '—'}</Td>
                  <Td className="hide-md r whitespace-nowrap tabular-nums text-muted-foreground">
                    {formatBytes(n.upload_bytes + n.download_bytes)}
                    {n.usage_coefficient !== 1 && <span className="ml-1 text-faint">×{n.usage_coefficient}</span>}
                  </Td>
                  <Td className="hide-sm whitespace-nowrap text-muted-foreground">{relativeTime(n.last_seen, lang)}</Td>
                  <Td>
                    <div className="flex justify-end gap-0.5 sm:gap-1">
                      <Button variant={n.status === 'connecting' ? 'primary' : 'secondary'} size="sm" className="max-sm:w-7 max-sm:px-0" onClick={() => setInstalling(n)} aria-label={t('nodes.install')}>
                        <Terminal />
                        <span className="hide-sm">{t('nodes.install')}</span>
                      </Button>
                      <IconButton onClick={() => setEditing(n)} aria-label={t('common.edit')}>
                        <Pencil />
                      </IconButton>
                      <IconButton onClick={() => setDeleting(n)} aria-label={t('common.delete')}>
                        <Trash2 />
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
        <NodeForm
          node={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={(saved, created) => {
            setEditing(null)
            reload()
            if (created) setInstalling(saved)
          }}
        />
      )}
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

function NodeForm({ node, onClose, onSaved }: { node: Node | null; onClose: () => void; onSaved: (n: Node, created: boolean) => void }) {
  const { t } = useI18n()
  const [form, setForm] = useState({
    name: node?.name ?? '',
    address: node?.address ?? '',
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
