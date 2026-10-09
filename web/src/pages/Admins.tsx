import { useState, type FormEvent } from 'react'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { del, post, put } from '../lib/api'
import { useFetch } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import { useAuth } from '../lib/auth'
import { formatDate } from '../lib/format'
import type { Admin, Role } from '../lib/types'
import { Badge, Button, Card, Confirm, ErrorNote, Field, IconButton, Input, Modal, PageHeader, Select, Table, Td, Th, Toggle } from '../components/ui'

const roleTone = { sudo: 'violet', admin: 'blue', reseller: 'gray' } as const

export default function Admins() {
  const { t, lang } = useI18n()
  const { admin: me } = useAuth()
  const { data, error, reload } = useFetch<Admin[]>('/api/admins')
  const [editing, setEditing] = useState<Admin | 'new' | null>(null)
  const [deleting, setDeleting] = useState<Admin | null>(null)
  const [delError, setDelError] = useState<string | null>(null)

  return (
    <>
      <PageHeader
        title={t('nav.admins')}
        actions={
          <Button onClick={() => setEditing('new')}>
            <Plus className="h-4 w-4" />
            {t('admins.new')}
          </Button>
        }
      />
      <ErrorNote error={error ?? delError} />
      <Card>
        <Table>
          <thead>
            <tr>
              <Th>{t('login.username')}</Th>
              <Th>{t('admins.role')}</Th>
              <Th>{t('admins.userLimit')}</Th>
              <Th>{t('users.status')}</Th>
              <Th>{t('users.created')}</Th>
              <Th className="w-24" />
            </tr>
          </thead>
          <tbody>
            {data?.map((a) => (
              <tr key={a.id}>
                <Td className="font-medium text-slate-900 dark:text-white">{a.username}</Td>
                <Td>
                  <Badge tone={roleTone[a.role]}>{t(`admins.role.${a.role}`)}</Badge>
                </Td>
                <Td className="tabular-nums">{a.user_limit || '∞'}</Td>
                <Td>
                  <Badge tone={a.disabled ? 'gray' : 'green'} dot>
                    {a.disabled ? t('common.disabled') : t('common.enabled')}
                  </Badge>
                </Td>
                <Td className="text-slate-500">{formatDate(a.created_at, lang)}</Td>
                <Td>
                  <div className="flex justify-end gap-1">
                    <IconButton onClick={() => setEditing(a)} aria-label={t('common.edit')}>
                      <Pencil className="h-4 w-4" />
                    </IconButton>
                    {a.id !== me?.id && (
                      <IconButton onClick={() => setDeleting(a)} aria-label={t('common.delete')}>
                        <Trash2 className="h-4 w-4" />
                      </IconButton>
                    )}
                  </div>
                </Td>
              </tr>
            ))}
          </tbody>
        </Table>
      </Card>
      {editing && (
        <AdminForm
          admin={editing === 'new' ? null : editing}
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
        message={t('common.confirmDelete', { name: deleting?.username ?? '' })}
        onConfirm={async () => {
          setDelError(null)
          try {
            await del(`/api/admins/${deleting!.id}`)
          } catch (e) {
            setDelError((e as Error).message)
          }
          reload()
        }}
      />
    </>
  )
}

function AdminForm({ admin, onClose, onSaved }: { admin: Admin | null; onClose: () => void; onSaved: () => void }) {
  const { t } = useI18n()
  const [form, setForm] = useState({
    username: admin?.username ?? '',
    password: '',
    role: admin?.role ?? ('admin' as Role),
    disabled: admin?.disabled ?? false,
    user_limit: admin?.user_limit ?? 0,
  })
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      if (admin) await put(`/api/admins/${admin.id}`, form)
      else await post('/api/admins', form)
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
      title={admin ? `${t('common.edit')}: ${admin.username}` : t('admins.new')}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="admin-form" loading={busy}>
            {admin ? t('common.save') : t('common.create')}
          </Button>
        </>
      }
    >
      <form id="admin-form" onSubmit={submit} className="space-y-4">
        <ErrorNote error={error} />
        <Field label={t('login.username')}>
          <Input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} disabled={!!admin} required />
        </Field>
        <Field label={t('login.password')} hint={admin ? t('admins.passwordHint') : undefined}>
          <Input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} required={!admin} minLength={8} autoComplete="new-password" />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t('admins.role')}>
            <Select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value as Role })}>
              {(['sudo', 'admin', 'reseller'] as const).map((r) => (
                <option key={r} value={r}>
                  {t(`admins.role.${r}`)}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={t('admins.userLimit')} hint={t('users.dataLimitHint')}>
            <Input type="number" min={0} value={form.user_limit} onChange={(e) => setForm({ ...form, user_limit: Number(e.target.value) })} />
          </Field>
        </div>
        <Toggle checked={form.disabled} onChange={(v) => setForm({ ...form, disabled: v })} label={t('common.disabled')} />
      </form>
    </Modal>
  )
}
