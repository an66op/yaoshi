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

  it('uses fixed diagnostic endpoints, source keys and cancellation instead of import or signed URLs', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, text: async () => JSON.stringify({ code: 200, data: {} }) })
    vi.stubGlobal('fetch', fetchMock)
    const { adminApi } = await import('./api')
    const controller = new AbortController()
    await adminApi.sourceDiagnostics(controller.signal)
    await adminApi.probeSource('163:169', controller.signal)
    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/admin\/source-diagnostics$/)
    expect(String(fetchMock.mock.calls[1][0])).toMatch(/\/admin\/source-diagnostics\/probe$/)
    expect(fetchMock.mock.calls[1][1]).toMatchObject({ method: 'POST', body: JSON.stringify({ source_key: '163:169' }), credentials: 'include' })
    expect(fetchMock.mock.calls[1][1].signal).toBeInstanceOf(AbortSignal)
    expect(fetchMock.mock.calls.flat().map(String).join(' ')).not.toContain('sign=')
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('reads persistent system logs with bounded cursor filters', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, text: async () => JSON.stringify({ code: 200, data: { items: [], has_more: false } }) })
    vi.stubGlobal('fetch', fetchMock)
    const { adminApi } = await import('./api')
    await adminApi.systemLogs({ beforeId: 88, limit: 50, category: 'source', type: 'sync_error', status: 'error', gameId: 'pc-canada', sourceGroup: 'pc28-163', from: '2026-09-01T00:00:00Z', to: '2026-09-05T00:00:00Z', query: '母源过期' })
    const url = new URL(String(fetchMock.mock.calls[0][0]))
    expect(url.pathname).toBe('/api/admin/system-logs')
    expect(Object.fromEntries(url.searchParams)).toEqual({
      limit: '50', before_id: '88', category: 'source', type: 'sync_error', status: 'error', game_id: 'pc-canada',
      source_group: 'pc28-163', from: '2026-09-01T00:00:00Z', to: '2026-09-05T00:00:00Z', q: '母源过期',
    })
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ credentials: 'include' })
  })

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
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ cache: 'no-store', credentials: 'include' })
    expect(fetchMock.mock.calls[0][1].signal).toBeInstanceOf(AbortSignal)
    expect(fetchMock.mock.calls[0][1].signal.aborted).toBe(false)
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

  it('sends optimistic-lock guards for odds saves and clears without exposing a default-price sync endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, text: async () => JSON.stringify({ code: 200, data: {} }) })
    vi.stubGlobal('fetch', fetchMock)
    const { adminApi } = await import('./api')
    const guard = { expected_rule_version: 'digits5-v3', expected_revision: 'revision-12' }
    const payload = { ...guard, items: [] }
    await adminApi.updateOddsLimits('sg-ssc', payload)
    await adminApi.resetOddsLimits('sg-ssc', guard)
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'PUT', body: JSON.stringify(payload) })
    expect(fetchMock.mock.calls[1][1]).toMatchObject({ method: 'POST', body: JSON.stringify(guard) })
    expect(adminApi).not.toHaveProperty('syncOddsLimits')
  })

  it('reads SG backfill cursors and sends audited queue-only requests that accept HTTP 202', async () => {
    const result = { queued_issues: 3, message: '等待后台执行' }
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 202, text: async () => JSON.stringify({ code: 202, data: result }) })
    vi.stubGlobal('fetch', fetchMock)
    const { adminApi } = await import('./api')
    await adminApi.sgSSCBackfillStatus()
    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/admin\/sources\/sg-ssc\/backfill\?before_id=0&limit=20$/)
    await adminApi.sgSSCBackfillStatus(168, 20)
    expect(String(fetchMock.mock.calls[1][0])).toMatch(/before_id=168&limit=20$/)
    await expect(adminApi.queueSGSSCBackfill()).resolves.toEqual(result)
    await adminApi.queueSGSSCBackfill()
    expect(String(fetchMock.mock.calls[2][0])).toMatch(/\/admin\/sources\/sg-ssc\/backfill$/)
    expect(fetchMock.mock.calls[2][1]).toMatchObject({ method: 'POST', body: '{}', credentials: 'include' })
    const headers = new Headers(fetchMock.mock.calls[2][1].headers)
    expect(headers.get('X-Request-ID')).toBeTruthy()
    expect(new Headers(fetchMock.mock.calls[3][1].headers).get('X-Request-ID')).not.toBe(headers.get('X-Request-ID'))
  })

  it('preserves actionable odds conflict codes for the draft editor', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 409, text: async () => JSON.stringify({ code: 409, error_code: 'ODDS_CONFIGURATION_CONFLICT', message: '配置已更新，请刷新' }) }))
    const { adminApi } = await import('./api')
    await expect(adminApi.updateOddsLimits('speed-racing', { expected_rule_version: 'racing-v2', expected_revision: 'old', items: [] })).rejects.toMatchObject({
      name: 'ApiError', status: 409, code: 'ODDS_CONFIGURATION_CONFLICT', message: '配置已更新，请刷新',
    })
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

