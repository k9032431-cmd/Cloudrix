import { useState, type FormEvent, type ReactNode } from 'react'
import { Pencil, Plus, Trash2 } from '../components/icons'
import { del, post, put } from '../lib/api'
import { useFetch } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import { useAuth } from '../lib/auth'
import { GB, daysLeft, formatBytes, formatDate, fromLocalInput, toLocalInput } from '../lib/format'
import type { Admin, Role } from '../lib/types'
import { Badge, Button, Card, Confirm, ErrorNote, Field, IconButton, Input, Modal, PageHeader, ProgressBar, Select, Table, Td, Th, Toggle, cx } from '../components/ui'

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
              <Th className="hide-sm">{t('admins.role')}</Th>
              <Th className="hide-sm">{t('admins.users')}</Th>
              <Th className="hide-md">{t('admins.traffic')}</Th>
              <Th className="hide-sm">{t('admins.left')}</Th>
              <Th className="hide-sm">{t('users.status')}</Th>
              <Th className="w-20" />
            </tr>
          </thead>
          <tbody>
            {data?.map((a) => {
              const sudo = a.role === 'sudo'
              return (
                <tr key={a.id}>
                  <Td>
                    <div className="flex items-center gap-2">
                      <span className="truncate font-medium text-foreground">{a.username}</span>
                      <span className="show-sm">
                        <Badge tone={roleTone[a.role]}>{t(`admins.role.${a.role}`)}</Badge>
                      </span>
                    </div>
                    {/* phones: the whole tariff under the name */}
                    {!sudo && (
                      <div className="show-sm mt-2 min-w-0">
                        <div className="grid grid-cols-[auto_auto_minmax(0,1fr)] gap-x-4 text-xs">
                          <Stat label={t('admins.users')} value={`${a.users_count} / ${a.user_limit || '∞'}`} />
                          <Stat label={t('admins.left')} value={<DaysLeft iso={a.expire_at} />} />
                          <Stat wrap label={t('admins.traffic')} value={`${formatBytes(a.used_traffic)} / ${a.data_limit ? formatBytes(a.data_limit) : '∞'}`} />
                        </div>
                        <div className="mt-2 flex items-center gap-2">
                          <div className="min-w-0 flex-1">
                            <ProgressBar value={a.used_traffic} max={a.data_limit} />
                          </div>
                          <AdminStatus a={a} />
                        </div>
                      </div>
                    )}
                  </Td>
                  <Td className="hide-sm">
                    <Badge tone={roleTone[a.role]}>{t(`admins.role.${a.role}`)}</Badge>
                  </Td>
                  <Td className="hide-sm min-w-[110px]">
                    <div className="mb-1 text-xs tabular-nums text-muted-foreground">
                      {a.users_count} / {sudo ? '∞' : a.user_limit || '∞'}
                    </div>
                    {!sudo && a.user_limit > 0 && <ProgressBar value={a.users_count} max={a.user_limit} />}
                  </Td>
                  <Td className="hide-md min-w-[140px]">
                    {sudo ? (
                      <span className="text-muted-foreground">—</span>
                    ) : (
                      <>
                        <div className="mb-1 text-xs tabular-nums text-muted-foreground">
                          {formatBytes(a.used_traffic)} / {a.data_limit ? formatBytes(a.data_limit) : '∞'}
                        </div>
                        <ProgressBar value={a.used_traffic} max={a.data_limit} />
                      </>
                    )}
                  </Td>
                  <Td className="hide-sm whitespace-nowrap">
                    {sudo ? (
                      <span className="text-muted-foreground">∞</span>
                    ) : (
                      <>
                        <div className="text-sm">
                          <DaysLeft iso={a.expire_at} />
                        </div>
                        {a.expire_at && <div className="text-xs text-muted-foreground">{formatDate(a.expire_at, lang)}</div>}
                      </>
                    )}
                  </Td>
                  <Td className="hide-sm">
                    <AdminStatus a={a} />
                  </Td>
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
              )
            })}
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

function Stat({ label, value, wrap }: { label: string; value: ReactNode; wrap?: boolean }) {
  return (
    <div className="min-w-0">
      <div className="truncate text-faint">{label}</div>
      <div className={cx('font-medium tabular-nums text-foreground', wrap ? 'break-words' : 'truncate')}>{value}</div>
    </div>
  )
}

// DaysLeft is an admin's remaining term: "12 d", amber in the last 3 days,
// red once it is over.
export function DaysLeft({ iso }: { iso: string | null }) {
  const { t } = useI18n()
  const left = daysLeft(iso)
  if (left === null) return <>∞</>
  if (left <= 0) return <span className="text-destructive">{t('users.expiredAgo')}</span>
  return <span className={cx(left <= 3 && 'text-warning')}>{t('users.daysLeft', { n: left })}</span>
}

