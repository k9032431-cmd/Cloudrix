import { useEffect, useLayoutEffect, useMemo, useRef, useState, type CSSProperties, type ReactNode } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { ChevronRight, Clock, Cpu, Export, HardDrive, Hourglass, Server, TrendingDown, TrendingUp, Users, Wifi, Zap, ChartUp, type Icon } from '../components/icons'
import { UserStatusBadge } from '../components/StatusBadge'
import { useToast } from '../components/toast'
import { useAuth } from '../lib/auth'
import { useFetch } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import { formatBytes, formatDuration, relativeTime } from '../lib/format'
import type { AuditEntry, DailyTraffic, Stats, User } from '../lib/types'
import { AnimatedNumber, Button, ErrorNote, PageHeader, Segmented, Skeleton, cx } from '../components/ui'

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

interface Bucket {
  start: string
  end: string
  bytes: number
}

// bucketize groups consecutive days; long ranges read better as weeks.
function bucketize(days: DailyTraffic[], size: number): Bucket[] {
  const out: Bucket[] = []
  for (let i = 0; i < days.length; i += size) {
    const part = days.slice(i, i + size)
    out.push({ start: part[0].day, end: part[part.length - 1].day, bytes: part.reduce((a, d) => a + d.bytes, 0) })
  }
  return out
}

const sum = (s: { bytes: number }[]) => s.reduce((a, d) => a + d.bytes, 0)
// change is null when there is no base to compare with (never NaN or Infinity)
const change = (cur: number, prev: number) => (prev > 0 ? ((cur - prev) / prev) * 100 : null)

