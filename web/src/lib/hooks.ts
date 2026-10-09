import { useCallback, useEffect, useRef, useState } from 'react'
import { get } from './api'

// useFetch loads a GET endpoint and exposes a reload function.
export function useFetch<T>(path: string | null) {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(!!path)
  const seq = useRef(0)

  const reload = useCallback(async () => {
    if (!path) return
    const id = ++seq.current
    setLoading(true)
    try {
      const res = await get<T>(path)
      if (id === seq.current) {
        setData(res)
        setError(null)
      }
    } catch (e) {
      if (id === seq.current) setError((e as Error).message)
    } finally {
      if (id === seq.current) setLoading(false)
    }
  }, [path])

  useEffect(() => {
    reload()
  }, [reload])

  return { data, error, loading, reload, setData }
}

export function useDebounced<T>(value: T, ms = 300): T {
  const [v, setV] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return v
}

export function useTheme() {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains('dark'))
  const toggle = useCallback(() => {
    setDark((d) => {
      const next = !d
      document.documentElement.classList.toggle('dark', next)
      try {
        localStorage.setItem('theme', next ? 'dark' : 'light')
      } catch {
        /* ignore */
      }
      return next
    })
  }, [])
  return { dark, toggle }
}
