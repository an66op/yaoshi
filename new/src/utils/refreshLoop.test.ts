import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createRefreshLoop } from './refreshLoop'

const deferred = <T,>() => {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

describe('single-flight refresh recovery', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => { vi.clearAllTimers(); vi.useRealTimers() })

  it('coalesces an event burst into one request', async () => {
    const request = vi.fn().mockResolvedValue('ready')
    const onData = vi.fn()
    const loop = createRefreshLoop({ request, onData, onError: vi.fn(), delay: () => 30_000 })
    for (let index = 0; index < 20; index++) loop.refresh()
    await vi.advanceTimersByTimeAsync(99)
    expect(request).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    expect(request).toHaveBeenCalledTimes(1)
    expect(onData).toHaveBeenCalledWith('ready')
    loop.dispose()
  })

  it('keeps one trailing request when a new-period event arrives in flight', async () => {
    const first = deferred<string>()
    const request = vi.fn().mockReturnValueOnce(first.promise).mockResolvedValue('next-issue')
    const onData = vi.fn()
    const loop = createRefreshLoop({ request, onData, onError: vi.fn(), delay: () => 30_000 })
    loop.refresh(true)
    await vi.advanceTimersByTimeAsync(0)
    for (let index = 0; index < 20; index++) loop.refresh()
    expect(request).toHaveBeenCalledTimes(1)
    first.resolve('previous-issue')
    await vi.advanceTimersByTimeAsync(0)
    expect(onData).toHaveBeenLastCalledWith('previous-issue')
    await vi.advanceTimersByTimeAsync(100)
    expect(request).toHaveBeenCalledTimes(2)
    expect(onData).toHaveBeenLastCalledWith('next-issue')
    loop.dispose()
  })

  it('recovers a settled snapshot even when no further websocket event arrives', async () => {
    const request = vi.fn().mockResolvedValueOnce('settled').mockResolvedValue('accepting')
    const onData = vi.fn()
    const loop = createRefreshLoop({ request, onData, onError: vi.fn(), delay: data => data === 'settled' ? 2000 : 30_000 })
    loop.refresh(true)
    await vi.advanceTimersByTimeAsync(0)
    await vi.advanceTimersByTimeAsync(2000)
    expect(onData.mock.calls.map(call => call[0])).toEqual(['settled', 'accepting'])
    expect(request).toHaveBeenCalledTimes(2)
    loop.dispose()
  })

  it('backs off failures and does not replace the last good snapshot', async () => {
    const request = vi.fn().mockResolvedValueOnce('good').mockRejectedValue(new Error('offline'))
    const onData = vi.fn()
    const onError = vi.fn()
    const loop = createRefreshLoop({ request, onData, onError, delay: (_data, failures) => failures ? 4000 : 1000 })
    loop.refresh(true)
    await vi.advanceTimersByTimeAsync(1000)
    expect(request).toHaveBeenCalledTimes(2)
    expect(onData).toHaveBeenCalledTimes(1)
    expect(onError).toHaveBeenCalledTimes(1)
    loop.refresh()
    await vi.advanceTimersByTimeAsync(3999)
    expect(request).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(1)
    expect(request).toHaveBeenCalledTimes(3)
    loop.dispose()
  })

  it('aborts and ignores responses belonging to an old room', async () => {
    const pending = deferred<string>()
    const request = vi.fn((_signal: AbortSignal) => pending.promise)
    const onData = vi.fn()
    const onError = vi.fn()
    const loop = createRefreshLoop({ request, onData, onError, delay: () => 1000 })
    loop.refresh(true)
    await vi.advanceTimersByTimeAsync(0)
    const signal = request.mock.calls[0][0]
    loop.dispose()
    expect(signal.aborted).toBe(true)
    pending.resolve('old-room')
    await vi.advanceTimersByTimeAsync(5000)
    expect(onData).not.toHaveBeenCalled()
    expect(onError).not.toHaveBeenCalled()
    expect(request).toHaveBeenCalledTimes(1)
  })

  it('times out a stalled request and allows recovery', async () => {
    const request = vi.fn((signal: AbortSignal) => new Promise<string>((_resolve, reject) => signal.addEventListener('abort', () => reject(new Error('timeout')))))
    const onError = vi.fn()
    const loop = createRefreshLoop({ request, onData: vi.fn(), onError, delay: () => 2000, timeoutMs: 1000 })
    loop.refresh(true)
    await vi.advanceTimersByTimeAsync(1000)
    expect(request.mock.calls[0][0].aborted).toBe(true)
    expect(onError).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(2000)
    expect(request).toHaveBeenCalledTimes(2)
    loop.dispose()
  })

  it('pauses background refreshes and recovers immediately on visibility', async () => {
    const request = vi.fn().mockResolvedValue('ready')
    const loop = createRefreshLoop({ request, onData: vi.fn(), onError: vi.fn(), delay: () => 1000 })
    loop.refresh(true)
    await vi.advanceTimersByTimeAsync(0)
    loop.pause()
    loop.refresh()
    await vi.advanceTimersByTimeAsync(10_000)
    expect(request).toHaveBeenCalledTimes(1)
    loop.resume()
    await vi.advanceTimersByTimeAsync(0)
    expect(request).toHaveBeenCalledTimes(2)
    loop.dispose()
  })

  it('does not block catalog updates behind a stalled clock calibration', async () => {
    const clock = deferred<number>()
    const onClock = vi.fn()
    const onCatalog = vi.fn()
    const clockLoop = createRefreshLoop({ request: () => clock.promise, onData: onClock, onError: vi.fn(), delay: () => 60_000 })
    const catalogLoop = createRefreshLoop({ request: async () => 'next-issue', onData: onCatalog, onError: vi.fn(), delay: () => 30_000 })
    clockLoop.refresh(true)
    catalogLoop.refresh(true)
    await vi.advanceTimersByTimeAsync(0)
    expect(onClock).not.toHaveBeenCalled()
    expect(onCatalog).toHaveBeenCalledWith('next-issue')
    clockLoop.dispose()
    catalogLoop.dispose()
    clock.resolve(1)
  })
})
