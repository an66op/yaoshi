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

  it('does not expose server diagnostics from 5xx responses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 503,
      text: async () => JSON.stringify({ code: 503, message: 'redis://user:secret@internal-host', data: null }),
    }))
    const { adminApi } = await import('./api')

    await expect(adminApi.me()).rejects.toThrow('服务暂时不可用')
    await expect(adminApi.me()).rejects.not.toThrow('internal-host')
  })

  it('rejects executable, credentialed and protocol-relative asset URLs', async () => {
    const { resolveApiAsset } = await import('./api')

    expect(resolveApiAsset('javascript:alert(1)')).toBe('')
    expect(resolveApiAsset('//evil.example/cover.png')).toBe('')
    expect(resolveApiAsset('https://user:secret@cdn.example/cover.png')).toBe('')
    expect(resolveApiAsset('data:text/html;base64,PHNjcmlwdD4=')).toBe('')
    expect(resolveApiAsset('https://cdn.example/cover.png')).toBe('https://cdn.example/cover.png')
    expect(resolveApiAsset('data:image/webp;base64,UklGRg==')).toBe('data:image/webp;base64,UklGRg==')
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

  it('scopes plan automation reads and saves without exposing administrator generation', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: async () => JSON.stringify({ code: 200, data: {} }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const { adminApi, tenantApi, agentApi } = await import('./api')
    const payload = { workspace_id: 37, enabled: true, mode: 'demo' as const, game_ids: ['speed-racing', 'canada-28'], positions: [1, 2, 10], plan_keys: ['four-period-five-codes', 'size-three-periods'] }

    await adminApi.planAutomation(37)
    await adminApi.updatePlanAutomation(payload)

    expect(String(fetchMock.mock.calls[0][0])).toContain('/api/admin/plan-automation?workspace_id=37')
    expect(String(fetchMock.mock.calls[1][0])).toMatch(/\/api\/admin\/plan-automation$/)
    expect(fetchMock.mock.calls[1][1]).toMatchObject({ method: 'PUT', body: JSON.stringify(payload), credentials: 'include' })
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(adminApi).not.toHaveProperty('previewPlanAutomation')
    expect(tenantApi).not.toHaveProperty('updatePlanAutomation')
    expect(agentApi).not.toHaveProperty('updatePlanAutomation')
  })
})
