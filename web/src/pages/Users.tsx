import { useEffect, useMemo, useState } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { ChevronLeft, ChevronRight, Plus, QrCode, RefreshCw, Search } from '../components/icons'
import { useSearchParams } from 'react-router-dom'
import { get, post } from '../lib/api'
import { useToast } from '../components/toast'
import { useDebounced, useFetch } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import { useAuth } from '../lib/auth'
import { daysLeft, formatBytes, relativeTime } from '../lib/format'
import type { User, UserStatus } from '../lib/types'
import { Button, Card, Confirm, Empty, ErrorNote, IconButton, Input, PageHeader, ProgressBar, Select, Table, Td, Th, cx } from '../components/ui'
import { UserPresenceBadge, isOnline } from '../components/StatusBadge'
import UserForm from './users/UserForm'
import UserDetails from './users/UserDetails'

const PAGE = 25
const STATUSES: UserStatus[] = ['active', 'disabled', 'limited', 'expired', 'on_hold']

export default function UsersPage() {
  const { t, lang } = useI18n()
  const { admin } = useAuth()
  const [search, setSearch] = useState('')
  const [status, setStatus] = useState('')
  const [sort, setSort] = useState('-created_at')
  const [page, setPage] = useState(0)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [editing, setEditing] = useState<User | 'new' | null>(null)
  const [viewing, setViewing] = useState<User | null>(null)
  const toast = useToast()
  const [params, setParams] = useSearchParams()

  // Deep links from the command palette: ?new=1 opens the form, ?open=<id> a user.
  useEffect(() => {
    const open = params.get('open')
    if (params.get('new')) setEditing('new')
    if (open) get<User>(`/api/users/${open}`).then(setViewing).catch(() => {})
    if (params.get('new') || open) setParams({}, { replace: true })
  }, [params, setParams])
  const [confirmBulkDelete, setConfirmBulkDelete] = useState(false)
  const [bulkError, setBulkError] = useState<string | null>(null)
  const q = useDebounced(search)

  useEffect(() => setPage(0), [q, status, sort])

  const path = useMemo(() => {
    const p = new URLSearchParams({ limit: String(PAGE), offset: String(page * PAGE), sort })
    if (q) p.set('search', q)
    if (status) p.set('status', status)
    return `/api/users?${p}`
  }, [q, status, sort, page])
  const { data, error, loading, reload } = useFetch<{ users: User[]; total: number }>(path)
  // usage and presence change while keys are in use: keep the list fresh
  useEffect(() => {
    const id = window.setInterval(() => document.visibilityState === 'visible' && reload(), 15_000)
    return () => window.clearInterval(id)
  }, [reload])
  const users = data?.users ?? []
  const total = data?.total ?? 0
  const pages = Math.max(1, Math.ceil(total / PAGE))

  useEffect(() => setSelected(new Set()), [path])

  const toggleAll = () => setSelected(selected.size === users.length ? new Set() : new Set(users.map((u) => u.id)))
  const toggle = (id: number) => {
    const next = new Set(selected)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    setSelected(next)
  }

  const bulk = async (action: string, extra: Record<string, number> = {}) => {
    setBulkError(null)
    try {
      const res = await post<{ affected: number }>('/api/users/bulk', { ids: [...selected], action, ...extra })
      setSelected(new Set())
      reload()
      toast(t('toast.done', { n: res.affected }))
    } catch (e) {
      setBulkError((e as Error).message)
    }
  }

  const sortHeader = (field: string, label: string) => (
    <button
      className="inline-flex items-center gap-1 hover:text-foreground"
      onClick={() => setSort(sort === `-${field}` ? field : `-${field}`)}
    >
      {label}
      {sort.endsWith(field) && <span>{sort.startsWith('-') ? '↓' : '↑'}</span>}
    </button>
  )

  return (
    <>
      <PageHeader
        title={t('nav.users')}
        subtitle={t('users.total', { n: total })}
        actions={
          <>
            <Button variant="secondary" onClick={reload} aria-label="reload">
              <RefreshCw className={cx('h-4 w-4', loading && 'animate-spin')} />
            </Button>
            <Button onClick={() => setEditing('new')}>
              <Plus className="h-4 w-4" />
              {t('users.new')}
            </Button>
          </>
        }
      />
      <ErrorNote error={error ?? bulkError} />

      <Card>
        <div className="flex flex-wrap items-center gap-3 border-b border-border p-4">
          <div className="relative min-w-[200px] flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-faint" />
            <Input className="pl-9" placeholder={t('common.search')} value={search} onChange={(e) => setSearch(e.target.value)} />
          </div>
          <Select className="w-auto" value={status} onChange={(e) => setStatus(e.target.value)}>
            <option value="">{t('common.all')}</option>
            {STATUSES.map((s) => (
              <option key={s} value={s}>
                {t(`status.${s}`)}
              </option>
            ))}
          </Select>
        </div>

        <AnimatePresence initial={false}>
        {selected.size > 0 && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.25, ease: [0.22, 1, 0.36, 1] }}
            className="overflow-hidden"
          >
          <div className="flex flex-wrap items-center gap-2 border-b border-border bg-muted/60 px-4 py-2.5">
            <span className="mr-2 text-sm font-medium text-foreground">{t('users.selected', { n: selected.size })}</span>
            <Button size="sm" variant="secondary" onClick={() => bulk('enable')}>
              {t('users.bulk.enable')}
            </Button>
            <Button size="sm" variant="secondary" onClick={() => bulk('disable')}>
              {t('users.bulk.disable')}
            </Button>
            <Button size="sm" variant="secondary" onClick={() => bulk('reset_traffic')}>
              {t('users.bulk.reset')}
            </Button>
            <Button size="sm" variant="secondary" onClick={() => bulk('extend', { days: 30 })}>
              {t('users.bulk.extend')}
            </Button>
            <Button size="sm" variant="danger" onClick={() => setConfirmBulkDelete(true)}>
              {t('users.bulk.delete')}
            </Button>
          </div>
          </motion.div>
        )}
        </AnimatePresence>

        {users.length === 0 && !loading ? (
          <Empty />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th className="w-10">
                  <input type="checkbox" className="rounded" checked={users.length > 0 && selected.size === users.length} onChange={toggleAll} aria-label="select all" />
                </Th>
                <Th>{sortHeader('username', t('users.username'))}</Th>
                <Th className="hide-sm">{t('users.status')}</Th>
                <Th className="min-w-[110px] sm:min-w-[180px]">{sortHeader('used_traffic', t('users.usage'))}</Th>
                <Th className="hide-sm">{sortHeader('expire_at', t('users.expire'))}</Th>
                <Th className="hide-sm">{sortHeader('online_at', t('users.online'))}</Th>
                <Th className="hide-sm w-12" />
              </tr>
            </thead>
            <tbody>
              {users.map((u, idx) => {
                const left = daysLeft(u.expire_at)
                return (
                  <motion.tr
                    key={u.id}
                    initial={{ opacity: 0, y: 6 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={{ duration: 0.3, delay: Math.min(idx, 20) * 0.025, ease: [0.22, 1, 0.36, 1] }}
                    className={cx(
                      'group cursor-pointer transition-colors duration-150 hover:bg-muted/60',
                      selected.has(u.id) && 'bg-muted/60',
                    )}
                    onClick={() => setViewing(u)}
                  >
                    <Td className="w-10">
                      <input type="checkbox" className="rounded" checked={selected.has(u.id)} onClick={(e) => e.stopPropagation()} onChange={() => toggle(u.id)} aria-label={`select ${u.username}`} />
                    </Td>
                    <Td>
                      <div className="flex items-center gap-2.5">
                        <StatusDot user={u} />
                        <div className="min-w-0">
                          <div className="truncate font-medium text-foreground">{u.username}</div>
                          {u.note && <div className="max-w-[220px] truncate text-xs text-faint">{u.note}</div>}
                          {u.admin_name && (
                            <div className="hide-sm max-w-[220px] truncate text-xs text-faint" title={t('users.createdBy')}>
                              {t('users.by', { admin: u.admin_name })}
                            </div>
                          )}
                          {/* phones: status and creator in one small line */}
                          <div className="mt-0.5 show-sm">
                            <div className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
                              <PresenceText user={u} />
                              {u.admin_name && <span className="truncate text-faint">· {u.admin_name}</span>}
                            </div>
                          </div>
                        </div>
                      </div>
                    </Td>
                    <Td className="hide-sm">
                      <UserPresenceBadge user={u} />
                    </Td>
                    <Td>
                      <div className="mb-1.5 flex justify-between gap-2 text-xs tabular-nums text-muted-foreground">
                        <span>{formatBytes(u.used_traffic)}</span>
                        <span>{u.data_limit ? formatBytes(u.data_limit) : '∞'}</span>
                      </div>
                      <ProgressBar value={u.used_traffic} max={u.data_limit} />
                      <div className="mt-1 text-xs text-muted-foreground show-sm">
                        {left === null ? '∞' : left < 0 ? <span className="text-destructive">{t('users.expiredAgo')}</span> : t('users.daysLeft', { n: left })}
                      </div>
                    </Td>
                    <Td className="hide-sm whitespace-nowrap text-muted-foreground">
                      {u.status === 'on_hold' ? (
                        <span className="text-violet">{t('users.daysLeft', { n: Math.round(u.on_hold_duration / 86400) })}</span>
                      ) : left === null ? (
                        '∞'
                      ) : left < 0 ? (
                        <span className="text-destructive">{t('users.expiredAgo')}</span>
                      ) : (
                        <span className={cx(left <= 3 && 'text-warning')}>{t('users.daysLeft', { n: left })}</span>
                      )}
                    </Td>
                    <Td className="hide-sm whitespace-nowrap text-muted-foreground">
                      <OnlineDot iso={u.online_at} />
                      {isOnline(u.online_at) ? t('presence.now') : relativeTime(u.online_at, lang)}
                    </Td>
                    <Td className="hide-sm">
                      <IconButton
                        onClick={(e) => {
                          e.stopPropagation()
                          setViewing(u)
                        }}
                        aria-label="subscription"
                      >
                        <QrCode className="h-4 w-4" />
                      </IconButton>
                    </Td>
                  </motion.tr>
                )
              })}
            </tbody>
          </Table>
        )}

        {pages > 1 && (
          <div className="flex items-center justify-between px-4 py-3 text-sm text-muted-foreground">
            <span>
              {page + 1} / {pages}
            </span>
            <div className="flex gap-1">
              <IconButton disabled={page === 0} onClick={() => setPage(page - 1)} aria-label={t('common.prev')}>
                <ChevronLeft className="h-4 w-4" />
              </IconButton>
              <IconButton disabled={page + 1 >= pages} onClick={() => setPage(page + 1)} aria-label={t('common.next')}>
                <ChevronRight className="h-4 w-4" />
              </IconButton>
            </div>
          </div>
        )}
      </Card>

      {editing && (
        <UserForm
          user={editing === 'new' ? null : editing}
          isSudo={admin?.role === 'sudo'}
          onClose={() => setEditing(null)}
          onSaved={(u) => {
            toast(editing === 'new' ? t('toast.created') : t('toast.saved'))
            setEditing(null)
            reload()
            setViewing(u)
          }}
        />
      )}
      {viewing && (
        <UserDetails
          user={viewing}
          onClose={() => setViewing(null)}
          onEdit={() => {
            setEditing(viewing)
            setViewing(null)
          }}
          onChanged={(u) => {
            reload()
            toast(u ? t('toast.saved') : t('toast.deleted'))
            if (u) setViewing(u)
            else setViewing(null)
          }}
        />
      )}
      <Confirm
        open={confirmBulkDelete}
        onClose={() => setConfirmBulkDelete(false)}
        onConfirm={() => bulk('delete')}
        title={t('users.bulk.delete')}
        message={t('users.bulk.confirmDelete', { n: selected.size })}
      />
    </>
  )
}