export default function Dashboard() {
  const { t, lang } = useI18n()
  const { admin } = useAuth()
  const toast = useToast()
  const sudo = admin?.role === 'sudo'
  const [period, setPeriod] = useState<Period>('30')
  const [hidden, setHidden] = useState<Set<string>>(new Set())
  const days = Number(period)
  const { data, error, reload } = useFetch<Stats>(`/api/system/stats?days=${days}`)

  useEffect(() => {
    const id = setInterval(reload, 30_000)
    return () => clearInterval(id)
  }, [reload])

  const loaded = data?.days === days
  const { cur, prev } = useMemo(() => {
    const all = daySeries(loaded ? data.traffic : [], days * 2)
    return { cur: all.slice(days), prev: all.slice(0, days) }
  }, [data, days, loaded])
  const curTotal = sum(cur)
  const delta = change(curTotal, sum(prev))
  const u = data?.users

  const exportCSV = () => {
    const rows = ['date,bytes,previous_period_bytes', ...cur.map((d, i) => `${d.day},${d.bytes},${prev[i].bytes}`)]
    const url = URL.createObjectURL(new Blob([rows.join('\n') + '\n'], { type: 'text/csv' }))
    const a = document.createElement('a')
    a.href = url
    a.download = `cloudrix-traffic-${days}d.csv`
    a.click()
    URL.revokeObjectURL(url)
  }

  const toggleSeries = (key: string) => {
    const next = new Set(hidden)
    if (next.has(key)) next.delete(key)
    else if (2 - next.size <= 1) return toast(t('dash.oneSeries'), 'info')
    else next.add(key)
    setHidden(next)
  }

  return (
    <>
      <PageHeader
        title={t('nav.dashboard')}
        subtitle={t('dash.subtitle')}
        actions={
          <>
            <Segmented<Period>
              label={t('dash.trafficTitle')}
              value={period}
              onChange={setPeriod}
              options={(['7', '30', '90', '365'] as Period[]).map((p) => ({ value: p, label: t(`dash.period.${p}`) }))}
            />
            <Button variant="secondary" onClick={exportCSV} disabled={!loaded} aria-label={t('dash.export')}>
              <Export />
              <span className="hide-sm">{t('dash.export')}</span>
            </Button>
          </>
        }
      />
      <ErrorNote error={error} />

      <div className="grid12">
        <Kpi
          d={1}
          icon={ChartUp}
          label={t('dash.periodTraffic')}
          short={t('dash.kpiShort.traffic')}
          value={loaded ? curTotal : undefined}
          format={(n) => formatBytes(n)}
          meta={
            <>
              {delta !== null ? <Trend value={delta} /> : <span>—</span>}
              <span className="vs">{t('dash.vsPrev')}</span>
            </>
          }
          spark={loaded ? cur.map((d) => d.bytes) : undefined}
        />
        <Kpi
          d={2}
          icon={Users}
          label={t('dash.activeUsers')}
          short={t('dash.kpiShort.active')}
          value={u?.active}
          meta={u ? <span>{t('dash.ofTotal', { n: u.total })}</span> : undefined}
          meter={u && u.total ? u.active / u.total : 0}
        />
        <Kpi
          d={3}
          icon={Wifi}
          label={t('dash.onlineNow')}
          short={t('dash.kpiShort.online')}
          value={u?.online}
          live
          meta={u ? <span>{t('dash.ofActive', { pct: u.active > 0 ? Math.round((u.online / u.active) * 100) : 0 })}</span> : undefined}
          meter={u && u.active ? u.online / u.active : 0}
        />
        <Kpi
          d={4}
          icon={Hourglass}
          label={t('dash.expiring')}
          short={t('dash.kpiShort.expiring')}
          value={u?.expiring_soon}
          meta={u ? <span>{t('dash.expiredNow', { n: u.expired })}</span> : undefined}
          meter={u && u.active ? u.expiring_soon / u.active : 0}
          meterTone="warn"
        />
      </div>

      <div className="grid12 mt-4">
        <article className="card fill rise col-8" style={{ '--d': 5 } as CSSProperties}>
          <div className="card-head">
            <div>
              <h2 className="card-title">{t('dash.trafficTitle')}</h2>
              <p className="card-desc">{t('dash.daily', { n: days })}</p>
            </div>
            <div className="legend">
              {[
                { key: 'cur', name: t('dash.legend.current'), color: 'var(--chart-1)', dash: false },
                { key: 'prev', name: t('dash.legend.previous'), color: 'var(--chart-4)', dash: true },
              ].map((s) => (
                <button type="button" key={s.key} aria-pressed={!hidden.has(s.key)} onClick={() => toggleSeries(s.key)}>
                  <span className={cx('sw', s.dash && 'dash')} style={{ background: s.color, color: s.color }} />
                  {s.name}
                </button>
              ))}
            </div>
          </div>
          <div className="card-body">
            <TrafficChart data={loaded ? data.traffic : []} days={days} compare hidden={hidden} />
          </div>
        </article>
        <article className="card rise col-4" style={{ '--d': 6 } as CSSProperties}>
          <div className="card-head">
            <div>
              <h2 className="card-title">{t('dash.byStatus')}</h2>
              <p className="card-desc">{t('dash.totalUsers')}</p>
            </div>
          </div>
          <div className="card-body">
            <StatusBreakdown users={u} />
          </div>
        </article>
      </div>

      <div className="grid12 mt-4">
        <NewUsers />
        {sudo ? <ActivityFeed /> : <SystemCard stats={data} lang={lang} />}
      </div>
    </>
  )
}

function Trend({ value }: { value: number }) {
  const up = value >= 0
  const Arrow = up ? TrendingUp : TrendingDown
  const v = Math.abs(value)
  return (
    <span className={cx('trend trend-chip', up ? 'up' : 'down')}>
      <Arrow />
      {up ? '+' : '−'}
      {v >= 1000 ? '999+' : v.toFixed(1)}%
    </span>
  )
}

