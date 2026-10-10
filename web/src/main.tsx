import { StrictMode, lazy, Suspense, type ReactNode } from 'react'
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

const Inbounds = lazy(() => import('./pages/Inbounds'))
const Nodes = lazy(() => import('./pages/Nodes'))
const Admins = lazy(() => import('./pages/Admins'))
const Audit = lazy(() => import('./pages/Audit'))
const SettingsPage = lazy(() => import('./pages/Settings'))
const Subscription = lazy(() => import('./pages/Subscription'))

function Spinner() {
  return (
    <div className="flex min-h-[40vh] items-center justify-center">
      <Loader2 className="h-6 w-6 animate-spin text-faint" />
    </div>
  )
}

function Protected({ children, sudo }: { children: ReactNode; sudo?: boolean }) {
  const { admin, loading } = useAuth()
  if (loading) return <Spinner />
  if (!admin) return <Navigate to="/login" replace />
  if (sudo && admin.role !== 'sudo') return <Navigate to="/" replace />
  return <>{children}</>
}

function App() {
  return (
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
          <Route path="inbounds" element={<Protected sudo><Inbounds /></Protected>} />
          <Route path="nodes" element={<Protected sudo><Nodes /></Protected>} />
          <Route path="admins" element={<Protected sudo><Admins /></Protected>} />
          <Route path="audit" element={<Protected sudo><Audit /></Protected>} />
          <Route path="settings" element={<Protected sudo><SettingsPage /></Protected>} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Suspense>
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
