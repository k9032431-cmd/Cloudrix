import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Copy, Cpu, Download, Export, Pencil, Plus, RefreshCw, Search, Sparkle as Star, Trash2 } from '../components/icons'
import { ExportDialog, ImportDialog } from './core/Template'
import { del, post, put } from '../lib/api'
import { useFetch } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import type { Core, Group, Inbound } from '../lib/types'
import { Badge, Button, Card, Confirm, ErrorNote, Field, IconButton, Input, Modal, PageHeader, Skeleton } from '../components/ui'
import { ActionsMenu } from '../components/ActionsMenu'
import { NodesTabs } from '../components/NodesTabs'
import { useToast } from '../components/toast'

// Cores lists the core configurations, as in PasarGuard: each is a set of
// inbounds, and every node runs one of them (the first is the default).
export default function Cores() {
  const { t } = useI18n()
  const toast = useToast()
  const navigate = useNavigate()
  const { data, error, reload } = useFetch<Core[]>('/api/cores')
  const [creating, setCreating] = useState(false)
  const [renaming, setRenaming] = useState<Core | null>(null)
  const [deleting, setDeleting] = useState<Core | null>(null)
  const [q, setQ] = useState('')
  const [exporting, setExporting] = useState<Core | null>(null)
  const [importing, setImporting] = useState(false)
  const { data: inbounds, reload: reloadInbounds } = useFetch<Inbound[]>('/api/inbounds')
  const { data: groups } = useFetch<Group[]>('/api/groups')
  const defaultID = data?.find((c) => c.default)?.id ?? data?.[0]?.id
  const shown = (data ?? []).filter((c) => c.name.toLowerCase().includes(q.trim().toLowerCase()))

  const makeDefault = async (c: Core) => {
    try {
      await post(`/api/cores/${c.id}/default`)
      reload()
      toast(t('cores.isDefaultNow', { name: c.name }))
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  const duplicate = async (c: Core) => {
    try {
      await post(`/api/cores/${c.id}/duplicate`)
      reload()
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  return (
    <>
      <NodesTabs value="cores" />
      <PageHeader
        title={t('nav.cores')}
        subtitle={t('cores.subtitle')}
        actions={
          <>
            <Button variant="secondary" onClick={() => setImporting(true)}>
              <Download />
              {t('tpl.importCore')}
            </Button>
            <Button onClick={() => setCreating(true)}>
              <Plus />
              {t('cores.new')}
            </Button>
          </>
        }
      />
      <ErrorNote error={error} />
      <div className="mb-3 flex gap-2">
        <div className="relative min-w-0 flex-1 sm:max-w-xs">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-faint" />
          <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('cores.search')} className="pl-8" />
        </div>
        <IconButton onClick={() => reload()} aria-label={t('common.refresh')} title={t('common.refresh')}>
          <RefreshCw className="h-4 w-4" />
        </IconButton>
      </div>
      {!data ? (
        <Card className="p-4">
          <Skeleton className="h-12 w-full" />
        </Card>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {shown.map((c) => (
            <div
              key={c.id}
              role="button"
              tabIndex={0}
              onClick={() => navigate(`/cores/${c.id}`)}
              onKeyDown={(e) => e.key === 'Enter' && navigate(`/cores/${c.id}`)}
              className="card flex cursor-pointer items-center gap-3 px-4 py-3.5 transition-colors hover:border-border-strong"
            >
              <span className="h-2.5 w-2.5 shrink-0 rounded-full bg-success" aria-hidden />
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="truncate font-medium text-foreground">{c.name}</span>
                  {c.id === defaultID && <Badge tone="gray">{t('cores.default')}</Badge>}
                </div>
                <div className="mt-0.5 text-xs text-muted-foreground">{t('cores.counts', { inbounds: c.inbounds, nodes: c.nodes })}</div>
              </div>
              <ActionsMenu
                label={t('common.actions')}
                actions={[
                  { label: t('common.edit'), icon: Pencil, onClick: () => navigate(`/cores/${c.id}`) },
                  { label: t('cores.rename'), icon: Cpu, onClick: () => setRenaming(c) },
                  { label: t('cores.makeDefault'), icon: Star, hidden: c.id === defaultID, onClick: () => makeDefault(c) },
                  { label: t('cores.duplicate'), icon: Copy, onClick: () => duplicate(c) },
                  { label: t('tpl.exportCore'), icon: Export, hidden: !c.inbounds, onClick: () => setExporting(c) },
                  { label: t('common.delete'), icon: Trash2, danger: true, hidden: c.id === defaultID, onClick: () => setDeleting(c) },
                ]}
              />
            </div>
          ))}
        </div>
      )}
      {exporting && (
        <ExportDialog inbounds={(inbounds ?? []).filter((ib) => ib.core_id === exporting.id)} coreName={exporting.name} onClose={() => setExporting(null)} />
      )}
      {importing && (
        <ImportDialog
          coreID={null}
          existing={inbounds ?? []}
          groups={groups ?? []}
          onClose={() => setImporting(false)}
          onDone={() => {
            reload()
            reloadInbounds()
          }}
        />
      )}
      {creating && (
        <CoreNameDialog
          onClose={() => setCreating(false)}
          onSaved={(c) => {
            setCreating(false)
            navigate(`/cores/${c.id}`)
          }}
        />
      )}
      {renaming && (
        <CoreNameDialog
          core={renaming}
          onClose={() => setRenaming(null)}
          onSaved={() => {
            setRenaming(null)
            reload()
          }}
        />
      )}
      <Confirm
        open={!!deleting}
        onClose={() => setDeleting(null)}
        title={t('common.delete')}
        message={t('cores.confirmDelete', { name: deleting?.name ?? '', n: deleting?.inbounds ?? 0 })}
        onConfirm={async () => {
          await del(`/api/cores/${deleting!.id}`)
          reload()
        }}
      />
    </>
  )
}

// CoreNameDialog creates a core or renames one.
export function CoreNameDialog({ core, onClose, onSaved }: { core?: Core; onClose: () => void; onSaved: (c: Core) => void }) {
  const { t } = useI18n()
  const [name, setName] = useState(core?.name ?? '')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      onSaved(core ? await put<Core>(`/api/cores/${core.id}`, { name }) : await post<Core>('/api/cores', { name }))
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
      title={core ? t('cores.rename') : t('cores.new')}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="core-form" loading={busy}>
            {core ? t('common.save') : t('common.create')}
          </Button>
        </>
      }
    >
      <form id="core-form" onSubmit={submit} className="space-y-4">
        <ErrorNote error={error} />
        <Field label={t('cores.name')}>
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Germany" required autoFocus />
        </Field>
      </form>
    </Modal>
  )
}
