import { useState, type FormEvent } from 'react'
import { Navigate } from 'react-router-dom'
import { Languages, Moon, Sun } from 'lucide-react'
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
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden bg-slate-50 px-4 dark:bg-slate-950">
      <div className="pointer-events-none absolute -top-40 left-1/2 h-[480px] w-[480px] -translate-x-1/2 rounded-full bg-brand-400/20 blur-3xl dark:bg-brand-600/20" />
      <div className="absolute right-4 top-4 flex gap-1">
        <IconButton onClick={toggle} aria-label="toggle theme">
          {dark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
        </IconButton>
        <IconButton onClick={() => setLang(lang === 'ru' ? 'en' : 'ru')} aria-label="toggle language">
          <Languages className="h-4 w-4" />
        </IconButton>
      </div>
      <form
        onSubmit={submit}
        className="relative w-full max-w-sm rounded-2xl border border-slate-200/80 bg-white/90 p-8 shadow-xl backdrop-blur dark:border-slate-800 dark:bg-slate-900/80"
      >
        <div className="mb-6 flex justify-center">
          <Logo />
        </div>
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
      </form>
    </div>
  )
}
