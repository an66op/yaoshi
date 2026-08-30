import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LotteryGame } from '../api/lottery'
import { HookHarness } from '../test/hookHarness'
import { useLotteryGames } from './useLotteryGames'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null, games: vi.fn(), clock: vi.fn(), connected: true }))
vi.mock('react', async (importOriginal) => ({
  ...await importOriginal<typeof import('react')>(),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useMemo: <T,>(factory: () => T, dependencies: readonly unknown[]) => runtime.hooks!.useMemo(factory, dependencies),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))
vi.mock('../api/lottery', () => ({ lotteryApi: { enabledGames: runtime.games, clock: runtime.clock } }))
vi.mock('./useWebSocket', () => ({ WS_EVENT: 'catalog-test-event', useWebSocketConnected: () => runtime.connected }))

const start = Date.parse('2026-08-30T14:47:03+08:00')
const game: LotteryGame = {
  id: 'speed-racing', code: 'speed-racing', name: '赛车', category: 'racing', lobby_category: '彩票', lobby_sort_order: 0,
  badge: '', badge_color: '', enabled: true, issue: '34136854', current_issue: '34136855', latest_numbers: [1, 2, 3],
  source_kind: 'external', source_name: '', sync_status: 'ok', source_healthy: true, issue_status: 'accepting', draw_interval: 75, seal_seconds: 30,
  accept_at: new Date(start - 15_000).toISOString(), seal_at: new Date(start + 30_000).toISOString(), next_draw_at: new Date(start + 60_000).toISOString(),
}

const deferred = <T,>() => {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(accept => { resolve = accept })
  return { promise, resolve }
}

describe('member catalog hook recovery wiring', () => {
  let events: EventTarget
  let documentEvents: EventTarget & { visibilityState: string }
  const render = (room = '88001', enabled = true) => {
    const value = runtime.hooks!.render(() => useLotteryGames(enabled, room, 'speed-racing'))
    runtime.hooks!.flushEffects()
    return value
  }
  const notify = () => {
    const event = new Event('catalog-test-event')
    Object.defineProperty(event, 'detail', { value: { type: 'draw_update', data: { game_id: game.id } } })
    events.dispatchEvent(event)
  }

  beforeEach(() => {
    vi.useFakeTimers({ now: start, toFake: ['Date', 'setTimeout', 'clearTimeout', 'performance'] })
    runtime.hooks = new HookHarness()
    runtime.connected = true
    runtime.games.mockReset().mockResolvedValue([game])
    runtime.clock.mockReset().mockImplementation(async () => ({ server_time_ms: Date.now() }))
    events = new EventTarget()
    documentEvents = Object.assign(new EventTarget(), { visibilityState: 'visible' })
    vi.stubGlobal('document', documentEvents)
    vi.stubGlobal('window', {
      setTimeout, clearTimeout,
      addEventListener: events.addEventListener.bind(events), removeEventListener: events.removeEventListener.bind(events),
    })
  })
  afterEach(() => {
    runtime.hooks?.unmount()
    vi.clearAllTimers()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('leaves a stale settled issue without needing another websocket event', async () => {
    const stale = { ...game, current_issue: '34136854', issue_status: 'settled', next_draw_at: new Date(start - 15_000).toISOString() }
    runtime.games.mockResolvedValueOnce([stale]).mockResolvedValue([game])
    render()
    await vi.advanceTimersByTimeAsync(0)
    expect(render().games[0]).toMatchObject({ period: '34136854', timing: { phase: 'settled', accepting: false } })
    await vi.advanceTimersByTimeAsync(2000)
    expect(runtime.games).toHaveBeenCalledTimes(2)
    expect(render().games[0]).toMatchObject({ period: '34136855', timing: { phase: 'accepting' } })
    expect(runtime.clock).toHaveBeenCalledTimes(1)
  })

  it('does not delay a new catalog behind the initial clock request', async () => {
    const clock = deferred<{ server_time_ms: number }>()
    runtime.clock.mockReturnValueOnce(clock.promise)
    render()
    await vi.advanceTimersByTimeAsync(0)
    expect(render()).toMatchObject({ live: true, loading: false })
    expect(render().games[0].period).toBe('34136855')
    expect(render().games[0].timing.accepting).toBe(false)
    clock.resolve({ server_time_ms: start })
    await vi.advanceTimersByTimeAsync(0)
    expect(render().games[0].timing.accepting).toBe(true)
  })

  it('coalesces draw events without recalibrating the clock on every event', async () => {
    render()
    await vi.advanceTimersByTimeAsync(0)
    for (let index = 0; index < 20; index++) notify()
    await vi.advanceTimersByTimeAsync(100)
    expect(runtime.games).toHaveBeenCalledTimes(2)
    expect(runtime.clock).toHaveBeenCalledTimes(1)
  })

  it('ignores old-room results and aborts their requests', async () => {
    const oldRoom = deferred<LotteryGame[]>()
    runtime.games.mockReturnValueOnce(oldRoom.promise).mockResolvedValueOnce([{ ...game, current_issue: 'other-room' }])
    render()
    await vi.advanceTimersByTimeAsync(0)
    const oldSignal = runtime.games.mock.calls[0][0] as AbortSignal
    render('99001')
    await vi.advanceTimersByTimeAsync(0)
    expect(oldSignal.aborted).toBe(true)
    oldRoom.resolve([game])
    await vi.advanceTimersByTimeAsync(0)
    expect(render('99001').games[0].period).toBe('other-room')
  })

  it('retains a good snapshot during a transient failure and retries', async () => {
    render()
    await vi.advanceTimersByTimeAsync(0)
    runtime.games.mockRejectedValueOnce(new Error('offline'))
    notify()
    await vi.advanceTimersByTimeAsync(100)
    expect(render()).toMatchObject({ live: true, error: 'offline', loading: false })
    expect(render().games[0].period).toBe('34136855')
    await vi.advanceTimersByTimeAsync(2000)
    expect(render().error).toBe('')
  })

  it('pauses background polling and refreshes catalog plus clock on return', async () => {
    render()
    await vi.advanceTimersByTimeAsync(0)
    documentEvents.visibilityState = 'hidden'
    documentEvents.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(65_000)
    expect(runtime.games).toHaveBeenCalledTimes(1)
    expect(runtime.clock).toHaveBeenCalledTimes(1)
    documentEvents.visibilityState = 'visible'
    documentEvents.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(0)
    expect(runtime.games).toHaveBeenCalledTimes(2)
    expect(runtime.clock).toHaveBeenCalledTimes(2)
  })
})
