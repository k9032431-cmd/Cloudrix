import { useState } from 'react'
import { NavLink, useLocation, useOutlet } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import { Activity, KeyRound, LayoutDashboard, Languages, LogOut, Menu, Moon, Network, ScrollText, Server, Settings, ShieldCheck, Sun, Users, X } from './icons'
import { useAuth } from '../lib/auth'
import { useI18n } from '../lib/i18n'
import { useTheme } from '../lib/hooks'
import { post } from '../lib/api'
import { Button, ErrorNote, Field, IconButton, Input, Modal, cx } from './ui'

export function Logo() {
  return (
    <div className="group flex items-center gap-2.5">
      <span className="relative">
        <span className="absolute inset-0 rounded-xl bg-brand-500/50 blur-md transition-opacity duration-500 group-hover:opacity-100 opacity-60" />
        <motion.img
          src="/favicon.svg"
          alt=""
          className="relative h-8 w-8"
          whileHover={{ rotate: -8, scale: 1.08 }}
          transition={{ type: 'spring', stiffness: 400, damping: 15 }}
        />
      </span>
      <span className="text-lg font-semibold tracking-tight text-slate-900 dark:text-white">
        Cloud<span className="text-gradient">rix</span>
      </span>
    </div>
  )
}

export default function Layout() {
  const { admin, logout } = useAuth()
  const { t, lang, setLang } = useI18n()
  const { dark, toggle } = useTheme()
  const [open, setOpen] = useState(false)
  const [pwOpen, setPwOpen] = useState(false)
  const location = useLocation()
  const outlet = useOutlet()
  const sudo = admin?.role === 'sudo'

  const nav = [
    { to: '/', label: t('nav.dashboard'), icon: LayoutDashboard, show: true },
    { to: '/users', label: t('nav.users'), icon: Users, show: true },
    { to: '/inbounds', label: t('nav.inbounds'), icon: Network, show: sudo },
    { to: '/nodes', label: t('nav.nodes'), icon: Server, show: sudo },
    { to: '/admins', label: t('nav.admins'), icon: ShieldCheck, show: sudo },
    { to: '/audit', label: t('nav.audit'), icon: ScrollText, show: sudo },
    { to: '/settings', label: t('nav.settings'), icon: Settings, show: sudo },
  ].filter((n) => n.show)

  // `instance` keeps the sliding highlight of the desktop and mobile menus apart.
  const sidebar = (instance: string) => (
    <div className="flex h-full flex-col">
      <div className="flex h-16 items-center justify-between px-5">
        <Logo />
        <IconButton className="lg:hidden" onClick={() => setOpen(false)} aria-label="close menu">
          <X className="h-4 w-4" />
        </IconButton>
      </div>
      <nav className="flex-1 space-y-1 px-3 py-3">
        {nav.map(({ to, label, icon: Icon }, i) => (
          <motion.div key={to} initial={{ opacity: 0, x: -10 }} animate={{ opacity: 1, x: 0 }} transition={{ delay: 0.04 * i, duration: 0.3 }}>
            <NavLink
              to={to}
              end={to === '/'}
              onClick={() => setOpen(false)}
              className={({ isActive }) =>
                cx(
                  'group relative flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors duration-200',
                  isActive ? 'text-brand-700 dark:text-white' : 'text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white',
                )
              }
            >
              {({ isActive }) => (
                <>
                  {isActive && (
                    <motion.span
                      layoutId={`nav-pill-${instance}`}
                      className="absolute inset-0 rounded-xl bg-gradient-to-r from-brand-500/15 to-violet-500/10 ring-1 ring-brand-500/20 dark:from-brand-500/25 dark:to-violet-500/15 dark:ring-brand-400/20"
                      transition={{ type: 'spring', stiffness: 380, damping: 32 }}
                    />
                  )}
                  {isActive && (
                    <motion.span
                      layoutId={`nav-bar-${instance}`}
                      className="absolute -left-3 top-2 bottom-2 w-1 rounded-r-full bg-gradient-to-b from-brand-400 to-violet-500"
                      transition={{ type: 'spring', stiffness: 380, damping: 32 }}
                    />
                  )}
                  <span
                    className={cx(
                      'relative flex h-7 w-7 items-center justify-center rounded-lg transition-all duration-300',
                      isActive
                        ? 'bg-gradient-to-br from-brand-500 to-violet-600 text-white shadow-md shadow-brand-500/30'
                        : 'bg-slate-100 text-slate-500 group-hover:scale-110 group-hover:bg-white group-hover:text-brand-600 group-hover:shadow-sm dark:bg-slate-800/80 dark:text-slate-400 dark:group-hover:bg-slate-700 dark:group-hover:text-brand-300',
                    )}
                  >
                    <Icon className="h-4 w-4" />
                  </span>
                  <span className="relative">{label}</span>
                </>
              )}
            </NavLink>
          </motion.div>
        ))}
      </nav>
      <div className="m-3 rounded-2xl border border-slate-200/70 bg-slate-50/80 p-3 dark:border-white/5 dark:bg-white/[0.03]">
        <div className="mb-2 flex items-center gap-3 px-1 py-1">
          <div className="relative flex h-9 w-9 items-center justify-center rounded-full bg-gradient-to-br from-brand-400 via-violet-500 to-fuchsia-500 text-sm font-semibold uppercase text-white shadow-md shadow-violet-500/30">
            {admin?.username[0]}
            <span className="absolute -bottom-0.5 -right-0.5 h-3 w-3 rounded-full border-2 border-white bg-emerald-500 dark:border-slate-900" />
          </div>
          <div className="min-w-0">
            <div className="truncate text-sm font-semibold text-slate-900 dark:text-white">{admin?.username}</div>
            <div className="text-xs text-slate-500">{admin && t(`admins.role.${admin.role}`)}</div>
          </div>
        </div>
        <div className="flex items-center gap-1">
          <IconButton onClick={toggle} title="theme" aria-label="toggle theme">
            <AnimatePresence mode="wait" initial={false}>
              <motion.span
                key={dark ? 'sun' : 'moon'}
                initial={{ rotate: -90, opacity: 0, scale: 0.5 }}
                animate={{ rotate: 0, opacity: 1, scale: 1 }}
                exit={{ rotate: 90, opacity: 0, scale: 0.5 }}
                transition={{ duration: 0.2 }}
                className="inline-flex"
              >
                {dark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
              </motion.span>
            </AnimatePresence>
          </IconButton>
          <IconButton onClick={() => setLang(lang === 'ru' ? 'en' : 'ru')} title="language" aria-label="toggle language">
            <Languages className="h-4 w-4" />
          </IconButton>
          <IconButton onClick={() => setPwOpen(true)} title={t('nav.password')} aria-label={t('nav.password')}>
            <KeyRound className="h-4 w-4" />
          </IconButton>
          <IconButton onClick={logout} title={t('nav.logout')} aria-label={t('nav.logout')} className="ml-auto hover:!bg-rose-50 hover:!text-rose-600 dark:hover:!bg-rose-500/10">
            <LogOut className="h-4 w-4" />
          </IconButton>
        </div>
      </div>
    </div>
  )

  return (
    <div className="min-h-screen">
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-64 border-r border-slate-200/70 bg-white/70 backdrop-blur-xl dark:border-white/[0.06] dark:bg-slate-950/50 lg:block">
        {sidebar('desktop')}
      </aside>
      <AnimatePresence>
        {open && (
          <div className="fixed inset-0 z-40 lg:hidden">
            <motion.div
              className="absolute inset-0 bg-slate-950/50 backdrop-blur-sm"
              onClick={() => setOpen(false)}
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
            />
            <motion.aside
              className="absolute inset-y-0 left-0 w-72 bg-white shadow-2xl dark:bg-slate-900"
              initial={{ x: '-100%' }}
              animate={{ x: 0 }}
              exit={{ x: '-100%' }}
              transition={{ type: 'spring', stiffness: 380, damping: 38 }}
            >
              {sidebar('mobile')}
            </motion.aside>
          </div>
        )}
      </AnimatePresence>
      <div className="lg:pl-64">
        <header className="sticky top-0 z-20 flex h-14 items-center gap-3 border-b border-slate-200/70 bg-white/70 px-4 backdrop-blur-xl dark:border-white/[0.06] dark:bg-slate-950/60 lg:hidden">
          <IconButton onClick={() => setOpen(true)} aria-label="open menu">
            <Menu className="h-5 w-5" />
          </IconButton>
          <Logo />
          <Activity className="ml-auto h-5 w-5 text-emerald-500" />
        </header>
        <main className="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
          <AnimatePresence mode="wait" initial={false}>
            <motion.div
              key={location.pathname}
              initial={{ opacity: 0, y: 10, filter: 'blur(4px)' }}
              animate={{ opacity: 1, y: 0, filter: 'blur(0px)' }}
              exit={{ opacity: 0, y: -6, filter: 'blur(4px)' }}
              transition={{ duration: 0.25, ease: [0.22, 1, 0.36, 1] }}
            >
              {outlet}
            </motion.div>
          </AnimatePresence>
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