function Kpi({
  d,
  icon: Icon,
  label,
  short,
  value,
  format,
  meta,
  spark,
  meter,
  meterTone,
  live,
}: {
  d: number
  icon: Icon
  label: string
  short: string
  value?: number
  format?: (n: number) => string
  meta?: ReactNode
  spark?: number[]
  meter?: number
  meterTone?: 'warn'
  live?: boolean
}) {
  return (
    <div className="card kpi rise col-3" style={{ '--d': d } as CSSProperties}>
      <div className="kpi-top">
        <span className="kpi-label">
          <Icon />
          <span className="full">{label}</span>
          <span className="short">{short}</span>
        </span>
      </div>
      <div className="kpi-value">
        {value === undefined ? <Skeleton className="h-8 w-24" /> : <AnimatedNumber value={value} format={format} />}
        {live && value !== undefined && <span className="live-dot" aria-hidden />}
      </div>
      <div className="kpi-meta">{meta ?? <Skeleton className="h-4 w-28" />}</div>
      {spark ? (
        <div className="kpi-spark">
          <Sparkline values={spark} />
        </div>
      ) : spark === undefined && meter === undefined ? (
        <div className="kpi-spark" />
      ) : (
        <div className={cx('meter mb-[18px] mt-4', meterTone)} aria-hidden>
          <i style={{ '--v': Math.min(1, meter ?? 0) } as CSSProperties} />
        </div>
      )}
    </div>
  )
}

/* ---- chart engine (after the slate template): SVG drawn at the element's
   pixel size, colours as CSS variables so theme switches need no redraw ---- */

const f1 = (v: number) => Math.round(v * 10) / 10
const clamp = (v: number, a: number, b: number) => Math.min(b, Math.max(a, v))

// Monotone cubic interpolation: smooth curves that never overshoot the data.
function monoPath(pts: [number, number][]) {
  const n = pts.length
  if (n < 2) return n ? `M${f1(pts[0][0])},${f1(pts[0][1])}` : ''
  const dx: number[] = []
  const m: number[] = []
  const tg: number[] = []
  for (let i = 0; i < n - 1; i++) {
    dx[i] = pts[i + 1][0] - pts[i][0]
    m[i] = (pts[i + 1][1] - pts[i][1]) / (dx[i] || 1)
  }
  tg[0] = m[0]
  tg[n - 1] = m[n - 2]
  for (let i = 1; i < n - 1; i++) {
    if (m[i - 1] * m[i] <= 0) tg[i] = 0
    else {
      const w1 = 2 * dx[i] + dx[i - 1]
      const w2 = dx[i] + 2 * dx[i - 1]
      tg[i] = (w1 + w2) / (w1 / m[i - 1] + w2 / m[i])
    }
  }
  let d = `M${f1(pts[0][0])},${f1(pts[0][1])}`
  for (let i = 0; i < n - 1; i++) {
    const h = dx[i] / 3
    d += `C${f1(pts[i][0] + h)},${f1(pts[i][1] + tg[i] * h)} ${f1(pts[i + 1][0] - h)},${f1(pts[i + 1][1] - tg[i + 1] * h)} ${f1(pts[i + 1][0])},${f1(pts[i + 1][1])}`
  }
  return d
}

// niceScale picks an axis maximum whose quarter steps are round numbers in the
// value's own unit (KB, MB, GB...), e.g. 9.7 GB -> 10 GB in 2.5 GB steps.
function niceScale(v: number, ticks = 4) {
  if (!(v > 0)) return { max: 4 * 1024 ** 3, step: 1024 ** 3 }
  const unit = 1024 ** Math.max(0, Math.floor(Math.log(v) / Math.log(1024)))
  const raw = v / unit / ticks
  const mag = Math.pow(10, Math.floor(Math.log10(raw)))
  const step = [1, 2, 2.5, 5, 10].map((x) => x * mag).find((s) => s * ticks >= v / unit)! * unit
  return { max: step * ticks, step }
}
const axisLabel = (b: number) => (b === 0 ? '0' : formatBytes(b, 1).replace('.0 ', ' '))

function useSize<T extends HTMLElement>() {
  const ref = useRef<T>(null)
  const [size, setSize] = useState({ w: 0, h: 0 })
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const measure = () => setSize((s) => (Math.abs(s.w - el.clientWidth) > 1 || Math.abs(s.h - el.clientHeight) > 1 ? { w: el.clientWidth, h: el.clientHeight } : s))
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  return { ref, ...size }
}

const reduced = () => typeof matchMedia !== 'undefined' && matchMedia('(prefers-reduced-motion: reduce)').matches

