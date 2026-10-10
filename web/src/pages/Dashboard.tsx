import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { motion } from 'framer-motion'
import { Link, useNavigate } from 'react-router-dom'
import { ArrowDownRight, ArrowRight, ArrowUpRight, ChartUp, Clock, Cpu, Export, HardDrive, Hourglass, Server, Users, Wifi, Zap, type Icon } from '../components/icons'
import { UserStatusBadge } from '../components/StatusBadge'
import { useAuth } from '../lib/auth'
import { useFetch } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import { formatBytes, formatDuration, relativeTime } from '../lib/format'
import type { AuditEntry, DailyTraffic, Stats, User } from '../lib/types'
import { AnimatedNumber, Button, Card, ErrorNote, PageHeader, Segmented, Skeleton, cx } from '../components/ui'

type Period = '7' | '30' | '90' | '365'

// daySeries returns `n` consecutive UTC days ending today, filling gaps with 0.
function daySeries(data: DailyTraffic[], n: number): DailyTraffic[] {
  const byDay = new Map(data.map((d) => [d.day, d.bytes]))
  const out: DailyTraffic[] = []
  for (let i = n - 1; i >= 0; i--) {
    const day = new Date(Date.now() - i * 86_400_000).toISOString().slice(0, 10)
    out.push({ day, bytes: byDay.get(day) ?? 0 })
  }
  return out
}

const sum = (s: DailyTraffic[]) => s.reduce((a, d) => a + d.bytes, 0)

export default function Dashboard() {
  const { t, lang } = useI18n()
  const { admin } = useAuth()
  const sudo = admin?.role === 'sudo'
  const [period, setPeriod] = useState<Period>('30')
  const days = Number(period)
  const { data, error, reload } = useFetch<Stats>(`/api/system/stats?days=${days}`)

  useEffect(() => {
    const id = setInterval(reload, 15_000)
    return () => clearInterval(id)
  }, [reload])

  // The API returns two periods so the current one can be compared with the one before.
  const { cur, prev } = useMemo(() => {
    const all = daySeries(data?.days === days ? data.traffic : [], days * 2)
    return { cur: all.slice(days), prev: all.slice(0, days) }
  }, [data, days])
  const loaded = data?.days === days
  const curTotal = sum(cur)
  const prevTotal = sum(prev)
  const delta = prevTotal > 0 ? ((curTotal - prevTotal) / prevTotal) * 100 : null

  const u = data?.users
  const exportCSV = () => {
    const rows = ['date,bytes', ...cur.map((d) => `${d.day},${d.bytes}`)]
    const url = URL.createObjectURL(new Blob([rows.join('\n') + '\n'], { type: 'text/csv' }))
    const a = document.createElement('a')
    a.href = url
    a.download = `cloudrix-traffic-${days}d.csv`
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <>
      <PageHeader
        title={t('nav.dashboard')}
        subtitle={t('dash.subtitle')}
        actions={
          <>
            <Segmented<Period>
              id="period"
              value={period}
              onChange={setPeriod}
              options={(['7', '30', '90', '365'] as Period[]).map((p) => ({ value: p, label: t(`dash.period.${p}`) }))}
            />
            <Button variant="secondary" onClick={exportCSV} disabled={!loaded}>
              <Export className="h-4 w-4" />
              {t('dash.export')}
            </Button>
          </>
        }
      />
      <ErrorNote error={error} />

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <Kpi
          label={t('dash.periodTraffic')}
          icon={ChartUp}
          value={loaded ? curTotal : undefined}
          format={(n) => formatBytes(n)}
          delay={0}
          spark={loaded ? cur.map((d) => d.bytes) : undefined}
          footer={
            delta !== null ? (
              <>
                <Delta value={delta} />
                {t('dash.vsPrev')}
              </>
            ) : (
              t('dash.vsPrev') + ': —'
            )
          }
        />
        <Kpi
          label={t('dash.activeUsers')}
          icon={Users}
          value={u?.active}
          delay={0.05}
          footer={u ? t('dash.ofTotal', { n: u.total }) : undefined}
        />
        <Kpi
          label={t('dash.onlineNow')}
          icon={Wifi}
          value={u?.online}
          delay={0.1}
          live
          footer={u ? t('dash.ofActive', { pct: u.active > 0 ? Math.round((u.online / u.active) * 100) : 0 }) : undefined}
        />
        <Kpi
          label={t('dash.expiring')}
          icon={Hourglass}
          value={u?.expiring_soon}
          delay={0.15}
          footer={u ? t('dash.expiredNow', { n: u.expired }) : undefined}
        />
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <Card className="p-5 lg:col-span-2" delay={0.2}>
          <div className="mb-5 flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-sm font-semibold text-slate-900 dark:text-white">{t('dash.trafficTitle')}</h2>
              <p className="mt-0.5 text-xs text-slate-500">{t('dash.daily', { n: days })}</p>
            </div>
            <div className="flex items-center gap-4 text-xs text-slate-500">
              <span className="flex items-center gap-1.5">
                <span className="h-0.5 w-4 rounded-full bg-slate-900 dark:bg-slate-100" />
                {t('dash.current')}
              </span>
              <span className="flex items-center gap-1.5">
                <span className="w-4 border-t-2 border-dashed border-slate-300 dark:border-slate-600" />
                {t('dash.previous')}
              </span>
            </div>
          </div>
          <TrafficChart data={loaded ? data.traffic : []} days={days} compare />
        </Card>
        <Card className="p-5" delay={0.25}>
          <h2 className="text-sm font-semibold text-slate-900 dark:text-white">{t('dash.byStatus')}</h2>
          <StatusBreakdown users={u} />
        </Card>
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <NewUsers />
        {sudo ? <ActivityFeed /> : <SystemCard stats={data} lang={lang} />}
      </div>
    </>
  )
}

