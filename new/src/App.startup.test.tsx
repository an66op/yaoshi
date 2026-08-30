import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { HookHarness } from './test/hookHarness'
import type { AppRoute } from './router'
import { AuthError } from './api/client'
import { Login } from './pages/Login'
import { SessionStartup } from './components/SessionStartup'
import App from './App'

const runtime = vi.hoisted(() => ({
  hooks: null as HookHarness | null,
  route: { kind: 'login' } as AppRoute,
  theme: 'day',
  cached: null as Record<string, unknown> | null,
  me: vi.fn(), refreshSession: vi.fn(), roomHistory: vi.fn(),
  clearBusiness: vi.fn(), navigate: vi.fn(),
}))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useMemo: <T,>(factory: () => T, dependencies: readonly unknown[]) => runtime.hooks!.useMemo(factory, dependencies),
  useCallback: <T,>(value: T, dependencies: readonly unknown[]) => runtime.hooks!.useMemo(() => value, dependencies),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
  useLayoutEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))
vi.mock('./hooks/usePersistentState', () => ({
  usePersistentState: <T,>(key: string, fallback: T) => runtime.hooks!.useState(
    key === 'seven-star-session' ? runtime.cached
      : key === 'seven-star-demo-state' ? { theme: runtime.theme, checkedIn: false, chatUnread: 0 }
        : fallback,
  ),
}))
vi.mock('./hooks/useLotteryGames', () => ({ useLotteryGames: () => ({ games: [], live: false, loading: false, error: '' }) }))
vi.mock('./hooks/useMemberPreferences', () => ({ useMemberPreferences: () => ({ fontScale: 'standard', displayStyle: 'simple' }) }))
vi.mock('./hooks/useFontScale', () => ({ useFontScale: () => undefined }))
vi.mock('./hooks/useWebSocket', () => ({ useWebSocket: () => undefined, useWebSocketConnected: () => true }))
vi.mock('./utils/notificationAudio', () => ({ stopNotificationSounds: () => undefined }))
vi.mock('./api/client', () => ({ AuthError: class AuthError extends Error {} }))
vi.mock('./api/member', () => ({ memberApi: { me: runtime.me, refreshSession: runtime.refreshSession, roomHistory: runtime.roomHistory } }))
vi.mock('./api/portal', () => ({ portalApi: { unreadCount: async () => ({ unread: 0 }), roomSettings: async () => ({}), activities: async () => [] } }))
vi.mock('./utils/businessStorage', () => ({
  clearMemberBusinessStorage: runtime.clearBusiness,
  clearLoginAnnouncementMarkers: () => undefined, broadcastMemberLogout: () => undefined,
  MEMBER_AUTH_EVENT_KEY: 'auth', MEMBER_SESSION_KEY: 'session',
}))
vi.mock('./router', async importOriginal => ({
  ...await importOriginal<typeof import('./router')>(),
  useAppRouter: () => ({ route: runtime.route, pathname: `/${runtime.route.kind}`, navigate: runtime.navigate, replace: vi.fn() }),
}))
vi.mock('./components/BottomNav', () => ({ BottomNav: () => null }))
vi.mock('./pages/Login', () => ({ Login: () => null }))
vi.mock('./pages/Register', () => ({ Register: () => null }))
vi.mock('./pages/RoomEntry', () => ({ RoomEntry: () => null }))
vi.mock('./pages/GameRoom', () => ({ GameRoom: () => null }))
vi.mock('./pages/DrawResults', () => ({ DrawResults: () => null }))
vi.mock('./pages/Lobby', () => ({ Lobby: () => null }))
vi.mock('./pages/Wallet', () => ({ Wallet: () => null }))
vi.mock('./pages/Chats', () => ({ Chats: () => null }))
vi.mock('./pages/Profile', () => ({ Profile: () => null }))

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

describe('App startup authentication boundary', () => {
  const render = () => {
    const result = runtime.hooks!.render(() => App())
    runtime.hooks!.flushEffects()
    return result
  }
  const settle = async () => { await Promise.resolve(); await Promise.resolve(); await Promise.resolve() }
  beforeEach(() => {
    runtime.hooks = new HookHarness()
    runtime.route = { kind: 'login' }
    runtime.theme = 'day'
    runtime.cached = null
    runtime.me.mockReset()
    runtime.refreshSession.mockReset().mockResolvedValue(undefined)
    runtime.roomHistory.mockReset().mockResolvedValue([])
    runtime.clearBusiness.mockClear()
    runtime.navigate.mockClear()
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn(), scrollTo: vi.fn(), location: { reload: vi.fn() } })
    vi.stubGlobal('document', { documentElement: { dataset: {} } })
  })
  afterEach(() => { runtime.hooks?.unmount(); vi.unstubAllGlobals() })

  it.each(['day', 'night'])('immediately paints the %s login form while still checking the server session', theme => {
    runtime.theme = theme
    runtime.me.mockReturnValue(deferred().promise)
    const result = render()
    expect(result.type).toBe(Login)
    expect(result.props).toMatchObject({ theme, verificationPending: true })
    expect(runtime.me).toHaveBeenCalledExactlyOnceWith()
  })

  it('never renders a cached account or room while a private-route refresh is pending', () => {
    runtime.route = { kind: 'tab', tab: 'lobby' }
    runtime.cached = { account: 'cached-user', nickname: 'Cached name', room: '88001', balance: 99999 }
    runtime.me.mockReturnValue(deferred().promise)
    const result = render()
    expect(result.type).toBe(SessionStartup)
    expect(result.props).toEqual({ theme: 'day' })
  })

  it('keeps transient session failures recoverable without trusting or deleting the cached account', async () => {
    runtime.route = { kind: 'tab', tab: 'lobby' }
    runtime.cached = { account: 'cached-user', room: '88001', balance: 99999 }
    const request = deferred()
    runtime.me.mockReturnValue(request.promise)
    render()
    request.reject(new Error('请求超时，请稍后重试'))
    await settle()
    const result = render()
    expect(result.type).toBe(SessionStartup)
    expect(result.props.error).toBe('请求超时，请稍后重试')
    expect(result.props).not.toHaveProperty('session')
    expect(result.props.onRetry).toBeTypeOf('function')
    expect(runtime.clearBusiness).not.toHaveBeenCalled()
    expect(runtime.refreshSession).not.toHaveBeenCalled()
  })

  it('clears an expired server session and then enables the ordinary login form', async () => {
    runtime.cached = { account: 'cached-user', room: '88001', balance: 99999 }
    const request = deferred()
    runtime.me.mockReturnValue(request.promise)
    render()
    request.reject(new AuthError('登录失效'))
    await settle()
    const result = render()
    expect(result.type).toBe(Login)
    expect(result.props.verificationPending).not.toBe(true)
    expect(runtime.clearBusiness).toHaveBeenCalledOnce()
    expect(runtime.refreshSession).not.toHaveBeenCalled()
  })

  it('renders private content only after the server profile has been accepted', async () => {
    runtime.route = { kind: 'tab', tab: 'lobby' }
    runtime.cached = { account: 'cached-user', room: '88001', balance: 99999 }
    const request = deferred()
    runtime.me.mockReturnValue(request.promise)
    expect(render().type).toBe(SessionStartup)
    request.resolve({ username: 'verified-user', nickname: 'Verified name', room_code: '99001', balance: 123 })
    await settle()
    const result = render()
    expect(result.type).toBe('main')
    expect(result.props.className).toContain('theme-day')
    expect(runtime.refreshSession).toHaveBeenCalledOnce()
  })
})
