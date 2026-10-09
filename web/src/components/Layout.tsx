import { useState } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import { Activity, KeyRound, LayoutDashboard, Languages, LogOut, Menu, Moon, Network, ScrollText, Server, ShieldCheck, Sun, Users, X } from 'lucide-react'
import { useAuth } from '../lib/auth'
import { useI18n } from '../lib/i18n'
import { useTheme } from '../lib/hooks'
import { post } from '../lib/api'
import { Button, ErrorNote, Field, IconButton, Input, Modal, cx } from './ui'

export function Logo() {
  return (
    <div className="flex items-center gap-2.5">
      <img src="/favicon.svg" alt="" className="h-8 w-8" />
      <span className="text-lg font-semibold tracking-tight text-slate-900 dark:text-white">Cloudrix</span>
    </div>
  )
}

export default function Layout() {
  const { admin, logout } = useAuth()
  const { t, lang, setLang } = useI18n()
  const { dark, toggle } = useTheme()
  const [open, setOpen] = useState(false)
  const [pwOpen, setPwOpen] = useState(false)
  const sudo = admin?.role === 'sudo'

  const nav = [
    { to: '/', label: t('nav.dashboard'), icon: LayoutDashboard, show: true },
    { to: '/users', label: t('nav.users'), icon: Users, show: true },
    { to: '/inbounds', label: t('nav.inbounds'), icon: Network, show: sudo },
    { to: '/nodes', label: t('nav.nodes'), icon: Server, show: sudo },
    { to: '/admins', label: t('nav.admins'), icon: ShieldCheck, show: sudo },
    { to: '/audit', label: t('nav.audit'), icon: ScrollText, show: sudo },
  ].filter((n) => n.show)

  const sidebar = (
    <div className="flex h-full flex-col">
      <div className="flex h-16 items-center justify-between px-5">
        <Logo />
        <IconButton className="lg:hidden" onClick={() => setOpen(false)} aria-label="close menu">
          <X className="h-4 w-4" />
        </IconButton>
      </div>
      <nav className="flex-1 space-y-1 px-3 py-2">
        {nav.map(({ to, label, icon: Icon }) => (
          <NavLink
            key={to}
            to={to}
            end={to === '/'}
            onClick={() => setOpen(false)}
            className={({ isActive }) =>
              cx(
                'flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
                isActive
                  ? 'bg-brand-50 text-brand-700 dark:bg-brand-500/10 dark:text-brand-300'
                  : 'text-slate-600 hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-800/60 dark:hover:text-white',
              )
            }
          >
            <Icon className="h-4 w-4" />
            {label}
          </NavLink>
        ))}
      </nav>
      <div className="border-t border-slate-100 p-3 dark:border-slate-800">
        <div className="mb-2 flex items-center gap-3 px-2 py-1.5">
          <div className="flex h-8 w-8 items-center justify-center rounded-full bg-gradient-to-br from-brand-400 to-brand-700 text-sm font-semibold uppercase text-white">
            {admin?.username[0]}
          </div>
          <div className="min-w-0">
            <div className="truncate text-sm font-medium text-slate-900 dark:text-white">{admin?.username}</div>
            <div className="text-xs text-slate-500">{admin && t(`admins.role.${admin.role}`)}</div>
          </div>
        </div>
        <div className="flex items-center gap-1">
          <IconButton onClick={toggle} title="theme" aria-label="toggle theme">
            {dark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </IconButton>
          <IconButton onClick={() => setLang(lang === 'ru' ? 'en' : 'ru')} title="language" aria-label="toggle language">
            <Languages className="h-4 w-4" />
          </IconButton>
          <IconButton onClick={() => setPwOpen(true)} title={t('nav.password')} aria-label={t('nav.password')}>
            <KeyRound className="h-4 w-4" />
          </IconButton>
          <IconButton onClick={logout} title={t('nav.logout')} aria-label={t('nav.logout')} className="ml-auto">
            <LogOut className="h-4 w-4" />
          </IconButton>
        </div>
      </div>
    </div>
  )

  return (
    <div className="min-h-screen bg-slate-50 dark:bg-slate-950">
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-64 border-r border-slate-200/80 bg-white dark:border-slate-800 dark:bg-slate-900/40 lg:block">
        {sidebar}
      </aside>
      {open && (
        <div className="fixed inset-0 z-40 lg:hidden">
          <div className="absolute inset-0 bg-slate-950/50" onClick={() => setOpen(false)} />
          <aside className="absolute inset-y-0 left-0 w-72 bg-white dark:bg-slate-900">{sidebar}</aside>
        </div>
      )}
      <div className="lg:pl-64">
        <header className="sticky top-0 z-20 flex h-14 items-center gap-3 border-b border-slate-200/80 bg-white/80 px-4 backdrop-blur dark:border-slate-800 dark:bg-slate-950/80 lg:hidden">
          <IconButton onClick={() => setOpen(true)} aria-label="open menu">
            <Menu className="h-5 w-5" />
          </IconButton>
          <Logo />
          <Activity className="ml-auto h-4 w-4 text-emerald-500" />
        </header>
        <main className="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
          <Outlet />
        </main>
      </div>
      <PasswordModal open={pwOpen} onClose={() => setPwOpen(false)} />
    </div>
  )
}

function PasswordModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useI18n()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const close = () => {
    setCurrent('')
    setNext('')
    setError(null)
    onClose()
  }
  return (
    <Modal
      open={open}
      onClose={close}
      title={t('nav.password')}
      footer={
        <Button
          loading={busy}
          onClick={async () => {
            setBusy(true)
            try {
              await post('/api/auth/password', { current, new: next })
              close()
            } catch (e) {
              setError((e as Error).message)
            } finally {
              setBusy(false)
            }
          }}
        >
          {t('common.save')}
        </Button>
      }
    >
      <ErrorNote error={error} />
      <div className="space-y-4">
        <Field label={t('password.current')}>
          <Input type="password" value={current} onChange={(e) => setCurrent(e.target.value)} autoComplete="current-password" />
        </Field>
        <Field label={t('password.new')}>
          <Input type="password" value={next} onChange={(e) => setNext(e.target.value)} autoComplete="new-password" minLength={8} />
        </Field>
      </div>
    </Modal>
  )
}
