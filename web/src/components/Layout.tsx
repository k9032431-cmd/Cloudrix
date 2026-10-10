import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { NavLink, useLocation, useNavigate, useOutlet } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import {
  ChevronRight,
  ChevronsUpDown,
  KeyRound,
  LayoutDashboard,
  Languages,
  LogOut,
  Moon,
  Network,
  PanelLeft,
  Plus,
  ScrollText,
  Search,
  Server,
  Settings,
  ShieldCheck,
  Sun,
  Users,
  type Icon,
} from './icons'
import CommandPalette, { openPalette, shortcut, type Command } from './CommandPalette'
import { useAuth } from '../lib/auth'
import { useI18n } from '../lib/i18n'
import { useFetch, useTheme } from '../lib/hooks'
import { post } from '../lib/api'
import { formatBytes, formatDuration } from '../lib/format'
import type { Stats } from '../lib/types'
import { Button, ErrorNote, Field, IconButton, Input, Modal, ProgressBar, cx } from './ui'

// LogoMark is the square app mark; it follows the theme (dark tile on light, light tile on dark).
export function LogoMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" aria-hidden className={cx('shrink-0', className)}>
      <rect width="32" height="32" rx="8" className="fill-slate-900 dark:fill-white" />
      <path d="M10 20.5a4.5 4.5 0 0 1 .6-8.96A6 6 0 0 1 22 13a3.75 3.75 0 0 1 0 7.5Z" className="fill-white dark:fill-slate-900" />
    </svg>
  )
}

export function Logo() {
  return (
    <div className="flex items-center gap-2.5">
      <LogoMark className="h-8 w-8" />
      <span className="text-base font-semibold tracking-tight text-slate-900 dark:text-white">Cloudrix</span>
    </div>
  )
}

// Popover is a small floating panel. It closes on a click outside its
// [data-popover-root] wrapper (which also holds the button that opens it).
function Popover({ open, onClose, className, children }: { open: boolean; onClose: () => void; className?: string; children: ReactNode }) {
  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (!(e.target as Element).closest?.('[data-popover-root]')) onClose()
    }
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, onClose])
  return (
    <AnimatePresence>
      {open && (
        <motion.div
          initial={{ opacity: 0, scale: 0.97 }}
          animate={{ opacity: 1, scale: 1 }}
          exit={{ opacity: 0, scale: 0.97, transition: { duration: 0.1 } }}
          transition={{ duration: 0.15 }}
          className={cx(
            'absolute z-50 rounded-lg border border-slate-200 bg-white p-1 shadow-lg shadow-slate-900/5 dark:border-slate-800 dark:bg-slate-900',
            className,
          )}
        >
          {children}
        </motion.div>
      )}
    </AnimatePresence>
  )
}

function MenuItem({ icon: Icon, children, onClick, danger }: { icon: Icon; children: ReactNode; onClick: () => void; danger?: boolean }) {
  return (
    <button
      onClick={onClick}
      className={cx(
        'flex w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-left text-sm transition-colors',
        danger
          ? 'text-rose-600 hover:bg-rose-50 dark:text-rose-400 dark:hover:bg-rose-500/10'
          : 'text-slate-700 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800',
      )}
    >
      <Icon className="h-4 w-4 opacity-70" />
      {children}
    </button>
  )
}

const desktop = () => window.matchMedia('(min-width: 1024px)').matches

