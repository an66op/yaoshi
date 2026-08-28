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

describe('management API cookie credentials', () => {
  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
    vi.stubGlobal('window', {
      location: { protocol: 'http:', hostname: '192.168.31.84', origin: 'http://192.168.31.84:5174' },
      localStorage: storage,
      setTimeout,
      clearTimeout,
      dispatchEvent: vi.fn(),
    })
  })

  afterEach(() => vi.unstubAllGlobals())

  it('uses cookie credentials and never copies a legacy token to Authorization', async () => {
    storage.setItem('yaotu-admin-token', 'legacy-secret')
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: async () => JSON.stringify({
        code: 200,
        data: { id: 1, username: 'admin', email: '', nickname: 'Admin', role: 'admin', status: 1 },
      }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const { adminApi } = await import('./api')

    await adminApi.me()

    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(init.credentials).toBe('include')
    expect(new Headers(init.headers).has('Authorization')).toBe(false)
  })

  it('sends robot reset request id as the audit idempotency header', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: async () => JSON.stringify({
        code: 200,
        data: { request_id: 'robot-reset-request-001', mode: 'random', count: 10, duplicate: false, items: [] },
      }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const { adminApi } = await import('./api')

    await adminApi.resetRobots({ workspace_id: 37, request_id: 'robot-reset-request-001', mode: 'random', balance_min: 100, balance_max: 200 })

    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(new Headers(init.headers).get('Idempotency-Key')).toBe('robot-reset-request-001')
  })

  it('loads platform robot game candidates from the selected workspace', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: async () => JSON.stringify({ code: 200, data: [] }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const { adminApi } = await import('./api')

    await adminApi.robotWorkspaceGames(37)

    expect(String(fetchMock.mock.calls[0][0])).toContain('/api/admin/robot-workspaces/37/games')
  })
})
