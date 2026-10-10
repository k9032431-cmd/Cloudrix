import { useEffect, useState } from 'react'
import { motion } from 'framer-motion'
import { Link } from 'react-router-dom'
import { ChartUp, Clock, Cpu, Gauge, HardDrive, Network, Plus, Server, Settings, Sparkle, Users, Wifi, Zap, type Icon } from '../components/icons'
import { openPalette, shortcut } from '../components/CommandPalette'
import { useAuth } from '../lib/auth'
import { useFetch } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import { formatBytes, formatDuration } from '../lib/format'
import type { DailyTraffic, Stats } from '../lib/types'
import { AnimatedNumber, Card, ErrorNote, PageHeader, Skeleton, cx } from '../components/ui'

export default function Dashboard() {
  const { t, lang } = useI18n()
  const { admin } = useAuth()
  const { data, error, reload } = useFetch<Stats>('/api/system/stats')

  useEffect(() => {
    const id = setInterval(reload, 15_000)
    return () => clearInterval(id)
  }, [reload])

  const u = data?.users
  return (
    <>
      <PageHeader title={t('nav.dashboard')} subtitle={admin ? t('dash.welcome', { name: admin.username }) : undefined} />
      <ErrorNote error={error} />
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <Stat icon={Users} label={t('dash.users')} value={u?.total} gradient="from-brand-500 to-indigo-600" delay={0} />
        <Stat icon={Zap} label={t('dash.active')} value={u?.active} gradient="from-emerald-400 to-teal-600" delay={0.06} />
        <Stat icon={Wifi} label={t('dash.online')} value={u?.online} gradient="from-sky-400 to-cyan-600" delay={0.12} live />
        <Stat icon={Gauge} label={t('dash.traffic')} value={u?.traffic} bytes gradient="from-violet-500 to-fuchsia-600" delay={0.18} />
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <Card className="p-5 lg:col-span-2" delay={0.22}>
          <div className="mb-4 flex items-center gap-2">
            <span className="flex h-7 w-7 items-center justify-center rounded-lg bg-brand-50 text-brand-600 dark:bg-brand-500/10 dark:text-brand-300">
              <ChartUp className="h-4 w-4" />
            </span>
            <h2 className="text-sm font-semibold text-slate-800 dark:text-slate-200">{t('dash.trafficChart')}</h2>
          </div>
          <TrafficChart data={data?.traffic ?? []} />
        </Card>
        <div className="grid gap-4">
          <Card className="p-5" delay={0.28} hover>
            <h2 className="mb-3 text-sm font-semibold text-slate-800 dark:text-slate-200">{t('dash.statuses')}</h2>
            <StatusBar users={u} />
          </Card>
          <Card className="p-5" delay={0.34}>
            <h2 className="mb-3 text-sm font-semibold text-slate-800 dark:text-slate-200">{t('dash.system')}</h2>
            <dl className="space-y-1">
              {data?.nodes && <Row icon={Server} label={t('dash.nodes')} value={`${data.nodes.connected} / ${data.nodes.total}`} />}
              <Row icon={Clock} label={t('dash.uptime')} value={data ? formatDuration(data.system.uptime, lang) : undefined} />
              <Row icon={HardDrive} label={t('dash.memory')} value={data ? formatBytes(data.system.mem_alloc) : undefined} />
              <Row icon={Cpu} label="CPU" value={data ? String(data.system.cpus) : undefined} />
              <Row icon={Zap} label={t('dash.version')} value={data?.system.version} />
            </dl>
          </Card>
        </div>
      </div>

      <Card className="mt-4 p-5" delay={0.4}>
        <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold text-slate-800 dark:text-slate-200">
          <Sparkle className="h-4 w-4 text-violet-500" />
          {t('dash.quick')}
        </h2>
        <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
          <Quick to="/users?new=1" icon={Plus} label={t('users.new')} gradient="from-brand-500 to-indigo-600" />
          {admin?.role === 'sudo' && <Quick to="/inbounds" icon={Network} label={t('inbounds.new')} gradient="from-emerald-400 to-teal-600" />}
          {admin?.role === 'sudo' && <Quick to="/settings" icon={Settings} label={t('nav.settings')} gradient="from-violet-500 to-fuchsia-600" />}
          <button onClick={openPalette} className="group flex items-center gap-3 rounded-xl border border-dashed border-slate-200 p-3 text-left text-sm text-slate-500 transition-all duration-200 hover:border-brand-400 hover:text-brand-600 dark:border-white/10 dark:hover:border-brand-500/50 dark:hover:text-brand-300">
            <span className="flex h-9 min-w-9 items-center justify-center rounded-lg bg-slate-100 px-1.5 font-mono text-[10px] font-semibold dark:bg-white/5">{shortcut}</span>
            {t('cmd.search')}
          </button>
        </div>
      </Card>
    </>
  )
}

