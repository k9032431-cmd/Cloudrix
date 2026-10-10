import {
  forwardRef,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type CSSProperties,
  type InputHTMLAttributes,
  type ReactNode,
  type SelectHTMLAttributes,
} from 'react'
import { createPortal } from 'react-dom'
import { AnimatePresence, animate, motion, useInView, useReducedMotion } from 'framer-motion'
import QRCode from 'qrcode'
import { Check, CircleAlert, Copy, Inbox, Loader2, X } from './icons'
import { copyText } from '../lib/format'
import { useI18n } from '../lib/i18n'

// Base components on the slate design system classes (src/slate.css).

export function cx(...c: (string | false | null | undefined)[]) {
  return c.filter(Boolean).join(' ')
}

type Variant = 'primary' | 'secondary' | 'danger' | 'ghost'
const variants: Record<Variant, string> = {
  primary: 'btn-primary',
  secondary: 'btn-outline',
  danger: 'btn-danger',
  ghost: 'btn-ghost',
}

export function Button({
  variant = 'primary',
  loading,
  className,
  children,
  size = 'md',
  type = 'button',
  ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; loading?: boolean; size?: 'sm' | 'md' }) {
  return (
    <button {...rest} type={type} disabled={rest.disabled || loading} className={cx('btn', variants[variant], size === 'sm' && 'btn-sm', className)}>
      {loading && <Loader2 className="spinner" />}
      {children}
    </button>
  )
}

export function IconButton({ className, type = 'button', ...rest }: ButtonHTMLAttributes<HTMLButtonElement>) {
  return <button {...rest} type={type} className={cx('btn btn-ghost btn-icon', className)} />
}

export function Input({ className, ...rest }: InputHTMLAttributes<HTMLInputElement>) {
  return <input {...rest} className={cx('input', className)} />
}

export function Select({ className, children, ...rest }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select {...rest} className={cx('select', className)}>
      {children}
    </select>
  )
}

export const Textarea = forwardRef<HTMLTextAreaElement, React.TextareaHTMLAttributes<HTMLTextAreaElement>>(function Textarea({ className, ...rest }, ref) {
  return <textarea ref={ref} {...rest} className={cx('textarea', className)} />
})

// Field puts the label above the control and the hint below it.
export function Field({ label, hint, children, className }: { label: string; hint?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <label className={cx('grid gap-1.5', className)}>
      <span className="label">{label}</span>
      {children}
      {hint && <span className="hint">{hint}</span>}
    </label>
  )
}

export function Toggle({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label: string }) {
  return (
    <label className="flex select-none items-center gap-3">
      <button type="button" role="switch" aria-checked={checked} aria-label={label} onClick={() => onChange(!checked)} className="switch" />
      <span className="text-[13.5px] text-foreground">{label}</span>
    </label>
  )
}

// Card is a plain bordered surface. Inside a routed view it rises in with the
// view; `delay` (seconds) staggers a group of cards in 55ms steps.
export function Card({ children, className, delay = 0, hover }: { children: ReactNode; className?: string; delay?: number; hover?: boolean }) {
  return (
    <div
      className={cx('card rise', hover && 'transition-colors duration-200 hover:border-border-strong', className)}
      style={{ '--d': Math.round(delay / 0.055) + 1 } as CSSProperties}
    >
      {children}
    </div>
  )
}

// AnimatedNumber counts up once per value change (700ms, cubic out).
export function AnimatedNumber({ value, format = (n) => Math.round(n).toLocaleString() }: { value: number; format?: (n: number) => string }) {
  const ref = useRef<HTMLSpanElement>(null)
  const inView = useInView(ref, { once: true })
  const reduce = useReducedMotion()
  const from = useRef(0)
  const fmt = useRef(format)
  fmt.current = format
  useEffect(() => {
    const format = fmt.current
    const el = ref.current
    if (!el || !inView) return
    if (reduce) {
      el.textContent = format(value)
      from.current = value
      return
    }
    const controls = animate(from.current, value, {
      duration: 0.7,
      ease: [0.33, 1, 0.68, 1],
      onUpdate: (v) => {
        el.textContent = format(v)
      },
    })
    from.current = value
    return () => controls.stop()
  }, [value, inView, reduce])
  return <span ref={ref}>{format(0)}</span>
}

export function Skeleton({ className }: { className?: string }) {
  return <span className={cx('shimmer inline-block rounded-md bg-muted', className)} />
}

