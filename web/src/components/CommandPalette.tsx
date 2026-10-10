import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useNavigate } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import { Search, Users, type Icon } from './icons'
import { get } from '../lib/api'
import { useDebounced } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import type { User } from '../lib/types'
import { UserStatusBadge } from './StatusBadge'
import { cx } from './ui'

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
    if (open) {
      setQuery('')
      setActive(0)
      requestAnimationFrame(() => input.current?.focus())
    }
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
  return createPortal(
    <AnimatePresence>
      {open && (
        <motion.div
          key="palette"
          className="fixed inset-0 z-[55] flex items-start justify-center p-4 pt-[12vh]"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0, pointerEvents: 'none', transition: { duration: 0.15 } }}
        >
          <div className="absolute inset-0 bg-slate-950/40" onClick={() => setOpen(false)} />
          <motion.div
            role="dialog"
            aria-modal="true"
            initial={{ opacity: 0, scale: 0.96, y: -8 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.97, y: -6 }}
            transition={{ type: 'spring', stiffness: 500, damping: 36 }}
            className="relative w-full max-w-xl overflow-hidden rounded-xl border border-slate-200 bg-white shadow-xl dark:border-slate-800 dark:bg-slate-900"
            onKeyDown={onKeyDown}
          >
            <div className="flex items-center gap-3 border-b border-slate-200 px-4 dark:border-slate-800">
              <Search className="h-4 w-4 text-slate-400" />
              <input
                ref={input}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={t('cmd.placeholder')}
                className="h-14 flex-1 border-0 bg-transparent text-base text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-0 dark:text-white sm:text-sm"
              />
              <kbd className="hidden rounded-md border border-slate-200 px-1.5 py-0.5 font-mono text-[10px] text-slate-400 dark:border-white/10 sm:block">ESC</kbd>
            </div>
            <div ref={list} className="max-h-[50vh] overflow-y-auto p-2">
              {items.length === 0 && <p className="px-3 py-8 text-center text-sm text-slate-400">{t('cmd.empty')}</p>}
              {items.map((c, i) => {
                const header = c.group !== lastGroup
                lastGroup = c.group
                const Icon = c.icon
                return (
                  <div key={c.id}>
                    {header && <div className="px-3 pb-1 pt-3 text-xs font-medium text-slate-500">{c.group}</div>}
                    <button
                      data-index={i}
                      onMouseMove={() => setActive(i)}
                      onClick={() => run(c)}
                      className={cx(
                        'relative flex w-full items-center gap-3 rounded-md px-2.5 py-2 text-left text-sm transition-colors duration-150',
                        i === active
                          ? 'bg-slate-100 text-slate-900 dark:bg-slate-800 dark:text-white'
                          : 'text-slate-600 dark:text-slate-300',
                      )}
                    >
                      <span className="relative flex h-7 w-7 items-center justify-center rounded-md border border-slate-200 bg-white text-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300">
                        <Icon className="h-4 w-4" />
                      </span>
                      <span className="relative flex-1 truncate">{c.label}</span>
                      {c.hint && <span className="relative">{c.hint}</span>}
                    </button>
                  </div>
                )
              })}
            </div>
            <div className="flex items-center gap-4 border-t border-slate-200 px-4 py-2 text-[11px] text-slate-400 dark:border-slate-800">
              <span>
                <kbd className="font-mono">↑↓</kbd> {t('cmd.navigate')}
              </span>
              <span>
                <kbd className="font-mono">↵</kbd> {t('cmd.open')}
              </span>
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>,
    document.body,
  )
}