function Delta({ value }: { value: number }) {
  const up = value >= 0
  const Arrow = up ? ArrowUpRight : ArrowDownRight
  return (
    <span
      className={cx(
        'inline-flex items-center gap-0.5 rounded px-1 py-px font-medium tabular-nums',
        up ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400' : 'bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-400',
      )}
    >
      <Arrow className="h-3 w-3" />
      {Math.abs(value) >= 1000 ? '999+' : Math.abs(value).toFixed(1)}%
    </span>
  )
}

function Kpi({
  label,
  icon: Icon,
  value,
  format,
  footer,
  spark,
  delay,
  live,
}: {
  label: string
  icon: Icon
  value?: number
  format?: (n: number) => string
  footer?: ReactNode
  spark?: number[]
  delay: number
  live?: boolean
}) {
  return (
    <Card className="p-5" delay={delay} hover>
      <div className="flex items-center justify-between gap-2">
        <span className="truncate text-sm font-medium text-slate-500 dark:text-slate-400">{label}</span>
        <Icon className="h-4 w-4 shrink-0 text-slate-400" />
      </div>
      <div className="mt-2 flex items-end justify-between gap-3">
        <div className="flex items-center gap-2 text-2xl font-semibold tabular-nums tracking-tight text-slate-900 dark:text-white">
          {value === undefined ? <Skeleton className="h-8 w-24" /> : <AnimatedNumber value={value} format={format} />}
          {live && value !== undefined && (
            <span className="relative flex h-2 w-2">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-60" />
              <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
            </span>
          )}
        </div>
        {spark && <Sparkline values={spark} />}
      </div>
      <div className="mt-2 flex min-h-[1.25rem] flex-wrap items-center gap-1.5 text-xs text-slate-500">{footer}</div>
    </Card>
  )
}

function Sparkline({ values }: { values: number[] }) {
  const max = Math.max(...values, 1)
  const pts = values.map((v, i) => `${(i / Math.max(values.length - 1, 1)) * 100},${30 - (v / max) * 28}`).join(' ')
  return (
    <motion.svg
      viewBox="0 0 100 32"
      preserveAspectRatio="none"
      className="mb-1 h-7 w-20 shrink-0 text-slate-900 dark:text-slate-100"
      initial={{ clipPath: 'inset(0 100% 0 0)' }}
      animate={{ clipPath: 'inset(0 0% 0 0)' }}
      transition={{ duration: 0.8, delay: 0.2, ease: [0.22, 1, 0.36, 1] }}
    >
      <polyline points={pts} fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
    </motion.svg>
  )
}