// TrafficChart draws daily traffic as an area line. With `compare` it expects
// 2×days of history and adds the previous period as a dashed line, aligned
// day by day. Hover or arrow keys show a tooltip with the change.
export function TrafficChart({ data, days = 30, compare = false, hidden }: { data: DailyTraffic[]; days?: number; compare?: boolean; hidden?: Set<string> }) {
  const { t, lang } = useI18n()
  const { ref, w: W, h: H } = useSize<HTMLDivElement>()
  const tip = useRef<HTMLDivElement>(null)
  const [idx, setIdx] = useState(-1)
  const [drawn, setDrawn] = useState(false)
  const size = days > 120 ? 7 : 1

  const { cur, prev } = useMemo(() => {
    const all = daySeries(data, compare ? days * 2 : days)
    return {
      cur: bucketize(compare ? all.slice(days) : all, size),
      prev: compare ? bucketize(all.slice(0, days), size) : null,
    }
  }, [data, days, compare, size])

  const showCur = !hidden?.has('cur')
  const showPrev = !!prev && !hidden?.has('prev')
  const n = cur.length
  const narrow = W < 520
  const pad = { l: narrow ? 42 : 52, r: 10, t: 10, b: 28 }
  const pw = Math.max(10, W - pad.l - pad.r)
  const ph = Math.max(10, H - pad.t - pad.b)
  const vis = [...(showCur ? cur : []), ...(showPrev && prev ? prev : [])].map((b) => b.bytes)
  const { max, step } = niceScale(Math.max(1, ...vis) * 1.04)
  const x = (i: number) => pad.l + (n === 1 ? pw / 2 : (i * pw) / (n - 1))
  const y = (v: number) => pad.t + ph - (v / max) * ph
  const curD = monoPath(cur.map((b, i) => [x(i), y(b.bytes)]))
  const prevD = prev ? monoPath(prev.map((b, i) => [x(i), y(b.bytes)])) : ''
  const empty = sum(cur) === 0 && (!prev || sum(prev) === 0)

  const fmtDay = (day: string, opts: Intl.DateTimeFormatOptions = { day: 'numeric', month: 'short' }) =>
    new Date(day + 'T00:00:00Z').toLocaleDateString(lang, { ...opts, timeZone: 'UTC' })
  const label = (b: Bucket) => (size === 1 ? fmtDay(b.end) : fmtDay(b.start))
  const title = (b: Bucket) => (size === 1 ? fmtDay(b.end, { weekday: 'short', day: 'numeric', month: 'short' }) : `${fmtDay(b.start)} – ${fmtDay(b.end)}`)

  // replay the left-to-right reveal when the range changes, not on resize
  useEffect(() => {
    if (reduced()) return setDrawn(true)
    setDrawn(false)
    let id = requestAnimationFrame(() => (id = requestAnimationFrame(() => setDrawn(true))))
    return () => cancelAnimationFrame(id)
  }, [days, compare])

  const ticks: number[] = []
  for (let v = 0; v <= max + 1e-6; v += step) ticks.push(v)
  const longest = Math.max(...cur.map((b) => label(b).length), 4)
  const every = Math.ceil(n / Math.max(2, Math.floor(pw / Math.max(64, longest * 7 + 26))))

  const pick = (clientX: number) => {
    const r = ref.current!.getBoundingClientRect()
    const px = clientX - r.left
    if (px < pad.l - 8 || px > W - pad.r + 8) return -1
    return clamp(Math.round(((px - pad.l) / pw) * (n - 1)), 0, n - 1)
  }
  const onKey = (e: React.KeyboardEvent) => {
    const map: Record<string, number> = { ArrowLeft: -1, ArrowRight: 1 }
    if (e.key in map) {
      e.preventDefault()
      setIdx((i) => clamp((i < 0 ? n - 1 : i) + map[e.key], 0, n - 1))
    } else if (e.key === 'Home') {
      e.preventDefault()
      setIdx(0)
    } else if (e.key === 'End') {
      e.preventDefault()
      setIdx(n - 1)
    }
  }

  // position the tooltip beside the point, flipping at the right edge
  useLayoutEffect(() => {
    const el = tip.current
    if (!el || idx < 0) return
    const tw = el.offsetWidth
    const th = el.offsetHeight
    const cx = x(idx)
    let tx = cx + 16
    if (tx + tw > W - 4) tx = cx - tw - 16
    tx = clamp(tx, 0, Math.max(0, W - tw))
    const anchors = [showCur ? y(cur[idx].bytes) : H, showPrev && prev ? y(prev[idx].bytes) : H]
    const ty = clamp(Math.min(...anchors) - th / 2, 0, Math.max(0, H - th - 24))
    el.style.translate = `${Math.round(tx)}px ${Math.round(ty)}px`
  })

  const hb = idx >= 0 ? cur[idx] : null
  const hp = idx >= 0 && prev ? prev[idx] : null
  const hd = hb && hp ? change(hb.bytes, hp.bytes) : null
  const vb = { width: W, height: H, viewBox: `0 0 ${W} ${H}` }

  return (
    <div
      ref={ref}
      className={cx('chart', idx >= 0 && 'is-hover', drawn && 'drawn')}
      tabIndex={0}
      role="img"
      aria-label={t('dash.chartAria')}
      onPointerMove={(e) => setIdx(pick(e.clientX))}
      onPointerDown={(e) => setIdx(pick(e.clientX))}
      onPointerLeave={() => setIdx(-1)}
      onFocus={() => setIdx((i) => (i < 0 ? n - 1 : i))}
      onBlur={() => setIdx(-1)}
      onKeyDown={onKey}
    >
      {W > 0 && (
        <>
          <svg {...vb} style={{ position: 'absolute', inset: 0 }} aria-hidden>
            {ticks.map((v) => {
              const yy = Math.round(y(v)) + 0.5
              return (
                <g key={v}>
                  <line className={cx('grid-line', v === 0 && 'base')} x1={pad.l} x2={W - pad.r} y1={yy} y2={yy} />
                  <text className="axis" x={pad.l - 10} y={yy + 3.5} textAnchor="end">
                    {axisLabel(v)}
                  </text>
                </g>
              )
            })}
            {cur.map((b, i) =>
              i % every === 0 ? (
                <text key={b.end} className="axis" x={f1(x(i))} y={H - 8} textAnchor={i === 0 ? 'start' : 'middle'}>
                  {label(b)}
                </text>
              ) : null,
            )}
          </svg>
          <div className="reveal-clip" style={{ position: 'absolute', inset: 0 }}>
            <svg {...vb} aria-hidden>
              <defs>
                <linearGradient id="traffic-area" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0" style={{ stopColor: 'var(--chart-1)', stopOpacity: 0.2 }} />
                  <stop offset=".75" style={{ stopColor: 'var(--chart-1)', stopOpacity: 0.04 }} />
                  <stop offset="1" style={{ stopColor: 'var(--chart-1)', stopOpacity: 0 }} />
                </linearGradient>
              </defs>
              {prev && (
                <g className={cx('series', !showPrev && 'off')}>
                  <path d={prevD} fill="none" style={{ stroke: 'var(--chart-4)' }} strokeWidth={1.6} strokeDasharray="3 4" strokeLinejoin="round" strokeLinecap="round" />
                </g>
              )}
              <g className={cx('series', !showCur && 'off')}>
                {!empty && <path d={`${curD}L${f1(x(n - 1))},${f1(y(0))}L${f1(x(0))},${f1(y(0))}Z`} style={{ fill: 'url(#traffic-area)' }} />}
                <path d={curD} fill="none" style={{ stroke: 'var(--chart-1)' }} strokeWidth={2} strokeLinejoin="round" strokeLinecap="round" />
              </g>
            </svg>
          </div>
          <svg {...vb} style={{ position: 'absolute', inset: 0 }} aria-hidden>
            <line className="xhair" y1={pad.t} y2={H - pad.b} x1={idx >= 0 ? f1(x(idx)) : 0} x2={idx >= 0 ? f1(x(idx)) : 0} />
            {hp && showPrev && <circle className="hover-dot" r={4} cx={f1(x(idx))} cy={f1(y(hp.bytes))} style={{ fill: 'var(--chart-4)' }} />}
            {hb && showCur && <circle className="hover-dot" r={4} cx={f1(x(idx))} cy={f1(y(hb.bytes))} style={{ fill: 'var(--chart-1)' }} />}
          </svg>
          {empty && <div className="chart-empty">{t('dash.noData')}</div>}
          <div ref={tip} className="chart-tip" aria-hidden>
            {hb && (
              <>
                <div className="tip-title">{title(hb)}</div>
                {showCur && (
                  <div className="tip-row">
                    <span className="sw" style={{ background: 'var(--chart-1)' }} />
                    {prev ? t('dash.legend.current') : t('dash.traffic.col')}
                    <span className="v">{formatBytes(hb.bytes)}</span>
                  </div>
                )}
                {hp && showPrev && (
                  <div className="tip-row">
                    <span className="sw line" style={{ background: 'var(--chart-4)' }} />
                    {title(hp)}
                    <span className="v">{formatBytes(hp.bytes)}</span>
                  </div>
                )}
                {hp && (
                  <div className="tip-foot">
                    {hd === null ? '—' : <Trend value={hd} />} {t('dash.vsPrevShort')}
                  </div>
                )}
              </>
            )}
          </div>
        </>
      )}
    </div>
  )
}

