import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from 'react'
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

export type Mode = 'light' | 'dark' | 'system'
export type Preset = 'slate' | 'tangerine'
export type Density = 'comfortable' | 'compact'
export interface Prefs {
  mode: Mode
  preset: Preset
  density: Density
}

// Appearance preferences live in one storage key, read before first paint by
// the script in index.html. Storage access is wrapped: it can throw in private
// windows or sandboxed frames, and the panel must still work then.
const PREFS_KEY = 'cloudrix.prefs'
const systemDark = () => typeof matchMedia !== 'undefined' && matchMedia('(prefers-color-scheme: dark)').matches

function readPrefs(): Prefs {
  let raw: Partial<Prefs> & { theme?: string } = {}
  try {
    raw = JSON.parse(localStorage.getItem(PREFS_KEY) || '{}')
    const legacy = localStorage.getItem('theme')
    if (!raw.mode && (legacy === 'dark' || legacy === 'light')) raw.mode = legacy
  } catch {
    /* ignore */
  }
  return {
    mode: raw.mode === 'dark' || raw.mode === 'system' ? raw.mode : 'light',
    preset: raw.preset === 'tangerine' ? 'tangerine' : 'slate',
    density: raw.density === 'compact' ? 'compact' : 'comfortable',
  }
}

let prefs = readPrefs()
const listeners = new Set<() => void>()

function applyPrefs(p: Prefs) {
  const root = document.documentElement
  // switch every colour in the same frame instead of a ripple of transitions
  root.classList.add('no-tr')
  root.classList.toggle('dark', p.mode === 'dark' || (p.mode === 'system' && systemDark()))
  root.setAttribute('data-theme-preset', p.preset)
  root.setAttribute('data-density', p.density)
  requestAnimationFrame(() => requestAnimationFrame(() => root.classList.remove('no-tr')))
}

export function setPrefs(patch: Partial<Prefs>) {
  prefs = { ...prefs, ...patch }
  try {
    localStorage.setItem(PREFS_KEY, JSON.stringify(prefs))
    localStorage.removeItem('theme')
  } catch {
    /* ignore */
  }
  applyPrefs(prefs)
  listeners.forEach((l) => l())
}

if (typeof matchMedia !== 'undefined') {
  matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (prefs.mode === 'system') {
      applyPrefs(prefs)
      listeners.forEach((l) => l())
    }
  })
}

const subscribe = (l: () => void) => {
  listeners.add(l)
  return () => listeners.delete(l)
}

// useTheme exposes the appearance preferences; `dark` is the resolved mode.
export function useTheme() {
  const p = useSyncExternalStore(subscribe, () => prefs)
  const dark = p.mode === 'dark' || (p.mode === 'system' && systemDark())
  const toggle = useCallback(() => setPrefs({ mode: dark ? 'light' : 'dark' }), [dark])
  return { prefs: p, dark, toggle, setPrefs }
}
