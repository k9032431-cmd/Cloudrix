import { useEffect, useRef, useState, type ReactNode } from 'react'
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
// duplicate, delete…), as on the PasarGuard cards.
export function ActionsMenu({ actions, label, icon: Trigger = MoreHorizontal, children }: { actions: Action[]; label: string; icon?: Icon; children?: ReactNode }) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    requestAnimationFrame(() => ref.current?.querySelector<HTMLElement>('.menu-item')?.focus({ preventScroll: true }))
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])
  return (
    <div ref={ref} className="relative" onClick={(e) => e.stopPropagation()}>
      {children}
      <IconButton onClick={() => setOpen((v) => !v)} aria-label={label} aria-haspopup="menu" aria-expanded={open}>
        <Trigger className="h-4 w-4" />
      </IconButton>
      {open && (
        <div role="menu" className="pop right-0 top-full mt-1 !min-w-[180px]">
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
        </div>
      )}
    </div>
  )
}
