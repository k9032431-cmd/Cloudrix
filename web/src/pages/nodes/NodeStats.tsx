import { useMemo, useState, type CSSProperties } from 'react'
import { ArrowDown, ArrowUp, Users, Wifi } from '../../components/icons'
import { Modal, Segmented, Skeleton, cx } from '../../components/ui'
import { useFetch } from '../../lib/hooks'
import { useI18n } from '../../lib/i18n'
import { formatBytes } from '../../lib/format'
import type { DailyTraffic, Node } from '../../lib/types'
import { TrafficChart, Trend } from '../Dashboard'

export type StatsPeriod = '7' | '30' | '90'

export interface NodeUsage {
  id: number
  name: string
  status: Node['status']
  up: number
  down: number
  users: number
  online: number
}

interface NodesStats {
  days: number
  nodes: NodeUsage[]
  history: DailyTraffic[]
}

interface NodeDetail {
  days: number
  node: NodeUsage
  history: DailyTraffic[]
  top_users: { user_id: number; username: string; bytes: number }[]
}

// totals of the current and the previous period from 2×days of history
function periodTotals(history: DailyTraffic[], days: number) {
  const cut = new Date(Date.now() - (days - 1) * 86_400_000).toISOString().slice(0, 10)
  let cur = 0
  let prev = 0
  for (const d of history) (d.day >= cut ? (cur += d.bytes) : (prev += d.bytes))
  return { cur, delta: prev > 0 ? ((cur - prev) / prev) * 100 : null }
}

function ChartLegend() {
  const { t } = useI18n()
  return (
    <div className="legend" aria-hidden>
      {[
        { name: t('dash.legend.current'), color: 'var(--chart-1)', dash: false },
        { name: t('dash.legend.previous'), color: 'var(--chart-4)', dash: true },
      ].map((s) => (
        <span key={s.name} className="inline-flex items-center gap-1.5 px-2 py-1 text-xs text-muted-foreground">
          <span className={cx('sw', s.dash && 'dash')} style={{ background: s.color, color: s.color }} />
          {s.name}
        </span>
      ))}
    </div>
  )
}

export function PeriodSwitch({ value, onChange }: { value: StatsPeriod; onChange: (p: StatsPeriod) => void }) {
  const { t } = useI18n()
  return (
    <Segmented<StatsPeriod>
      label={t('nodes.stats.period')}
      value={value}
      onChange={onChange}
      options={(['7', '30', '90'] as StatsPeriod[]).map((p) => ({ value: p, label: t(`dash.period.${p}`) }))}
    />
  )
}

// NodesOverview: all nodes' traffic over time and each node's share of it.
export function NodesOverview({ period, onOpen }: { period: StatsPeriod; onOpen: (n: NodeUsage) => void }) {
  const { t } = useI18n()
  const days = Number(period)
  const { data } = useFetch<NodesStats>(`/api/nodes/stats?days=${days}`)
  const loaded = data?.days === days
  const nodes = useMemo(() => [...(loaded ? data.nodes : [])].sort((a, b) => b.up + b.down - (a.up + a.down)), [data, loaded])
  const total = nodes.reduce((a, n) => a + n.up + n.down, 0)
  const { cur, delta } = periodTotals(loaded ? data.history : [], days)

  return (
    <div className="grid12 mb-4">
      <article className="card fill rise col-8" style={{ '--d': 1 } as CSSProperties}>
        <div className="card-head">
          <div>
            <h2 className="card-title">{t('nodes.stats.title')}</h2>
            <p className="card-desc flex flex-wrap items-center gap-2">
              <span className="whitespace-nowrap font-medium tabular-nums text-foreground">{loaded ? formatBytes(cur) : '—'}</span>
              {loaded && delta !== null && <Trend value={delta} />}
              <span>{t('dash.daily', { n: days })}</span>
            </p>
          </div>
          <ChartLegend />
        </div>
        <div className="card-body">
          <TrafficChart data={loaded ? data.history : []} days={days} compare />
        </div>
      </article>
      <article className="card rise col-4" style={{ '--d': 2 } as CSSProperties}>
        <div className="card-head">
          <div>
            <h2 className="card-title">{t('nodes.stats.share')}</h2>
            <p className="card-desc">{t('nodes.stats.shareDesc')}</p>
          </div>
        </div>
        <div className="card-body pt-1">
          {!loaded ? (
            <Skeleton className="h-24 w-full" />
          ) : nodes.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('nodes.empty')}</p>
          ) : (
            <div className="rows">
              {nodes.map((n) => {
                const b = n.up + n.down
                return (
                  <button
                    type="button"
                    key={n.id}
                    onClick={() => onOpen(n)}
                    className="r -mx-2 rounded-md px-2 py-2 text-left transition-colors hover:bg-muted/60 max-sm:mx-0 max-sm:px-0"
                    style={{ display: 'block' }}
                    title={t('nodes.stats.open')}
                  >
                    <div className="flex items-center gap-2">
                      <span className="name truncate font-medium">{n.name}</span>
                      {n.online > 0 && (
                        <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                          <Wifi className="h-3.5 w-3.5" />
                          {n.online}
                        </span>
                      )}
                      <span className="v ml-auto">{formatBytes(b)}</span>
                      <span className="s">{total > 0 ? `${Math.round((b / total) * 100)}%` : '—'}</span>
                    </div>
                    <div className="meter !mt-1.5" aria-hidden>
                      <i style={{ '--v': total > 0 ? b / total : 0 } as CSSProperties} />
                    </div>
                  </button>
                )
              })}
            </div>
          )}
        </div>
      </article>
    </div>
  )
}

