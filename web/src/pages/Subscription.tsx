import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { CloudUpload, Download, Languages, MessageCircle, Moon, Sun } from '../components/icons'
import { useFetch, useTheme } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import { daysLeft, formatBytes, formatDate } from '../lib/format'
import type { SubscriptionInfo } from '../lib/types'
import { Button, Card, CopyButton, IconButton, ProgressBar, QR, Segmented } from '../components/ui'
import { UserStatusBadge } from '../components/StatusBadge'
import { Logo } from '../components/Layout'
import { AuroraBackground } from './Login'

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
    <div className="relative min-h-screen overflow-hidden px-4 py-8">
      <AuroraBackground />
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

        {error && !loading && <Card delay={0.07} className="p-8 text-center text-muted-foreground">{t('sub.notFound')}</Card>}

        {data && (
          <div className="space-y-4">
            <Card delay={0.14} className="p-6">
              <div className="mb-5 flex items-center justify-between gap-3">
                <div>
                  <div className="text-xs text-muted-foreground">{data.title || t('sub.title')}</div>
                  <div className="text-xl font-semibold text-foreground">{data.username}</div>
                </div>
                <UserStatusBadge status={data.status} />
              </div>
              <div className="mb-2 flex justify-between text-sm">
                <span className="text-muted-foreground">{t('sub.used')}</span>
                <span className="font-medium tabular-nums text-foreground">
                  {formatBytes(data.used_traffic)} / {data.data_limit ? formatBytes(data.data_limit) : '∞'}
                </span>
              </div>
              <ProgressBar value={data.used_traffic} max={data.data_limit} />
              <div className="mt-4 grid grid-cols-2 gap-4 text-sm">
                <div>
                  <div className="text-muted-foreground">{t('sub.left')}</div>
                  <div className="font-medium tabular-nums text-foreground">
                    {data.data_limit ? formatBytes(Math.max(data.data_limit - data.used_traffic, 0)) : '∞'}
                  </div>
                </div>
                <div>
                  <div className="text-muted-foreground">{t('sub.expires')}</div>
                  <div className="font-medium text-foreground">
                    {data.expire_at ? `${formatDate(data.expire_at, lang)}${left !== null && left >= 0 ? ` · ${t('users.daysLeft', { n: left })}` : ''}` : '∞'}
                  </div>
                </div>
              </div>
            </Card>

            {(data.announce || data.support_url) && (
              <Card delay={0.21} className="p-6 text-center">
                {data.announce && <p className="whitespace-pre-line break-words text-sm leading-relaxed text-foreground">{data.announce}</p>}
                {data.support_url && (
                  <a href={data.support_url} target="_blank" rel="noreferrer" className={data.announce ? 'mt-4 inline-block' : 'inline-block'}>
                    <Button variant="secondary" size="sm">
                      <MessageCircle className="h-3.5 w-3.5" />
                      {t('sub.support')}
                    </Button>
                  </a>
                )}
              </Card>
            )}

            <Card delay={0.28} className="flex flex-col items-center gap-4 p-6">
              {data.gdrive_url && (
                                <Segmented
                  id="public-sub-kind"
                  className="w-full max-w-xs"
                  value={kind}
                  onChange={setKind}
                  options={[
                    { value: 'direct', label: t('sub.kind.direct') },
                    {
                      value: 'gdrive',
                      label: (
                        <>
                          <CloudUpload className="h-3.5 w-3.5" />
                          {`${t('sub.backup')} · Google`}
                        </>
                      ),
                    },
                  ]}
                />
              )}
              <QR value={url} size={200} />
              <p className="text-center text-sm text-muted-foreground">{kind === 'gdrive' ? t('sub.gdriveHint') : t('sub.scan')}</p>
              <CopyButton text={url} label={t('sub.copyLink')} />
            </Card>

            <Card delay={0.35} className="p-6">
              <h2 className="mb-3 text-sm font-medium text-foreground">{t('sub.addToApp')}</h2>
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
              <Card delay={0.42} className="p-6">
                <h2 className="mb-3 text-sm font-medium text-foreground">{t('sub.configs')}</h2>
                <div className="space-y-2">
                  {data.links.map((l, i) => (
                    <div key={i} className="flex items-center gap-2 rounded-lg bg-muted/60 p-2">
                      <code className="min-w-0 flex-1 truncate text-xs text-muted-foreground">{decodeURIComponent(l.split('#')[1] ?? l.split('://')[0])}</code>
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
