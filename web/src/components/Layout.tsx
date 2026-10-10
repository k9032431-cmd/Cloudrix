import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type ReactNode } from 'react'
import { Link, NavLink, useLocation, useNavigate, useOutlet } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import {
  Bell,
  Check,
  ChevronRight,
  ChevronsUpDown,
  Contrast,
  Hourglass,
  KeyRound,
  LayoutDashboard,
  Languages,
  LogOut,
  Menu,
  Monitor,
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
  Zap,
  type Icon,
} from './icons'
import CommandPalette, { openPalette, shortcut, type Command } from './CommandPalette'
import { useAuth } from '../lib/auth'
import { useI18n } from '../lib/i18n'
import { useFetch, useTheme, type Density, type Mode, type Preset } from '../lib/hooks'
import { get, post } from '../lib/api'
import { formatBytes, formatDuration, relativeTime } from '../lib/format'
import type { Node, Stats, User } from '../lib/types'
import { Button, ErrorNote, Field, Input, Modal, cx, useFocusTrap } from './ui'

// BrandMark is the Cloudrix cloud on a primary tile (follows theme and preset).
export function BrandMark({ className }: { className?: string }) {
  return (
    <span className={cx('brand-mark', className)}>
      <svg viewBox="0 0 24 24" aria-hidden>
        <circle cx="9" cy="12" r="5.2" />
        <circle cx="15.5" cy="10.5" r="4.3" />
        <rect x="3" y="12" width="18" height="7.5" rx="3.75" />
      </svg>
    </span>
  )
}

export function Logo() {
  return (
    <span className="flex items-center gap-2.5">
      <BrandMark />
      <span className="text-[15px] font-semibold tracking-tight text-foreground">Cloudrix</span>
    </span>
  )
}

// Popover is a small floating panel. It closes on Esc or a click outside its
// [data-popover-root] wrapper (which also holds the button that opens it),
// moves focus into itself and lets arrow keys walk its menu items.
function Popover({ open, onClose, className, children, label }: { open: boolean; onClose: () => void; className?: string; children: ReactNode; label: string }) {
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (!(e.target as Element).closest?.('[data-popover-root]')) onClose()
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
      if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
      const items = Array.from(ref.current?.querySelectorAll<HTMLElement>('.menu-item, .notif, .theme-card') ?? [])
      if (!items.length) return
      e.preventDefault()
      const i = items.indexOf(document.activeElement as HTMLElement)
      items[(i + (e.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length].focus()
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    requestAnimationFrame(() => ref.current?.querySelector<HTMLElement>('.menu-item, .notif, .theme-card, button')?.focus({ preventScroll: true }))
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, onClose])
  return (
    <AnimatePresence>
      {open && (
        <motion.div
          ref={ref}
          role="dialog"
          aria-label={label}
          initial={{ opacity: 0, scale: 0.97 }}
          animate={{ opacity: 1, scale: 1 }}
          exit={{ opacity: 0, scale: 0.97, transition: { duration: 0.12 } }}
          transition={{ duration: 0.18, ease: [0.23, 1, 0.32, 1] }}
          className={cx('pop', className)}
        >
          {children}
        </motion.div>
      )}
    </AnimatePresence>
  )
}

function MenuItem({ icon: Icon, children, onClick, danger, checked }: { icon?: Icon; children: ReactNode; onClick: () => void; danger?: boolean; checked?: boolean }) {
  return (
    <button type="button" onClick={onClick} className={cx('menu-item', danger && 'danger')} role={checked === undefined ? undefined : 'menuitemradio'} aria-checked={checked}>
      {Icon && <Icon />}
      <span className="min-w-0 flex-1 truncate">{children}</span>
      {checked && <Check className="check h-4 w-4" />}
    </button>
  )
}

type SidebarState = 'rail' | 'open' | ''
const width = () => window.innerWidth
const storedRail = () => {
  try {
    return localStorage.getItem('sidebar') === 'collapsed'
  } catch {
    return false
  }
}

interface Notif {
  key: string
  icon: Icon
  tone: string
  name: string
  text: string
  when?: string
  to: string
}

