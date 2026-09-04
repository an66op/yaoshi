import type { DependencyList, SetStateAction } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AdminGame } from '../api'
import { describeIssueState } from '../utils/drawTiming'
import { useSGGameCatalog } from './useSGGameCatalog'

type Slot = { value?: unknown; deps?: DependencyList; effect?: () => void | (() => void); cleanup?: void | (() => void) }
const sameDeps = (left?: DependencyList, right?: DependencyList) => Boolean(left && right && left.length === right.length && left.every((value, index) => Object.is(value, right[index])))
class HookHarness {
  private slots: Slot[] = []
  private cursor = 0
  private effects: number[] = []
  render<T>(factory: () => T): T { this.cursor = 0; return factory() }
  useRef<T>(initial: T) {
    const slot = this.slots[this.cursor++] ??= { value: { current: initial } }
    return slot.value as { current: T }
  }
  useMemo<T>(factory: () => T, deps: DependencyList): T {
    const index = this.cursor++
    if (!this.slots[index] || !sameDeps(this.slots[index].deps, deps)) this.slots[index] = { value: factory(), deps }
    return this.slots[index].value as T
  }
  useEffect(effect: () => void | (() => void), deps?: DependencyList) {
    const index = this.cursor++
    const previous = this.slots[index]
    if (!previous || !sameDeps(previous.deps, deps)) {
      previous?.cleanup?.()
      this.slots[index] = { effect, deps }
      this.effects.push(index)
    }
  }
  flushEffects() { for (const index of this.effects.splice(0)) this.slots[index].cleanup = this.slots[index].effect?.() }
  unmount() { for (const slot of this.slots) slot.cleanup?.() }
}
const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useCallback: <T,>(callback: T, deps: DependencyList) => runtime.hooks!.useMemo(() => callback, deps),
  useEffect: (effect: () => void | (() => void), deps?: DependencyList) => runtime.hooks!.useEffect(effect, deps),
}))

const now = Date.parse('2026-09-03T06:00:00Z')
const sg: AdminGame = {
  id: 'sg-ssc', code: 'SGSSC', name: 'SG时时彩', category: 'ssc', lobby_category: '彩票', lobby_sort_order: 0,
  badge: '', badge_color: '', enabled: true, issue: '20260903168', current_issue: '20260903169', issue_status: 'accepting',
  latest_numbers: [0, 9, 2, 7, 4], accept_at: '2026-09-03T06:00:00Z', seal_at: '2026-09-03T06:04:30Z', next_draw_at: '2026-09-03T06:05:00Z',
  turnover: 0, profit: 0, source_kind: 'external', source_name: 'SG双站核对', source_url: '', source_healthy: true,
  sync_status: 'ok', last_sync_at: '2026-09-03T06:00:00Z', last_sync_error: '', schedule_mode: 'external-feed', rules_ready: true, rule_version: 'digits5-v3',
}
const deferred = <T,>() => {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((accept, fail) => { resolve = accept; reject = fail })
  return { promise, resolve, reject }
}

