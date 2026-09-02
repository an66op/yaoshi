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

  it('sends credentials and captcha, never a client-chosen login owner or role', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, text: async () => JSON.stringify({ code: 200, data: {} }) })
    vi.stubGlobal('fetch', fetchMock)
    const { adminApi } = await import('./api')
    const captcha = { captcha_id: 'management-challenge', captcha_code: '123456', role: 'admin', workspace: 'untrusted' }
    await adminApi.login('agent-account', 'test-password', captcha)
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ username: 'agent-account', password: 'test-password', captcha_id: captcha.captcha_id, captcha_code: captcha.captcha_code })
  })

  it('fetches a fresh management challenge with cookie credentials and cancellation', async () => {
    const challenge = { id: 'management-challenge', image: 'data:image/png;base64,AAAA', expires_in: 120 }
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, text: async () => JSON.stringify({ code: 200, data: challenge }) })
    vi.stubGlobal('fetch', fetchMock)
    const { adminApi } = await import('./api')
    const controller = new AbortController()
    await expect(adminApi.loginCaptcha(controller.signal)).resolves.toEqual(challenge)
    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/api\/login\/captcha$/)
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ cache: 'no-store', signal: controller.signal, credentials: 'include' })
  })

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

  it('serializes game-room retention and recycle-bin expiry without executing a cleanup', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, text: async () => JSON.stringify({ code: 200, data: {} }) })
    vi.stubGlobal('fetch', fetchMock)
    const { adminApi } = await import('./api')
    const payload = { workspace_id: 37, enabled: true, retention_days: 7, purge_after_days: 14 }
    await adminApi.updateRetentionPolicy('game_chat_messages', payload)
    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/api\/admin\/data-lifecycle\/policies\/game_chat_messages$/)
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'PUT', body: JSON.stringify(payload) })
    expect(fetchMock).toHaveBeenCalledOnce()
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

  describe.each(['tenant', 'agent'] as const)('%s room membership lookup', role => {
    it('sends an exact user_id with a one-row limit and preserves historical nulls', async () => {
      const historical = {
        id: 41, public_id: 100041, username: 'history-member', nickname: '历史会员', role: 'member',
        in_current_room: false, can_manage: false, balance: null, status: null, online: null,
      }
      const data = { items: [historical], total: 1, page: 1, page_size: 1 }
      const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, text: async () => JSON.stringify({ code: 200, data }) })
      vi.stubGlobal('fetch', fetchMock)
      const { tenantApi, agentApi } = await import('./api')
      const api = role === 'tenant' ? tenantApi : agentApi
      await expect(api.users({ userId: 41, page: 1, pageSize: 1 })).resolves.toEqual(data)
      const url = new URL(String(fetchMock.mock.calls[0][0]), 'http://localhost')
      expect(url.pathname).toBe(`/api/${role}/users`)
      expect(url.searchParams.get('user_id')).toBe('41')
      expect(url.searchParams.get('page_size')).toBe('1')
      expect(url.searchParams.get('query')).toBe('')
      expect(fetchMock.mock.calls[0][1].credentials).toBe('include')
    })

    it('does not silently turn an invalid explicit ID into an unfiltered lookup', async () => {
      const fetchMock = vi.fn().mockResolvedValue({ ok: false, status: 400, text: async () => JSON.stringify({ code: 400, message: '会员编号无效' }) })
      vi.stubGlobal('fetch', fetchMock)
      const { tenantApi, agentApi } = await import('./api')
      await expect((role === 'tenant' ? tenantApi : agentApi).users({ userId: 0 })).rejects.toThrow()
      const url = new URL(String(fetchMock.mock.calls[0][0]), 'http://localhost')
      expect(url.searchParams.get('user_id')).toBe('0')
    })

    it('keeps normal room list filters without sending user_id', async () => {
      const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, text: async () => JSON.stringify({ code: 200, data: { items: [], total: 0, page: 2, page_size: 20 } }) })
      vi.stubGlobal('fetch', fetchMock)
      const { tenantApi, agentApi } = await import('./api')
      await (role === 'tenant' ? tenantApi : agentApi).users({ query: '公开昵称', page: 2, pageSize: 20 })
      const url = new URL(String(fetchMock.mock.calls[0][0]), 'http://localhost')
      expect(url.searchParams.has('user_id')).toBe(false)
      expect(url.searchParams.get('query')).toBe('公开昵称')
      expect(url.searchParams.get('page')).toBe('2')
    })
  })
})
