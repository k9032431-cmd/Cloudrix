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
        initial={{ opacity: 0, y: 8 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.35, ease: [0.22, 1, 0.36, 1] }}
        className="relative w-full max-w-sm rounded-panel border border-border bg-card p-8 shadow-lg"
      >
        <motion.div className="mb-6 flex justify-center" initial={{ opacity: 0, y: -8 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.15 }}>
          <Logo />
        </motion.div>
        <h1 className="mb-6 text-center text-lg font-semibold text-foreground">{t('login.title')}</h1>
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

// AuroraBackground: a faint dot grid on the sidebar ground, fading at the edges.
export function AuroraBackground() {
  return (
    <div
      aria-hidden
      className="pointer-events-none absolute inset-0 -z-10 bg-sidebar [background-size:20px_20px] [mask-image:radial-gradient(ellipse_at_center,black_35%,transparent_80%)]"
      style={{ backgroundImage: 'radial-gradient(var(--border-strong) 1px, transparent 1px)' }}
    />
  )
}
