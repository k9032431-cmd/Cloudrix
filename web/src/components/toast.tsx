import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { AnimatePresence, motion } from 'framer-motion'
import { CheckCircle2, Info, X } from './icons'
import { cx } from './ui'

type Tone = 'success' | 'error' | 'info'
interface Toast {
  id: number
  text: string
  tone: Tone
}

const Ctx = createContext<(text: string, tone?: Tone) => void>(() => {})

// ToastProvider shows short-lived notifications in the corner (bottom on phones).
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const seq = useRef(0)

  const dismiss = useCallback((id: number) => setToasts((t) => t.filter((x) => x.id !== id)), [])
  const push = useCallback(
    (text: string, tone: Tone = 'success') => {
      const id = ++seq.current
      setToasts((t) => [...t.slice(-3), { id, text, tone }])
      window.setTimeout(() => dismiss(id), tone === 'error' ? 6000 : 3200)
    },
    [dismiss],
  )

  return (
    <Ctx.Provider value={push}>
      {children}
      {createPortal(
        <div
          aria-live="polite"
          className="pointer-events-none fixed inset-x-0 bottom-0 z-[60] flex flex-col items-center gap-2 p-4 sm:inset-x-auto sm:right-0 sm:items-end"
        >
          <AnimatePresence initial={false}>
            {toasts.map((t) => (
              <motion.div
                key={t.id}
                layout
                initial={{ opacity: 0, y: 24, scale: 0.9 }}
                animate={{ opacity: 1, y: 0, scale: 1 }}
                exit={{ opacity: 0, x: 40, scale: 0.95 }}
                transition={{ type: 'spring', stiffness: 500, damping: 34 }}
                className={cx(
                  'pointer-events-auto flex w-full max-w-sm items-center gap-3 rounded-2xl border px-4 py-3 text-sm shadow-2xl backdrop-blur-xl',
                  'border-slate-200/70 bg-white/90 text-slate-800 dark:border-white/10 dark:bg-slate-900/90 dark:text-slate-100',
                )}
              >
                <span
                  className={cx(
                    'flex h-8 w-8 shrink-0 items-center justify-center rounded-xl',
                    t.tone === 'success' && 'bg-emerald-500/15 text-emerald-500',
                    t.tone === 'error' && 'bg-rose-500/15 text-rose-500',
                    t.tone === 'info' && 'bg-brand-500/15 text-brand-500',
                  )}
                >
                  {t.tone === 'success' ? <CheckCircle2 className="h-5 w-5" /> : <Info className="h-5 w-5" />}
                </span>
                <span className="min-w-0 flex-1">{t.text}</span>
                <button onClick={() => dismiss(t.id)} className="rounded-lg p-1 text-slate-400 transition-colors hover:text-slate-700 dark:hover:text-white" aria-label="close">
                  <X className="h-3.5 w-3.5" />
                </button>
              </motion.div>
            ))}
          </AnimatePresence>
        </div>,
        document.body,
      )}
    </Ctx.Provider>
  )
}

export const useToast = () => useContext(Ctx)
