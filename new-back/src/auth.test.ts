import { describe, expect, it } from 'vitest'
import {
  ADMIN_AUTH_EVENT_KEY,
  broadcastAdminLogout,
  clearLegacyAdminSession,
  getStoredUser,
  LEGACY_ADMIN_TOKEN_KEY,
  LEGACY_ADMIN_USER_KEY,
  setCurrentUser,
} from './auth'

class MemoryStorage implements Storage {
  private values = new Map<string, string>()
  get length() { return this.values.size }
  clear() { this.values.clear() }
  getItem(key: string) { return this.values.get(key) ?? null }
  key(index: number) { return [...this.values.keys()][index] ?? null }
  removeItem(key: string) { this.values.delete(key) }
  setItem(key: string, value: string) { this.values.set(key, String(value)) }
}

describe('admin cookie session storage', () => {
  it('removes legacy access tokens while preserving non-sensitive preferences', () => {
    const storage = new MemoryStorage()
    storage.setItem(LEGACY_ADMIN_TOKEN_KEY, 'legacy-token')
    storage.setItem(LEGACY_ADMIN_USER_KEY, '{"role":"admin"}')
    storage.setItem('yaotu-back-theme', 'dark')

    clearLegacyAdminSession(storage)

    expect(storage.getItem(LEGACY_ADMIN_TOKEN_KEY)).toBeNull()
    expect(storage.getItem(LEGACY_ADMIN_USER_KEY)).toBeNull()
    expect(storage.getItem('yaotu-back-theme')).toBe('dark')
  })

  it('broadcasts only a non-secret logout event', () => {
    const storage = new MemoryStorage()
    broadcastAdminLogout(storage)
    const event = JSON.parse(storage.getItem(ADMIN_AUTH_EVENT_KEY) ?? '{}')
    expect(event.type).toBe('logout')
    expect(typeof event.at).toBe('number')
    expect(JSON.stringify(event)).not.toContain('token')
  })

  it('keeps the verified profile in memory only', () => {
    const storage = new MemoryStorage()
    const profile = { id: 1, username: 'admin', email: '', nickname: 'Admin', role: 'admin', status: 1 }
    setCurrentUser(profile)
    expect(getStoredUser()).toEqual(profile)
    expect(storage.getItem(LEGACY_ADMIN_USER_KEY)).toBeNull()

    broadcastAdminLogout(storage)
    expect(getStoredUser()).toBeNull()
  })
})
