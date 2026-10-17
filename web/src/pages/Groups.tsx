import { useState, type FormEvent } from 'react'
import { Layers, Pencil, Plus, Trash2 } from '../components/icons'
import { del, post, put } from '../lib/api'
import { useFetch } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import type { Core, Group, Inbound } from '../lib/types'
import { Badge, Button, Card, Confirm, ErrorNote, Field, IconButton, Input, Modal, PageHeader, Skeleton, Table, Td, Th, Toggle, cx } from '../components/ui'

// Groups bundle inbounds: a user picks groups and gets every inbound inside,
// so an inbound added to a group shows up in its users' apps on the next refresh.
export default function Groups() {
  const { t } = useI18n()
  const { data, error, reload } = useFetch<Group[]>('/api/groups')
  const { data: inbounds } = useFetch<Inbound[]>('/api/inbounds')
  const [editing, setEditing] = useState<Group | 'new' | null>(null)
  const [deleting, setDeleting] = useState<Group | null>(null)
  const byID = new Map((inbounds ?? []).map((ib) => [ib.id, ib]))

  return (
    <>
      <PageHeader
        title={t('nav.groups')}
        subtitle={t('groups.subtitle')}
        actions={
          <Button onClick={() => setEditing('new')}>
            <Plus />
            {t('groups.new')}
          </Button>
        }
      />
      <ErrorNote error={error} />
      <Card>
        {data?.length === 0 ? (
          <div className="empty">
            <span className="empty-icon">
              <Layers />
            </span>
            <h3>{t('groups.empty')}</h3>
            <Button className="mt-2" onClick={() => setEditing('new')}>
              <Plus />
              {t('groups.new')}
            </Button>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>{t('groups.name')}</Th>
                <Th>{t('nav.inbounds')}</Th>
                <Th className="hide-sm r">{t('nav.users')}</Th>
                <Th className="w-24" />
              </tr>
            </thead>
            <tbody>
              {!data && (
                <tr>
                  <td colSpan={4}>
                    <Skeleton className="h-5 w-full" />
                  </td>
                </tr>
              )}
              {data?.map((g) => (
                <tr key={g.id}>
                  <Td>
                    <div className="ent-name flex min-w-0 flex-wrap items-center gap-2">
                      <span className="min-w-0 truncate">{g.name}</span>
                      {g.disabled && <Badge tone="gray">{t('common.disabled')}</Badge>}
                    </div>
                  </Td>
                  <Td>
                    <div className="flex flex-wrap gap-1.5">
                      {g.inbound_ids.length === 0 && <span className="text-xs text-muted-foreground">{t('groups.noInbounds')}</span>}
                      {g.inbound_ids.map((id) => {
                        const ib = byID.get(id)
                        return ib ? (
                          <Badge key={id} tone={ib.enabled ? 'blue' : 'gray'}>
                            {ib.tag}
                          </Badge>
                        ) : null
                      })}
                    </div>
                  </Td>
                  <Td className="hide-sm r tabular-nums text-muted-foreground">{g.users}</Td>
                  <Td>
                    <div className="flex justify-end gap-1">
                      <IconButton onClick={() => setEditing(g)} aria-label={t('common.edit')}>
                        <Pencil />
                      </IconButton>
                      <IconButton onClick={() => setDeleting(g)} aria-label={t('common.delete')}>
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
        <GroupForm
          group={editing === 'new' ? null : editing}
          inbounds={inbounds ?? []}
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
        message={deleting && deleting.users > 0 ? t('groups.deleteHasUsers', { n: String(deleting.users) }) : t('common.confirmDelete', { name: deleting?.name ?? '' })}
        onConfirm={async () => {
          await del(`/api/groups/${deleting!.id}`)
          reload()
        }}
      />
    </>
  )
}

function GroupForm({ group, inbounds, onClose, onSaved }: { group: Group | null; inbounds: Inbound[]; onClose: () => void; onSaved: () => void }) {
  const { t } = useI18n()
  const [form, setForm] = useState({ name: group?.name ?? '', inbound_ids: group?.inbound_ids ?? [], disabled: group?.disabled ?? false })
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const { data: cores } = useFetch<Core[]>('/api/cores')
  const coreName = (id: number) => cores?.find((c) => c.id === id)?.name

  const toggle = (id: number) =>
    setForm((f) => ({ ...f, inbound_ids: f.inbound_ids.includes(id) ? f.inbound_ids.filter((x) => x !== id) : [...f.inbound_ids, id] }))

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      if (group) await put(`/api/groups/${group.id}`, form)
      else await post('/api/groups', form)
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
      title={group ? `${t('common.edit')}: ${group.name}` : t('groups.new')}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="group-form" loading={busy}>
            {group ? t('common.save') : t('common.create')}
          </Button>
        </>
      }
    >
      <form id="group-form" onSubmit={submit} className="grid gap-4">
        <ErrorNote error={error} />
        <Field label={t('groups.name')}>
          <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Default" required autoFocus maxLength={64} />
        </Field>
        <Field label={t('nav.inbounds')} hint={t('groups.inboundsHint')}>
          {inbounds.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('groups.noInboundsYet')}</p>
          ) : (
            <div className="grid gap-1.5">
              {inbounds.map((ib) => {
                const on = form.inbound_ids.includes(ib.id)
                return (
                  <button
                    type="button"
                    key={ib.id}
                    onClick={() => toggle(ib.id)}
                    className={cx(
                      'flex items-center gap-3 rounded-lg px-3 py-2 text-left text-[13px] ring-1 ring-inset transition-colors',
                      on ? 'bg-primary/10 text-foreground ring-primary/40' : 'text-muted-foreground ring-border hover:bg-muted/60',
                    )}
                  >
                    <span className={cx('flex h-4 w-4 shrink-0 items-center justify-center rounded border text-[10px]', on ? 'border-primary bg-primary text-primary-foreground' : 'border-border-strong')}>
                      {on && '✓'}
                    </span>
                    <span className="min-w-0 flex-1 truncate font-medium text-foreground">{ib.tag}</span>
                    <Badge tone="gray">{ib.protocol}</Badge>
                    <span className="hide-sm w-28 truncate text-right text-xs">{coreName(ib.core_id) ?? '—'}</span>
                  </button>
                )
              })}
            </div>
          )}
        </Field>
        <Toggle checked={form.disabled} onChange={(v) => setForm({ ...form, disabled: v })} label={t('groups.disabled')} />
      </form>
    </Modal>
  )
}
