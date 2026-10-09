import { useEffect, useMemo, useState } from 'react'
import { ChevronLeft, ChevronRight, Plus, QrCode, RefreshCw, Search } from 'lucide-react'
import { post } from '../lib/api'
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
      await post('/api/users/bulk', { ids: [...selected], action, ...extra })
      setSelected(new Set())
      reload()
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

        {selected.size > 0 && (
          <div className="flex flex-wrap items-center gap-2 border-b border-slate-100 bg-brand-50/60 px-4 py-2.5 dark:border-slate-800 dark:bg-brand-500/5">
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
        )}

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
              {users.map((u) => {
                const left = daysLeft(u.expire_at)
                return (
                  <tr key={u.id} className="cursor-pointer transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/40" onClick={() => setViewing(u)}>
                    <Td className="w-10">
                      <input type="checkbox" className="rounded" checked={selected.has(u.id)} onClick={(e) => e.stopPropagation()} onChange={() => toggle(u.id)} aria-label={`select ${u.username}`} />
                    </Td>
                    <Td>
                      <div className="font-medium text-slate-900 dark:text-white">{u.username}</div>
                      {u.note && <div className="max-w-[220px] truncate text-xs text-slate-400">{u.note}</div>}
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
                  </tr>
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
