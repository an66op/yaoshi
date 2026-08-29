import { useEffect, useState } from 'react'

/** 将主题、字体和声音等个人偏好保留在当前浏览器。 */
export function usePersistentState<T>(key: string, fallback: T) {
  const [value, setValue] = useState<T>(() => {
    try {
      const stored = window.localStorage.getItem(key)
      if (!stored) return fallback
      const parsed = JSON.parse(stored)
      if (parsed === null || parsed === undefined) return fallback
      if (typeof fallback === 'object' && fallback !== null && typeof parsed === 'object' && !Array.isArray(parsed)) return { ...fallback, ...parsed }
      return parsed as T
    } catch {
      return fallback
    }
  })

  useEffect(() => {
    try {
      window.localStorage.setItem(key, JSON.stringify(value))
    } catch {
      // Preferences remain valid in memory when storage is unavailable/full.
    }
    // `storage` does not fire in the same browser tab. Broadcast locally so
    // independent hook instances (app shell, profile sheet, game room) update
    // immediately instead of waiting for a page refresh.
    window.dispatchEvent(new CustomEvent(`seven-star-state:${key}`, { detail: value }))
  }, [key, value])

  useEffect(() => {
    const sync = (event: Event) => setValue((event as CustomEvent<T>).detail)
    window.addEventListener(`seven-star-state:${key}`, sync)
    return () => window.removeEventListener(`seven-star-state:${key}`, sync)
  }, [key])

  return [value, setValue] as const
}
