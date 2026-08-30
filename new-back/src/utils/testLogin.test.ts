import { afterEach, describe, expect, it, vi } from 'vitest'
import { loadTestLogin, parseTestLogin } from './testLogin'

const profile = { username: ' test-admin ', password: 'test-password', workspace: ' 平台 ' }
const config = { enabled: true, profiles: { platform: profile, tenant: { ...profile, username: 'test-tenant' }, agent: { ...profile, username: 'test-agent', workspace: 'test-tenant' } } }
const response = (value: unknown) => new Response(JSON.stringify(value), { headers: { 'Content-Type': 'application/json; charset=utf-8' } })
afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers() })

describe('explicit management test login configuration', () => {
  it('requires literal enabled true and valid management profile objects', () => {
    expect(parseTestLogin(config)).toMatchObject({ platform: { ...profile, username: 'test-admin', workspace: '平台' }, agent: { username: 'test-agent', workspace: 'test-tenant' } })
    for (const invalid of [null, [], {}, { ...config, enabled: 'true' }, { ...config, enabled: false }, { enabled: true, profiles: [] }, { enabled: true, profiles: { member: profile } }]) expect(parseTestLogin(invalid)).toBeUndefined()
  })
  it('fails closed for invalid accounts/passwords/workspace and validates all supplied identities', () => {
    for (const invalid of [{ ...profile, username: '' }, { ...profile, username: 'x'.repeat(51) }, { ...profile, password: null }, { ...profile, password: '密码'.repeat(13) }, { ...profile, workspace: '' }, { ...profile, workspace: 'x'.repeat(81) }]) {
      expect(parseTestLogin({ enabled: true, profiles: { platform: profile, agent: invalid } })).toBeUndefined()
    }
    expect(parseTestLogin({ enabled: true, profiles: { platform: { ...profile, password: ' password ' } } })?.platform?.password).toBe(' password ')
  })
  it('fetches only the same-origin file with no cookies, cache or redirects', async () => {
    const fetcher = vi.fn().mockResolvedValue(response(config))
    vi.stubGlobal('fetch', fetcher)
    expect(await loadTestLogin(new AbortController().signal)).toHaveProperty('platform.username', 'test-admin')
    expect(fetcher).toHaveBeenCalledWith('/test-login.json', expect.objectContaining({ cache: 'no-store', credentials: 'omit', redirect: 'error', signal: expect.any(AbortSignal) }))
  })
  it('ignores not found, SPA HTML, malformed JSON, oversized and disabled responses', async () => {
    for (const reply of [new Response('', { status: 404 }), new Response('<html></html>', { headers: { 'Content-Type': 'text/html' } }), new Response('{', { headers: { 'Content-Type': 'application/json' } }), response({ ...config, padding: 'x'.repeat(12_000) }), response({ ...config, enabled: false })]) {
      vi.stubGlobal('fetch', vi.fn().mockResolvedValue(reply))
      expect(await loadTestLogin(new AbortController().signal)).toBeUndefined()
    }
  })
  it('does not request when already aborted and absorbs a network failure', async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error('offline'))
    vi.stubGlobal('fetch', fetcher)
    const request = new AbortController()
    request.abort()
    expect(await loadTestLogin(request.signal)).toBeUndefined()
    expect(fetcher).not.toHaveBeenCalled()
    expect(await loadTestLogin(new AbortController().signal)).toBeUndefined()
  })
  it.each(['timeout', 'unmount'])('aborts a slow request on %s', async reason => {
    vi.useFakeTimers()
    const request = new AbortController()
    let signal!: AbortSignal
    vi.stubGlobal('fetch', vi.fn((_url, options: RequestInit) => new Promise((_resolve, reject) => {
      signal = options.signal as AbortSignal
      signal.addEventListener('abort', () => reject(new Error('aborted')), { once: true })
    })))
    const pending = loadTestLogin(request.signal)
    if (reason === 'timeout') await vi.advanceTimersByTimeAsync(3000)
    else request.abort()
    expect(await pending).toBeUndefined()
    expect(signal.aborted).toBe(true)
    expect(vi.getTimerCount()).toBe(0)
  })
})
