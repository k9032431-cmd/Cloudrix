import { useState, type FormEvent } from 'react'
import { Info, Pencil, Plus, Trash2 } from '../components/icons'
import { del, post, put } from '../lib/api'
import { useFetch } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import { formatBytes, relativeTime } from '../lib/format'
import type { Node } from '../lib/types'
import { Button, Card, Confirm, Empty, ErrorNote, Field, IconButton, Input, Modal, PageHeader, Table, Td, Th, Toggle } from '../components/ui'
import { NodeStatusBadge } from '../components/StatusBadge'

export default function Nodes() {
  const { t, lang } = useI18n()
  const { data, error, reload } = useFetch<Node[]>('/api/nodes')
  const [editing, setEditing] = useState<Node | 'new' | null>(null)
  const [deleting, setDeleting] = useState<Node | null>(null)

  return (
    <>
      <PageHeader
        title={t('nav.nodes')}
        actions={
          <Button onClick={() => setEditing('new')}>
            <Plus className="h-4 w-4" />
            {t('nodes.new')}
          </Button>
        }
      />
      <div className="mb-4 flex gap-2 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-sm text-slate-700 dark:border-slate-800 dark:bg-slate-900/60 dark:text-slate-300">
        <Info className="mt-0.5 h-4 w-4 shrink-0" />
        {t('nodes.agentNote')}
      </div>
      <ErrorNote error={error} />
      <Card>
        {data?.length === 0 ? (
          <Empty />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>{t('nodes.name')}</Th>
                <Th>{t('nodes.address')}</Th>
                <Th>{t('nodes.status')}</Th>
                <Th>{t('nodes.coefficient')}</Th>
                <Th>{t('dash.traffic')}</Th>
                <Th>{t('nodes.lastSeen')}</Th>
                <Th className="w-24" />
              </tr>
            </thead>
            <tbody>
              {data?.map((n) => (
                <tr key={n.id}>
                  <Td className="font-medium text-slate-900 dark:text-white">{n.name}</Td>
                  <Td className="font-mono text-xs text-slate-500">
                    {n.address}:{n.api_port}
                  </Td>
                  <Td>
                    <NodeStatusBadge status={n.status} />
                    {n.message && <div className="mt-1 max-w-[200px] truncate text-xs text-rose-500">{n.message}</div>}
                  </Td>
                  <Td className="tabular-nums">×{n.usage_coefficient}</Td>
                  <Td className="tabular-nums text-slate-500">{formatBytes(n.upload_bytes + n.download_bytes)}</Td>
                  <Td className="text-slate-500">{relativeTime(n.last_seen, lang)}</Td>
                  <Td>
                    <div className="flex justify-end gap-1">
                      <IconButton onClick={() => setEditing(n)} aria-label={t('common.edit')}>
                        <Pencil className="h-4 w-4" />
                      </IconButton>
                      <IconButton onClick={() => setDeleting(n)} aria-label={t('common.delete')}>
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
        <NodeForm
          node={editing === 'new' ? null : editing}
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
        message={t('common.confirmDelete', { name: deleting?.name ?? '' })}
        onConfirm={async () => {
          await del(`/api/nodes/${deleting!.id}`)
          reload()
        }}
      />
    </>
  )
}

function NodeForm({ node, onClose, onSaved }: { node: Node | null; onClose: () => void; onSaved: () => void }) {
  const { t } = useI18n()
  const [form, setForm] = useState({
    name: node?.name ?? '',
    address: node?.address ?? '',
    api_port: node?.api_port ?? 62050,
    usage_coefficient: node?.usage_coefficient ?? 1,
    disabled: node?.status === 'disabled',
  })
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      if (node) await put(`/api/nodes/${node.id}`, form)
      else await post('/api/nodes', form)
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
          <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
        </Field>
        <Field label={t('nodes.address')}>
          <Input value={form.address} onChange={(e) => setForm({ ...form, address: e.target.value })} placeholder="de1.example.com" required />
        </Field>
        <Field label={t('nodes.apiPort')}>
          <Input type="number" min={1} max={65535} value={form.api_port} onChange={(e) => setForm({ ...form, api_port: Number(e.target.value) })} />
        </Field>
        <Field label={t('nodes.coefficient')}>
          <Input type="number" min={0.1} step={0.1} value={form.usage_coefficient} onChange={(e) => setForm({ ...form, usage_coefficient: Number(e.target.value) })} />
        </Field>
        <div className="sm:col-span-2">
          <Toggle checked={form.disabled} onChange={(v) => setForm({ ...form, disabled: v })} label={t('nodes.disabled')} />
        </div>
      </form>
    </Modal>
  )
}
