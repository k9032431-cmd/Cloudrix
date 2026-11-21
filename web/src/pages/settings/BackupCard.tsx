import { useRef, useState } from 'react'
import { ArrowUp, CloudUpload, Download, RefreshCw, Server } from '../../components/icons'
import { getToken, post, put } from '../../lib/api'
import { formatBytes, relativeTime } from '../../lib/format'
import { useFetch } from '../../lib/hooks'
import { useI18n } from '../../lib/i18n'
import { Badge, Button, Card, Field, Input, Select } from '../../components/ui'

interface DriveBackups {
  connected: boolean
  hours: number
  keep: number
  last_at?: string
  last_error?: string
  error?: string
  files: { id: string; name: string; created: string; size: number }[]
}

// BackupCard moves the panel to another server: a backup is the whole
// database (users, nodes and their keys, cores, hosts, settings). It is kept
// in Google Drive on a schedule, or downloaded, and restored on the new panel.
export default function BackupCard() {
  const { t, lang } = useI18n()
  const { data, reload, setData } = useFetch<DriveBackups>('/api/system/drive-backups')
  const [busy, setBusy] = useState<string | null>(null)
  const [note, setNote] = useState<{ ok: boolean; text: string } | null>(null)
  const [hours, setHours] = useState<number | null>(null)
  const [keep, setKeep] = useState<number | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const run = async (key: string, fn: () => Promise<unknown>, okText?: string) => {
    setBusy(key)
    setNote(null)
    try {
      await fn()
      if (okText) setNote({ ok: true, text: okText })
    } catch (e) {
      setNote({ ok: false, text: (e as Error).message })
    } finally {
      setBusy(null)
    }
  }

  // After a restore the panel restarts with the restored database; the
  // admins and passwords are now those of the backup.
  const restarting = () => {
    setNote({ ok: true, text: t('bk.restarting') })
    setTimeout(() => window.location.reload(), 8000)
  }

  const download = () =>
    run('download', async () => {
      const res = await fetch('/api/system/backup', { headers: { Authorization: `Bearer ${getToken() ?? ''}` } })
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error ?? res.statusText)
      const name = /filename="([^"]+)"/.exec(res.headers.get('Content-Disposition') ?? '')?.[1] ?? 'cloudrix.db'
      const a = document.createElement('a')
      a.href = URL.createObjectURL(await res.blob())
      a.download = name
      a.click()
      URL.revokeObjectURL(a.href)
    })

  const restoreFile = (file: File) => {
    if (!window.confirm(t('bk.confirmRestore'))) return
    run('file', async () => {
      const res = await fetch('/api/system/restore', {
        method: 'POST',
        headers: { Authorization: `Bearer ${getToken() ?? ''}`, 'Content-Type': 'application/octet-stream' },
        body: file,
      })
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error ?? res.statusText)
      restarting()
    })
  }

  const restoreDrive = (id: string) => {
    if (!window.confirm(t('bk.confirmRestore'))) return
    run('drive-' + id, async () => {
      await post(`/api/system/drive-backups/${encodeURIComponent(id)}/restore`)
      restarting()
    })
  }

  const h = hours ?? data?.hours ?? 0
  const k = keep ?? data?.keep ?? 10
  const saveSchedule = () =>
    run('schedule', async () => {
      setData(await put<DriveBackups>('/api/system/drive-backups', { hours: h, keep: k }))
      setHours(null)
      setKeep(null)
    }, t('common.saved'))

  return (
    <Card className="p-6">
      <div className="mb-4 flex items-center gap-3">
        <div className="flex h-11 w-11 items-center justify-center rounded-lg border border-border text-muted-foreground">
          <Server className="h-5 w-5" />
        </div>
        <div className="min-w-0">
          <h2 className="font-semibold text-foreground">{t('bk.title')}</h2>
          <p className="text-xs text-muted-foreground">{t('bk.desc')}</p>
        </div>
      </div>

      {note && (
        <p className={`mb-4 rounded-lg px-3 py-2 text-sm ${note.ok ? 'bg-success/10 text-success' : 'bg-destructive/10 text-destructive'}`}>{note.text}</p>
      )}

      <section className="grid gap-3 border-b border-border pb-5">
        <h3 className="text-sm font-semibold text-foreground">{t('bk.fileTitle')}</h3>
        <div className="flex flex-wrap gap-2">
          <Button variant="secondary" onClick={download} loading={busy === 'download'}>
            <Download className="h-4 w-4" />
            {t('bk.download')}
          </Button>
          <Button variant="secondary" onClick={() => fileRef.current?.click()} loading={busy === 'file'}>
            <ArrowUp className="h-4 w-4" />
            {t('bk.restoreFile')}
          </Button>
          <input
            ref={fileRef}
            id="backup-file"
            type="file"
            accept=".db,application/x-sqlite3,application/octet-stream"
            className="hidden"
            onChange={(e) => {
              const f = e.target.files?.[0]
              e.target.value = ''
              if (f) restoreFile(f)
            }}
          />
        </div>
        <p className="hint">{t('bk.fileHint')}</p>
      </section>

      <section className="grid gap-3 border-b border-border py-5">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-sm font-semibold text-foreground">{t('bk.driveTitle')}</h3>
          {data?.last_at && <Badge tone={data.last_error ? 'amber' : 'green'} dot>{t('bk.lastBackup', { when: relativeTime(data.last_at, lang) })}</Badge>}
        </div>
        {!data?.connected ? (
          <p className="hint">{t('bk.driveNeeded')}</p>
        ) : (
          <>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label={t('bk.every')}>
                <Select id="backup-hours" value={h} onChange={(e) => setHours(Number(e.target.value))}>
                  <option value={0}>{t('bk.off')}</option>
                  {[1, 3, 6, 12, 24, 48].map((n) => (
                    <option key={n} value={n}>
                      {t('bk.hours', { n })}
                    </option>
                  ))}
                </Select>
              </Field>
              <Field label={t('bk.keep')}>
                <Input id="backup-keep" type="number" min={1} max={100} value={k} onChange={(e) => setKeep(Number(e.target.value) || 1)} />
              </Field>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button onClick={saveSchedule} loading={busy === 'schedule'} disabled={hours === null && keep === null}>
                {t('common.save')}
              </Button>
              <Button
                variant="secondary"
                onClick={() => run('now', async () => setData(await post<DriveBackups>('/api/system/drive-backups')), t('bk.done'))}
                loading={busy === 'now'}
              >
                <CloudUpload className="h-4 w-4" />
                {t('bk.now')}
              </Button>
              <Button variant="ghost" onClick={reload} aria-label={t('bk.refresh')} title={t('bk.refresh')}>
                <RefreshCw className="h-4 w-4" />
              </Button>
            </div>
            {data?.last_error && <p className="text-xs text-destructive">{data.last_error}</p>}
            {data?.error && <p className="text-xs text-destructive">{data.error}</p>}
            {data && data.files.length > 0 && (
              <ul className="grid gap-1.5">
                {data.files.map((f) => (
                  <li key={f.id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border px-3 py-2">
                    <div className="min-w-0">
                      <div className="truncate font-mono text-xs text-foreground">{f.name}</div>
                      <div className="text-[11px] text-faint">
                        {relativeTime(f.created, lang)} · {formatBytes(f.size)}
                      </div>
                    </div>
                    <Button size="sm" variant="secondary" onClick={() => restoreDrive(f.id)} loading={busy === 'drive-' + f.id}>
                      {t('bk.restore')}
                    </Button>
                  </li>
                ))}
              </ul>
            )}
          </>
        )}
      </section>

      <section className="grid gap-2 pt-5">
        <h3 className="text-sm font-semibold text-foreground">{t('bk.moveTitle')}</h3>
        <ol className="grid list-decimal gap-1.5 pl-5 text-sm text-muted-foreground">
          {(['bk.move1', 'bk.move2', 'bk.move3', 'bk.move4', 'bk.move5'] as const).map((k) => (
            <li key={k}>{t(k)}</li>
          ))}
        </ol>
      </section>
    </Card>
  )
}