export default function Layout() {
  const { admin, logout } = useAuth()
  const { t, lang, setLang } = useI18n()
  const { dark, toggle } = useTheme()
  const [open, setOpen] = useState(false)
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem('sidebar') === 'collapsed'
    } catch {
      return false
    }
  })
  const [menu, setMenu] = useState<'' | 'workspace' | 'account'>('')
  const [pwOpen, setPwOpen] = useState(false)
  const location = useLocation()
  const outlet = useOutlet()
  const navigate = useNavigate()
  const sudo = admin?.role === 'sudo'
  const { data: stats, reload } = useFetch<Stats>('/api/system/stats?days=1')

  useEffect(() => {
    const id = setInterval(reload, 30_000)
    return () => clearInterval(id)
  }, [reload])

  useEffect(() => {
    setOpen(false)
    setMenu('')
  }, [location.pathname])

  const closeMenu = useCallback(() => setMenu(''), [])

  const toggleSidebar = () => {
    if (!desktop()) return setOpen(true)
    setCollapsed((c) => {
      try {
        localStorage.setItem('sidebar', c ? 'open' : 'collapsed')
      } catch {
        /* ignore */
      }
      return !c
    })
  }

  type Item = { to: string; label: string; icon: Icon; show: boolean; count?: string }
  const groups: { label: string; items: Item[] }[] = [
    {
      label: t('nav.groupMain'),
      items: [
        { to: '/', label: t('nav.dashboard'), icon: LayoutDashboard, show: true },
        { to: '/users', label: t('nav.users'), icon: Users, show: true, count: stats ? String(stats.users.total) : undefined },
        { to: '/inbounds', label: t('nav.inbounds'), icon: Network, show: sudo },
        { to: '/nodes', label: t('nav.nodes'), icon: Server, show: sudo, count: stats?.nodes?.total ? `${stats.nodes.connected}/${stats.nodes.total}` : undefined },
      ],
    },
    {
      label: t('nav.groupAdmin'),
      items: [
        { to: '/admins', label: t('nav.admins'), icon: ShieldCheck, show: sudo },
        { to: '/audit', label: t('nav.audit'), icon: ScrollText, show: sudo },
        { to: '/settings', label: t('nav.settings'), icon: Settings, show: sudo },
      ],
    },
  ]
    .map((g) => ({ ...g, items: g.items.filter((i) => i.show) }))
    .filter((g) => g.items.length > 0)
  const nav = groups.flatMap((g) => g.items)
  const current = nav.find((n) => (n.to === '/' ? location.pathname === '/' : location.pathname.startsWith(n.to)))

  const switchLang = () => setLang(lang === 'ru' ? 'en' : 'ru')
  const commands: Command[] = [
    ...nav.map((n) => ({ id: `page-${n.to}`, label: n.label, group: t('cmd.pages'), icon: n.icon, run: () => navigate(n.to) })),
    { id: 'new-user', label: t('users.new'), group: t('cmd.actions'), icon: Plus, run: () => navigate('/users?new=1') },
    { id: 'theme', label: t('cmd.theme'), group: t('cmd.actions'), icon: dark ? Sun : Moon, run: toggle },
    { id: 'lang', label: t('cmd.lang'), group: t('cmd.actions'), icon: Languages, run: switchLang },
    { id: 'password', label: t('nav.password'), group: t('cmd.actions'), icon: KeyRound, run: () => setPwOpen(true) },
    { id: 'logout', label: t('nav.logout'), group: t('cmd.actions'), icon: LogOut, run: logout },
  ]

  const u = stats?.users
  // `instance` keeps the sliding highlight of the desktop and mobile menus apart.
  const sidebar = (instance: string) => (
    <div className="flex h-full flex-col">
      <div className="relative p-2" data-popover-root>
        <button
          onClick={() => setMenu(menu === 'workspace' ? '' : 'workspace')}
          className="flex w-full items-center gap-2.5 rounded-lg p-2 text-left transition-colors hover:bg-slate-200/50 dark:hover:bg-slate-800/60"
        >
          <LogoMark className="h-8 w-8" />
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-semibold text-slate-900 dark:text-white">Cloudrix</span>
            <span className="block truncate text-xs text-slate-500">{t('nav.workspace')}</span>
          </span>
          <ChevronsUpDown className="h-4 w-4 text-slate-400" />
        </button>
        <Popover open={menu === 'workspace'} onClose={closeMenu} className="left-2 right-2 top-[3.75rem] origin-top p-3">
          <dl className="space-y-1.5 text-xs">
            <InfoRow label={t('dash.version')} value={stats?.system.version} />
            <InfoRow label={t('dash.uptime')} value={stats ? formatDuration(stats.system.uptime, lang) : undefined} />
            {stats?.nodes && <InfoRow label={t('dash.nodes')} value={`${stats.nodes.connected} / ${stats.nodes.total}`} />}
            <InfoRow label={t('dash.memory')} value={stats ? formatBytes(stats.system.mem_alloc) : undefined} />
            <InfoRow label="CPU" value={stats ? String(stats.system.cpus) : undefined} />
          </dl>
        </Popover>
      </div>

      <nav className="flex-1 overflow-y-auto px-2">
        {groups.map((g) => (
          <div key={g.label} className="pb-2">
            <div className="px-2 pb-1 pt-3 text-xs font-medium text-slate-500 dark:text-slate-500">{g.label}</div>
            <div className="space-y-0.5">
              {g.items.map(({ to, label, icon: Icon, count }) => (
                <NavLink
                  key={to}
                  to={to}
                  end={to === '/'}
                  className={({ isActive }) =>
                    cx(
                      'relative flex h-9 items-center gap-2.5 rounded-md px-2 text-sm transition-colors duration-150 lg:h-8',
                      isActive
                        ? 'font-medium text-slate-900 dark:text-white'
                        : 'text-slate-600 hover:bg-slate-200/50 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-800/50 dark:hover:text-white',
                    )
                  }
                >
                  {({ isActive }) => (
                    <>
                      {isActive && (
                        <motion.span
                          layoutId={`nav-${instance}`}
                          className="absolute inset-0 rounded-md bg-white shadow-sm ring-1 ring-slate-200 dark:bg-slate-800/80 dark:ring-slate-700/60"
                          transition={{ type: 'spring', stiffness: 500, damping: 40 }}
                        />
                      )}
                      <Icon className={cx('relative h-4 w-4', isActive ? 'text-slate-900 dark:text-white' : 'text-slate-500')} />
                      <span className="relative flex-1 truncate">{label}</span>
                      {count && <span className="relative font-mono text-[11px] tabular-nums text-slate-400">{count}</span>}
                    </>
                  )}
                </NavLink>
              ))}
            </div>
          </div>
        ))}
      </nav>

      {u && u.total > 0 && (
        <div className="mx-2 mb-2 rounded-lg border border-slate-200 bg-white p-3 dark:border-slate-800 dark:bg-slate-900/60">
          <div className="mb-2 flex items-baseline justify-between gap-2 text-xs">
            <span className="font-medium text-slate-900 dark:text-white">{t('nav.activeUsers')}</span>
            <span className="tabular-nums text-slate-500">{t('nav.ofTotal', { n: u.active, total: u.total })}</span>
          </div>
          <ProgressBar value={u.active} max={u.total} />
          {u.expiring_soon > 0 && <p className="mt-2 text-xs text-slate-500">{t('nav.expiringHint', { n: u.expiring_soon })}</p>}
        </div>
      )}

      <div className="relative border-t border-slate-200 p-2 dark:border-slate-800" data-popover-root>
        <button
          onClick={() => setMenu(menu === 'account' ? '' : 'account')}
          className="flex w-full items-center gap-2.5 rounded-lg p-2 text-left transition-colors hover:bg-slate-200/50 dark:hover:bg-slate-800/60"
        >
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-slate-200 text-sm font-medium uppercase text-slate-700 dark:bg-slate-800 dark:text-slate-200">
            {admin?.username[0]}
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-medium text-slate-900 dark:text-white">{admin?.username}</span>
            <span className="block truncate text-xs text-slate-500">{admin && t(`admins.role.${admin.role}`)}</span>
          </span>
          <ChevronsUpDown className="h-4 w-4 text-slate-400" />
        </button>
        <Popover open={menu === 'account'} onClose={closeMenu} className="bottom-[3.75rem] left-2 right-2 origin-bottom">
          <MenuItem icon={dark ? Sun : Moon} onClick={toggle}>
            {t('cmd.theme')}
          </MenuItem>
          <MenuItem icon={Languages} onClick={switchLang}>
            {t('cmd.lang')}
          </MenuItem>
          <MenuItem
            icon={KeyRound}
            onClick={() => {
              setMenu('')
              setPwOpen(true)
            }}
          >
            {t('nav.password')}
          </MenuItem>
          <div className="my-1 h-px bg-slate-100 dark:bg-slate-800" />
          <MenuItem icon={LogOut} onClick={logout} danger>
            {t('nav.logout')}
          </MenuItem>
        </Popover>
      </div>
    </div>
  )

  return (
    <div className="min-h-screen">
      <motion.aside
        initial={false}
        animate={{ x: collapsed ? '-100%' : 0 }}
        transition={{ type: 'spring', stiffness: 400, damping: 40 }}
        className="fixed inset-y-0 left-0 z-30 hidden w-60 border-r border-slate-200 bg-slate-50 dark:border-slate-800 dark:bg-slate-950 lg:block"
      >
        {sidebar('desktop')}
      </motion.aside>
      <AnimatePresence>
        {open && (
          <div className="fixed inset-0 z-40 lg:hidden">
            <motion.div
              className="absolute inset-0 bg-slate-950/40"
              onClick={() => setOpen(false)}
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
            />
            <motion.aside
              className="absolute inset-y-0 left-0 w-72 border-r border-slate-200 bg-slate-50 shadow-xl dark:border-slate-800 dark:bg-slate-950"
              initial={{ x: '-100%' }}
              animate={{ x: 0 }}
              exit={{ x: '-100%' }}
              transition={{ type: 'spring', stiffness: 400, damping: 40 }}
            >
              {sidebar('mobile')}
            </motion.aside>
          </div>
        )}
      </AnimatePresence>

      <div className={cx('transition-[padding] duration-300', !collapsed && 'lg:pl-60')}>
        <header className="sticky top-0 z-20 flex h-14 items-center gap-2 border-b border-slate-200 bg-white/80 px-4 backdrop-blur-md dark:border-slate-800 dark:bg-slate-950/80 lg:px-6">
          <IconButton onClick={toggleSidebar} aria-label={t('nav.toggleSidebar')} title={t('nav.toggleSidebar')}>
            <PanelLeft className="h-[18px] w-[18px]" />
          </IconButton>
          <span className="mx-1 h-4 w-px bg-slate-200 dark:bg-slate-800" />
          <nav aria-label="breadcrumb" className="flex min-w-0 items-center gap-1.5 text-sm">
            <span className="hidden text-slate-500 sm:inline">Cloudrix</span>
            <ChevronRight className="hidden h-3 w-3 text-slate-400 sm:block" />
            <span className="truncate font-medium text-slate-900 dark:text-white">{current?.label}</span>
          </nav>
          <div className="ml-auto flex items-center gap-1.5">
            <button
              onClick={openPalette}
              className="hidden h-9 w-64 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-sm text-slate-500 shadow-sm transition-colors hover:border-slate-300 hover:text-slate-700 dark:border-slate-800 dark:bg-slate-900 dark:hover:border-slate-700 dark:hover:text-slate-300 sm:flex"
            >
              <Search className="h-4 w-4" />
              <span className="flex-1 truncate text-left">{t('cmd.searchLong')}</span>
              <kbd className="rounded border border-slate-200 bg-slate-50 px-1.5 py-px font-mono text-[10px] text-slate-500 dark:border-slate-700 dark:bg-slate-800">
                {shortcut}
              </kbd>
            </button>
            <IconButton onClick={openPalette} className="sm:hidden" aria-label={t('cmd.search')}>
              <Search className="h-[18px] w-[18px]" />
            </IconButton>
            <IconButton onClick={toggle} aria-label={t('cmd.theme')} title={t('cmd.theme')}>
              <AnimatePresence mode="wait" initial={false}>
                <motion.span
                  key={dark ? 'sun' : 'moon'}
                  initial={{ rotate: -90, opacity: 0 }}
                  animate={{ rotate: 0, opacity: 1 }}
                  exit={{ rotate: 90, opacity: 0 }}
                  transition={{ duration: 0.15 }}
                  className="inline-flex"
                >
                  {dark ? <Sun className="h-[18px] w-[18px]" /> : <Moon className="h-[18px] w-[18px]" />}
                </motion.span>
              </AnimatePresence>
            </IconButton>
          </div>
        </header>
        <main className="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
          {/* Only the incoming page animates: waiting for the old page's exit can
              stall on shared-layout elements (e.g. a Segmented highlight). */}
          <motion.div
            key={location.pathname}
            initial={{ opacity: 0, y: 4 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.2, ease: [0.22, 1, 0.36, 1] }}
          >
            {outlet}
          </motion.div>
        </main>
      </div>
      <PasswordModal open={pwOpen} onClose={() => setPwOpen(false)} />
      <CommandPalette commands={commands} />
    </div>
  )
}

function InfoRow({ label, value }: { label: string; value?: string }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <dt className="text-slate-500">{label}</dt>
      <dd className="font-medium tabular-nums text-slate-900 dark:text-white">{value ?? '—'}</dd>
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
