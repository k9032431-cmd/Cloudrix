import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useNavigate } from 'react-router-dom'
import { ArrowDown, ArrowUp, CornerDownLeft, Search, Users, type Icon } from './icons'
import { get } from '../lib/api'
import { useDebounced } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import type { User } from '../lib/types'
import { UserStatusBadge } from './StatusBadge'

export interface Command {
  id: string
  label: string
  group: string
  icon: Icon
  hint?: ReactNode
  run: () => void
}

// shortcut is the key hint for this platform.
export const shortcut = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘K' : 'Ctrl K'

// openPalette lets any button open the palette.
export const openPalette = () => window.dispatchEvent(new Event('cloudrix:palette'))

// CommandPalette is a Ctrl/Cmd+K launcher for pages, actions and users.
export default function CommandPalette({ commands }: { commands: Command[] }) {
  const { t } = useI18n()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const [users, setUsers] = useState<User[]>([])
  const q = useDebounced(query.trim(), 180)
  const input = useRef<HTMLInputElement>(null)
  const list = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setOpen((o) => !o)
      }
    }
    const onOpen = () => setOpen(true)
    window.addEventListener('keydown', onKey)
    window.addEventListener('cloudrix:palette', onOpen)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('cloudrix:palette', onOpen)
    }
  }, [])

  useEffect(() => {
    if (!open) return
    const prev = document.activeElement as HTMLElement | null
    setQuery('')
    setActive(0)
    requestAnimationFrame(() => input.current?.focus())
    return () => prev?.focus?.()
  }, [open])

  useEffect(() => {
    if (!open || q.length < 2) {
      setUsers([])
      return
    }
    let stale = false
    get<{ users: User[] }>(`/api/users?limit=6&search=${encodeURIComponent(q)}`)
      .then((r) => !stale && setUsers(r.users))
      .catch(() => !stale && setUsers([]))
    return () => {
      stale = true
    }
  }, [q, open])

  const items = useMemo(() => {
    const needle = query.trim().toLowerCase()
    const matched = commands.filter((c) => !needle || c.label.toLowerCase().includes(needle))
    const userItems: Command[] = users.map((u) => ({
      id: `user-${u.id}`,
      label: u.username,
      group: t('nav.users'),
      icon: Users,
      hint: <UserStatusBadge status={u.status} />,
      run: () => navigate(`/users?open=${u.id}`),
    }))
    return [...matched, ...userItems]
  }, [commands, users, query, navigate, t])

  useEffect(() => setActive(0), [query, users])

  const run = (c?: Command) => {
    if (!c) return
    setOpen(false)
    c.run()
  }

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActive((a) => Math.min(a + 1, items.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((a) => Math.max(a - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      run(items[active])
    } else if (e.key === 'Escape') {
      setOpen(false)
    }
  }

  useEffect(() => {
    list.current?.querySelector(`[data-index="${active}"]`)?.scrollIntoView({ block: 'nearest' })
  }, [active])

  let lastGroup = ''
  // The palette is used many times a day, so it opens and closes instantly.
  if (!open) return null
  return createPortal(
    <div className="fixed inset-0 z-[80]">
      <div className="layer-scrim" onClick={() => setOpen(false)} />
      <div className="palette" role="dialog" aria-modal="true" aria-label={t('cmd.placeholder')} onKeyDown={onKeyDown}>
        <div className="palette-in">
          <Search />
          <input
            ref={input}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t('cmd.placeholder')}
            role="combobox"
            aria-expanded="true"
            aria-controls="palette-list"
            aria-activedescendant={items[active] ? `pi-${items[active].id}` : undefined}
          />
          <kbd className="kbd hidden sm:inline-flex">Esc</kbd>
        </div>
        <div ref={list} id="palette-list" role="listbox" className="palette-list">
          {items.length === 0 && <p className="palette-empty">{t('cmd.empty')}</p>}
          {items.map((c, i) => {
            const header = c.group !== lastGroup
            lastGroup = c.group
            const Icon = c.icon
            return (
              <div key={c.id}>
                {header && <div className="palette-group">{c.group}</div>}
                <button
                  type="button"
                  id={`pi-${c.id}`}
                  role="option"
                  aria-selected={i === active}
                  data-index={i}
                  tabIndex={-1}
                  onMouseMove={() => setActive(i)}
                  onClick={() => run(c)}
                  className="palette-item"
                >
                  <Icon />
                  <span className="min-w-0 flex-1 truncate">{c.label}</span>
                  {c.hint && <span className="sub">{c.hint}</span>}
                </button>
              </div>
            )
          })}
        </div>
        <div className="palette-foot">
          <span>
            <kbd className="kbd">
              <ArrowUp />
            </kbd>
            <kbd className="kbd">
              <ArrowDown />
            </kbd>
            {t('cmd.navigate')}
          </span>
          <span>
            <kbd className="kbd">
              <CornerDownLeft />
            </kbd>
            {t('cmd.open')}
          </span>
        </div>
      </div>
    </div>,
    document.body,
  )
}
