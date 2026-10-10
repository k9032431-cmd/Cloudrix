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
import { UserStatusBadge } from '../components/StatusBadge'
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
      className="inline-flex items-center gap-1 uppercase hover:text-slate-900 dark:hover:text-white"
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
        <div className="flex flex-wrap items-center gap-3 border-b border-slate-100 p-4 dark:border-slate-800">
          <div className="relative min-w-[200px] flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" />
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
          <div className="flex flex-wrap items-center gap-2 border-b border-slate-100 bg-gradient-to-r from-brand-50 to-violet-50/60 px-4 py-2.5 dark:border-slate-800 dark:from-brand-500/10 dark:to-violet-500/5">
            <span className="mr-2 text-sm font-medium text-brand-700 dark:text-brand-300">{t('users.selected', { n: selected.size })}</span>
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
                <Th>{t('users.status')}</Th>
                <Th className="min-w-[180px]">{sortHeader('used_traffic', t('users.usage'))}</Th>
                <Th>{sortHeader('expire_at', t('users.expire'))}</Th>
                <Th>{sortHeader('online_at', t('users.online'))}</Th>
                <Th className="w-12" />
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
                      'group cursor-pointer transition-colors duration-150 hover:bg-gradient-to-r hover:from-brand-50/70 hover:to-transparent dark:hover:from-brand-500/[0.07]',
                      selected.has(u.id) && 'bg-brand-50/50 dark:bg-brand-500/[0.06]',
                    )}
                    onClick={() => setViewing(u)}
                  >
                    <Td className="w-10">
                      <input type="checkbox" className="rounded" checked={selected.has(u.id)} onClick={(e) => e.stopPropagation()} onChange={() => toggle(u.id)} aria-label={`select ${u.username}`} />
                    </Td>
                    <Td>
                      <div className="flex items-center gap-2.5">
                        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-slate-100 to-slate-200 text-xs font-semibold uppercase text-slate-600 ring-1 ring-white transition-transform duration-300 group-hover:scale-110 dark:from-slate-700 dark:to-slate-800 dark:text-slate-200 dark:ring-slate-900">
                          {u.username.slice(0, 2)}
                        </span>
                        <div className="min-w-0">
                          <div className="font-medium text-slate-900 dark:text-white">{u.username}</div>
                          {u.note && <div className="max-w-[220px] truncate text-xs text-slate-400">{u.note}</div>}
                        </div>
                      </div>
                    </Td>
                    <Td>
                      <UserStatusBadge status={u.status} />
                    </Td>
                    <Td>
                      <div className="mb-1.5 flex justify-between gap-2 text-xs tabular-nums text-slate-600 dark:text-slate-400">
                        <span>{formatBytes(u.used_traffic)}</span>
                        <span>{u.data_limit ? formatBytes(u.data_limit) : '∞'}</span>
                      </div>
                      <ProgressBar value={u.used_traffic} max={u.data_limit} />
                    </Td>
                    <Td className="whitespace-nowrap text-slate-600 dark:text-slate-400">
                      {u.status === 'on_hold' ? (
                        <span className="text-violet-600 dark:text-violet-300">{t('users.daysLeft', { n: Math.round(u.on_hold_duration / 86400) })}</span>
                      ) : left === null ? (
                        '∞'
                      ) : left < 0 ? (
                        <span className="text-rose-600 dark:text-rose-400">{t('users.expiredAgo')}</span>
                      ) : (
                        <span className={cx(left <= 3 && 'text-amber-600 dark:text-amber-400')}>{t('users.daysLeft', { n: left })}</span>
                      )}
                    </Td>
                    <Td className="whitespace-nowrap text-slate-500">
                      <OnlineDot iso={u.online_at} />
                      {relativeTime(u.online_at, lang)}
                    </Td>
                    <Td>
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
          <div className="flex items-center justify-between px-4 py-3 text-sm text-slate-500">
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
  const online = iso && Date.now() - new Date(iso).getTime() < 2 * 60_000
  return <span className={cx('mr-2 inline-block h-2 w-2 rounded-full', online ? 'bg-emerald-500' : 'bg-slate-300 dark:bg-slate-700')} />
}
