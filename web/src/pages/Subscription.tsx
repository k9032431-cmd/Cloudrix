import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { CloudUpload, Download, Languages, Moon, Sun } from 'lucide-react'
import { useFetch, useTheme } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import { daysLeft, formatBytes, formatDate } from '../lib/format'
import type { SubscriptionInfo } from '../lib/types'
import { Button, Card, CopyButton, IconButton, ProgressBar, QR, cx } from '../components/ui'
import { UserStatusBadge } from '../components/StatusBadge'
import { Logo } from '../components/Layout'

// One-tap import deep links for popular clients.
const APPS: { name: string; link: (url: string) => string }[] = [
  { name: 'Happ', link: (u) => `happ://add/${u}` },
  { name: 'v2RayTun', link: (u) => `v2raytun://import/${u}` },
  { name: 'Hiddify', link: (u) => `hiddify://import/${u}#Cloudrix` },
  { name: 'Streisand', link: (u) => `streisand://import/${u}` },
  { name: 'sing-box', link: (u) => `sing-box://import-remote-profile?url=${encodeURIComponent(u)}#Cloudrix` },
  { name: 'Clash / mihomo', link: (u) => `clash://install-config?url=${encodeURIComponent(u)}` },
]

export default function Subscription() {
  const { token } = useParams()
  const { t, lang, setLang } = useI18n()
  const { dark, toggle } = useTheme()
  const { data, error, loading } = useFetch<SubscriptionInfo>(`/sub/${encodeURIComponent(token ?? '')}/info`)
  const left = data ? daysLeft(data.expire_at) : null
  const [kind, setKind] = useState<'direct' | 'gdrive'>('direct')
  const url = data ? (kind === 'gdrive' && data.gdrive_url ? data.gdrive_url : data.url) : ''

  return (
    <div className="relative min-h-screen overflow-hidden bg-slate-50 px-4 py-8 dark:bg-slate-950">
      <div className="pointer-events-none absolute -top-48 left-1/2 h-[520px] w-[520px] -translate-x-1/2 rounded-full bg-brand-400/20 blur-3xl dark:bg-brand-600/20" />
      <div className="relative mx-auto max-w-xl">
        <div className="mb-6 flex items-center justify-between">
          <Logo />
          <div className="flex gap-1">
            <IconButton onClick={toggle} aria-label="toggle theme">
              {dark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            </IconButton>
            <IconButton onClick={() => setLang(lang === 'ru' ? 'en' : 'ru')} aria-label="toggle language">
              <Languages className="h-4 w-4" />
            </IconButton>
          </div>
        </div>

        {error && !loading && <Card className="p-8 text-center text-slate-500">{t('sub.notFound')}</Card>}

        {data && (
          <div className="space-y-4">
            <Card className="p-6">
              <div className="mb-5 flex items-center justify-between gap-3">
                <div>
                  <div className="text-xs text-slate-500">{t('sub.title')}</div>
                  <div className="text-xl font-semibold text-slate-900 dark:text-white">{data.username}</div>
                </div>
                <UserStatusBadge status={data.status} />
              </div>
              <div className="mb-2 flex justify-between text-sm">
                <span className="text-slate-500">{t('sub.used')}</span>
                <span className="font-medium tabular-nums text-slate-900 dark:text-white">
                  {formatBytes(data.used_traffic)} / {data.data_limit ? formatBytes(data.data_limit) : '∞'}
                </span>
              </div>
              <ProgressBar value={data.used_traffic} max={data.data_limit} />
              <div className="mt-4 grid grid-cols-2 gap-4 text-sm">
                <div>
                  <div className="text-slate-500">{t('sub.left')}</div>
                  <div className="font-medium tabular-nums text-slate-900 dark:text-white">
                    {data.data_limit ? formatBytes(Math.max(data.data_limit - data.used_traffic, 0)) : '∞'}
                  </div>
                </div>
                <div>
                  <div className="text-slate-500">{t('sub.expires')}</div>
                  <div className="font-medium text-slate-900 dark:text-white">
                    {data.expire_at ? `${formatDate(data.expire_at, lang)}${left !== null && left >= 0 ? ` · ${t('users.daysLeft', { n: left })}` : ''}` : '∞'}
                  </div>
                </div>
              </div>
            </Card>

            <Card className="flex flex-col items-center gap-4 p-6">
              {data.gdrive_url && (
                <div className="grid w-full max-w-xs grid-cols-2 gap-1 rounded-lg bg-slate-100 p-1 dark:bg-slate-800">
                  {(['direct', 'gdrive'] as const).map((k) => (
                    <button
                      key={k}
                      onClick={() => setKind(k)}
                      className={cx(
                        'flex h-8 items-center justify-center gap-1.5 rounded-md text-xs font-medium transition-colors',
                        kind === k ? 'bg-white text-slate-900 shadow-sm dark:bg-slate-950 dark:text-white' : 'text-slate-500 hover:text-slate-900 dark:hover:text-white',
                      )}
                    >
                      {k === 'gdrive' && <CloudUpload className="h-3.5 w-3.5" />}
                      {k === 'direct' ? t('sub.kind.direct') : `${t('sub.backup')} · Google`}
                    </button>
                  ))}
                </div>
              )}
              <QR value={url} size={200} />
              <p className="text-center text-sm text-slate-500">{kind === 'gdrive' ? t('sub.gdriveHint') : t('sub.scan')}</p>
              <CopyButton text={url} label={t('sub.copyLink')} />
            </Card>

            <Card className="p-6">
              <h2 className="mb-3 text-sm font-medium text-slate-700 dark:text-slate-300">{t('sub.addToApp')}</h2>
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
                {APPS.map((a) => (
                  <a key={a.name} href={a.link(url)}>
                    <Button variant="secondary" className="w-full">
                      {a.name}
                    </Button>
                  </a>
                ))}
              </div>
            </Card>

            {(data.links.length > 0 || data.wireguard.length > 0) && (
              <Card className="p-6">
                <h2 className="mb-3 text-sm font-medium text-slate-700 dark:text-slate-300">{t('sub.configs')}</h2>
                <div className="space-y-2">
                  {data.links.map((l, i) => (
                    <div key={i} className="flex items-center gap-2 rounded-lg bg-slate-50 p-2 dark:bg-slate-800/60">
                      <code className="min-w-0 flex-1 truncate text-xs text-slate-600 dark:text-slate-300">{decodeURIComponent(l.split('#')[1] ?? l.split('://')[0])}</code>
                      <CopyButton text={l} />
                    </div>
                  ))}
                  {data.wireguard.map((tag) => (
                    <a key={tag} href={`/sub/${token}/wireguard/${tag}.conf`} className="block">
                      <Button variant="secondary" size="sm">
                        <Download className="h-3.5 w-3.5" />
                        {tag} · {t('sub.download')}
                      </Button>
                    </a>
                  ))}
                </div>
              </Card>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