// useNotifications derives notifications from live data: subscriptions ending
// within a week, users at their data limit and nodes that stopped answering.
function useNotifications(sudo: boolean, tick: number) {
  const { t, lang } = useI18n()
  const [items, setItems] = useState<Notif[]>([])
  const [read, setRead] = useState<Set<string>>(() => {
    try {
      return new Set(JSON.parse(localStorage.getItem('cloudrix.notifs.read') || '[]'))
    } catch {
      return new Set()
    }
  })
  useEffect(() => {
    let stale = false
    const week = Date.now() + 7 * 86_400_000
    Promise.all([
      get<{ users: User[] }>('/api/users?status=active&sort=expire_at&limit=50').catch(() => ({ users: [] as User[] })),
      get<{ users: User[] }>('/api/users?status=limited&limit=10').catch(() => ({ users: [] as User[] })),
      sudo ? get<Node[]>('/api/nodes').catch(() => [] as Node[]) : Promise.resolve([] as Node[]),
    ]).then(([active, limited, nodes]) => {
      if (stale) return
      const out: Notif[] = []
      for (const n of nodes) {
        if (n.status === 'error') out.push({ key: `node-${n.id}-${n.last_seen ?? ''}`, icon: Server, tone: 'var(--destructive)', name: n.name, text: t('notif.nodeError'), when: n.last_seen ?? undefined, to: '/nodes' })
      }
      for (const u of active.users) {
        if (!u.expire_at) continue
        const at = new Date(u.expire_at).getTime()
        if (at < Date.now() || at > week) continue
        out.push({ key: `exp-${u.id}-${u.expire_at}`, icon: Hourglass, tone: 'var(--warning)', name: u.username, text: t('notif.expiring', { when: relativeTime(u.expire_at, lang) }), to: `/users?open=${u.id}` })
      }
      for (const u of limited.users) {
        out.push({ key: `lim-${u.id}-${u.last_reset_at}`, icon: Zap, tone: 'var(--chart-1)', name: u.username, text: t('notif.limited'), to: `/users?open=${u.id}` })
      }
      setItems(out.slice(0, 12))
    })
    return () => {
      stale = true
    }
  }, [sudo, tick, t, lang])
  const save = (next: Set<string>) => {
    setRead(next)
    try {
      localStorage.setItem('cloudrix.notifs.read', JSON.stringify(Array.from(next).slice(-300)))
    } catch {
      /* ignore */
    }
  }
  const markRead = (key: string) => save(new Set(read).add(key))
  const markAll = () => save(new Set([...read, ...items.map((i) => i.key)]))
  const unread = items.filter((i) => !read.has(i.key)).length
  return { items, read, unread, markRead, markAll }
}

