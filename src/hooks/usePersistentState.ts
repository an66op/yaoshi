import { useEffect, useState } from 'react'

/** 将纯前端演示状态保留在当前浏览器，刷新页面后也保持一致。 */
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
    window.localStorage.setItem(key, JSON.stringify(value))
  }, [key, value])

  return [value, setValue] as const
}
