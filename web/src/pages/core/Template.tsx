import { useState } from 'react'
import { Check, CircleAlert, Download } from '../../components/icons'
import { post } from '../../lib/api'
import { useI18n } from '../../lib/i18n'
import type { Core, Group, Inbound, InboundSettings } from '../../lib/types'
import { Button, CopyButton, ErrorNote, Modal } from '../../components/ui'

// An inbound template carries a core's inbounds to another panel: every
// setting except the secrets. Secrets that were set are marked GENERATE and
// the importing panel makes new ones, so a shared template never hands out
// the keys of the panel it came from.

const GENERATE = 'GENERATE'

// secret fields: dropped from the template, made anew on import
const SECRETS = [
  'reality_private_key',
  'reality_public_key',
  'ss_server_key',
  'obfs_password',
  'kcp_seed',
  'wg_private_key',
  'wg_public_key',
] as const
// fields that only make sense on the server they came from
const LOCAL = ['cert_file', 'key_file', 'reality_mldsa65_seed', 'reality_mldsa65_verify'] as const

interface TemplateInbound {
  tag: string
  protocol: Inbound['protocol']
  listen: string
  port: number
  enabled: boolean
  remark: string
  settings: InboundSettings
}

interface Template {
  cloudrix: 'inbounds'
  version: 1
  name?: string // the core the inbounds came from
  inbounds: TemplateInbound[]
}

export function makeTemplate(inbounds: Inbound[], name?: string): string {
  const tpl: Template = {
    cloudrix: 'inbounds',
    version: 1,
    name,
    inbounds: inbounds.map((ib) => {
      const s: Record<string, unknown> = { ...ib.settings }
      for (const k of SECRETS) if (s[k]) s[k] = GENERATE
      if (ib.settings.reality_short_ids?.length) s.reality_short_ids = [GENERATE]
      for (const k of LOCAL) delete s[k]
      return {
        tag: ib.tag,
        protocol: ib.protocol,
        listen: ib.listen,
        port: ib.port,
        enabled: ib.enabled,
        remark: ib.hosts?.[0]?.remark || ib.remark || '',
        settings: s as InboundSettings,
      }
    }),
  }
  return JSON.stringify(tpl, null, 2)
}

function parseTemplate(text: string): Template {
  let tpl: Template
  try {
    tpl = JSON.parse(text)
  } catch {
    throw new Error('bad')
  }
  if (tpl?.cloudrix !== 'inbounds' || !Array.isArray(tpl.inbounds) || tpl.inbounds.some((ib) => !ib?.protocol || !ib?.port)) {
    throw new Error('bad')
  }
  return tpl
}

// fresh fills in new secrets where the template asks for them.
async function fresh(s: InboundSettings): Promise<InboundSettings> {
  const out: Record<string, unknown> = { ...s }
  const random = () => post<{ base64: string; hex: string }>('/api/tools/random?bytes=' + (s.ss_method?.includes('aes-128') ? 16 : 32))
  if (out.reality_private_key === GENERATE || out.reality_public_key === GENERATE || (s.reality_short_ids ?? []).includes(GENERATE)) {
    const k = await post<{ private_key: string; public_key: string; short_ids: string[] }>('/api/tools/reality-keys')
    Object.assign(out, { reality_private_key: k.private_key, reality_public_key: k.public_key, reality_short_ids: k.short_ids })
  }
  if (out.wg_private_key === GENERATE || out.wg_public_key === GENERATE) {
    const k = await post<{ private_key: string; public_key: string }>('/api/tools/wireguard-keys')
    Object.assign(out, { wg_private_key: k.private_key, wg_public_key: k.public_key })
  }
  if (out.ss_server_key === GENERATE) out.ss_server_key = (await random()).base64
  if (out.obfs_password === GENERATE) out.obfs_password = (await random()).hex.slice(0, 24)
  if (out.kcp_seed === GENERATE) out.kcp_seed = (await random()).hex.slice(0, 24)
  // anything else still marked (an unknown secret) is better empty than literal
  for (const [k, v] of Object.entries(out)) if (v === GENERATE) delete out[k]
  return out as InboundSettings
}

