import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { DrawResult } from '../api/lottery'
import { HookHarness } from '../test/hookHarness'
import { reuseUnchangedDraws, useGameDraws } from './useGameDraws'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null, draws: vi.fn() }))
vi.mock('react', async (importOriginal) => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))
vi.mock('../api/lottery', () => ({ lotteryApi: { draws: runtime.draws } }))
vi.mock('./useWebSocket', () => ({ WS_EVENT: 'draw-test-event', useWebSocketConnected: () => true }))

const draw: DrawResult = { id: 11, game_id: 'speed-racing', issue: '34136854', numbers: [6, 1, 2, 8, 9, 10, 4, 7, 5, 3], draw_at: '2026-08-30T06:46:00Z' }

describe('unchanged draw snapshot references', () => {
  it('retains the array and rows for equal API snapshots', () => {
    const previous = [draw]
    expect(reuseUnchangedDraws(previous, structuredClone(previous))).toBe(previous)
  })

  it('retains the current row when older history is appended', () => {
    const older = { ...draw, id: 10, issue: '34136853' }
    const previous = [draw]
    const next = reuseUnchangedDraws(previous, [structuredClone(draw), older])
    expect(next).not.toBe(previous)
    expect(next[0]).toBe(draw)
    expect(next[1]).toBe(older)
  })

  it.each([
    { numbers: [1, 6, 2, 8, 9, 10, 4, 7, 5, 3] },
    { numbers: [6, 1, 2] },
    { issue: 'corrected-issue' },
    { draw_at: '2026-08-30T06:46:10Z' },
    { id: 12 },
    { game_id: 'speed-fly' },
  ])('preserves corrections in every draw field: %j', correction => {
    const changed = { ...structuredClone(draw), ...correction }
    expect(reuseUnchangedDraws([draw], [changed])[0]).toBe(changed)
  })

  it('preserves server row ordering and removals', () => {
    const older = { ...draw, id: 10, issue: '34136853' }
    const previous = [draw, older]
    expect(reuseUnchangedDraws(previous, [structuredClone(older), structuredClone(draw)])).toEqual([older, draw])
    expect(reuseUnchangedDraws(previous, [])).toEqual([])
  })
})

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<T>((accept, fail) => { resolve = accept; reject = fail })
  return { promise, resolve, reject }
}