function Quick({ to, icon: Icon, label, gradient }: { to: string; icon: Icon; label: string; gradient: string }) {
  return (
    <Link
      to={to}
      className="group flex items-center gap-3 rounded-xl border border-slate-200/70 bg-white/60 p-3 text-sm font-medium text-slate-700 transition-all duration-200 hover:-translate-y-0.5 hover:border-transparent hover:shadow-lg dark:border-white/[0.06] dark:bg-white/[0.02] dark:text-slate-200"
    >
      <span className={cx('flex h-9 w-9 items-center justify-center rounded-lg bg-gradient-to-br text-white shadow-md transition-transform duration-300 group-hover:scale-110 group-hover:rotate-3', gradient)}>
        <Icon className="h-4 w-4" />
      </span>
      {label}
    </Link>
  )
}

// StatusBar shows how users split across statuses as one stacked bar plus a legend.
function StatusBar({ users }: { users?: Stats['users'] }) {
  const { t } = useI18n()
  const parts = [
    { key: 'active', n: users?.active ?? 0, color: 'bg-emerald-500', label: t('status.active') },
    { key: 'limited', n: users?.limited ?? 0, color: 'bg-amber-500', label: t('status.limited') },
    { key: 'expired', n: users?.expired ?? 0, color: 'bg-rose-500', label: t('status.expired') },
    { key: 'on_hold', n: users?.on_hold ?? 0, color: 'bg-violet-500', label: t('status.on_hold') },
    { key: 'disabled', n: users?.disabled ?? 0, color: 'bg-slate-400', label: t('status.disabled') },
  ]
  const total = parts.reduce((a, p) => a + p.n, 0)
  return (
    <div>
      <div className="flex h-3 w-full gap-[2px] overflow-hidden rounded-full bg-slate-100 dark:bg-slate-800">
        {total > 0 &&
          parts
            .filter((p) => p.n > 0)
            .map((p, i) => (
              <motion.div
                key={p.key}
                className={cx('h-full first:rounded-l-full last:rounded-r-full', p.color)}
                initial={{ width: 0 }}
                animate={{ width: `${(p.n / total) * 100}%` }}
                transition={{ duration: 0.8, delay: 0.3 + i * 0.08, ease: [0.22, 1, 0.36, 1] }}
                title={`${p.label}: ${p.n}`}
              />
            ))}
      </div>
      <dl className="mt-3 grid grid-cols-2 gap-x-4 gap-y-1.5 text-xs">
        {parts.map((p) => (
          <div key={p.key} className="flex items-center gap-2">
            <span className={cx('h-2 w-2 rounded-full', p.color)} />
            <dt className="text-slate-500 dark:text-slate-400">{p.label}</dt>
            <dd className="ml-auto font-mono font-medium tabular-nums text-slate-900 dark:text-white">{users ? p.n : '—'}</dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

function Stat({
  icon: Icon,
  label,
  value,
  gradient,
  delay,
  bytes,
  live,
}: {
  icon: Icon
  label: string
  value?: number
  gradient: string
  delay: number
  bytes?: boolean
  live?: boolean
}) {
  return (
    <Card className="group relative overflow-hidden p-5" delay={delay} hover>
      {/* soft glow in the corner, brighter on hover */}
      <div className={cx('pointer-events-none absolute -right-10 -top-10 h-32 w-32 rounded-full bg-gradient-to-br opacity-[0.12] blur-2xl transition-opacity duration-500 group-hover:opacity-25', gradient)} />
      <div className="relative flex items-start justify-between gap-2">
        <span className="min-w-0 break-words text-sm font-medium text-slate-500 dark:text-slate-400">{label}</span>
        <span className={cx('flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br text-white shadow-lg transition-transform duration-300 group-hover:rotate-6 group-hover:scale-110', gradient)}>
          <Icon className="h-5 w-5" />
        </span>
      </div>
      <div className="relative mt-3 flex items-center gap-2 font-mono text-3xl font-semibold tabular-nums tracking-tight text-slate-900 dark:text-white">
        {value === undefined ? <Skeleton className="h-8 w-20" /> : <AnimatedNumber value={value} format={bytes ? (n) => formatBytes(n) : undefined} />}
        {live && value !== undefined && (
          <span className="relative ml-1 flex h-2.5 w-2.5">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
            <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-emerald-500" />
          </span>
        )}
      </div>
    </Card>
  )
}

function Row({ icon: Icon, label, value }: { icon: Icon; label: string; value?: string }) {
  return (
    <div className="flex items-center justify-between rounded-lg px-2 py-1.5 text-sm transition-colors hover:bg-slate-50 dark:hover:bg-white/[0.03]">
      <dt className="flex items-center gap-2 text-slate-500 dark:text-slate-400">
        <Icon className="h-4 w-4 text-slate-400" />
        {label}
      </dt>
      <dd className="font-medium tabular-nums text-slate-900 dark:text-white">{value ?? <Skeleton className="h-4 w-14" />}</dd>
    </div>
  )
}

// TrafficChart is a single-series daily bar chart; bars grow in one after another.
export function TrafficChart({ data, days = 30 }: { data: DailyTraffic[]; days?: number }) {
  const { lang } = useI18n()
  const [hover, setHover] = useState<number | null>(null)

  const byDay = new Map(data.map((d) => [d.day, d.bytes]))
  const series: DailyTraffic[] = []
  for (let i = days - 1; i >= 0; i--) {
    const day = new Date(Date.now() - i * 86_400_000).toISOString().slice(0, 10)
    series.push({ day, bytes: byDay.get(day) ?? 0 })
  }
  const maxBytes = Math.max(...series.map((d) => d.bytes), 1)
  const label = (day: string) => new Date(day + 'T00:00:00Z').toLocaleDateString(lang, { day: 'numeric', month: 'short', timeZone: 'UTC' })
  const h = hover !== null ? series[hover] : null

  return (
    <div>
      <div className="mb-2 h-5 text-xs text-slate-500 dark:text-slate-400">
        {h ? (
          <>
            <span className="font-semibold text-slate-900 dark:text-white">{formatBytes(h.bytes)}</span> · {label(h.day)}
          </>
        ) : (
          <>
            max <span className="font-semibold text-slate-900 dark:text-white">{formatBytes(maxBytes === 1 ? 0 : maxBytes)}</span>
          </>
        )}
      </div>
      <div className="relative flex h-44 items-end gap-[2px] lg:h-72 border-b border-slate-200 dark:border-slate-800" onMouseLeave={() => setHover(null)}>
        {/* recessive grid */}
        {[0.25, 0.5, 0.75].map((y) => (
          <div key={y} className="pointer-events-none absolute inset-x-0 border-t border-dashed border-slate-100 dark:border-white/[0.04]" style={{ bottom: `${y * 100}%` }} />
        ))}
        {series.map((d, i) => (
          <div
            key={d.day}
            className="relative flex h-full flex-1 items-end"
            onMouseEnter={() => setHover(i)}
            onTouchStart={() => setHover(i)}
            title={`${label(d.day)}: ${formatBytes(d.bytes)}`}
          >
            <motion.div
              className={cx(
                'w-full origin-bottom rounded-t-[4px] bg-gradient-to-t transition-[filter] duration-200',
                hover === i ? 'from-brand-600 to-violet-500 brightness-110' : 'from-brand-500 to-brand-400 dark:from-brand-500 dark:to-brand-300',
              )}
              style={{ height: `${Math.max((d.bytes / maxBytes) * 100, d.bytes > 0 ? 2 : 0)}%` }}
              initial={{ scaleY: 0 }}
              animate={{ scaleY: 1 }}
              transition={{ duration: 0.6, delay: 0.25 + i * 0.015, ease: [0.22, 1, 0.36, 1] }}
            />
          </div>
        ))}
      </div>
      <div className="mt-1.5 flex justify-between text-[11px] text-slate-400">
        <span>{label(series[0].day)}</span>
        <span>{label(series[series.length - 1].day)}</span>
      </div>
    </div>
  )
}
