import { useEffect, useState, type FormEvent } from 'react'
import { CheckCircle2, CloudUpload, ExternalLink, Link2Off, RefreshCw, ShieldCheck } from '../components/icons'
import { post, put } from '../lib/api'
import { useFetch } from '../lib/hooks'
import { relativeTime } from '../lib/format'
import { useI18n } from '../lib/i18n'
import type { DriveSettings } from '../lib/types'
import BrandingCard from './settings/BrandingCard'
import { Badge, Button, Card, CopyButton, ErrorNote, Field, Input, PageHeader, Select, Toggle } from '../components/ui'

export default function Settings() {
  const { t } = useI18n()
  return (
    <>
      <PageHeader title={t('nav.settings')} />
      <BrandingCard />
      <DriveCard />
    </>
  )
}

function DriveCard() {
  const { t, lang } = useI18n()
  const { data, error, reload, setData } = useFetch<DriveSettings>('/api/settings/gdrive')
  const [form, setForm] = useState({ enabled: false, api_key: '', client_id: '', client_secret: '', format: 'base64', interval_minutes: 10 })
  const [busy, setBusy] = useState<string | null>(null)
  const [note, setNote] = useState<{ ok: boolean; text: string } | null>(null)

  useEffect(() => {
    if (data) {
      setForm((f) => ({
        ...f,
        enabled: data.enabled,
        api_key: data.api_key,
        client_id: data.client_id,
        format: data.format,
        interval_minutes: data.interval_minutes,
      }))
    }
    // Only when settings are (re)loaded from the server, not while polling the connect state.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data?.api_key, data?.client_id, data?.enabled, data?.format, data?.interval_minutes])

  // Poll while Google waits for the user to enter the device code.
  useEffect(() => {
    if (!data?.connect.pending) return
    const id = setInterval(reload, 2000)
    return () => clearInterval(id)
  }, [data?.connect.pending, reload])

  const run = async (key: string, fn: () => Promise<unknown>, okText?: string) => {
    setBusy(key)
    setNote(null)
    try {
      await fn()
      if (okText) setNote({ ok: true, text: okText })
      await reload()
    } catch (e) {
      setNote({ ok: false, text: (e as Error).message })
    } finally {
      setBusy(null)
    }
  }

  const save = (e: FormEvent) => {
    e.preventDefault()
    run('save', async () => {
      setData(await put<DriveSettings>('/api/settings/gdrive', form))
      setForm((f) => ({ ...f, client_secret: '' }))
    }, t('common.saved'))
  }

  const conn = data?.connect
  return (
    <Card className="p-6">
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div className="flex items-center gap-3">
          <div className="flex h-11 w-11 items-center justify-center rounded-lg border border-border text-muted-foreground">
            <CloudUpload className="h-5 w-5" />
          </div>
          <div>
            <h2 className="font-semibold text-foreground">{t('gd.title')}</h2>
            {data && <p className="text-xs text-muted-foreground">{t('gd.stats', { files: data.files, errors: data.errors })}</p>}
            {data?.last_run?.at && (
              <p className="text-xs text-faint">
                {t('gd.lastRun', {
                  when: relativeTime(data.last_run.at, lang),
                  uploaded: data.last_run.uploaded,
                  files: data.last_run.files,
                  sec: data.last_run.seconds.toFixed(1),
                })}
              </p>
            )}
          </div>
        </div>
        {data && (
          <Badge tone={data.connected ? 'green' : 'gray'} dot>
            {data.connected ? `${t('gd.connected')}${data.account ? ` · ${data.account}` : ''}` : t('gd.notConnected')}
          </Badge>
        )}
      </div>
      <p className="mb-2 max-w-3xl text-sm text-muted-foreground">{t('gd.desc')}</p>
      <p className="mb-5 max-w-3xl text-xs text-muted-foreground">⚡ {t('gd.instant')}</p>
      <ErrorNote error={error} />
      {note && (
        <div
          className={
            'mb-4 rounded-lg px-3 py-2 text-sm ' +
            (note.ok ? 'bg-success/10 text-success' : 'bg-destructive/10 text-destructive')
          }
        >
          {note.text}
        </div>
      )}

      <details className="mb-5 rounded-card bg-muted/60 p-4 text-sm" open={!data?.connected}>
        <summary className="cursor-pointer font-medium text-foreground">{t('gd.setupTitle')}</summary>
        <ol className="mt-3 list-decimal space-y-2 pl-5 text-muted-foreground">
          <li>
            {t('gd.step1')}{' '}
            <a className="text-chart-1 hover:underline" href="https://console.cloud.google.com/apis/library/drive.googleapis.com" target="_blank" rel="noreferrer">
              <ExternalLink className="inline h-3.5 w-3.5" />
            </a>
          </li>
          <li>{t('gd.step2')}</li>
          <li>
            {t('gd.step3')}{' '}
            <a className="text-chart-1 hover:underline" href="https://console.cloud.google.com/apis/credentials" target="_blank" rel="noreferrer">
              <ExternalLink className="inline h-3.5 w-3.5" />
            </a>
          </li>
          <li>{t('gd.step4')}</li>
          <li>{t('gd.step5')}</li>
        </ol>
      </details>

      <form onSubmit={save} className="grid gap-4 sm:grid-cols-2">
        <div className="sm:col-span-2">
          <Toggle checked={form.enabled} onChange={(v) => setForm({ ...form, enabled: v })} label={t('gd.enabled')} />
        </div>
        <Field label={t('gd.clientId')}>
          <Input value={form.client_id} onChange={(e) => setForm({ ...form, client_id: e.target.value })} placeholder="1234-abc.apps.googleusercontent.com" className="font-mono text-xs" />
        </Field>
        <Field label={t('gd.clientSecret')} hint={data?.has_client_secret ? t('gd.secretSaved') : undefined}>
          <Input
            type="password"
            value={form.client_secret}
            onChange={(e) => setForm({ ...form, client_secret: e.target.value })}
            placeholder={data?.has_client_secret ? '••••••••' : 'GOCSPX-…'}
            autoComplete="off"
            className="font-mono text-xs"
          />
        </Field>
        <Field label={t('gd.apiKey')} hint={t('gd.apiKeyHint')}>
          <Input value={form.api_key} onChange={(e) => setForm({ ...form, api_key: e.target.value })} placeholder="AIzaSy…" className="font-mono text-xs" />
        </Field>
        <Field label={t('gd.format')}>
          <Select value={form.format} onChange={(e) => setForm({ ...form, format: e.target.value })}>
            <option value="base64">{t('gd.format.base64')}</option>
            <option value="plain">{t('gd.format.plain')}</option>
          </Select>
        </Field>
        <Field label={t('gd.interval')}>
          <Input type="number" min={1} max={1440} value={form.interval_minutes} onChange={(e) => setForm({ ...form, interval_minutes: Number(e.target.value) })} />
        </Field>
        <div className="flex items-end">
          <Button type="submit" loading={busy === 'save'}>
            {t('common.save')}
          </Button>
        </div>
      </form>

      <div className="mt-6 border-t border-border pt-5">
        <div className="mb-1 text-xs font-medium text-muted-foreground">{t('gd.account')}</div>
        {conn?.pending ? (
          <div className="rounded-lg border border-border bg-muted/60 p-4">
            <p className="mb-2 text-sm text-foreground">
              {t('gd.deviceStep')}{' '}
              <a href={conn.verification_url} target="_blank" rel="noreferrer" className="font-medium text-chart-1 underline">
                {conn.verification_url}
              </a>
            </p>
            <div className="mb-3 flex items-center gap-3">
              <code className="rounded-lg bg-card px-4 py-2 text-2xl font-semibold tracking-widest text-foreground shadow-sm">{conn.user_code}</code>
              <CopyButton text={conn.user_code ?? ''} />
            </div>
            <p className="flex items-center gap-2 text-xs text-muted-foreground">
              <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              {t('gd.waiting')}
            </p>
          </div>
        ) : (
          <div className="flex flex-wrap items-center gap-2">
            {conn?.error && <span className="mr-2 text-sm text-destructive">{conn.error}</span>}
            <Button
              variant={data?.connected ? 'secondary' : 'primary'}
              loading={busy === 'connect'}
              disabled={!data?.client_id || !data?.has_client_secret}
              onClick={() => run('connect', () => post('/api/settings/gdrive/connect'))}
            >
              <ShieldCheck className="h-4 w-4" />
              {data?.connected ? t('gd.reconnect') : t('gd.connect')}
            </Button>
            {data?.connected && (
              <>
                <Button variant="secondary" loading={busy === 'test'} onClick={() => run('test', () => post('/api/settings/gdrive/test'), t('gd.testOk'))}>
                  <CheckCircle2 className="h-4 w-4" />
                  {t('gd.test')}
                </Button>
                <Button variant="secondary" loading={busy === 'sync'} disabled={!data.enabled} onClick={() => run('sync', () => post('/api/settings/gdrive/sync'), t('common.saved'))}>
                  <RefreshCw className="h-4 w-4" />
                  {t('gd.syncNow')}
                </Button>
                <Button variant="ghost" loading={busy === 'disconnect'} onClick={() => run('disconnect', () => post('/api/settings/gdrive/disconnect'))}>
                  <Link2Off className="h-4 w-4" />
                  {t('gd.disconnect')}
                </Button>
              </>
            )}
          </div>
        )}
      </div>
    </Card>
  )
}
