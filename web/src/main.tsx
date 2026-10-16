import { Component, StrictMode, lazy, Suspense, type ComponentType, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { Loader2 } from './components/icons'
import '@fontsource-variable/geist'
import '@fontsource-variable/geist-mono'
import './index.css'
import { MotionConfig } from 'framer-motion'
import { I18nProvider } from './lib/i18n'
import { AuthProvider, useAuth } from './lib/auth'
import { ToastProvider } from './components/toast'
import Layout from './components/Layout'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import UsersPage from './pages/Users'

// page loads that fail (usually files of an older panel version after an
// update) reload the page once to fetch the new ones
function page<T extends ComponentType<object>>(load: () => Promise<{ default: T }>) {
  return lazy(() =>
    load()
      .then((m) => {
        try {
          sessionStorage.removeItem('cloudrix_reloaded')
        } catch {
          /* storage unavailable */
        }
        return m
      })
      .catch((e) => {
      try {
        if (!sessionStorage.getItem('cloudrix_reloaded')) {
          sessionStorage.setItem('cloudrix_reloaded', '1')
          location.reload()
          return new Promise<never>(() => {})
        }
      } catch {
        /* storage unavailable */
      }
      throw e
    }),
  )
}
const Cores = page(() => import('./pages/Cores'))
const CoreEditor = page(() => import('./pages/CoreEditor'))
const Hosts = page(() => import('./pages/Hosts'))
const Nodes = page(() => import('./pages/Nodes'))
const Groups = page(() => import('./pages/Groups'))
const Admins = page(() => import('./pages/Admins'))
const Audit = page(() => import('./pages/Audit'))
const SettingsPage = page(() => import('./pages/Settings'))
const Subscription = page(() => import('./pages/Subscription'))

// Retry is shown instead of an endless spinner or a blank page.
function Retry({ text }: { text: string }) {
  return (
    <div className="flex min-h-[40vh] flex-col items-center justify-center gap-3 px-4 text-center">
      <p className="max-w-sm text-sm text-muted-foreground">{text}</p>
      <button type="button" className="btn btn-primary" onClick={() => location.reload()}>
        ↻ Обновить / Reload
      </button>
    </div>
  )
}

class PageBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false }
  static getDerivedStateFromError() {
    return { failed: true }
  }
  render() {
    return this.state.failed ? <Retry text="Не удалось открыть страницу. / The page failed to load." /> : this.props.children
  }
}

function Spinner() {
  return (
    <div className="flex min-h-[40vh] items-center justify-center">
      <Loader2 className="h-6 w-6 animate-spin text-faint" />
    </div>
  )
}

function Protected({ children, sudo }: { children: ReactNode; sudo?: boolean }) {
  const { admin, loading, offline } = useAuth()
  if (loading) return <Spinner />
  if (offline) return <Retry text="Панель не отвечает — проверьте соединение. / The panel does not answer — check the connection." />
  if (!admin) return <Navigate to="/login" replace />
  if (sudo && admin.role !== 'sudo') return <Navigate to="/" replace />
  return <>{children}</>
}

function App() {
  return (
    <PageBoundary>
    <Suspense fallback={<Spinner />}>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="/sub/:token" element={<Subscription />} />
        <Route
          element={
            <Protected>
              <Layout />
            </Protected>
          }
        >
          <Route index element={<Dashboard />} />
          <Route path="users" element={<UsersPage />} />
          <Route path="cores" element={<Protected sudo><Cores /></Protected>} />
          <Route path="cores/:id" element={<Protected sudo><CoreEditor /></Protected>} />
          <Route path="core" element={<Navigate to="/cores" replace />} />
          <Route path="inbounds" element={<Navigate to="/cores" replace />} />
          <Route path="hosts" element={<Protected sudo><Hosts /></Protected>} />
          <Route path="nodes" element={<Protected sudo><Nodes /></Protected>} />
          <Route path="groups" element={<Protected sudo><Groups /></Protected>} />
          <Route path="admins" element={<Protected sudo><Admins /></Protected>} />
          <Route path="audit" element={<Protected sudo><Audit /></Protected>} />
          <Route path="settings" element={<Protected sudo><SettingsPage /></Protected>} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Suspense>
    </PageBoundary>
  )
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <MotionConfig reducedMotion="user">
    <I18nProvider>
      <AuthProvider>
        <ToastProvider>
          <BrowserRouter>
            <App />
          </BrowserRouter>
        </ToastProvider>
      </AuthProvider>
    </I18nProvider>
    </MotionConfig>
  </StrictMode>,
)
