import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Gauge, Info, MessageSquareText, MoreHorizontal, RefreshCw } from 'lucide-react'
import { put } from '../../lib/api'
import { useFetch } from '../../lib/hooks'
import { useI18n } from '../../lib/i18n'
import type { Branding } from '../../lib/types'
import { Button, Card, ErrorNote, Field, Input, Textarea } from '../../components/ui'

const VARIABLES = ['USERNAME', 'NOTE', 'EXPIRE_DATE', 'DAYS_LEFT', 'DATA_LEFT', 'DATA_USED', 'DATA_LIMIT', 'STATUS'] as const

// Sample user for the preview; matches the server-side formatting.
const SAMPLE: Record<(typeof VARIABLES)[number], string> = {
  USERNAME: 'alice',
  NOTE: 'VIP User',
  EXPIRE_DATE: '29.10.2026',
  DAYS_LEFT: '19',
  DATA_LEFT: '249.2 GB',
  DATA_USED: '846.1 MB',
  DATA_LIMIT: '250.0 GB',
  STATUS: 'active',
}

const render = (text: string) => VARIABLES.reduce((s, v) => s.split(`{${v}}`).join(SAMPLE[v]), text).trim()

export default function BrandingCard() {
  const { t } = useI18n()
  const { data, error } = useFetch<Branding>('/api/settings/subscription')
  const [form, setForm] = useState<Branding | null>(null)
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState<{ ok: boolean; text: string } | null>(null)
  const area = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    if (data) setForm(data)
  }, [data])

  if (!form) return <ErrorNote error={error} />

  const insert = (v: string) => {
    const el = area.current
    const token = `{${v}}`
    const start = el?.selectionStart ?? form.announce.length
    const end = el?.selectionEnd ?? form.announce.length
    const next = form.announce.slice(0, start) + token + form.announce.slice(end)
    setForm({ ...form, announce: next })
    requestAnimationFrame(() => {
      el?.focus()
      el?.setSelectionRange(start + token.length, start + token.length)
    })
  }

  const save = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setNote(null)
    try {
      setForm(await put<Branding>('/api/settings/subscription', form))
      setNote({ ok: true, text: t('common.saved') })
    } catch (err) {
      setNote({ ok: false, text: (err as Error).message })
    } finally {
      setBusy(false)
    }
  }

  const preview = render(form.announce)
  return (
    <Card className="mb-6 p-6">
      <div className="mb-4 flex items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-violet-50 text-violet-600 dark:bg-violet-500/10 dark:text-violet-300">
          <MessageSquareText className="h-5 w-5" />
        </div>
        <h2 className="font-semibold text-slate-900 dark:text-white">{t('br.title')}</h2>
      </div>
      <p className="mb-5 max-w-3xl text-sm text-slate-600 dark:text-slate-400">{t('br.desc')}</p>
      {note && (
        <div
          className={
            'mb-4 rounded-lg px-3 py-2 text-sm ' +
            (note.ok ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300' : 'bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300')
          }
        >
          {note.text}
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-[1fr_360px]">
        <form onSubmit={save} className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-[1fr_200px]">
            <Field label={t('br.name')}>
              <Input value={form.title} maxLength={64} onChange={(e) => setForm({ ...form, title: e.target.value })} required />
            </Field>
            <Field label={t('br.interval')}>
              <Input type="number" min={1} max={168} value={form.update_hours} onChange={(e) => setForm({ ...form, update_hours: Number(e.target.value) })} />
            </Field>
          </div>

          <Field
            label={t('br.announce')}
            hint={
              <span className="flex flex-wrap items-center justify-between gap-2">
                <span>{t('br.announceHint')}</span>
                <span className={preview.length > 200 ? 'text-amber-600 dark:text-amber-400' : ''}>{t('br.chars', { n: preview.length })}</span>
              </span>
            }
          >
            <Textarea
              ref={area}
              rows={7}
              maxLength={1000}
              value={form.announce}
              onChange={(e) => setForm({ ...form, announce: e.target.value })}
              placeholder={'✦ VIP COREX ✦\n⚡ Your Exclusive Connection\n👤 Username: {USERNAME}\n📅 Expires: {EXPIRE_DATE}'}
              className="font-mono text-xs"
            />
          </Field>
          <div className="flex flex-wrap gap-1.5">
            {VARIABLES.map((v) => (
              <button
                type="button"
                key={v}
                onClick={() => insert(v)}
                title={t(`br.var.${v}`)}
                className="rounded-md bg-slate-100 px-2 py-1 font-mono text-[11px] text-slate-700 transition-colors hover:bg-brand-50 hover:text-brand-700 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-brand-500/10"
              >
                {`{${v}}`} <span className="font-sans text-slate-400">· {t(`br.var.${v}`)}</span>
              </button>
            ))}
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <Field label={t('br.support')} hint={t('br.supportHint')}>
              <Input value={form.support_url} onChange={(e) => setForm({ ...form, support_url: e.target.value })} placeholder="https://t.me/…" />
            </Field>
            <Field label={t('br.web')}>
              <Input value={form.web_page_url} onChange={(e) => setForm({ ...form, web_page_url: e.target.value })} placeholder="https://…" />
            </Field>
          </div>
          <Button type="submit" loading={busy}>
            {t('common.save')}
          </Button>
        </form>

        <div>
          <div className="mb-2 text-xs font-medium text-slate-500">{t('br.preview')}</div>
          {/* Mimics how Happ renders a subscription card. */}
          <div className="overflow-hidden rounded-2xl bg-[#25235a] text-white shadow-lg ring-1 ring-black/10">
            <div className="flex items-center gap-3 px-4 py-3">
              <div className="min-w-0 flex-1">
                <div className="truncate text-lg font-semibold">{form.title || '—'}</div>
                <div className="text-[11px] text-white/50">10.10.2026 12:25 | Autoupdate - {form.update_hours} h.</div>
              </div>
              <RefreshCw className="h-4 w-4 text-white/70" />
              <Gauge className="h-4 w-4 text-white/70" />
              <MoreHorizontal className="h-4 w-4 text-white/70" />
            </div>
            <div className="flex items-center gap-2 bg-[#2d2b6b] px-4 py-2 text-[11px]">
              <Info className="h-4 w-4 shrink-0 text-white/60" />
              <div className="flex-1 rounded border border-black/60 py-0.5 text-center">846.1MB/250.0GB</div>
              <span className="whitespace-nowrap">Expires: 29.10.2026</span>
            </div>
            {preview && <div className="whitespace-pre-line break-words px-4 py-4 text-center text-sm leading-relaxed">{preview}</div>}
            {form.support_url && <div className="border-t border-white/10 px-4 py-2 text-center text-xs text-white/70">💬 {t('sub.support')}</div>}
          </div>
          <p className="mt-2 text-xs text-slate-400">{t('br.previewNote')}</p>
        </div>
      </div>
    </Card>
  )
}