describe('selected SG management catalog polling', () => {
  let games: AdminGame[]
  let publications: number
  let request: ReturnType<typeof vi.fn<() => Promise<AdminGame[]>>>
  const setGames = (value: SetStateAction<AdminGame[]>) => { games = typeof value === 'function' ? value(games) : value; publications += 1 }
  const render = (gameID: string | undefined = 'sg-ssc', scope = 'platform', reader = request) => {
    const read = runtime.hooks!.render(() => useSGGameCatalog(reader, setGames, gameID, scope))
    runtime.hooks!.flushEffects()
    return read
  }
  beforeEach(() => {
    vi.useFakeTimers({ now })
    vi.stubGlobal('window', { setTimeout, clearTimeout })
    runtime.hooks = new HookHarness()
    games = [sg]
    publications = 0
    request = vi.fn().mockResolvedValue([sg])
  })
  afterEach(() => { runtime.hooks?.unmount(); vi.clearAllTimers(); vi.unstubAllGlobals(); vi.useRealTimers() })

  it.each(['platform', 'agent', 'tenant'])('refreshes %s SG source failure and recovery without any websocket event', async scope => {
    render('sg-ssc', scope)
    request.mockResolvedValueOnce([{ ...sg, source_healthy: false }])
    await vi.advanceTimersByTimeAsync(10_000)
    expect(request).toHaveBeenCalledTimes(1)
    expect(describeIssueState(games[0], Date.now())).toBe('开奖源异常 · 已停盘')
    await vi.advanceTimersByTimeAsync(10_000)
    expect(request).toHaveBeenCalledTimes(2)
    expect(describeIssueState(games[0], Date.now())).toBe('受理中')
  })

  it('does not poll other games and stops on switch or unmount', async () => {
    render('speed-racing')
    await vi.advanceTimersByTimeAsync(30_000)
    expect(request).not.toHaveBeenCalled()
    render()
    await vi.advanceTimersByTimeAsync(10_000)
    expect(request).toHaveBeenCalledTimes(1)
    render('speed-racing')
    await vi.advanceTimersByTimeAsync(30_000)
    expect(request).toHaveBeenCalledTimes(1)
    render()
    runtime.hooks!.unmount()
    await vi.advanceTimersByTimeAsync(30_000)
    expect(request).toHaveBeenCalledTimes(1)
  })

  it('coalesces manual, websocket and timer reads while a slow catalog request is pending', async () => {
    const slow = deferred<AdminGame[]>()
    request.mockReturnValueOnce(slow.promise)
    const read = render()
    const initial = read()
    const event = read()
    await vi.advanceTimersByTimeAsync(35_000)
    expect(request).toHaveBeenCalledTimes(1)
    slow.resolve([{ ...sg, source_healthy: false }])
    await Promise.all([initial, event])
    expect(publications).toBe(1)
    expect(games[0].source_healthy).toBe(false)
    await vi.advanceTimersByTimeAsync(9999)
    expect(request).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(request).toHaveBeenCalledTimes(2)
  })

  it('discards an old SG poll after SG → another game → SG, then polls the new selection', async () => {
    const old = deferred<AdminGame[]>()
    request.mockReturnValueOnce(old.promise)
    render()
    await vi.advanceTimersByTimeAsync(10_000)
    render('speed-racing')
    render()
    old.resolve([{ ...sg, current_issue: 'old-period' }])
    await vi.advanceTimersByTimeAsync(0)
    expect(publications).toBe(0)
    request.mockResolvedValue([{ ...sg, current_issue: 'new-period' }])
    await vi.advanceTimersByTimeAsync(10_000)
    expect(games[0].current_issue).toBe('new-period')
  })

  it('ignores both late data and late errors after switching request identity or unmounting', async () => {
    const old = deferred<AdminGame[]>()
    request.mockReturnValueOnce(old.promise)
    render('sg-ssc', 'agent')
    await vi.advanceTimersByTimeAsync(10_000)
    const tenant = vi.fn().mockResolvedValue([{ ...sg, current_issue: 'tenant-period' }])
    render('sg-ssc', 'tenant', tenant)
    await vi.advanceTimersByTimeAsync(10_000)
    old.reject(new Error('old-room failure'))
    await vi.advanceTimersByTimeAsync(0)
    expect(games[0].current_issue).toBe('tenant-period')
    expect(games[0].source_healthy).toBe(true)
    const late = deferred<AdminGame[]>()
    tenant.mockReturnValueOnce(late.promise)
    await vi.advanceTimersByTimeAsync(10_000)
    const count = publications
    runtime.hooks!.unmount()
    late.resolve([{ ...sg, current_issue: 'after-unmount' }])
    await vi.advanceTimersByTimeAsync(0)
    expect(publications).toBe(count)
  })

  it('closes SG on API failure without removing history or rules and restores only from a new response', async () => {
    render()
    request.mockRejectedValueOnce(new Error('offline'))
    await vi.advanceTimersByTimeAsync(10_000)
    expect(games[0]).toMatchObject({ current_issue: '', source_healthy: false, last_sync_error: 'offline',
      issue: sg.issue, latest_numbers: sg.latest_numbers, rules_ready: true, rule_version: 'digits5-v3' })
    await vi.advanceTimersByTimeAsync(10_000)
    expect(games[0]).toMatchObject({ current_issue: sg.current_issue, source_healthy: true })
  })
})