// niceMax picks an axis maximum whose quarter steps are round numbers in the
// value's own unit (KB, MB, GB…), e.g. 9.7 GB -> 10 GB with 2.5 GB steps.
function niceMax(v: number) {
  if (v <= 0) return 1024 ** 3
  const unit = 1024 ** Math.floor(Math.log(v) / Math.log(1024))
  const quarter = v / unit / 4
  const p = Math.pow(10, Math.floor(Math.log10(quarter)))
  const step = [1, 2, 2.5, 5, 10].find((m) => quarter <= m * p)! * p
  return step * 4 * unit
}

const axisLabel = (b: number) => (b === 0 ? '0' : formatBytes(b, 1).replace('.0 ', ' '))

// TrafficChart draws daily traffic as a line with a soft fill. With compare it expects
// 2×days of history and draws the earlier half as a dashed "previous period" line.
export function TrafficChart({ data, days = 30, compare = false }: { data: DailyTraffic[]; days?: number; compare?: boolean }) {
  const { t, lang } = useI18n()
  const [hover, setHover] = useState<number | null>(null)
  const all = daySeries(data, compare ? days * 2 : days)
  const cur = compare ? all.slice(days) : all
  const prev = compare ? all.slice(0, days) : null
  const max = niceMax(Math.max(...cur.map((d) => d.bytes), ...(prev ?? []).map((d) => d.bytes)))
  const n = cur.length
  const x = (i: number) => (n > 1 ? (i / (n - 1)) * 100 : 50)
  const y = (v: number) => 100 - (v / max) * 100
  const line = (s: DailyTraffic[]) => s.map((d, i) => `${i ? 'L' : 'M'}${x(i).toFixed(3)},${y(d.bytes).toFixed(3)}`).join(' ')
  const curPath = line(cur)
  const fmtDay = (day: string) =>
    new Date(day + 'T00:00:00Z').toLocaleDateString(lang, { day: 'numeric', month: 'short', ...(days > 90 ? { year: '2-digit' } : {}), timeZone: 'UTC' })
  const ticks = [1, 0.75, 0.5, 0.25, 0]
  const labelIdx = Array.from(new Set(Array.from({ length: 6 }, (_, k) => Math.round((k / 5) * (n - 1)))))
  const empty = sum(cur) === 0 && (!prev || sum(prev) === 0)

  const onMove = (e: React.PointerEvent<HTMLDivElement>) => {
    const r = e.currentTarget.getBoundingClientRect()
    setHover(Math.min(n - 1, Math.max(0, Math.round(((e.clientX - r.left) / r.width) * (n - 1)))))
  }
  const h = hover !== null ? { i: hover, cur: cur[hover], prev: prev?.[hover] } : null

  return (
    <div className="flex gap-3">
      <div className="relative h-56 w-12 shrink-0 text-right text-[11px] tabular-nums text-slate-400 lg:h-64">
        {ticks.map((f) => (
          <span key={f} className="absolute right-0 -translate-y-1/2" style={{ top: `${(1 - f) * 100}%` }}>
            {axisLabel(max * f)}
          </span>
        ))}
      </div>
      <div className="min-w-0 flex-1">
        <div className="relative h-56 touch-none lg:h-64" onPointerMove={onMove} onPointerDown={onMove} onPointerLeave={() => setHover(null)}>
          {ticks.map((f) => (
            <div
              key={f}
              className={cx('pointer-events-none absolute inset-x-0 border-t', f === 0 ? 'border-slate-200 dark:border-slate-800' : 'border-dashed border-slate-100 dark:border-slate-800/60')}
              style={{ top: `${(1 - f) * 100}%` }}
            />
          ))}
          <motion.svg
            key={`${days}-${compare}`}
            viewBox="0 0 100 100"
            preserveAspectRatio="none"
            className="absolute inset-0 h-full w-full overflow-visible text-slate-900 dark:text-slate-100"
            initial={{ clipPath: 'inset(0 100% 0 0)' }}
            animate={{ clipPath: 'inset(0 0% 0 0)' }}
            transition={{ duration: 0.9, ease: [0.22, 1, 0.36, 1] }}
          >
            <defs>
              <linearGradient id="traffic-fill" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0" stopColor="currentColor" stopOpacity="0.12" />
                <stop offset="1" stopColor="currentColor" stopOpacity="0" />
              </linearGradient>
            </defs>
            {prev && (
              <path
                d={line(prev)}
                fill="none"
                className="stroke-slate-300 dark:stroke-slate-600"
                strokeWidth="1.5"
                strokeDasharray="4 4"
                vectorEffect="non-scaling-stroke"
              />
            )}
            {!empty && <path d={`${curPath} L100,100 L0,100 Z`} fill="url(#traffic-fill)" />}
            <path d={curPath} fill="none" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
          </motion.svg>
          {empty && (
            <div className="pointer-events-none absolute inset-0 flex items-center justify-center text-xs text-slate-400">{t('dash.noData')}</div>
          )}
          {h && (
            <>
              <div className="pointer-events-none absolute inset-y-0 w-px bg-slate-300 dark:bg-slate-600" style={{ left: `${x(h.i)}%` }} />
              {h.prev && (
                <span
                  className="pointer-events-none absolute h-2 w-2 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-slate-300 bg-white dark:border-slate-600 dark:bg-slate-950"
                  style={{ left: `${x(h.i)}%`, top: `${y(h.prev.bytes)}%` }}
                />
              )}
              <span
                className="pointer-events-none absolute h-2.5 w-2.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white bg-slate-900 shadow dark:border-slate-950 dark:bg-white"
                style={{ left: `${x(h.i)}%`, top: `${y(h.cur.bytes)}%` }}
              />
              <div
                className={cx(
                  'pointer-events-none absolute top-2 z-10 w-max whitespace-nowrap rounded-lg border border-slate-200 bg-white px-3 py-2 text-xs shadow-lg shadow-slate-900/5 dark:border-slate-800 dark:bg-slate-900',
                  x(h.i) > 60 ? '-translate-x-[calc(100%+12px)]' : 'translate-x-3',
                )}
                style={{ left: `${x(h.i)}%` }}
              >
                <div className="mb-1.5 font-medium text-slate-900 dark:text-white">{fmtDay(h.cur.day)}</div>
                <TipRow swatch={<span className="h-0.5 w-3 rounded-full bg-slate-900 dark:bg-white" />} label={prev ? t('dash.current') : t('dash.traffic.col')} value={formatBytes(h.cur.bytes)} />
                {h.prev && (
                  <TipRow
                    swatch={<span className="w-3 border-t-2 border-dashed border-slate-300 dark:border-slate-600" />}
                    label={`${t('dash.previous')} · ${fmtDay(h.prev.day)}`}
                    value={formatBytes(h.prev.bytes)}
                  />
                )}
              </div>
            </>
          )}
        </div>
        <div className="relative mt-2 h-4 text-[11px] text-slate-400">
          {labelIdx.map((i, k) => (
            <span
              key={i}
              className={cx('absolute whitespace-nowrap', k === 0 ? '' : k === labelIdx.length - 1 ? '-translate-x-full' : '-translate-x-1/2', k % 2 === 1 && 'hidden sm:inline')}
              style={{ left: `${x(i)}%` }}
            >
              {fmtDay(cur[i].day)}
            </span>
          ))}
        </div>
      </div>
    </div>
  )
}

