import { useState, type FormEvent } from 'react'
import { Navigate } from 'react-router-dom'
import { motion } from 'framer-motion'
import { Languages, Moon, Sun } from '../components/icons'
import { useAuth } from '../lib/auth'
import { useI18n } from '../lib/i18n'
import { useTheme } from '../lib/hooks'
import { Button, ErrorNote, Field, IconButton, Input } from '../components/ui'
import { Logo } from '../components/Layout'

export default function Login() {
  const { admin, login } = useAuth()
  const { t, lang, setLang } = useI18n()
  const { dark, toggle } = useTheme()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  if (admin) return <Navigate to="/" replace />

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await login(username, password)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden px-4">
      <AuroraBackground />
      <div className="absolute right-4 top-4 flex gap-1">
        <IconButton onClick={toggle} aria-label="toggle theme">
          {dark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
        </IconButton>
        <IconButton onClick={() => setLang(lang === 'ru' ? 'en' : 'ru')} aria-label="toggle language">
          <Languages className="h-4 w-4" />
        </IconButton>
      </div>
      <motion.form
        onSubmit={submit}
        initial={{ opacity: 0, y: 24, scale: 0.97 }}
        animate={{ opacity: 1, y: 0, scale: 1 }}
        transition={{ type: 'spring', stiffness: 260, damping: 24 }}
        className="relative w-full max-w-sm rounded-3xl border border-white/60 bg-white/70 p-8 shadow-2xl shadow-brand-900/10 backdrop-blur-2xl dark:border-white/10 dark:bg-slate-900/60 dark:shadow-black/40"
      >
        <motion.div className="mb-6 flex justify-center" initial={{ opacity: 0, y: -8 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.15 }}>
          <Logo />
        </motion.div>
        <h1 className="mb-6 text-center text-lg font-semibold text-slate-900 dark:text-white">{t('login.title')}</h1>
        <ErrorNote error={error} />
        <div className="space-y-4">
          <Field label={t('login.username')}>
            <Input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" autoFocus required />
          </Field>
          <Field label={t('login.password')}>
            <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" required />
          </Field>
          <Button type="submit" loading={busy} className="w-full">
            {t('login.submit')}
          </Button>
        </div>
      </motion.form>
    </div>
  )
}

// AuroraBackground: slowly drifting colour blobs behind the login card.
export function AuroraBackground() {
  return (
    <div aria-hidden className="pointer-events-none absolute inset-0 -z-10 overflow-hidden">
      <div className="absolute -left-32 -top-32 h-[28rem] w-[28rem] animate-float rounded-full bg-brand-500/30 blur-3xl dark:bg-brand-600/30" />
      <div className="absolute -right-24 top-1/4 h-[24rem] w-[24rem] animate-float-slow rounded-full bg-violet-500/25 blur-3xl [animation-delay:-4s] dark:bg-violet-600/25" />
      <div className="absolute -bottom-40 left-1/3 h-[26rem] w-[26rem] animate-float rounded-full bg-cyan-400/20 blur-3xl [animation-delay:-7s] dark:bg-cyan-500/15" />
      <div className="absolute inset-0 bg-[radial-gradient(rgba(15,23,42,0.06)_1px,transparent_1px)] [background-size:22px_22px] dark:bg-[radial-gradient(rgba(255,255,255,0.05)_1px,transparent_1px)]" />
    </div>
  )
}