export function ExportDialog({ inbounds, coreName, onClose }: { inbounds: Inbound[]; coreName: string; onClose: () => void }) {
  const { t } = useI18n()
  const text = makeTemplate(inbounds, coreName)
  const download = () => {
    const a = document.createElement('a')
    a.href = URL.createObjectURL(new Blob([text], { type: 'application/json' }))
    a.download = `cloudrix-${coreName.replace(/[^\w-]+/g, '_') || 'core'}-inbounds.json`
    a.click()
    URL.revokeObjectURL(a.href)
  }
  return (
    <Modal
      open
      onClose={onClose}
      wide
      title={t('tpl.exportTitle', { name: coreName })}
      footer={
        <>
          <Button variant="secondary" onClick={download}>
            <Download className="h-4 w-4" />
            {t('tpl.download')}
          </Button>
          <CopyButton text={text} />
        </>
      }
    >
      <p className="mb-3 text-sm text-muted-foreground">{t('tpl.exportHint', { n: inbounds.length })}</p>
      <pre className="max-h-[50vh] overflow-auto whitespace-pre-wrap break-all rounded-lg bg-muted p-3 text-xs text-foreground ring-1 ring-inset ring-border">{text}</pre>
    </Modal>
  )
}

interface Result {
  tag: string
  error?: string
}

// ImportDialog adds a template's inbounds to a core, or to a new core named
// as in the template when coreID is null (Cores → Import core).
export function ImportDialog({
  coreID,
  existing,
  groups,
  onClose,
  onDone,
}: {
  coreID: number | null
  existing: Inbound[]
  groups: Group[]
  onClose: () => void
  onDone: (coreID: number) => void
}) {
  const { t } = useI18n()
  const [text, setText] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [results, setResults] = useState<Result[] | null>(null)
  const [created, setCreated] = useState<string | null>(null) // name of the new core

  const run = async () => {
    setError(null)
    let tpl: Template
    try {
      tpl = parseTemplate(text.trim())
    } catch {
      setError(t('tpl.bad'))
      return
    }
    setBusy(true)
    let target = coreID
    if (target === null) {
      const base = tpl.name?.trim() || 'Imported'
      for (let i = 1; target === null && i <= 20; i++) {
        try {
          const c = await post<Core>('/api/cores', { name: i === 1 ? base : `${base} ${i}` })
          target = c.id
          setCreated(c.name)
        } catch (e) {
          if (i === 20) {
            setError((e as Error).message)
            setBusy(false)
            return
          }
        }
      }
    }
    const core = target as number
    const taken = new Set(existing.map((ib) => ib.tag))
    const out: Result[] = []
    // new inbounds go to the first group, as the editor does
    const groupIDs = groups.slice(0, 1).map((g) => g.id)
    for (const ib of tpl.inbounds) {
      let tag = ib.tag || `${ib.protocol}-${ib.port}`
      for (let i = 2; taken.has(tag); i++) tag = `${ib.tag}-${i}`
      try {
        const settings = await fresh(ib.settings ?? {})
        await post(`/api/inbounds?groups=${groupIDs.join(',')}`, {
          tag,
          protocol: ib.protocol,
          listen: ib.listen || '0.0.0.0',
          port: ib.port,
          enabled: ib.enabled !== false,
          remark: ib.remark ?? '',
          address: '',
          core_id: core,
          node_id: null,
          settings,
        })
        taken.add(tag)
        out.push({ tag })
      } catch (e) {
        out.push({ tag, error: (e as Error).message })
      }
    }
    setResults(out)
    setBusy(false)
    onDone(core)
  }

  return (
    <Modal
      open
      onClose={onClose}
      wide
      title={t(coreID === null ? 'tpl.importCoreTitle' : 'tpl.importTitle')}
      footer={
        results ? (
          <Button onClick={onClose}>{t('common.close')}</Button>
        ) : (
          <>
            <Button variant="secondary" onClick={onClose}>
              {t('common.cancel')}
            </Button>
            <Button onClick={run} loading={busy} disabled={!text.trim()}>
              {t('tpl.importRun')}
            </Button>
          </>
        )
      }
    >
      {results ? (
        <ul className="grid gap-2 text-sm">
          {results.map((r) => (
            <li key={r.tag} className="flex items-start gap-2">
              {r.error ? <CircleAlert className="mt-0.5 h-4 w-4 shrink-0 text-destructive" /> : <Check className="mt-0.5 h-4 w-4 shrink-0 text-success" />}
              <div className="min-w-0">
                <div className="break-all font-medium text-foreground">{r.tag}</div>
                {r.error && <div className="break-words text-xs text-destructive">{r.error}</div>}
              </div>
            </li>
          ))}
          {created && <li className="mt-1 text-xs font-medium text-foreground">{t('tpl.importCoreAfter', { name: created })}</li>}
          <li className="mt-1 text-xs text-muted-foreground">{t('tpl.importAfter')}</li>
        </ul>
      ) : (
        <>
          <p className="mb-3 text-sm text-muted-foreground">{t('tpl.importHint')}</p>
          <ErrorNote error={error} />
          <textarea
            className="input min-h-[220px] w-full font-mono text-xs"
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder='{ "cloudrix": "inbounds", … }'
            spellCheck={false}
          />
        </>
      )}
    </Modal>
  )
}