function TipRow({ swatch, label, value }: { swatch: ReactNode; label: string; value: string }) {
  return (
    <div className="flex items-center gap-2 py-0.5">
      {swatch}
      <span className="text-slate-500">{label}</span>
      <span className="ml-auto pl-3 font-medium tabular-nums text-slate-900 dark:text-white">{value}</span>
    </div>
  )
}

function StatusBreakdown({ users }: { users?: Stats['users'] }) {
  const { t } = useI18n()
  const parts = [
    { key: 'active', n: users?.active ?? 0, color: 'bg-emerald-500', label: t('status.active') },
    { key: 'limited', n: users?.limited ?? 0, color: 'bg-amber-500', label: t('status.limited') },
    { key: 'expired', n: users?.expired ?? 0, color: 'bg-rose-500', label: t('status.expired') },
    { key: 'on_hold', n: users?.on_hold ?? 0, color: 'bg-violet-500', label: t('status.on_hold') },
    { key: 'disabled', n: users?.disabled ?? 0, color: 'bg-slate-300 dark:bg-slate-600', label: t('status.disabled') },
  ]
  const total = parts.reduce((a, p) => a + p.n, 0)
  return (
    <div>
      <div className="mb-4 mt-3 flex items-baseline gap-2">
        <span className="text-2xl font-semibold tabular-nums tracking-tight text-slate-900 dark:text-white">
          {users ? <AnimatedNumber value={total} /> : <Skeleton className="h-8 w-16" />}
        </span>
        <span className="text-xs text-slate-500">{t('dash.totalUsers')}</span>
      </div>
      <div className="flex h-2 w-full gap-[3px] overflow-hidden rounded-full bg-slate-100 dark:bg-slate-800">
        {total > 0 &&
          parts
            .filter((p) => p.n > 0)
            .map((p, i) => (
              <motion.div
                key={p.key}
                className={cx('h-full', p.color)}
                initial={{ width: 0 }}
                animate={{ width: `${(p.n / total) * 100}%` }}
                transition={{ duration: 0.7, delay: 0.2 + i * 0.06, ease: [0.22, 1, 0.36, 1] }}
                title={`${p.label}: ${p.n}`}
              />
            ))}
      </div>
      <dl className="mt-4 divide-y divide-slate-100 dark:divide-slate-800">
        {parts.map((p) => (
          <div key={p.key} className="flex items-center gap-2.5 py-2 text-sm">
            <span className={cx('h-2 w-2 rounded-full', p.color)} />
            <dt className="text-slate-600 dark:text-slate-400">{p.label}</dt>
            <dd className="ml-auto font-medium tabular-nums text-slate-900 dark:text-white">{users ? p.n : '—'}</dd>
            <dd className="w-11 text-right text-xs tabular-nums text-slate-400">{total > 0 ? `${Math.round((p.n / total) * 100)}%` : '—'}</dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

function CardHead({ title, desc, action }: { title: string; desc: string; action?: ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-3 px-5 pb-3 pt-5">
      <div>
        <h2 className="text-sm font-semibold text-slate-900 dark:text-white">{title}</h2>
        <p className="mt-0.5 text-xs text-slate-500">{desc}</p>
      </div>
      {action}
    </div>
  )
}

function NewUsers() {
  const { t, lang } = useI18n()
  const navigate = useNavigate()
  const { data } = useFetch<{ users: User[] }>('/api/users?limit=5&sort=-created_at')
  return (
    <Card className="overflow-hidden lg:col-span-2" delay={0.3}>
      <CardHead
        title={t('dash.newUsers')}
        desc={t('dash.newUsersDesc')}
        action={
          <Link to="/users" className="group inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-white">
            {t('dash.viewAll')}
            <ArrowRight className="h-3 w-3 transition-transform group-hover:translate-x-0.5" />
          </Link>
        }
      />
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-y border-slate-100 bg-slate-50/60 text-xs text-slate-500 dark:border-slate-800 dark:bg-slate-900/40">
              <th className="px-5 py-2 font-medium">{t('users.username')}</th>
              <th className="px-3 py-2 font-medium">{t('users.status')}</th>
              <th className="hidden px-3 py-2 font-medium sm:table-cell">{t('dash.traffic.col')}</th>
              <th className="px-5 py-2 text-right font-medium">{t('users.created')}</th>
            </tr>
          </thead>
          <tbody>
            {!data &&
              Array.from({ length: 3 }, (_, i) => (
                <tr key={i} className="border-b border-slate-100 last:border-0 dark:border-slate-800/70">
                  <td className="px-5 py-3" colSpan={4}>
                    <Skeleton className="h-5 w-full" />
                  </td>
                </tr>
              ))}
            {data?.users.length === 0 && (
              <tr>
                <td colSpan={4} className="px-5 py-10 text-center text-sm text-slate-400">
                  {t('dash.noData')}
                </td>
              </tr>
            )}
            {data?.users.map((u) => (
              <tr
                key={u.id}
                onClick={() => navigate(`/users?open=${u.id}`)}
                className="cursor-pointer border-b border-slate-100 transition-colors last:border-0 hover:bg-slate-50 dark:border-slate-800/70 dark:hover:bg-slate-800/30"
              >
                <td className="px-5 py-2.5">
                  <div className="flex items-center gap-2.5">
                    <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-slate-100 text-xs font-medium uppercase text-slate-600 dark:bg-slate-800 dark:text-slate-300">
                      {u.username[0]}
                    </span>
                    <div className="min-w-0">
                      <div className="truncate font-medium text-slate-900 dark:text-white">{u.username}</div>
                      {u.note && <div className="truncate text-xs text-slate-500">{u.note}</div>}
                    </div>
                  </div>
                </td>
                <td className="px-3 py-2.5">
                  <UserStatusBadge status={u.status} />
                </td>
                <td className="hidden whitespace-nowrap px-3 py-2.5 tabular-nums text-slate-600 dark:text-slate-400 sm:table-cell">
                  {formatBytes(u.used_traffic)} <span className="text-slate-400">/ {u.data_limit > 0 ? formatBytes(u.data_limit) : '∞'}</span>
                </td>
                <td className="whitespace-nowrap px-5 py-2.5 text-right text-xs text-slate-500">{relativeTime(u.created_at, lang)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  )
}

function ActivityFeed() {
  const { t, lang } = useI18n()
  const { data } = useFetch<AuditEntry[]>('/api/audit?limit=6')
  const phrase = (action: string) => {
    const key = `act.${action}` as Parameters<typeof t>[0]
    const s = t(key)
    return s === key ? action : s
  }
  return (
    <Card className="overflow-hidden" delay={0.35}>
      <CardHead
        title={t('dash.activity')}
        desc={t('dash.activityDesc')}
        action={
          <Link to="/audit" className="group inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-white">
            {t('dash.viewAll')}
            <ArrowRight className="h-3 w-3 transition-transform group-hover:translate-x-0.5" />
          </Link>
        }
      />
      <ol className="px-5 pb-4">
        {!data && Array.from({ length: 4 }, (_, i) => <Skeleton key={i} className="my-2 block h-9 w-full" />)}
        {data?.length === 0 && <li className="py-8 text-center text-sm text-slate-400">{t('dash.noData')}</li>}
        {data?.map((e, i) => (
          <motion.li
            key={e.id}
            initial={{ opacity: 0, x: -4 }}
            animate={{ opacity: 1, x: 0 }}
            transition={{ delay: 0.35 + i * 0.04 }}
            className="relative flex gap-3 pb-4 last:pb-0"
          >
            {i < data.length - 1 && <span className="absolute left-[13px] top-8 h-[calc(100%-2rem)] w-px bg-slate-200 dark:bg-slate-800" />}
            <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border border-slate-200 bg-white text-xs font-medium uppercase text-slate-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300">
              {e.admin[0] ?? '?'}
            </span>
            <div className="min-w-0 pt-0.5">
              <p className="text-sm leading-snug text-slate-600 dark:text-slate-400">
                <span className="font-medium text-slate-900 dark:text-white">{e.admin}</span> {phrase(e.action)}{' '}
                {e.target && !(e.action === 'login' && e.target === e.admin) && <span className="break-all font-medium text-slate-900 dark:text-white">{e.target}</span>}
              </p>
              <p className="mt-0.5 text-xs text-slate-400">{relativeTime(e.created_at, lang)}</p>
            </div>
          </motion.li>
        ))}
      </ol>
    </Card>
  )
}

function SystemCard({ stats, lang }: { stats: Stats | null; lang: Parameters<typeof formatDuration>[1] }) {
  const { t } = useI18n()
  const rows: [Icon, string, string | undefined][] = [
    [Clock, t('dash.uptime'), stats ? formatDuration(stats.system.uptime, lang) : undefined],
    [HardDrive, t('dash.memory'), stats ? formatBytes(stats.system.mem_alloc) : undefined],
    [Cpu, 'CPU', stats ? String(stats.system.cpus) : undefined],
    [Zap, t('dash.version'), stats?.system.version],
  ]
  if (stats?.nodes) rows.unshift([Server, t('dash.nodes'), `${stats.nodes.connected} / ${stats.nodes.total}`])
  return (
    <Card className="p-5" delay={0.35}>
      <h2 className="mb-3 text-sm font-semibold text-slate-900 dark:text-white">{t('dash.system')}</h2>
      <dl className="divide-y divide-slate-100 dark:divide-slate-800">
        {rows.map(([Icon, label, value]) => (
          <div key={label} className="flex items-center justify-between py-2 text-sm">
            <dt className="flex items-center gap-2 text-slate-500">
              <Icon className="h-4 w-4 text-slate-400" />
              {label}
            </dt>
            <dd className="font-medium tabular-nums text-slate-900 dark:text-white">{value ?? <Skeleton className="h-4 w-14" />}</dd>
          </div>
        ))}
      </dl>
    </Card>
  )
}
