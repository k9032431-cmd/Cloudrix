import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { AnimatePresence, motion } from 'framer-motion'
import { CheckCircle2, CircleAlert, Info, X } from './icons'

type Tone = 'success' | 'error' | 'info'
interface Toast {
  id: number
  text: string
  tone: Tone
}

const Ctx = createContext<(text: string, tone?: Tone) => void>(() => {})

// ToastProvider shows short-lived notifications: a bottom-right stack of
// three (full width at the bottom on phones).
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const seq = useRef(0)

  const dismiss = useCallback((id: number) => setToasts((t) => t.filter((x) => x.id !== id)), [])
  const push = useCallback(
    (text: string, tone: Tone = 'success') => {
      const id = ++seq.current
      setToasts((t) => [...t.slice(-2), { id, text, tone }])
      window.setTimeout(() => dismiss(id), tone === 'error' ? 6000 : 3200)
    },
    [dismiss],
  )

  return (
    <Ctx.Provider value={push}>
      {children}
      {createPortal(
        <div aria-live="polite" className="toasts">
          <AnimatePresence initial={false}>
            {toasts.map((t) => (
              <motion.div
                key={t.id}
                layout
                initial={{ opacity: 0, y: 10, scale: 0.98 }}
                animate={{ opacity: 1, y: 0, scale: 1, transition: { duration: 0.32, ease: [0.23, 1, 0.32, 1] } }}
                exit={{ opacity: 0, y: 4, scale: 0.98, transition: { duration: 0.16 } }}
                className="toast"
                role={t.tone === 'error' ? 'alert' : 'status'}
              >
                {t.tone === 'success' ? <CheckCircle2 className="ok" /> : t.tone === 'error' ? <CircleAlert className="bad" /> : <Info className="info" />}
                <span className="t">{t.text}</span>
                <button type="button" onClick={() => dismiss(t.id)} className="btn btn-ghost btn-icon btn-sm -my-1 -mr-1" aria-label="close">
                  <X />
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
