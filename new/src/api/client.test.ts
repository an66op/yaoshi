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

  it('sends member credentials and captcha without a client-chosen role or workspace', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, text: async () => JSON.stringify({ code: 200, data: {} }) })
    vi.stubGlobal('fetch', fetchMock)
    const { memberApi } = await import('./member')
    const captcha = { captcha_id: 'member-challenge', captcha_code: '6543', role: 'admin', workspace: 'untrusted' }
    await memberApi.login('member-account', 'test-password', captcha)
    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/api\/member\/login$/)
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ username: 'member-account', password: 'test-password', captcha_id: captcha.captcha_id, captcha_code: captcha.captcha_code })
  })

  it('fetches a fresh member challenge with cookie credentials and cancellation', async () => {
    const challenge = { id: 'member-challenge', image: 'data:image/png;base64,AAAA', expires_in: 120 }
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, text: async () => JSON.stringify({ code: 200, data: challenge }) })
    vi.stubGlobal('fetch', fetchMock)
    const { memberApi } = await import('./member')
    const controller = new AbortController()
    await expect(memberApi.loginCaptcha(controller.signal)).resolves.toEqual(challenge)
    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/api\/member\/login\/captcha$/)
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ cache: 'no-store', signal: controller.signal, credentials: 'include' })
  })

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

  it('does not expose server diagnostics from 5xx responses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      text: async () => JSON.stringify({ code: 500, message: 'pq: password=database-secret', data: null }),
    }))
    const { request } = await import('./client')

    await expect(request('/member/me')).rejects.toThrow('服务暂时不可用')
    await expect(request('/member/me')).rejects.not.toThrow('database-secret')
  })

  it('broadcasts a credential-free event when authentication expires', async () => {
    storage.setItem('seven-star-session', JSON.stringify({ account: 'member-a' }))
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      text: async () => JSON.stringify({ code: 401, message: '请先登录', data: null }),
    }))
    const { request } = await import('./client')

    await expect(request('/member/me')).rejects.toThrow('请先登录')
    const authEvent = storage.getItem('yaotu-member-auth-event') ?? ''
    expect(JSON.parse(authEvent)).toMatchObject({ type: 'logout' })
    expect(authEvent).not.toContain('member-a')
    expect(storage.getItem('seven-star-session')).toBeNull()
  })
})