function Stat({ icon: Icon, label, value }: { icon: typeof Users; label: string; value: string | number }) {
  return (
    <div className="min-w-0 rounded-lg px-3 py-2.5 ring-1 ring-inset ring-border">
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <Icon className="h-3.5 w-3.5 shrink-0" />
        <span className="truncate">{label}</span>
      </div>
      <div className="mt-1 truncate text-base font-semibold tabular-nums text-foreground">{value}</div>
    </div>
  )
}

// NodeStatsDialog: one node's traffic, users online and its heaviest users.
export function NodeStatsDialog({ node, initial, onClose }: { node: { id: number; name: string }; initial: StatsPeriod; onClose: () => void }) {
  const { t } = useI18n()
  const [period, setPeriod] = useState<StatsPeriod>(initial)
  const days = Number(period)
  const { data } = useFetch<NodeDetail>(`/api/nodes/${node.id}/stats?days=${days}`)
  const loaded = data?.days === days
  const { delta } = periodTotals(loaded ? data.history : [], days)
  const top = loaded ? data.top_users : []
  const topMax = Math.max(1, ...top.map((u) => u.bytes))
  const n = data?.node

  return (
    <Modal open onClose={onClose} wide title={`${t('nodes.stats.open')}: ${node.name}`}>
      <div className="space-y-5">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <span className="whitespace-nowrap font-medium tabular-nums text-foreground">{loaded && n ? formatBytes(n.up + n.down) : '—'}</span>
            {loaded && delta !== null && <Trend value={delta} />}
            <span>{t('dash.daily', { n: days })}</span>
          </p>
          <PeriodSwitch value={period} onChange={setPeriod} />
        </div>
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
          <Stat icon={ArrowDown} label={t('nodes.stats.down')} value={loaded && n ? formatBytes(n.down) : '—'} />
          <Stat icon={ArrowUp} label={t('nodes.stats.up')} value={loaded && n ? formatBytes(n.up) : '—'} />
          <Stat icon={Users} label={t('nodes.stats.users')} value={loaded && n ? n.users : '—'} />
          <Stat icon={Wifi} label={t('nodes.stats.online')} value={loaded && n ? n.online : '—'} />
        </div>
        <div>
          <div className="mb-2 flex justify-end">
            <ChartLegend />
          </div>
          <TrafficChart data={loaded ? data.history : []} days={days} compare />
        </div>
        <div>
          <h3 className="mb-1 text-sm font-semibold text-foreground">{t('nodes.stats.top')}</h3>
          {!loaded ? (
            <Skeleton className="h-16 w-full" />
          ) : top.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('dash.noData')}</p>
          ) : (
            <div className="rows">
              {top.map((u, i) => (
                <div key={u.user_id} className="r py-2" style={{ display: 'block' }}>
                  <div className="flex items-center gap-2">
                    <span className="w-5 shrink-0 text-xs tabular-nums text-faint">{i + 1}</span>
                    <span className="name truncate">{u.username}</span>
                    <span className="v">{formatBytes(u.bytes)}</span>
                  </div>
                  <div className="meter !mt-1.5 ml-7" aria-hidden>
                    <i style={{ '--v': u.bytes / topMax } as CSSProperties} />
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
        <p className="hint">{t('nodes.stats.note')}</p>
      </div>
    </Modal>
  )
}