export default function Layout() {
  const { admin, logout } = useAuth()
  const { t, lang, setLang } = useI18n()
  const { prefs, dark, toggle, setPrefs } = useTheme()
  const [sidebar, setSidebar] = useState<SidebarState>(() => (width() >= 1024 && storedRail() ? 'rail' : ''))
  const [menu, setMenu] = useState<'' | 'workspace' | 'account' | 'notifs' | 'appearance'>('')
  const [pwOpen, setPwOpen] = useState(false)
  const [scrolled, setScrolled] = useState(false)
  const [tick, setTick] = useState(0)
  const location = useLocation()
  const outlet = useOutlet()
  const navigate = useNavigate()
  const sudo = admin?.role === 'sudo'
  const { data: stats, reload } = useFetch<Stats>('/api/system/stats?days=1')
  const notifs = useNotifications(sudo, tick)
  const sideRef = useRef<HTMLElement>(null)
  const mainRef = useRef<HTMLElement>(null)
  const sentinel = useRef<HTMLDivElement>(null)
  const firstRoute = useRef(true)

  useEffect(() => {
    const id = setInterval(() => {
      reload()
      setTick((n) => n + 1)
    }, 60_000)
    return () => clearInterval(id)
  }, [reload])

  // data-sidebar on <html> drives the rail (desktop/tablet) and the drawer (phones)
  useEffect(() => {
    const root = document.documentElement
    if (sidebar) root.setAttribute('data-sidebar', sidebar)
    else root.removeAttribute('data-sidebar')
    return () => root.removeAttribute('data-sidebar')
  }, [sidebar])
  const drawerOpen = sidebar === 'open' && width() < 768
  useFocusTrap(sideRef, drawerOpen)
  useEffect(() => {
    if (!drawerOpen) return
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setSidebar('')
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [drawerOpen])

  // header gets its hairline and blur only once content scrolls under it
  useEffect(() => {
    const el = sentinel.current
    if (!el) return
    const io = new IntersectionObserver(([e]) => setScrolled(!e.isIntersecting))
    io.observe(el)
    return () => io.disconnect()
  }, [])

  // route change: close overlays, scroll to top, move focus to the view's h1
  useEffect(() => {
    setMenu('')
    setSidebar((s) => (s === 'open' ? '' : s))
    if (firstRoute.current) {
      firstRoute.current = false
      return
    }
    mainRef.current?.scrollTo({ top: 0 })
    window.scrollTo({ top: 0 })
    requestAnimationFrame(() => mainRef.current?.querySelector<HTMLElement>('.page-title')?.focus({ preventScroll: true }))
  }, [location.pathname])

  const closeMenu = useCallback(() => setMenu(''), [])
  const toggleMenu = (m: typeof menu) => setMenu((cur) => (cur === m ? '' : m))

  const toggleSidebar = () => {
    const w = width()
    if (w < 1024) return setSidebar((s) => (s === 'open' ? '' : 'open'))
    setSidebar((s) => {
      const next = s === 'rail' ? '' : 'rail'
      try {
        localStorage.setItem('sidebar', next ? 'collapsed' : 'open')
      } catch {
        /* ignore */
      }
      return next
    })
  }

  type Item = { to: string; label: string; icon: Icon; show: boolean; count?: string }
  const groups = useMemo(
    () =>
      [
        {
          label: t('nav.groupMain'),
          items: [
            { to: '/', label: t('nav.dashboard'), icon: LayoutDashboard, show: true },
            { to: '/users', label: t('nav.users'), icon: Users, show: true, count: stats ? String(stats.users.total) : undefined },
            { to: '/inbounds', label: t('nav.inbounds'), icon: Network, show: sudo },
            { to: '/nodes', label: t('nav.nodes'), icon: Server, show: sudo, count: stats?.nodes?.total ? `${stats.nodes.connected}/${stats.nodes.total}` : undefined },
          ] as Item[],
        },
        {
          label: t('nav.groupAdmin'),
          items: [
            { to: '/admins', label: t('nav.admins'), icon: ShieldCheck, show: sudo },
            { to: '/audit', label: t('nav.audit'), icon: ScrollText, show: sudo },
            { to: '/settings', label: t('nav.settings'), icon: Settings, show: sudo },
          ] as Item[],
        },
      ]
        .map((g) => ({ ...g, items: g.items.filter((i) => i.show) }))
        .filter((g) => g.items.length > 0),
    [t, sudo, stats],
  )
  const nav = groups.flatMap((g) => g.items.map((i) => ({ ...i, group: g })))
  const current = nav.find((n) => (n.to === '/' ? location.pathname === '/' : location.pathname.startsWith(n.to)))

  const switchLang = () => setLang(lang === 'ru' ? 'en' : 'ru')
  const commands: Command[] = [
    ...nav.map((n) => ({ id: `page-${n.to}`, label: n.label, group: t('cmd.pages'), icon: n.icon, run: () => navigate(n.to) })),
    { id: 'new-user', label: t('users.new'), group: t('cmd.actions'), icon: Plus, run: () => navigate('/users?new=1') },
    { id: 'theme', label: t('cmd.theme'), group: t('cmd.actions'), icon: dark ? Sun : Moon, run: toggle },
    {
      id: 'preset',
      label: `${t('nav.preset')}: ${t(prefs.preset === 'slate' ? 'nav.preset.tangerine' : 'nav.preset.slate')}`,
      group: t('cmd.actions'),
      icon: Contrast,
      run: () => setPrefs({ preset: prefs.preset === 'slate' ? 'tangerine' : 'slate' }),
    },
    { id: 'lang', label: t('cmd.lang'), group: t('cmd.actions'), icon: Languages, run: switchLang },
    { id: 'password', label: t('nav.password'), group: t('cmd.actions'), icon: KeyRound, run: () => setPwOpen(true) },
    { id: 'logout', label: t('nav.logout'), group: t('cmd.actions'), icon: LogOut, run: logout },
  ]

  const u = stats?.users
  const share = u && u.total > 0 ? u.active / u.total : 0

  return (
    <div className="app">
      <a href="#main" className="skip-link" onClick={(e) => (e.preventDefault(), mainRef.current?.focus())}>
        {t('nav.skip')}
      </a>
      <AnimatePresence>
        {drawerOpen && (
          <motion.div className="layer-scrim z-[74] md:hidden" onClick={() => setSidebar('')} initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={{ duration: 0.24 }} />
        )}
      </AnimatePresence>

      <aside className="sidebar" id="sidebar" ref={sideRef} aria-label="Sidebar">
        <div className="sb-top" data-popover-root>
          <button type="button" className="ws-switch" onClick={() => toggleMenu('workspace')} aria-haspopup="dialog" aria-expanded={menu === 'workspace'}>
            <BrandMark />
            <span className="ws-text">
              <span className="ws-name">Cloudrix</span>
              <span className="ws-sub">{t('nav.workspace')}</span>
            </span>
            <ChevronsUpDown className="chev" />
          </button>
          <Popover open={menu === 'workspace'} onClose={closeMenu} label={t('nav.system')} className="left-0 top-[calc(100%+6px)] w-[240px] origin-top-left">
            <div className="menu-label">{t('nav.system')}</div>
            <dl className="grid gap-1.5 px-2.5 pb-2 text-xs">
              <InfoRow label={t('dash.version')} value={stats?.system.version} />
              <InfoRow label={t('dash.uptime')} value={stats ? formatDuration(stats.system.uptime, lang) : undefined} />
              {stats?.nodes && <InfoRow label={t('dash.nodes')} value={`${stats.nodes.connected} / ${stats.nodes.total}`} />}
              <InfoRow label={t('dash.memory')} value={stats ? formatBytes(stats.system.mem_alloc) : undefined} />
              <InfoRow label="CPU" value={stats ? String(stats.system.cpus) : undefined} />
            </dl>
            <div className="menu-sep" />
            <MenuItem icon={Plus} onClick={() => navigate('/users?new=1')}>
              {t('users.new')}
            </MenuItem>
            {sudo && (
              <MenuItem icon={Settings} onClick={() => navigate('/settings')}>
                {t('nav.settings')}
              </MenuItem>
            )}
          </Popover>
        </div>

        <nav className="sb-scroll" aria-label="Primary">
          {groups.map((g, gi) => (
            <div key={g.label} className="sb-group" role="group" aria-labelledby={`sbg-${gi}`}>
              <div className="sb-label" id={`sbg-${gi}`}>
                {g.label}
              </div>
              <div className="sb-nav">
                {g.items.map(({ to, label, icon: Icon, count }) => (
                  <NavLink key={to} to={to} end={to === '/'} className="sb-item" data-tip={label}>
                    <Icon />
                    <span className="sb-text">{label}</span>
                    {count && <span className="sb-count">{count}</span>}
                  </NavLink>
                ))}
              </div>
            </div>
          ))}
        </nav>

        <div className="sb-foot" data-popover-root>
          {u && u.total > 0 && (
            <Link to="/users" className="target-card" aria-label={t('nav.usersCard')}>
              <div className="target-row">
                <span className="target-label">{t('nav.activeUsers')}</span>
                <span className="target-pct">{Math.round(share * 100)}%</span>
              </div>
              <div className="target-value">
                {u.active} <span>{t('dash.ofTotal', { n: u.total })}</span>
              </div>
              <div className="meter" aria-hidden>
                <i style={{ '--v': share } as CSSProperties} />
              </div>
              <div className="target-foot">{u.expiring_soon > 0 ? t('nav.expiringHint', { n: u.expiring_soon }) : t('dash.ofActive', { pct: u.active ? Math.round((u.online / u.active) * 100) : 0 }) + ' · ' + t('dash.online').toLowerCase()}</div>
            </Link>
          )}
          <button type="button" className="user-row" onClick={() => toggleMenu('account')} aria-haspopup="dialog" aria-expanded={menu === 'account'} data-tip={admin?.username}>
            <span className="av av-md av-sq">{admin?.username[0]}</span>
            <span className="user-meta">
              <span className="user-name">{admin?.username}</span>
              <span className="user-mail">{admin && t(`admins.role.${admin.role}`)}</span>
            </span>
            <ChevronsUpDown className="chev" />
          </button>
          <Popover open={menu === 'account'} onClose={closeMenu} label={t('nav.account')} className="bottom-[calc(100%-2px)] left-0 w-[240px] origin-bottom-left">
            <div className="flex items-center gap-2.5 px-2.5 py-2">
              <span className="av av-md av-sq">{admin?.username[0]}</span>
              <div className="min-w-0">
                <div className="truncate text-[13.5px] font-medium text-foreground">{admin?.username}</div>
                <div className="text-xs text-muted-foreground">{admin && t(`admins.role.${admin.role}`)}</div>
              </div>
            </div>
            <div className="menu-sep" />
            <MenuItem
              icon={KeyRound}
              onClick={() => {
                closeMenu()
                setPwOpen(true)
              }}
            >
              {t('nav.password')}
            </MenuItem>
            <MenuItem icon={Languages} onClick={switchLang}>
              {t('cmd.lang')}
            </MenuItem>
            <div className="menu-sep" />
            <MenuItem icon={LogOut} onClick={logout} danger>
              {t('nav.logout')}
            </MenuItem>
          </Popover>
        </div>
      </aside>

      <div className="inset">
        <header className={cx('topbar', scrolled && 'scrolled')}>
          <button type="button" className="btn btn-ghost btn-icon" onClick={toggleSidebar} aria-label={t('nav.toggleSidebar')} aria-controls="sidebar" aria-expanded={sidebar !== 'rail'}>
            <PanelLeft className="hidden md:block" />
            <Menu className="md:hidden" />
          </button>
          <Link to="/" className="topbar-mobile-brand" aria-label="Cloudrix">
            <BrandMark />
            Cloudrix
          </Link>
          <span className="vsep" aria-hidden />
          <nav className="crumbs" aria-label="Breadcrumb">
            {current && current.to !== '/' && (
              <>
                <Link to={current.group.items[0].to}>{current.group.label}</Link>
                <ChevronRight />
              </>
            )}
            <span aria-current="page">{current?.label}</span>
          </nav>
          <div className="topbar-right">
            <button type="button" className="search-btn" onClick={openPalette} aria-label={t('cmd.searchLong')}>
              <Search />
              <span>{t('cmd.searchLong')}</span>
              <kbd className="kbd" aria-hidden>
                {shortcut}
              </kbd>
            </button>
            <div className="relative" data-popover-root>
              <button type="button" className="btn btn-ghost btn-icon" onClick={() => toggleMenu('notifs')} aria-haspopup="dialog" aria-expanded={menu === 'notifs'} aria-label={t('nav.notifications')}>
                <Bell />
                {notifs.unread > 0 && <span className="bell-dot" />}
              </button>
              <Popover open={menu === 'notifs'} onClose={closeMenu} label={t('nav.notifications')} className="right-0 top-[calc(100%+6px)] w-[min(380px,calc(100vw-16px))] origin-top-right">
                <div className="flex items-center justify-between px-2.5 pb-1 pt-1.5">
                  <span className="text-[13.5px] font-semibold text-foreground">{t('nav.notifications')}</span>
                  {notifs.unread > 0 && (
                    <button type="button" className="btn btn-ghost btn-sm" onClick={notifs.markAll}>
                      {t('nav.markAllRead')}
                    </button>
                  )}
                </div>
                <div className="max-h-[min(60vh,420px)] overflow-y-auto">
                  {notifs.items.length === 0 && <p className="px-3 py-8 text-center text-[13px] text-muted-foreground">{t('nav.noNotifs')}</p>}
                  {notifs.items.map((n) => (
                    <button
                      type="button"
                      key={n.key}
                      className="notif"
                      onClick={() => {
                        notifs.markRead(n.key)
                        closeMenu()
                        navigate(n.to)
                      }}
                    >
                      <span className="notif-ic" style={{ '--c': n.tone } as CSSProperties}>
                        <n.icon />
                      </span>
                      <span className="min-w-0">
                        <p>
                          <b>{n.name}</b> {n.text}
                        </p>
                        {n.when && <time>{relativeTime(n.when, lang)}</time>}
                      </span>
                      {!notifs.read.has(n.key) ? <span className="unread" /> : <span />}
                    </button>
                  ))}
                </div>
              </Popover>
            </div>
            <div className="relative" data-popover-root>
              <button type="button" className="btn btn-ghost btn-sm preset-btn" onClick={() => toggleMenu('appearance')} aria-haspopup="dialog" aria-expanded={menu === 'appearance'} aria-label={t('nav.appearance')}>
                <Contrast />
                <span className="lbl">{t(prefs.preset === 'slate' ? 'nav.preset.slate' : 'nav.preset.tangerine')}</span>
              </button>
              <Popover open={menu === 'appearance'} onClose={closeMenu} label={t('nav.appearance')} className="right-0 top-[calc(100%+6px)] w-[264px] origin-top-right p-2">
                <AppearanceMenu preset={prefs.preset} mode={prefs.mode} density={prefs.density} set={setPrefs} />
              </Popover>
            </div>
            <button type="button" className="btn btn-ghost btn-icon" onClick={toggle} aria-label={t(dark ? 'nav.toLight' : 'nav.toDark')} title={t(dark ? 'nav.toLight' : 'nav.toDark')}>
              {dark ? <Sun /> : <Moon />}
            </button>
          </div>
        </header>

        <main className="main" id="main" ref={mainRef} tabIndex={-1}>
          <div ref={sentinel} style={{ height: 1, marginBottom: -1 }} aria-hidden />
          {/* Remounting on route change replays the staggered entrance. */}
          <div key={location.pathname} className="page view-enter">
            {outlet}
          </div>
        </main>
      </div>
      <PasswordModal open={pwOpen} onClose={() => setPwOpen(false)} />
      <CommandPalette commands={commands} />
    </div>
  )
}

const PRESETS: { id: Preset; ps: string; pm: string; pa: string }[] = [
  { id: 'slate', ps: 'oklch(0.968 0.005 262)', pm: 'oklch(0.993 0.002 262)', pa: 'oklch(0.56 0.17 257)' },
  { id: 'tangerine', ps: 'oklch(0.925 0.008 255)', pm: 'oklch(0.975 0.004 250)', pa: 'oklch(0.64 0.17 40)' },
]

function AppearanceMenu({ preset, mode, density, set }: { preset: Preset; mode: Mode; density: Density; set: (p: { preset?: Preset; mode?: Mode; density?: Density }) => void }) {
  const { t } = useI18n()
  const modes: { id: Mode; icon: Icon }[] = [
    { id: 'light', icon: Sun },
    { id: 'dark', icon: Moon },
    { id: 'system', icon: Monitor },
  ]
  return (
    <>
      <div className="menu-label !px-1">{t('nav.preset')}</div>
      <div className="grid grid-cols-2 gap-2" role="radiogroup" aria-label={t('nav.preset')}>
        {PRESETS.map((p) => (
          <button type="button" key={p.id} role="radio" aria-checked={preset === p.id} className="theme-card" onClick={() => set({ preset: p.id })}>
            <span className="theme-prev" style={{ '--ps': p.ps, '--pm': p.pm, '--pa': p.pa } as CSSProperties}>
              <span className="s" />
              <span className="m">
                <b />
                <i />
              </span>
            </span>
            <span className="nm">
              {t(`nav.preset.${p.id}`)}
              {preset === p.id && <Check className="h-3.5 w-3.5" />}
            </span>
          </button>
        ))}
      </div>
      <div className="menu-label mt-1 !px-1">{t('nav.mode')}</div>
      {modes.map((m) => (
        <MenuItem key={m.id} icon={m.icon} checked={mode === m.id} onClick={() => set({ mode: m.id })}>
          {t(`nav.mode.${m.id}`)}
        </MenuItem>
      ))}
      <div className="menu-sep" />
      <div className="menu-label !px-1">{t('nav.density')}</div>
      {(['comfortable', 'compact'] as Density[]).map((d) => (
        <MenuItem key={d} checked={density === d} onClick={() => set({ density: d })}>
          {t(`nav.density.${d}`)}
        </MenuItem>
      ))}
    </>
  )
}

function InfoRow({ label, value }: { label: string; value?: string }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="font-medium tabular-nums text-foreground">{value ?? '—'}</dd>
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
        <>
          <Button variant="secondary" onClick={close}>
            {t('common.cancel')}
          </Button>
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
        </>
      }
    >
      <ErrorNote error={error} />
      <div className="grid gap-3.5">
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