const badgeTones = {
  green: 'b-success',
  red: 'b-danger',
  amber: 'b-warning',
  blue: 'b-info',
  gray: 'b-neutral',
  violet: 'b-violet',
}
export type Tone = keyof typeof badgeTones

export function Badge({ tone = 'gray', children, dot }: { tone?: Tone; children: ReactNode; dot?: boolean; pulse?: boolean }) {
  return (
    <span className={cx('badge', badgeTones[tone])}>
      {dot && <span className="dot" />}
      {children}
    </span>
  )
}

const focusable = 'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

// useFocusTrap keeps Tab inside `ref` while active and returns focus to the
// element that opened it when it closes.
export function useFocusTrap(ref: React.RefObject<HTMLElement>, active: boolean) {
  useEffect(() => {
    if (!active) return
    const prev = document.activeElement as HTMLElement | null
    const el = ref.current
    requestAnimationFrame(() => {
      const first = el?.querySelector<HTMLElement>('[autofocus], input:not([type=hidden]), select, textarea') ?? el?.querySelector<HTMLElement>(focusable)
      ;(first ?? el)?.focus()
    })
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Tab' || !el) return
      const items = Array.from(el.querySelectorAll<HTMLElement>(focusable)).filter((n) => n.offsetParent !== null)
      if (!items.length) return
      const first = items[0]
      const last = items[items.length - 1]
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('keydown', onKey)
      prev?.focus?.()
    }
  }, [active, ref])
}

export function Modal({
  open,
  onClose,
  title,
  children,
  footer,
  wide,
}: {
  open: boolean
  onClose: () => void
  title: ReactNode
  children: ReactNode
  footer?: ReactNode
  wide?: boolean
}) {
  const { t } = useI18n()
  const panel = useRef<HTMLDivElement>(null)
  useFocusTrap(panel, open)
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    document.addEventListener('keydown', onKey)
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = prev
    }
  }, [open, onClose])
  return createPortal(
    <AnimatePresence>
      {open && (
        <motion.div key="modal" className="fixed inset-0 z-[70] flex items-center justify-center p-3" exit={{ pointerEvents: 'none' }}>
          <motion.div className="layer-scrim" onClick={onClose} initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={{ duration: 0.24 }} />
          <motion.div
            ref={panel}
            role="dialog"
            aria-modal="true"
            tabIndex={-1}
            initial={{ opacity: 0, scale: 0.98, y: 6 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.98, transition: { duration: 0.16 } }}
            transition={{ duration: 0.24, ease: [0.23, 1, 0.32, 1] }}
            className={cx('dialog', wide && 'wide')}
          >
            <div className="dialog-head">
              <h2>{title}</h2>
            </div>
            <IconButton className="x" onClick={onClose} aria-label={t('common.close')}>
              <X />
            </IconButton>
            <div className="dialog-body">{children}</div>
            {footer && <div className="dialog-foot">{footer}</div>}
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>,
    document.body,
  )
}

export function Confirm({
  open,
  onClose,
  onConfirm,
  title,
  message,
  danger = true,
}: {
  open: boolean
  onClose: () => void
  onConfirm: () => Promise<void> | void
  title: string
  message: string
  danger?: boolean
}) {
  const { t } = useI18n()
  const [busy, setBusy] = useState(false)
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={title}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            variant={danger ? 'danger' : 'primary'}
            loading={busy}
            onClick={async () => {
              setBusy(true)
              try {
                await onConfirm()
                onClose()
              } finally {
                setBusy(false)
              }
            }}
          >
            {title}
          </Button>
        </>
      }
    >
      <p className="text-[13.5px] text-muted-foreground">{message}</p>
    </Modal>
  )
}

export function CopyButton({ text, label }: { text: string; label?: string }) {
  const { t } = useI18n()
  const [done, setDone] = useState(false)
  const timer = useRef<number>()
  useEffect(() => () => window.clearTimeout(timer.current), [])
  return (
    <Button
      variant="secondary"
      size="sm"
      onClick={async () => {
        if (await copyText(text)) {
          setDone(true)
          timer.current = window.setTimeout(() => setDone(false), 1500)
        }
      }}
    >
      {done ? <Check className="text-success" /> : <Copy />}
      {done ? t('common.copied') : label ?? t('common.copy')}
    </Button>
  )
}

