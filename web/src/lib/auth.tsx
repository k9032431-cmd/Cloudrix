import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'
import { ApiError, get, getToken, onUnauthorized, post, setToken } from './api'
import type { Admin } from './types'

interface AuthState {
  admin: Admin | null
  loading: boolean
  offline: boolean // the panel could not be reached to check the session
  login: (username: string, password: string) => Promise<void>
  logout: () => void
}

const Ctx = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [admin, setAdmin] = useState<Admin | null>(null)
  const [loading, setLoading] = useState(() => !!getToken())
  const [offline, setOffline] = useState(false)

  // check the saved session; network trouble retries and never logs out
  // (only a 401 does, in api())
  useEffect(() => {
    if (!getToken()) return
    let alive = true
    const check = async (attempt: number) => {
      try {
        const a = await get<Admin>('/api/auth/me')
        if (alive) setAdmin(a)
      } catch (e) {
        if (!alive) return
        if (e instanceof ApiError && e.status === 0 && attempt < 3) {
          setTimeout(() => check(attempt + 1), 1500 * (attempt + 1))
          return
        }
        if (e instanceof ApiError && e.status === 0) setOffline(true)
      }
      if (alive) setLoading(false)
    }
    check(0)
    return () => {
      alive = false
    }
  }, [])

  useEffect(() => onUnauthorized(() => setAdmin(null)), [])

  const login = useCallback(async (username: string, password: string) => {
    const res = await post<{ token: string; admin: Admin }>('/api/auth/login', { username, password })
    setToken(res.token)
    setAdmin(res.admin)
  }, [])

  const logout = useCallback(() => {
    setToken(null)
    setAdmin(null)
  }, [])

  return <Ctx.Provider value={{ admin, loading, offline, login, logout }}>{children}</Ctx.Provider>
}

export function useAuth() {
  const ctx = useContext(Ctx)
  if (!ctx) throw new Error('useAuth outside provider')
  return ctx
}
