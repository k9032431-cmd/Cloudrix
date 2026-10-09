import { useState } from 'react'
import { CloudUpload, KeyRound, Pencil, RotateCcw, Smartphone, Trash2 } from 'lucide-react'
import { del, post } from '../../lib/api'
import { useFetch } from '../../lib/hooks'
import { useI18n } from '../../lib/i18n'
import { useAuth } from '../../lib/auth'
import { formatBytes, formatDate, relativeTime } from '../../lib/format'
import type { DailyTraffic, Device, User, UserDrive } from '../../lib/types'
import { Button, Confirm, CopyButton, ErrorNote, IconButton, Modal, ProgressBar, QR, cx } from '../../components/ui'
import { UserStatusBadge } from '../../components/StatusBadge'
import { TrafficChart } from '../Dashboard'

type Tab = 'sub' | 'links' | 'devices'

interface SubInfo {
  url: string
  links: string[]
  gdrive: UserDrive
}

export default function UserDetails({
  user,
  onClose,
  onEdit,
  onChanged,
}: {
  user: User
  onClose: () => void
  onEdit: () => void
  onChanged: (u: User | null) => void
}) {
  const { t, lang } = useI18n()
  const [tab, setTab] = useState<Tab>('sub')
  const [confirm, setConfirm] = useState<'delete' | 'revoke' | null>(null)
  const [error, setError] = useState<string | null>(null)
  const { data: sub, reload: reloadSub } = useFetch<SubInfo>(`/api/users/${user.id}/subscription?v=${user.sub_token}`)
  const { data: traffic } = useFetch<DailyTraffic[]>(`/api/users/${user.id}/traffic?days=14`)
  const devices = useFetch<Device[]>(tab === 'devices' ? `/api/users/${user.id}/devices` : null)

  const act = async (fn: () => Promise<User | null>) => {
    setError(null)
    try {
      onChanged(await fn())
    } catch (e) {
      setError((e as Error).message)
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      wide
      title={
        <span className="flex items-center gap-3">
          {user.username}
          <UserStatusBadge status={user.status} />
        </span>
      }
      footer={
        <div className="flex w-full flex-wrap gap-2">
          <Button variant="secondary" size="sm" onClick={() => act(() => post<User>(`/api/users/${user.id}/reset-traffic`))}>
            <RotateCcw className="h-3.5 w-3.5" />
            {t('users.resetTraffic')}
          </Button>
          <Button variant="secondary" size="sm" onClick={() => setConfirm('revoke')}>
            <KeyRound className="h-3.5 w-3.5" />
            {t('users.revoke')}
          </Button>
          <Button variant="danger" size="sm" onClick={() => setConfirm('delete')}>
            <Trash2 className="h-3.5 w-3.5" />
            {t('common.delete')}
          </Button>
          <Button size="sm" className="ml-auto" onClick={onEdit}>
            <Pencil className="h-3.5 w-3.5" />
            {t('common.edit')}
          </Button>
        </div>
      }
    >
      <ErrorNote error={error} />
      <div className="mb-5 grid gap-4 sm:grid-cols-3">
        <Info label={t('users.usage')}>
          <div className="mb-1.5 tabular-nums">
            {formatBytes(user.used_traffic)} <span className="text-slate-400">/ {user.data_limit ? formatBytes(user.data_limit) : '∞'}</span>
          </div>
          <ProgressBar value={user.used_traffic} max={user.data_limit} />
        </Info>
        <Info label={t('users.expire')}>{user.expire_at ? formatDate(user.expire_at, lang) : '∞'}</Info>
        <Info label={t('users.lifetime')}>{formatBytes(user.lifetime_used)}</Info>
        <Info label={t('users.online')}>{relativeTime(user.online_at, lang)}</Info>
        <Info label={t('users.lastSub')}>
          {relativeTime(user.sub_updated_at, lang)}
          {user.sub_user_agent && <div className="truncate text-xs text-slate-400">{user.sub_user_agent}</div>}
        </Info>
        <Info label={t('users.created')}>{formatDate(user.created_at, lang)}</Info>
      </div>

      {traffic && traffic.length > 0 && (
        <div className="mb-5">
          <TrafficChart data={traffic} days={14} />
        </div>
      )}

      <div className="mb-4 flex gap-1 border-b border-slate-100 dark:border-slate-800">
        {(['sub', 'links', 'devices'] as const).map((k) => (
          <button
            key={k}
            onClick={() => setTab(k)}
            className={cx(
              '-mb-px border-b-2 px-3 py-2 text-sm font-medium transition-colors',
              tab === k ? 'border-brand-600 text-brand-700 dark:text-brand-300' : 'border-transparent text-slate-500 hover:text-slate-900 dark:hover:text-white',
            )}
          >
            {k === 'sub' ? t('users.subscription') : k === 'links' ? `${t('users.links')} (${sub?.links.length ?? 0})` : t('users.devices')}
          </button>
        ))}
      </div>

      {tab === 'sub' && sub && <SubscriptionLinks user={user} sub={sub} onDriveChanged={reloadSub} />}

      {tab === 'links' && (
        <div className="space-y-2">
          {sub?.links.map((l, i) => (
            <div key={i} className="flex items-center gap-2 rounded-lg bg-slate-50 p-2 dark:bg-slate-800/60">
              <code className="min-w-0 flex-1 truncate text-xs text-slate-600 dark:text-slate-300">{l}</code>
              <CopyButton text={l} />
            </div>
          ))}
        </div>
      )}

      {tab === 'devices' && (
        <div className="space-y-2">
          {devices.data?.length === 0 && <p className="py-6 text-center text-sm text-slate-400">{t('users.noDevices')}</p>}
          {devices.data?.map((d) => (
            <div key={d.hwid} className="flex items-center gap-3 rounded-lg bg-slate-50 p-3 dark:bg-slate-800/60">
              <Smartphone className="h-4 w-4 shrink-0 text-slate-400" />
              <div className="min-w-0 flex-1 text-sm">
                <div className="truncate font-medium text-slate-900 dark:text-white">
                  {[d.device_model, d.platform, d.os_version].filter(Boolean).join(' · ') || d.hwid}
                </div>
                <div className="truncate text-xs text-slate-400">
                  {d.user_agent} · {relativeTime(d.last_seen, lang)}
                </div>
              </div>
              <IconButton
                aria-label="remove device"
                onClick={async () => {
                  await del(`/api/users/${user.id}/devices?hwid=${encodeURIComponent(d.hwid)}`)
                  devices.reload()
                }}
              >
                <Trash2 className="h-4 w-4" />
              </IconButton>
            </div>
          ))}
        </div>
      )}

      <Confirm
        open={confirm === 'delete'}
        onClose={() => setConfirm(null)}
        title={t('common.delete')}
        message={t('common.confirmDelete', { name: user.username })}
        onConfirm={() => act(async () => (await del(`/api/users/${user.id}`), null))}
      />
      <Confirm
        open={confirm === 'revoke'}
        onClose={() => setConfirm(null)}
        title={t('users.revoke')}
        message={t('users.revokeConfirm')}
        onConfirm={() => act(() => post<User>(`/api/users/${user.id}/revoke`))}
      />
    </Modal>
  )
}

function Info({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <div className="mb-1 text-xs text-slate-500 dark:text-slate-400">{label}</div>
      <div className="text-sm font-medium text-slate-900 dark:text-white">{children}</div>
    </div>
  )
}

// SubscriptionLinks shows the regular subscription URL and, next to it, the
// backup Google Drive link (www.googleapis.com/drive/v3/files/…?key=…&alt=media).
function SubscriptionLinks({ user, sub, onDriveChanged }: { user: User; sub: SubInfo; onDriveChanged: () => void }) {
  const { t, lang } = useI18n()
  const { admin } = useAuth()
  const [kind, setKind] = useState<'direct' | 'gdrive'>('direct')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const drive = sub.gdrive
  const url = kind === 'direct' ? sub.url : drive.url

  const createDrive = async () => {
    setBusy(true)
    setError(null)
    try {
      await post(`/api/users/${user.id}/gdrive`)
      onDriveChanged()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div>
      <div className="mb-4 inline-grid grid-cols-2 gap-1 rounded-lg bg-slate-100 p-1 dark:bg-slate-800">
        {(['direct', 'gdrive'] as const).map((k) => (
          <button
            key={k}
            onClick={() => setKind(k)}
            className={cx(
              'flex h-8 items-center justify-center gap-1.5 rounded-md px-3 text-xs font-medium transition-colors',
              kind === k ? 'bg-white text-slate-900 shadow-sm dark:bg-slate-950 dark:text-white' : 'text-slate-500 hover:text-slate-900 dark:hover:text-white',
            )}
          >
            {k === 'gdrive' && <CloudUpload className="h-3.5 w-3.5" />}
            {t(`sub.kind.${k}`)}
          </button>
        ))}
      </div>

      {kind === 'gdrive' && <p className="mb-3 text-xs text-slate-500 dark:text-slate-400">{t('sub.gdriveHint')}</p>}
      <ErrorNote error={error ?? (kind === 'gdrive' ? drive.error ?? null : null)} />

      {url ? (
        <div className="flex flex-col items-center gap-4 sm:flex-row sm:items-start">
          <QR value={url} size={180} />
          <div className="min-w-0 flex-1 space-y-3">
            <code className="block break-all rounded-lg bg-slate-50 p-3 text-xs text-slate-700 dark:bg-slate-800 dark:text-slate-300">{url}</code>
            <div className="flex flex-wrap items-center gap-2">
              <CopyButton text={url} />
              <a href={url} target="_blank" rel="noreferrer">
                <Button variant="secondary" size="sm">
                  ↗
                </Button>
              </a>
              {kind === 'gdrive' && drive.synced_at && (
                <span className="text-xs text-slate-400">
                  {t('sub.gdriveSynced')}: {relativeTime(drive.synced_at, lang)}
                </span>
              )}
            </div>
          </div>
        </div>
      ) : !drive.available ? (
        <p className="rounded-lg bg-slate-50 p-4 text-sm text-slate-500 dark:bg-slate-800/60">
          {admin?.role === 'sudo' ? t('sub.gdriveNotSetUp') : t('sub.gdriveNotSetUpAdmin')}
        </p>
      ) : user.device_limit > 0 ? (
        <p className="rounded-lg bg-amber-50 p-4 text-sm text-amber-800 dark:bg-amber-500/10 dark:text-amber-300">{t('sub.gdriveDeviceLimit')}</p>
      ) : (
        <Button onClick={createDrive} loading={busy}>
          <CloudUpload className="h-4 w-4" />
          {t('sub.gdriveCreate')}
        </Button>
      )}
    </div>
  )
}