function AdminStatus({ a }: { a: Admin }) {
  const { t } = useI18n()
  const expired = a.role !== 'sudo' && a.expire_at && new Date(a.expire_at).getTime() <= Date.now()
  const used = a.role !== 'sudo' && a.data_limit > 0 && a.used_traffic >= a.data_limit
  if (a.disabled) return <Badge tone="gray" dot>{t('common.disabled')}</Badge>
  if (expired) return <Badge tone="red" dot>{t('status.expired')}</Badge>
  if (used) return <Badge tone="amber" dot>{t('status.limited')}</Badge>
  return <Badge tone="green" dot>{t('common.enabled')}</Badge>
}

const QUICK_DAYS = [30, 90, 180, 365]

function AdminForm({ admin, onClose, onSaved }: { admin: Admin | null; onClose: () => void; onSaved: () => void }) {
  const { t } = useI18n()
  const [form, setForm] = useState({
    username: admin?.username ?? '',
    password: '',
    role: admin?.role ?? ('admin' as Role),
    disabled: admin?.disabled ?? false,
    user_limit: admin?.user_limit ?? 0,
    expire_at: admin?.expire_at ?? null,
    data_limit: admin?.data_limit ?? 0,
    max_user_data: admin?.max_user_data ?? 0,
    max_user_devices: admin?.max_user_devices ?? 0,
    reset_traffic: false,
  })
  const limited = form.role !== 'sudo'
  const gb = (v: number) => (v ? +(v / GB).toFixed(3) : 0)
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
          {limited && (
            <Field label={t('admins.userLimit')} hint={t('users.dataLimitHint')}>
              <Input type="number" min={0} value={form.user_limit} onChange={(e) => setForm({ ...form, user_limit: Math.max(0, Number(e.target.value)) })} />
            </Field>
          )}
        </div>
        {limited && (
          <fieldset className="grid gap-4 rounded-card border border-border p-4 sm:grid-cols-2">
            <legend className="px-1 text-xs font-medium text-muted-foreground">{t('admins.limits')}</legend>
            <Field
              label={t('admins.until')}
              className="sm:col-span-2"
              hint={
                <span className="flex flex-wrap gap-1.5">
                  {QUICK_DAYS.map((d) => (
                    <button
                      type="button"
                      key={d}
                      className="rounded bg-muted px-1.5 py-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                      onClick={() => setForm({ ...form, expire_at: new Date(Date.now() + d * 86_400_000).toISOString() })}
                    >
                      +{d}d
                    </button>
                  ))}
                  <button type="button" className="rounded bg-muted px-1.5 py-0.5 text-muted-foreground hover:bg-accent" onClick={() => setForm({ ...form, expire_at: null })}>
                    ∞ {t('users.expireNone')}
                  </button>
                </span>
              }
            >
              <Input type="datetime-local" value={toLocalInput(form.expire_at)} onChange={(e) => setForm({ ...form, expire_at: fromLocalInput(e.target.value) })} />
            </Field>
            <Field label={t('admins.dataLimit')} hint={admin ? t('admins.used', { used: formatBytes(admin.used_traffic) }) : t('users.dataLimitHint')}>
              <Input type="number" min={0} step="any" value={gb(form.data_limit)} onChange={(e) => setForm({ ...form, data_limit: Math.round(Math.max(0, Number(e.target.value)) * GB) })} />
            </Field>
            <Field label={t('admins.maxUserData')} hint={t('admins.capHint')}>
              <Input type="number" min={0} step="any" value={gb(form.max_user_data)} onChange={(e) => setForm({ ...form, max_user_data: Math.round(Math.max(0, Number(e.target.value)) * GB) })} />
            </Field>
            <Field label={t('admins.maxUserDevices')} hint={t('admins.capHint')}>
              <Input type="number" min={0} value={form.max_user_devices} onChange={(e) => setForm({ ...form, max_user_devices: Math.max(0, Number(e.target.value)) })} />
            </Field>
            {admin && admin.used_traffic > 0 && (
              <div className="flex items-end pb-1.5">
                <Toggle checked={form.reset_traffic} onChange={(v) => setForm({ ...form, reset_traffic: v })} label={t('admins.resetTraffic')} />
              </div>
            )}
            <p className="hint sm:col-span-2">{t('admins.limitsHint')}</p>
          </fieldset>
        )}
        <Toggle checked={form.disabled} onChange={(v) => setForm({ ...form, disabled: v })} label={t('common.disabled')} />
      </form>
    </Modal>
  )
}