// Sparkline stretches to its box; the stroke stays crisp.
function Sparkline({ values }: { values: number[] }) {
  const min = Math.min(...values)
  const max = Math.max(...values)
  const span = max - min || 1
  const pts = values.map((v, i) => [(i / (values.length - 1 || 1)) * 100, 36 - ((v - min) / span) * 28 + 2] as [number, number])
  const d = monoPath(pts)
  return (
    <svg viewBox="0 0 100 40" preserveAspectRatio="none" aria-hidden>
      <defs>
        <linearGradient id="kpi-spark" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" style={{ stopColor: 'var(--chart-1)', stopOpacity: 0.2 }} />
          <stop offset="1" style={{ stopColor: 'var(--chart-1)', stopOpacity: 0 }} />
        </linearGradient>
      </defs>
      <path d={`${d}L100,40L0,40Z`} style={{ fill: 'url(#kpi-spark)' }} />
      <path d={d} fill="none" style={{ stroke: 'var(--chart-1)' }} strokeWidth={1.6} vectorEffect="non-scaling-stroke" strokeLinejoin="round" />
    </svg>
  )
}

function StatusBreakdown({ users }: { users?: Stats['users'] }) {
  const { t } = useI18n()
  const parts = [
    { key: 'active', n: users?.active ?? 0, color: 'var(--success)', label: t('status.active') },
    { key: 'limited', n: users?.limited ?? 0, color: 'var(--warning)', label: t('status.limited') },
    { key: 'expired', n: users?.expired ?? 0, color: 'var(--destructive)', label: t('status.expired') },
    { key: 'on_hold', n: users?.on_hold ?? 0, color: 'var(--violet)', label: t('status.on_hold') },
    { key: 'disabled', n: users?.disabled ?? 0, color: 'var(--chart-5)', label: t('status.disabled') },
  ]
  const total = parts.reduce((a, p) => a + p.n, 0)
  return (
    <div>
      <div className="mb-3 flex items-baseline gap-2">
        <span className="text-2xl font-semibold tabular-nums tracking-tight text-foreground">{users ? <AnimatedNumber value={total} /> : <Skeleton className="h-8 w-16" />}</span>
      </div>
      <div className="stackbar" aria-hidden>
        {total > 0 &&
          parts
            .filter((p) => p.n > 0)
            .map((p, i) => <i key={p.key} style={{ width: `${(p.n / total) * 100}%`, background: p.color, '--i': i } as CSSProperties} title={`${p.label}: ${p.n}`} />)}
      </div>
      <div className="rows mt-3">
        {parts.map((p) => (
          <div key={p.key} className="r">
            <span className="sw" style={{ background: p.color }} />
            <span className="name">{p.label}</span>
            <span className="v">{users ? p.n : '—'}</span>
            <span className="s">{total > 0 ? `${Math.round((p.n / total) * 100)}%` : '—'}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

function CardHead({ title, desc, to }: { title: string; desc: string; to?: string }) {
  const { t } = useI18n()
  return (
    <div className="card-head">
      <div>
        <h2 className="card-title">{title}</h2>
        <p className="card-desc">{desc}</p>
      </div>
      {to && (
        <Link to={to} className="btn btn-ghost btn-sm">
          {t('dash.viewAll')}
          <ChevronRight />
        </Link>
      )}
    </div>
  )
}

function NewUsers() {
  const { t, lang } = useI18n()
  const navigate = useNavigate()
  const { data } = useFetch<{ users: User[] }>('/api/users?limit=6&sort=-created_at')
  return (
    <article className="card rise col-8 overflow-hidden" style={{ '--d': 7 } as CSSProperties}>
      <CardHead title={t('dash.newUsers')} desc={t('dash.newUsersDesc')} to="/users" />
      <div className="table-wrap mt-3">
        <table className="dt">
          <thead>
            <tr>
              <th>{t('users.username')}</th>
              <th>{t('users.status')}</th>
              <th className="hide-sm r">{t('dash.traffic.col')}</th>
              <th className="hide-sm r">{t('users.created')}</th>
            </tr>
          </thead>
          <tbody>
            {!data &&
              Array.from({ length: 4 }, (_, i) => (
                <tr key={i}>
                  <td colSpan={4}>
                    <Skeleton className="h-5 w-full" />
                  </td>
                </tr>
              ))}
            {data?.users.length === 0 && (
              <tr>
                <td colSpan={4} className="!h-28 text-center text-muted-foreground">
                  {t('dash.noData')}
                </td>
              </tr>
            )}
            {data?.users.map((u) => (
              <tr
                key={u.id}
                className="clickable"
                tabIndex={0}
                onClick={() => navigate(`/users?open=${u.id}`)}
                onKeyDown={(e) => e.key === 'Enter' && navigate(`/users?open=${u.id}`)}
              >
                <td>
                  <div className="ent">
                    <span className="av">{u.username[0]}</span>
                    <div>
                      <div className="ent-name">{u.username}</div>
                      {u.note && <div className="ent-sub">{u.note}</div>}
                    </div>
                  </div>
                </td>
                <td>
                  <UserStatusBadge status={u.status} />
                </td>
                <td className="hide-sm r whitespace-nowrap tabular-nums">
                  {formatBytes(u.used_traffic)} <span className="text-faint">/ {u.data_limit > 0 ? formatBytes(u.data_limit) : '∞'}</span>
                </td>
                <td className="hide-sm r whitespace-nowrap text-muted-foreground">{relativeTime(u.created_at, lang)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </article>
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
    <article className="card rise col-4" style={{ '--d': 8 } as CSSProperties}>
      <CardHead title={t('dash.activity')} desc={t('dash.activityDesc')} to="/audit" />
      <div className="card-body !pt-1.5">
        <div className="feed">
          {!data && Array.from({ length: 4 }, (_, i) => <Skeleton key={i} className="my-2 block h-9 w-full" />)}
          {data?.length === 0 && <p className="py-8 text-center text-[13px] text-muted-foreground">{t('dash.noData')}</p>}
          {data?.map((e) => (
            <div key={e.id} className="feed-item">
              <span className="av">{e.admin[0] ?? '?'}</span>
              <div className="min-w-0">
                <p className="feed-text">
                  <b>{e.admin}</b> {phrase(e.action)} {e.target && !(e.action === 'login' && e.target === e.admin) && <b>{e.target}</b>}
                </p>
                <p className="feed-time">{relativeTime(e.created_at, lang)}</p>
              </div>
            </div>
          ))}
        </div>
      </div>
    </article>
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
    <article className="card rise col-4" style={{ '--d': 8 } as CSSProperties}>
      <CardHead title={t('dash.system')} desc={t('nav.workspace')} />
      <div className="card-body">
        <div className="rows">
          {rows.map(([Icon, label, value]) => (
            <div key={label} className="r">
              <Icon className="h-4 w-4 text-faint" />
              <span className="name text-muted-foreground">{label}</span>
              <span className="v">{value ?? <Skeleton className="h-4 w-14" />}</span>
            </div>
          ))}
        </div>
      </div>
    </article>
  )
}
