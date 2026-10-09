import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'
import { get, getToken, onUnauthorized, post, setToken } from './api'
import type { Admin } from './types'

interface AuthState {
  admin: Admin | null
  loading: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => void
}

const Ctx = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [admin, setAdmin] = useState<Admin | null>(null)
  const [loading, setLoading] = useState(() => !!getToken())

  useEffect(() => {
    if (!getToken()) return
    get<Admin>('/api/auth/me')
      .then(setAdmin)
      .catch(() => setToken(null))
      .finally(() => setLoading(false))
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

  return <Ctx.Provider value={{ admin, loading, login, logout }}>{children}</Ctx.Provider>
}

export function useAuth() {
  const ctx = useContext(Ctx)
  if (!ctx) throw new Error('useAuth outside provider')
  return ctx
}