export function QR({ value, size = 200 }: { value: string; size?: number }) {
  const [src, setSrc] = useState('')
  useEffect(() => {
    QRCode.toDataURL(value, { width: size * 2, margin: 1, errorCorrectionLevel: 'M' })
      .then(setSrc)
      .catch(() => setSrc(''))
  }, [value, size])
  if (!src) return <div style={{ width: size, height: size }} className="shimmer rounded-card bg-muted" />
  return (
    <motion.img
      key={src}
      src={src}
      width={size}
      height={size}
      alt="QR"
      decoding="async"
      initial={{ opacity: 0, scale: 0.97 }}
      animate={{ opacity: 1, scale: 1 }}
      transition={{ duration: 0.3, ease: [0.23, 1, 0.32, 1] }}
      className="rounded-card bg-white p-2 ring-1 ring-border"
    />
  )
}

// ProgressBar is the slate meter: chart accent, warning past 70%, red past 90%.
export function ProgressBar({ value, max }: { value: number; max: number }) {
  const pct = max > 0 ? Math.min(1, value / max) : 0
  return (
    <div className={cx('meter !mt-0', pct >= 0.9 ? 'bad' : pct >= 0.7 && 'warn')} style={{ height: 5 }}>
      {max > 0 && <i style={{ '--v': pct } as CSSProperties} />}
    </div>
  )
}

export function PageHeader({ title, actions, subtitle }: { title: string; actions?: ReactNode; subtitle?: ReactNode }) {
  return (
    <div className="page-head rise">
      <div>
        <h1 className="page-title" tabIndex={-1}>
          {title}
        </h1>
        {subtitle && <p className="page-desc">{subtitle}</p>}
      </div>
      {actions && <div className="page-actions">{actions}</div>}
    </div>
  )
}

export function Empty({ children }: { children?: ReactNode }) {
  const { t } = useI18n()
  return (
    <div className="empty">
      <span className="empty-icon">
        <Inbox />
      </span>
      <h3>{children ?? t('common.empty')}</h3>
    </div>
  )
}

export function ErrorNote({ error }: { error: string | null }) {
  return (
    <AnimatePresence>
      {error && (
        <motion.div
          initial={{ opacity: 0, height: 0, marginBottom: 0 }}
          animate={{ opacity: 1, height: 'auto', marginBottom: 16 }}
          exit={{ opacity: 0, height: 0, marginBottom: 0 }}
          transition={{ duration: 0.22, ease: [0.23, 1, 0.32, 1] }}
          className="overflow-hidden rounded-lg bg-destructive/10 text-[13px] text-destructive ring-1 ring-inset ring-destructive/20"
          role="alert"
        >
          <div className="flex items-center gap-2 px-3 py-2">
            <CircleAlert className="h-4 w-4 shrink-0" />
            {error}
          </div>
        </motion.div>
      )}
    </AnimatePresence>
  )
}

export function Table({ children }: { children: ReactNode }) {
  return (
    <div className="table-wrap">
      <table className="dt">{children}</table>
    </div>
  )
}

export function Th({ children, className }: { children?: ReactNode; className?: string }) {
  return <th className={className}>{children}</th>
}

export function Td({ children, className }: { children?: ReactNode; className?: string }) {
  return <td className={className}>{children}</td>
}

// Segmented is a pill switch whose indicator slides to the selected option.
export function Segmented<T extends string>({
  value,
  onChange,
  options,
  className,
  label,
}: {
  value: T
  onChange: (v: T) => void
  options: { value: T; label: ReactNode }[]
  id?: string
  className?: string
  label?: string
}) {
  const wrap = useRef<HTMLDivElement>(null)
  const ind = useRef<HTMLSpanElement>(null)
  useLayoutEffect(() => {
    const place = () => {
      const btn = wrap.current?.querySelector<HTMLElement>('button[aria-pressed="true"]')
      if (!btn || !ind.current) return
      ind.current.style.setProperty('--x', `${btn.offsetLeft}px`)
      ind.current.style.setProperty('--w', `${btn.offsetWidth}px`)
    }
    place()
    const ro = new ResizeObserver(place)
    if (wrap.current) ro.observe(wrap.current)
    return () => ro.disconnect()
  }, [value, options.length])
  return (
    <div ref={wrap} className={cx('seg', className)} role="group" aria-label={label}>
      <span ref={ind} className="seg-ind" aria-hidden />
      {options.map((o) => (
        <button type="button" key={o.value} aria-pressed={value === o.value} onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  )
}
