import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const localStorage = new Map<string, string>()
const storage = {
  get length() { return localStorage.size },
  clear: () => localStorage.clear(),
  getItem: (key: string) => localStorage.get(key) ?? null,
  key: (index: number) => [...localStorage.keys()][index] ?? null,
  removeItem: (key: string) => { localStorage.delete(key) },
  setItem: (key: string, value: string) => { localStorage.set(key, String(value)) },
}

describe('member API cookie credentials', () => {
  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
    vi.stubGlobal('window', {
      location: { protocol: 'http:', hostname: '192.168.31.84', origin: 'http://192.168.31.84:5173' },
      localStorage: storage,
      sessionStorage: storage,
      setTimeout,
      clearTimeout,
      dispatchEvent: vi.fn(),
    })
  })

  afterEach(() => vi.unstubAllGlobals())

  it('sends the HttpOnly session cookie without restoring a legacy bearer token', async () => {
    storage.setItem('yaotu-member-token', 'legacy-secret')
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: async () => JSON.stringify({ code: 200, data: { id: 1 } }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const { request } = await import('./client')

    await request('/member/me')

    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(init.credentials).toBe('include')
    expect(new Headers(init.headers).has('Authorization')).toBe(false)
  })
})
