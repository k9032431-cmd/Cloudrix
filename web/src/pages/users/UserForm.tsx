import { useState, type FormEvent } from 'react'
import { post, put } from '../../lib/api'
import { useFetch } from '../../lib/hooks'
import { useI18n } from '../../lib/i18n'
import { GB, fromLocalInput, toLocalInput } from '../../lib/format'
import type { Admin, ResetStrategy, User, UserInput } from '../../lib/types'
import { Badge, Button, ErrorNote, Field, Input, Modal, Select, Textarea, cx, Segmented } from '../../components/ui'

interface BriefInbound {
  tag: string
  protocol: string
  remark: string
  enabled: boolean
}

const QUICK_DAYS = [7, 30, 90, 365]

export default function UserForm({
  user,
  isSudo,
  onClose,
  onSaved,
}: {
  user: User | null
  isSudo: boolean
  onClose: () => void
  onSaved: (u: User) => void
}) {
  const { t } = useI18n()
  const { data: inbounds } = useFetch<BriefInbound[]>('/api/inbounds')
  const { data: admins } = useFetch<Admin[]>(isSudo ? '/api/admins' : null)
  const [form, setForm] = useState<UserInput>(() => ({
    username: user?.username ?? '',
    status: user && (user.status === 'disabled' || user.status === 'on_hold') ? user.status : 'active',
    data_limit: user?.data_limit ?? 0,
    expire_at: user?.expire_at ?? null,
    on_hold_duration: user?.on_hold_duration || 30 * 86400,
    reset_strategy: user?.reset_strategy ?? 'no_reset',
    device_limit: user?.device_limit ?? 0,
    inbounds: user?.inbounds ?? [],
    note: user?.note ?? '',
    admin_id: user?.admin_id,
  }))
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const set = <K extends keyof UserInput>(k: K, v: UserInput[K]) => setForm((f) => ({ ...f, [k]: v }))

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const body = { ...form, on_hold_duration: form.status === 'on_hold' ? form.on_hold_duration : 0 }
      const saved = user ? await put<User>(`/api/users/${user.id}`, body) : await post<User>('/api/users', body)
      onSaved(saved)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const toggleInbound = (tag: string) =>
    set('inbounds', form.inbounds.includes(tag) ? form.inbounds.filter((x) => x !== tag) : [...form.inbounds, tag])

  return (
    <Modal
      open
      onClose={onClose}
      title={user ? `${t('common.edit')}: ${user.username}` : t('users.new')}
      wide
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="user-form" loading={busy}>
            {user ? t('common.save') : t('common.create')}
          </Button>
        </>
      }
    >
      <form id="user-form" onSubmit={submit} className="space-y-5">
        <ErrorNote error={error} />
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t('users.username')}>
            <Input value={form.username} onChange={(e) => set('username', e.target.value)} required pattern="[a-zA-Z0-9_.@\-]{3,64}" autoFocus={!user} />
          </Field>
          <Field label={t('users.status')}>
            <Segmented
              id="user-status"
              className="w-full"
              value={form.status}
              onChange={(v) => set('status', v)}
              options={(['active', 'disabled', 'on_hold'] as const).map((s) => ({ value: s, label: t(`status.${s}`) }))}
            />
          </Field>

          <Field label={t('users.dataLimit')} hint={t('users.dataLimitHint')}>
            <Input
              type="number"
              min={0}
              step="any"
              value={form.data_limit ? +(form.data_limit / GB).toFixed(3) : 0}
              onChange={(e) => set('data_limit', Math.round(Number(e.target.value) * GB))}
            />
          </Field>
          <Field label={t('users.reset')}>
            <Select value={form.reset_strategy} onChange={(e) => set('reset_strategy', e.target.value as ResetStrategy)}>
              {(['no_reset', 'day', 'week', 'month'] as const).map((r) => (
                <option key={r} value={r}>
                  {t(`users.reset.${r}`)}
                </option>
              ))}
            </Select>
          </Field>

          {form.status === 'on_hold' ? (
            <Field label={t('users.onHoldDays')}>
              <Input
                type="number"
                min={1}
                value={Math.round(form.on_hold_duration / 86400)}
                onChange={(e) => set('on_hold_duration', Math.max(1, Number(e.target.value)) * 86400)}
              />
            </Field>
          ) : (
            <Field
              label={t('users.expireAt')}
              hint={
                <span className="flex flex-wrap gap-1.5">
                  {QUICK_DAYS.map((d) => (
                    <button
                      type="button"
                      key={d}
                      className="rounded bg-slate-100 px-1.5 py-0.5 text-slate-600 hover:bg-slate-200 hover:text-slate-900 dark:bg-slate-800 dark:text-slate-300"
                      onClick={() => set('expire_at', new Date(Date.now() + d * 86_400_000).toISOString())}
                    >
                      +{d}d
                    </button>
                  ))}
                  <button type="button" className="rounded bg-slate-100 px-1.5 py-0.5 text-slate-600 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300" onClick={() => set('expire_at', null)}>
                    ∞ {t('users.expireNone')}
                  </button>
                </span>
              }
            >
              <Input type="datetime-local" value={toLocalInput(form.expire_at)} onChange={(e) => set('expire_at', fromLocalInput(e.target.value))} />
            </Field>
          )}
          <Field label={t('users.deviceLimit')} hint={t('users.deviceLimitHint')}>
            <Input type="number" min={0} value={form.device_limit} onChange={(e) => set('device_limit', Math.max(0, Number(e.target.value)))} />
          </Field>

          {isSudo && admins && (
            <Field label={t('users.owner')}>
              <Select value={form.admin_id ?? ''} onChange={(e) => set('admin_id', e.target.value ? Number(e.target.value) : undefined)}>
                {!user && <option value="">—</option>}
                {admins.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.username} ({t(`admins.role.${a.role}`)})
                  </option>
                ))}
              </Select>
            </Field>
          )}
        </div>

        <Field label={t('users.inbounds')} hint={t('users.inboundsHint')}>
          <div className="flex flex-wrap gap-2">
            {(inbounds ?? []).map((ib) => {
              const on = form.inbounds.includes(ib.tag)
              return (
                <button
                  type="button"
                  key={ib.tag}
                  onClick={() => toggleInbound(ib.tag)}
                  className={cx(
                    'flex items-center gap-2 rounded-lg px-3 py-1.5 text-xs font-medium ring-1 ring-inset transition-colors',
                    on
                      ? 'bg-slate-900 text-white ring-slate-900 dark:bg-white dark:text-slate-900 dark:ring-white'
                      : 'text-slate-600 ring-slate-200 hover:bg-slate-50 dark:text-slate-300 dark:ring-slate-700 dark:hover:bg-slate-800',
                    !ib.enabled && 'opacity-50',
                  )}
                >
                  {ib.tag}
                  <Badge tone="gray">{ib.protocol}</Badge>
                </button>
              )
            })}
          </div>
        </Field>

        <Field label={t('users.note')}>
          <Textarea rows={2} maxLength={500} value={form.note} onChange={(e) => set('note', e.target.value)} />
        </Field>
      </form>
    </Modal>
  )
}
