import { useEffect, useState } from 'react'
import { Clock, Cpu, Gauge, HardDrive, Server, Users, Wifi, Zap } from 'lucide-react'
import { useFetch } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import { formatBytes, formatDuration } from '../lib/format'
import type { DailyTraffic, Stats } from '../lib/types'
import { Card, ErrorNote, PageHeader, cx } from '../components/ui'

export default function Dashboard() {
  const { t, lang } = useI18n()
  const { data, error, reload } = useFetch<Stats>('/api/system/stats')

  useEffect(() => {
    const id = setInterval(reload, 15_000)
    return () => clearInterval(id)
  }, [reload])

  const u = data?.users
  return (
    <>
      <PageHeader title={t('nav.dashboard')} />
      <ErrorNote error={error} />
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <Stat icon={Users} label={t('dash.users')} value={u?.total} />
        <Stat icon={Zap} label={t('dash.active')} value={u?.active} accent="text-emerald-600 dark:text-emerald-400" />
        <Stat icon={Wifi} label={t('dash.online')} value={u?.online} accent="text-brand-600 dark:text-brand-400" />
        <Stat icon={Gauge} label={t('dash.traffic')} value={u ? formatBytes(u.traffic) : undefined} />
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <Card className="p-5 lg:col-span-2">
          <h2 className="mb-4 text-sm font-medium text-slate-700 dark:text-slate-300">{t('dash.trafficChart')}</h2>
          <TrafficChart data={data?.traffic ?? []} />
        </Card>
        <div className="grid gap-4">
          <Card className="p-5">
            <div className="grid grid-cols-3 gap-3 text-center">
              <Mini label={t('dash.expired')} value={u?.expired} />
              <Mini label={t('dash.limited')} value={u?.limited} />
              <Mini label={t('dash.onHold')} value={u?.on_hold} />
            </div>
          </Card>
          <Card className="p-5">
            <h2 className="mb-3 text-sm font-medium text-slate-700 dark:text-slate-300">{t('dash.system')}</h2>
            <dl className="space-y-2 text-sm">
              {data?.nodes && <Row icon={Server} label={t('dash.nodes')} value={`${data.nodes.connected} / ${data.nodes.total}`} />}
              <Row icon={Clock} label={t('dash.uptime')} value={data ? formatDuration(data.system.uptime, lang) : '—'} />
              <Row icon={HardDrive} label={t('dash.memory')} value={data ? formatBytes(data.system.mem_alloc) : '—'} />
              <Row icon={Cpu} label="CPU" value={data ? String(data.system.cpus) : '—'} />
              <Row icon={Zap} label={t('dash.version')} value={data?.system.version ?? '—'} />
            </dl>
          </Card>
        </div>
      </div>
    </>
  )
}

function Stat({ icon: Icon, label, value, accent }: { icon: typeof Users; label: string; value?: number | string; accent?: string }) {
  return (
    <Card className="p-5">
      <div className="flex items-center justify-between">
        <span className="text-sm text-slate-500 dark:text-slate-400">{label}</span>
        <Icon className="h-4 w-4 text-slate-400" />
      </div>
      <div className={cx('mt-2 text-2xl font-semibold tabular-nums tracking-tight text-slate-900 dark:text-white', accent)}>
        {value ?? <span className="inline-block h-7 w-16 animate-pulse rounded bg-slate-100 dark:bg-slate-800" />}
      </div>
    </Card>
  )
}

function Mini({ label, value }: { label: string; value?: number }) {
  return (
    <div>
      <div className="text-xl font-semibold tabular-nums text-slate-900 dark:text-white">{value ?? '—'}</div>
      <div className="mt-0.5 text-xs text-slate-500">{label}</div>
    </div>
  )
}

function Row({ icon: Icon, label, value }: { icon: typeof Users; label: string; value: string }) {
  return (
    <div className="flex items-center justify-between">
      <dt className="flex items-center gap-2 text-slate-500 dark:text-slate-400">
        <Icon className="h-3.5 w-3.5" />
        {label}
      </dt>
      <dd className="font-medium tabular-nums text-slate-900 dark:text-white">{value}</dd>
    </div>
  )
}

// TrafficChart is a single-series daily bar chart with a per-bar hover tooltip.
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
            <span className="font-medium text-slate-900 dark:text-white">{formatBytes(h.bytes)}</span> · {label(h.day)}
          </>
        ) : (
          <>
            max <span className="font-medium text-slate-900 dark:text-white">{formatBytes(maxBytes === 1 ? 0 : maxBytes)}</span>
          </>
        )}
      </div>
      <div className="flex h-40 items-end gap-[2px] border-b border-slate-200 dark:border-slate-800" onMouseLeave={() => setHover(null)}>
        {series.map((d, i) => (
          <div
            key={d.day}
            className="flex h-full flex-1 items-end"
            onMouseEnter={() => setHover(i)}
            onTouchStart={() => setHover(i)}
            title={`${label(d.day)}: ${formatBytes(d.bytes)}`}
          >
            <div
              className={cx(
                'w-full rounded-t-[4px] transition-colors',
                hover === i ? 'bg-brand-700 dark:bg-brand-300' : 'bg-brand-500 dark:bg-brand-400',
              )}
              style={{ height: `${Math.max((d.bytes / maxBytes) * 100, d.bytes > 0 ? 2 : 0)}%` }}
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
