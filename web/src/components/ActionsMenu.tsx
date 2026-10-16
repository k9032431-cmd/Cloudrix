import { useEffect, useLayoutEffect, useRef, useState, type CSSProperties, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { MoreHorizontal, type Icon } from './icons'
import { IconButton, cx } from './ui'

export interface Action {
  label: string
  icon?: Icon
  onClick: () => void
  danger?: boolean
  hidden?: boolean
}

// ActionsMenu is the "⋯" button with a small menu of row actions (edit,
// duplicate, delete…), as on the PasarGuard cards. The menu is drawn over the
// page, so a scrolling table cannot cut it off, and opens upwards when there
// is no room below.
export function ActionsMenu({ actions, label, icon: Trigger = MoreHorizontal, children }: { actions: Action[]; label: string; icon?: Icon; children?: ReactNode }) {
  const [open, setOpen] = useState(false)
  const [pos, setPos] = useState<CSSProperties | null>(null)
  const ref = useRef<HTMLDivElement>(null)
  const menu = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    if (!open) return setPos(null)
    const btn = ref.current?.querySelector('button')?.getBoundingClientRect()
    const m = menu.current?.getBoundingClientRect()
    if (!btn || !m) return
    const right = Math.max(8, window.innerWidth - btn.right)
    const below = window.innerHeight - btn.bottom
    setPos(below < m.height + 12 && btn.top > below ? { right, bottom: window.innerHeight - btn.top + 4 } : { right, top: btn.bottom + 4 })
  }, [open])

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent | TouchEvent) => {
      const target = e.target as Node
      if (!ref.current?.contains(target) && !menu.current?.contains(target)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    const close = () => setOpen(false)
    document.addEventListener('mousedown', onDown)
    document.addEventListener('touchstart', onDown)
    document.addEventListener('keydown', onKey)
    window.addEventListener('scroll', close, true)
    window.addEventListener('resize', close)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('touchstart', onDown)
      document.removeEventListener('keydown', onKey)
      window.removeEventListener('scroll', close, true)
      window.removeEventListener('resize', close)
    }
  }, [open])

  useEffect(() => {
    if (pos) menu.current?.querySelector<HTMLElement>('.menu-item')?.focus({ preventScroll: true })
  }, [pos])

  return (
    <div ref={ref} className="relative" onClick={(e) => e.stopPropagation()}>
      {children}
      <IconButton onClick={() => setOpen((v) => !v)} aria-label={label} aria-haspopup="menu" aria-expanded={open}>
        <Trigger className="h-4 w-4" />
      </IconButton>
      {open &&
        createPortal(
          <div
            ref={menu}
            role="menu"
            className="pop !fixed !min-w-[180px]"
            style={pos ?? { top: 0, right: 0, visibility: 'hidden' }}
            onClick={(e) => e.stopPropagation()}
          >
            {actions
              .filter((a) => !a.hidden)
              .map((a) => (
                <button
                  key={a.label}
                  type="button"
                  role="menuitem"
                  className={cx('menu-item', a.danger && 'danger')}
                  onClick={() => {
                    setOpen(false)
                    a.onClick()
                  }}
                >
                  {a.icon && <a.icon />}
                  <span className="min-w-0 flex-1 truncate">{a.label}</span>
                </button>
              ))}
          </div>,
          document.body,
        )}
    </div>
  )
}