function OnlineDot({ iso }: { iso: string | null }) {
  const online = isOnline(iso)
  return <span className={cx('mr-2 inline-block h-2 w-2 rounded-full', online ? 'bg-success' : 'bg-border-strong')} />
}

// StatusDot stands in for the avatar: green while the key is in use, grey
// when it is not, the status colour for keys that cannot be used.
function StatusDot({ user }: { user: User }) {
  const { t } = useI18n()
  const online = user.status === 'active' && isOnline(user.online_at)
  const color =
    user.status === 'active'
      ? online
        ? 'bg-success'
        : 'bg-border-strong'
      : { expired: 'bg-destructive', limited: 'bg-warning', on_hold: 'bg-violet', disabled: 'bg-border-strong' }[user.status as string] ?? 'bg-border-strong'
  return (
    <span className="relative flex h-3 w-3 shrink-0" title={t(online ? 'presence.online' : 'presence.offline')}>
      {online && <span className={'absolute inset-0 animate-ping rounded-full opacity-60 ' + color} aria-hidden />}
      <span className={'relative h-3 w-3 rounded-full ' + color} aria-hidden />
    </span>
  )
}

// PresenceText is the status line of phone rows: "Online", "Online 5 minutes
// ago" for an idle key, or the status of a key that cannot be used.
function PresenceText({ user }: { user: User }) {
  const { t, lang } = useI18n()
  if (user.status !== 'active') {
    const color = { expired: 'text-destructive', limited: 'text-warning', on_hold: 'text-violet' }[user.status as string] ?? ''
    return <span className={'shrink-0 ' + color}>{t(`status.${user.status}`)}</span>
  }
  if (isOnline(user.online_at)) return <span className="shrink-0 text-success">{t('presence.online')}</span>
  if (!user.online_at) return <span className="shrink-0">{t('presence.never')}</span>
  return <span className="shrink-0">{t('presence.ago', { when: relativeTime(user.online_at, lang) })}</span>
}