describe('management API full-response deadline and cancellation', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.useFakeTimers()
    localStorage.clear()
    vi.stubGlobal('window', {
      location: { protocol: 'http:', hostname: '127.0.0.1', origin: 'http://127.0.0.1:5174' },
      localStorage: storage,
      setTimeout,
      clearTimeout,
      dispatchEvent: vi.fn(),
    })
  })

  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  const untilAborted = <T,>(signal: AbortSignal) => new Promise<T>((_resolve, reject) => {
    signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true })
  })

  it.each([
    { stage: 'headers', withCaller: false },
    { stage: 'body', withCaller: false },
    { stage: 'headers', withCaller: true },
    { stage: 'body', withCaller: true },
  ])('ends a stalled $stage at 15 seconds, external signal: $withCaller', async ({ stage, withCaller }) => {
    const caller = new AbortController()
    const removed = vi.spyOn(caller.signal, 'removeEventListener')
    let fetchSignal!: AbortSignal
    const readBody = vi.fn(() => untilAborted<string>(fetchSignal))
    vi.stubGlobal('fetch', vi.fn(async (_url: string, init: RequestInit) => {
      fetchSignal = init.signal as AbortSignal
      if (stage === 'headers') return untilAborted<Response>(fetchSignal)
      return { ok: true, status: 200, text: readBody }
    }))
    const { adminApi } = await import('./api')
    let settled = false
    const pending = adminApi.oddsLimits('bingo-marksix', withCaller ? caller.signal : undefined)
      .catch(error => error)
      .then(result => { settled = true; return result })

    await vi.advanceTimersByTimeAsync(14_999)
    expect(settled).toBe(false)
    expect(fetchSignal.aborted).toBe(false)
    expect(readBody).toHaveBeenCalledTimes(stage === 'body' ? 1 : 0)
    expect(vi.getTimerCount()).toBe(1)

    await vi.advanceTimersByTimeAsync(1)
    const result = await pending
    expect(result).toBeInstanceOf(Error)
    expect(result).toMatchObject({ name: 'Error', message: '请求超时，请稍后重试' })
    expect(fetchSignal.aborted).toBe(true)
    expect(caller.signal.aborted).toBe(false)
    expect(vi.getTimerCount()).toBe(0)
    expect(removed).toHaveBeenCalledTimes(withCaller ? 1 : 0)
  })

  it.each(['headers', 'body'])('keeps caller cancellation distinct from a timeout during $stage', async stage => {
    const caller = new AbortController()
    const removed = vi.spyOn(caller.signal, 'removeEventListener')
    let fetchSignal!: AbortSignal
    const readBody = vi.fn(() => untilAborted<string>(fetchSignal))
    vi.stubGlobal('fetch', vi.fn(async (_url: string, init: RequestInit) => {
      fetchSignal = init.signal as AbortSignal
      if (stage === 'headers') return untilAborted<Response>(fetchSignal)
      return { ok: true, status: 200, text: readBody }
    }))
    const { adminApi } = await import('./api')
    const pending = adminApi.playCatalog('bingo-marksix', caller.signal).catch(error => error)
    await vi.advanceTimersByTimeAsync(1)
    expect(readBody).toHaveBeenCalledTimes(stage === 'body' ? 1 : 0)

    caller.abort('a different game was selected')
    const result = await pending
    expect(result).toBeInstanceOf(DOMException)
    expect(result).toMatchObject({ name: 'AbortError', message: '请求已取消' })
    expect(fetchSignal.aborted).toBe(true)
    expect(removed).toHaveBeenCalledOnce()
    expect(vi.getTimerCount()).toBe(0)
    await vi.advanceTimersByTimeAsync(15_000)
    expect(result.name).toBe('AbortError')
    expect(window.dispatchEvent).not.toHaveBeenCalled()
  })

  it('does not send a request or install timers for an already cancelled caller', async () => {
    const caller = new AbortController()
    caller.abort()
    const added = vi.spyOn(caller.signal, 'addEventListener')
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    const { adminApi } = await import('./api')

    await expect(adminApi.games(caller.signal)).rejects.toMatchObject({ name: 'AbortError', message: '请求已取消' })
    expect(fetchMock).not.toHaveBeenCalled()
    expect(added).not.toHaveBeenCalled()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('cleans timers and forwarding listeners after a successful body without aborting the caller', async () => {
    const caller = new AbortController()
    const added = vi.spyOn(caller.signal, 'addEventListener')
    const removed = vi.spyOn(caller.signal, 'removeEventListener')
    let fetchSignal!: AbortSignal
    const result = [{ id: 'bingo-marksix', name: '宾果六合彩' }]
    const fetchMock = vi.fn(async (_url: string, init: RequestInit) => {
      fetchSignal = init.signal as AbortSignal
      return { ok: true, status: 200, text: async () => JSON.stringify({ code: 200, data: result }) }
    })
    vi.stubGlobal('fetch', fetchMock)
    const { adminApi } = await import('./api')

    await expect(adminApi.games(caller.signal)).resolves.toEqual(result)
    expect(fetchMock.mock.calls[0][0]).toMatch(/\/admin\/games$/)
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ credentials: 'include', headers: { 'Content-Type': 'application/json' } })
    expect(removed).toHaveBeenCalledWith('abort', added.mock.calls[0][1])
    expect(vi.getTimerCount()).toBe(0)
    await vi.advanceTimersByTimeAsync(15_001)
    expect(fetchSignal.aborted).toBe(false)
    caller.abort()
    expect(fetchSignal.aborted).toBe(false)
  })

  it.each([
    { status: 409, raw: JSON.stringify({ code: 409, error_code: 'ODDS_CONFIGURATION_CONFLICT', message: '配置已更新，请刷新' }), name: 'ApiError', message: '配置已更新，请刷新', code: 'ODDS_CONFIGURATION_CONFLICT' },
    { status: 503, raw: JSON.stringify({ code: 503, message: 'postgres://internal:secret@private-host', data: null }), name: 'ApiError', message: '服务暂时不可用', code: '' },
    { status: 401, raw: JSON.stringify({ code: 401, message: '请重新登录', data: null }), name: 'AuthError', message: '请重新登录' },
    { status: 401, raw: '<html>expired</html>', name: 'AuthError', message: '登录状态已失效，请重新登录' },
    { status: 200, raw: '<html>bad proxy response</html>', name: 'Error', message: '服务返回了无效响应' },
  ])('preserves $name for status $status and cleans request resources', async ({ status, raw, name, message, code }) => {
    const caller = new AbortController()
    const removed = vi.spyOn(caller.signal, 'removeEventListener')
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: status >= 200 && status < 300, status, text: async () => raw })))
    const { adminApi } = await import('./api')

    const result = await adminApi.oddsLimits('bingo-marksix', caller.signal).catch(error => error)
    expect(result).toMatchObject({ name, message })
    if (name === 'ApiError') expect(result).toMatchObject({ status, code })
    expect(window.dispatchEvent).toHaveBeenCalledTimes(status === 401 ? 1 : 0)
    expect(removed).toHaveBeenCalledOnce()
    expect(vi.getTimerCount()).toBe(0)
  })
})