describe('draw refresh ordering', () => {
  let events: EventTarget
  const render = (gameId = 'speed-racing') => {
    const value = runtime.hooks!.render(() => useGameDraws(gameId))
    runtime.hooks!.flushEffects()
    return value
  }
  const notify = (gameId = 'speed-racing') => {
    const event = new Event('draw-test-event')
    Object.defineProperty(event, 'detail', { value: { type: 'draw_update', data: { game_id: gameId } } })
    events.dispatchEvent(event)
  }
  const flush = async () => { await Promise.resolve(); await Promise.resolve() }

  beforeEach(() => {
    vi.useFakeTimers()
    runtime.hooks = new HookHarness()
    runtime.draws.mockReset()
    events = new EventTarget()
    vi.stubGlobal('window', {
      addEventListener: events.addEventListener.bind(events),
      removeEventListener: events.removeEventListener.bind(events),
      setInterval, clearInterval, setTimeout, clearTimeout,
    })
  })
  afterEach(() => { runtime.hooks?.unmount(); vi.clearAllTimers(); vi.useRealTimers(); vi.unstubAllGlobals() })

  it('coalesces a burst into one follow-up request and cannot complete snapshots out of order', async () => {
    const first = deferred<DrawResult[]>()
    const next = deferred<DrawResult[]>()
    runtime.draws.mockReturnValueOnce(first.promise).mockReturnValueOnce(next.promise)
    render()
    notify(); notify(); notify()
    expect(runtime.draws).toHaveBeenCalledTimes(1)
    first.resolve([draw])
    await flush()
    expect(runtime.draws).toHaveBeenCalledTimes(2)
    const latest = { ...draw, id: 12, issue: '34136855' }
    next.resolve([latest, structuredClone(draw)])
    await flush()
    expect(render().draws).toEqual([latest, draw])
    expect(runtime.draws).toHaveBeenCalledTimes(2)
  })

  it('still runs the queued refresh after a temporary request error', async () => {
    const first = deferred<DrawResult[]>()
    runtime.draws.mockReturnValueOnce(first.promise).mockResolvedValueOnce([draw])
    render()
    notify()
    first.reject(new Error('timeout'))
    await flush()
    expect(runtime.draws).toHaveBeenCalledTimes(2)
    expect(render()).toMatchObject({ draws: [draw], error: '', loading: false })
  })

  it('ignores an old game response and its queued refresh after switching games', async () => {
    const oldGame = deferred<DrawResult[]>()
    const nextGame = deferred<DrawResult[]>()
    runtime.draws.mockReturnValueOnce(oldGame.promise).mockReturnValueOnce(nextGame.promise)
    render()
    const oldSignal = runtime.draws.mock.calls[0][2] as AbortSignal
    notify()
    render('speed-fly')
    expect(oldSignal.aborted).toBe(true)
    const otherDraw = { ...draw, game_id: 'speed-fly', issue: '54776109' }
    nextGame.resolve([otherDraw])
    await flush()
    oldGame.resolve([draw])
    await flush()
    expect(render('speed-fly').draws).toEqual([otherDraw])
    expect(runtime.draws).toHaveBeenCalledTimes(2)
  })

  it('does not fetch another game or replay queued requests after unmount', async () => {
    const first = deferred<DrawResult[]>()
    runtime.draws.mockReturnValueOnce(first.promise)
    render()
    const signal = runtime.draws.mock.calls[0][2] as AbortSignal
    notify('speed-fly')
    notify()
    runtime.hooks!.unmount()
    expect(signal.aborted).toBe(true)
    first.resolve([draw])
    await flush()
    expect(runtime.draws).toHaveBeenCalledTimes(1)
  })

  it('aborts a hanging response body at 15 seconds and runs the queued draw refresh', async () => {
    let headersReceived = false
    runtime.draws.mockImplementationOnce(async (_game: string, _limit: number, signal: AbortSignal) => {
      headersReceived = true
      // fetch has resolved headers, but its body reader still waits. The
      // request-wide AbortSignal must remain live for this entire operation.
      return await new Promise<DrawResult[]>((_resolve, reject) => {
        signal.addEventListener('abort', () => reject(new DOMException('Body read aborted', 'AbortError')), { once: true })
      })
    }).mockResolvedValueOnce([draw])
    render()
    expect(headersReceived).toBe(true)
    notify()
    const signal = runtime.draws.mock.calls[0][2] as AbortSignal
    await vi.advanceTimersByTimeAsync(14_999)
    expect(signal.aborted).toBe(false)
    expect(runtime.draws).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(signal.aborted).toBe(true)
    expect(runtime.draws).toHaveBeenCalledTimes(2)
    expect(render()).toMatchObject({ draws: [draw], loading: false, error: '' })
  })

  it('releases a timed-out request without queued events so a later event can recover', async () => {
    runtime.draws.mockImplementationOnce((_game: string, _limit: number, signal: AbortSignal) => new Promise<DrawResult[]>((_resolve, reject) => {
      signal.addEventListener('abort', () => reject(new Error('timeout')), { once: true })
    })).mockResolvedValueOnce([draw])
    render()
    await vi.advanceTimersByTimeAsync(15_000)
    expect(render()).toMatchObject({ loading: false, error: '读取开奖超时，请稍后重试' })
    expect(runtime.draws).toHaveBeenCalledTimes(1)
    notify()
    await flush()
    expect(render()).toMatchObject({ draws: [draw], error: '' })
    expect(runtime.draws).toHaveBeenCalledTimes(2)
  })

  it('clears a completed request deadline instead of aborting a later request', async () => {
    runtime.draws.mockResolvedValue([draw])
    render()
    await flush()
    const firstSignal = runtime.draws.mock.calls[0][2] as AbortSignal
    await vi.advanceTimersByTimeAsync(10_000)
    notify()
    await flush()
    const nextSignal = runtime.draws.mock.calls[1][2] as AbortSignal
    await vi.advanceTimersByTimeAsync(20_000)
    expect(firstSignal.aborted).toBe(false)
    expect(nextSignal.aborted).toBe(false)
  })
})
