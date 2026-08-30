import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { HookHarness } from '../test/hookHarness'
import { usePlanCatalog, usePlanDetail, useRacingPlanStream } from './usePlanFeed'
import { racingPlanDetail } from '../test/racingPlanFixtures'
import { DEFAULT_RACING_PLAN } from '../utils/racingPlans'
import type { RacingPlanDetail, RacingPlanSelection } from '../api/plans'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null, catalog: vi.fn(), detail: vi.fn(), activate: vi.fn(), racingDetail: vi.fn(), activateRacing: vi.fn() }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useMemo: <T,>(factory: () => T, dependencies: readonly unknown[]) => runtime.hooks!.useMemo(factory, dependencies),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))
vi.mock('../api/plans', () => ({ PLAN_HISTORY_LIMIT: 6, PLAN_HISTORY_MAX: 10, planApi: { catalog: runtime.catalog, detail: runtime.detail, activate: runtime.activate, racingDetail: runtime.racingDetail, activateRacing: runtime.activateRacing } }))
vi.mock('./useWebSocket', () => ({ WS_EVENT: 'plan-test-event' }))

const row = { game_id: 'speed-racing', current_issue: '100', latest_issue: '100', history_only: false, master_count: 3, updated_at: '2026-08-30T08:00:00Z' }

describe('plan feed refresh lifecycle', () => {
  let events: EventTarget
  let documentEvents: EventTarget & { visibilityState: string }
  const render = (room = '88001') => {
    const result = runtime.hooks!.render(() => usePlanCatalog(room))
    runtime.hooks!.flushEffects()
    return result
  }
  const renderRacing = (room = '88001') => {
    const result = runtime.hooks!.render(() => useRacingPlanStream(room))
    runtime.hooks!.flushEffects()
    return result
  }
  const renderLegacy = (game = 'speed-fly') => {
    const result = runtime.hooks!.render(() => usePlanDetail(game, '88001'))
    runtime.hooks!.flushEffects()
    return result
  }
  const setVisible = (isVisible: boolean) => {
    documentEvents.visibilityState = isVisible ? 'visible' : 'hidden'
    documentEvents.dispatchEvent(new Event('visibilitychange'))
  }
  const notify = (type = 'draw_update', game_id = 'speed-racing') => {
    const event = new Event('plan-test-event')
    Object.defineProperty(event, 'detail', { value: { type, game_id } })
    events.dispatchEvent(event)
  }
  beforeEach(() => {
    vi.useFakeTimers()
    runtime.hooks = new HookHarness()
    runtime.catalog.mockReset().mockResolvedValue([row])
    runtime.detail.mockReset().mockResolvedValue({ game_id: 'speed-racing', current_issue: '100', recommendations: [], latest_recommendations: [], history: [] })
    runtime.activate.mockReset()
    runtime.racingDetail.mockReset().mockImplementation((selection: RacingPlanSelection) => Promise.resolve(racingPlanDetail(selection)))
    runtime.activateRacing.mockReset().mockImplementation((selection: RacingPlanSelection) => Promise.resolve(racingPlanDetail(selection)))
    events = new EventTarget()
    documentEvents = Object.assign(new EventTarget(), { visibilityState: 'visible' })
    vi.stubGlobal('document', documentEvents)
    vi.stubGlobal('window', { addEventListener: events.addEventListener.bind(events), removeEventListener: events.removeEventListener.bind(events) })
  })
  afterEach(() => { runtime.hooks?.unmount(); vi.clearAllTimers(); vi.useRealTimers(); vi.unstubAllGlobals() })

  it('picks up a publication after an initially empty catalog without a draw event', async () => {
    runtime.catalog.mockResolvedValueOnce([])
    render()
    await vi.advanceTimersByTimeAsync(0)
    expect(render().data).toEqual([])
    await vi.advanceTimersByTimeAsync(15_000)
    expect(render().data).toEqual([row])
    expect(runtime.catalog).toHaveBeenCalledTimes(2)
  })
  it('coalesces draw bursts and retains confirmed data while retrying failures', async () => {
    render(); await vi.advanceTimersByTimeAsync(0)
    runtime.catalog.mockRejectedValueOnce(new Error('offline'))
    for (let index = 0; index < 20; index++) notify()
    await vi.advanceTimersByTimeAsync(100)
    expect(runtime.catalog).toHaveBeenCalledTimes(2)
    expect(render()).toMatchObject({ data: [row], error: 'offline', loading: false })
    await vi.advanceTimersByTimeAsync(2000)
    expect(render().error).toBe('')
  })
  it('pauses while hidden and refreshes immediately on return', async () => {
    render(); await vi.advanceTimersByTimeAsync(0)
    documentEvents.visibilityState = 'hidden'; documentEvents.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(60_000)
    expect(runtime.catalog).toHaveBeenCalledTimes(1)
    documentEvents.visibilityState = 'visible'; documentEvents.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(0)
    expect(runtime.catalog).toHaveBeenCalledTimes(2)
  })
  it('aborts old-room requests and ignores their late results', async () => {
    let finish!: (rows: typeof row[]) => void
    runtime.catalog.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    render(); await vi.advanceTimersByTimeAsync(0)
    const signal = runtime.catalog.mock.calls[0][0] as AbortSignal
    expect(render('99001').data).toBeNull()
    await vi.advanceTimersByTimeAsync(0)
    expect(signal.aborted).toBe(true)
    finish([{ ...row, current_issue: 'old-room' }]); await vi.advanceTimersByTimeAsync(0)
    expect(render('99001').data).toEqual([row])
  })
  it('filters unrelated draw events in game details and disposes listeners', async () => {
    runtime.hooks!.render(() => usePlanDetail('speed-racing', '88001')); runtime.hooks!.flushEffects()
    await vi.advanceTimersByTimeAsync(0)
    notify('draw_update', 'speed-fly'); await vi.advanceTimersByTimeAsync(100)
    expect(runtime.detail).toHaveBeenCalledTimes(1)
    notify('plan_update'); await vi.advanceTimersByTimeAsync(100)
    expect(runtime.detail).toHaveBeenCalledTimes(2)
    runtime.hooks!.unmount(); notify(); await vi.advanceTimersByTimeAsync(60_000)
    expect(runtime.detail).toHaveBeenCalledTimes(2)
  })
  it('touches even the default stream while visible, including polling and draw events', async () => {
    const inactive = racingPlanDetail()
    inactive.stream.active = false
    inactive.stream.activation_required = true
    runtime.racingDetail.mockResolvedValue(inactive)
    renderRacing(); await vi.advanceTimersByTimeAsync(0)
    expect(renderRacing().selection).toEqual(DEFAULT_RACING_PLAN)
    expect(runtime.racingDetail.mock.calls[0][0]).toEqual(DEFAULT_RACING_PLAN)
    notify(); await vi.advanceTimersByTimeAsync(100)
    await vi.advanceTimersByTimeAsync(15_000)
    expect(runtime.activateRacing).toHaveBeenCalledTimes(3)
    for (const call of runtime.activateRacing.mock.calls) expect(call[0]).toEqual(DEFAULT_RACING_PLAN)
  })
  it('does not read or touch on a hidden mount and resumes a visitor lease on return', async () => {
    setVisible(false)
    renderRacing(); await vi.advanceTimersByTimeAsync(60_000)
    expect(runtime.racingDetail).not.toHaveBeenCalled()
    expect(runtime.activateRacing).not.toHaveBeenCalled()
    expect(await renderRacing().activate(DEFAULT_RACING_PLAN)).toBe(false)
    setVisible(true); await vi.advanceTimersByTimeAsync(0)
    expect(runtime.racingDetail).toHaveBeenCalledTimes(1)
    expect(runtime.activateRacing).toHaveBeenCalledTimes(1)
  })
  it.each(['disabled', 'not-allowed'])('keeps %s streams read-only on every refresh', async condition => {
    const detail = racingPlanDetail()
    if (condition === 'disabled') detail.automation_enabled = false
    else detail.stream.allowed = false
    runtime.racingDetail.mockResolvedValue(detail)
    renderRacing(); await vi.advanceTimersByTimeAsync(30_000)
    expect(runtime.racingDetail).toHaveBeenCalledTimes(3)
    expect(runtime.activateRacing).not.toHaveBeenCalled()
    expect(renderRacing().data?.history).toEqual(detail.history)
  })
  it('does not activate saved plans when switching while automation is disabled', async () => {
    runtime.racingDetail.mockImplementation((selection: RacingPlanSelection) => Promise.resolve(racingPlanDetail(selection, { automation_enabled: false })))
    renderRacing(); await vi.advanceTimersByTimeAsync(0)
    const next = { position: 2, plan_key: 'size-five-periods' }
    expect(await renderRacing().activate(next)).toBe(true)
    expect(runtime.activateRacing).not.toHaveBeenCalled()
    expect(renderRacing().selection).toEqual(next)
  })
  it('preserves real publications when an initial visitor touch fails', async () => {
    runtime.activateRacing.mockRejectedValueOnce(new Error('访问计划已达上限'))
    renderRacing(); await vi.advanceTimersByTimeAsync(0)
    expect(renderRacing()).toMatchObject({ error: '访问计划已达上限', loading: false })
    expect(renderRacing().data?.history).toHaveLength(3)
    await vi.advanceTimersByTimeAsync(14_999)
    expect(runtime.activateRacing).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(renderRacing().error).toBe('')
  })
  it('retains the first successful read when its visitor POST times out', async () => {
    runtime.activateRacing.mockImplementationOnce((_selection: RacingPlanSelection, signal: AbortSignal) => new Promise((_resolve, reject) => {
      signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true })
    }))
    renderRacing(); await vi.advanceTimersByTimeAsync(15_000)
    expect(renderRacing()).toMatchObject({ loading: false, error: '更新计划超时，请稍后重试' })
    expect(renderRacing().data?.history).toHaveLength(3)
    expect(runtime.activateRacing).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(15_000)
    expect(renderRacing().error).toBe('')
  })
  it('aborts a hidden in-flight GET and never uses its late result to generate', async () => {
    let finish!: (detail: RacingPlanDetail) => void
    runtime.racingDetail.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    renderRacing(); await vi.advanceTimersByTimeAsync(0)
    const signal = runtime.racingDetail.mock.lastCall?.[1] as AbortSignal
    setVisible(false)
    expect(signal.aborted).toBe(true)
    finish(racingPlanDetail()); await vi.advanceTimersByTimeAsync(60_000)
    expect(runtime.activateRacing).not.toHaveBeenCalled()
    expect(renderRacing().data).toBeNull()
  })
  it('aborts a hidden visitor POST and ignores its late response until a fresh visible request', async () => {
    renderRacing(); await vi.advanceTimersByTimeAsync(0)
    let finish!: (detail: RacingPlanDetail) => void
    runtime.activateRacing.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    await vi.advanceTimersByTimeAsync(15_000)
    const signal = runtime.activateRacing.mock.lastCall?.[1] as AbortSignal
    setVisible(false)
    expect(signal.aborted).toBe(true)
    finish(racingPlanDetail(DEFAULT_RACING_PLAN, { current_issue: 'late-hidden' }))
    notify(); events.dispatchEvent(new Event('online'))
    await vi.advanceTimersByTimeAsync(60_000)
    expect(runtime.activateRacing).toHaveBeenCalledTimes(2)
    expect(renderRacing().data?.current_issue).toBe('100')
    setVisible(true); await vi.advanceTimersByTimeAsync(0)
    expect(runtime.activateRacing).toHaveBeenCalledTimes(3)
  })
  it('coalesces event bursts while touching without overlapping another POST', async () => {
    let finish!: (detail: RacingPlanDetail) => void
    runtime.activateRacing.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    renderRacing(); await vi.advanceTimersByTimeAsync(0)
    for (let index = 0; index < 20; index++) notify()
    await vi.advanceTimersByTimeAsync(100)
    expect(runtime.racingDetail).toHaveBeenCalledTimes(1)
    expect(runtime.activateRacing).toHaveBeenCalledTimes(1)
    finish(racingPlanDetail()); await vi.advanceTimersByTimeAsync(100)
    expect(runtime.activateRacing).toHaveBeenCalledTimes(2)
    expect(runtime.racingDetail).toHaveBeenCalledTimes(2)
  })
  it('aborts a pending selection on hide without changing the confirmed stream', async () => {
    renderRacing(); await vi.advanceTimersByTimeAsync(0)
    let finish!: (detail: RacingPlanDetail) => void
    runtime.activateRacing.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    const next = { position: 3, plan_key: 'size-five-periods' }
    const request = renderRacing().activate(next)
    const signal = runtime.activateRacing.mock.lastCall?.[1] as AbortSignal
    renderRacing()
    setVisible(false)
    expect(signal.aborted).toBe(true)
    finish(racingPlanDetail(next)); expect(await request).toBe(false)
    expect(renderRacing()).toMatchObject({ selection: DEFAULT_RACING_PLAN, activating: false })
    await vi.advanceTimersByTimeAsync(60_000)
    expect(runtime.activateRacing).toHaveBeenCalledTimes(2)
    setVisible(true); await vi.advanceTimersByTimeAsync(0)
    expect(runtime.activateRacing.mock.lastCall?.[0]).toEqual(DEFAULT_RACING_PLAN)
  })
  it('touches an enabled legacy game only while visible and returns to GET-only after disabling', async () => {
    const detail = { game_id: 'speed-fly', current_issue: '100', recommendations: [], latest_recommendations: [], history: [], automation_enabled: true }
    runtime.detail.mockResolvedValue(detail)
    runtime.activate.mockResolvedValue(detail)
    renderLegacy(); await vi.advanceTimersByTimeAsync(0)
    expect(runtime.activate).toHaveBeenCalledTimes(1)
    expect(runtime.activate.mock.lastCall?.[0]).toBe('speed-fly')
    setVisible(false); await vi.advanceTimersByTimeAsync(60_000)
    expect(runtime.activate).toHaveBeenCalledTimes(1)
    runtime.detail.mockResolvedValue({ ...detail, automation_enabled: false })
    setVisible(true); await vi.advanceTimersByTimeAsync(30_000)
    expect(runtime.activate).toHaveBeenCalledTimes(1)
    expect(renderLegacy().data?.automation_enabled).toBe(false)
  })
  it('keeps a manual legacy publication after a failed visit without manufacturing history', async () => {
    const published = racingPlanDetail().history[0]
    const detail = { game_id: 'speed-fly', current_issue: '100', recommendations: [], latest_recommendations: [], history: [published], automation_enabled: true }
    runtime.detail.mockResolvedValue(detail)
    runtime.activate.mockRejectedValue(new Error('自动推荐已关闭'))
    renderLegacy(); await vi.advanceTimersByTimeAsync(0)
    expect(renderLegacy()).toMatchObject({ error: '自动推荐已关闭', data: { history: [published] } })
    runtime.detail.mockResolvedValue({ ...detail, automation_enabled: false })
    await vi.advanceTimersByTimeAsync(15_000)
    expect(runtime.activate).toHaveBeenCalledTimes(1)
    expect(renderLegacy().error).toBe('')
  })
  it('changes to the confirmed independent stream and aborts old reads before they can overwrite it', async () => {
    let finishOld!: (detail: RacingPlanDetail) => void
    runtime.racingDetail.mockReturnValueOnce(new Promise(resolve => { finishOld = resolve }))
    renderRacing(); await vi.advanceTimersByTimeAsync(0)
    const oldSignal = runtime.racingDetail.mock.calls[0][1] as AbortSignal
    const next = { position: 10, plan_key: 'two-period-eight-codes' }
    expect(await renderRacing().activate(next)).toBe(true)
    const switched = renderRacing()
    expect(switched.selection).toEqual(next)
    expect(switched.data?.selection).toMatchObject(next)
    expect(oldSignal.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(0)
    finishOld(racingPlanDetail()); await vi.advanceTimersByTimeAsync(0)
    expect(renderRacing().data?.selection).toMatchObject(next)
    expect(runtime.racingDetail.mock.lastCall?.[0]).toEqual(next)
  })
  it('keeps the old selection after quota or closed-plan rejection', async () => {
    renderRacing(); await vi.advanceTimersByTimeAsync(0)
    runtime.activateRacing.mockRejectedValueOnce(new Error('活跃计划已达20组上限'))
    expect(await renderRacing().activate({ position: 2, plan_key: 'size-five-periods' })).toBe(false)
    expect(renderRacing()).toMatchObject({ selection: DEFAULT_RACING_PLAN, activating: false, activationError: '活跃计划已达20组上限' })
    expect(renderRacing().data?.selection).toMatchObject(DEFAULT_RACING_PLAN)
    runtime.activateRacing.mockRejectedValueOnce(new Error('房间尚未开放该计划'))
    expect(await renderRacing().activate({ position: 3, plan_key: 'parity-three-periods' })).toBe(false)
    expect(renderRacing().activationError).toBe('房间尚未开放该计划')
  })
  it('ignores a superseded activation response and cancels its request', async () => {
    renderRacing(); await vi.advanceTimersByTimeAsync(0)
    let finishFirst!: (detail: RacingPlanDetail) => void
    runtime.activateRacing.mockReturnValueOnce(new Promise(resolve => { finishFirst = resolve }))
    const first = { position: 2, plan_key: 'size-five-periods' }
    const second = { position: 6, plan_key: 'dragon-tiger-three-periods' }
    const firstPromise = renderRacing().activate(first)
    const firstSignal = runtime.activateRacing.mock.lastCall?.[1] as AbortSignal
    expect(await renderRacing().activate(second)).toBe(true)
    expect(firstSignal.aborted).toBe(true)
    finishFirst(racingPlanDetail(first))
    expect(await firstPromise).toBe(false)
    expect(renderRacing().selection).toEqual(second)
    expect(renderRacing().data?.selection).toMatchObject(second)
  })
  it('aborts an activation when leaving the room and never applies its late result in the next room', async () => {
    renderRacing(); await vi.advanceTimersByTimeAsync(0)
    let finish!: (detail: RacingPlanDetail) => void
    runtime.activateRacing.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    const requested = { position: 3, plan_key: 'three-period-six-codes' }
    const pending = renderRacing().activate(requested)
    const signal = runtime.activateRacing.mock.lastCall?.[1] as AbortSignal
    expect(renderRacing('99001').data).toBeNull()
    expect(signal.aborted).toBe(true)
    finish(racingPlanDetail(requested)); expect(await pending).toBe(false)
    await vi.advanceTimersByTimeAsync(0)
    expect(renderRacing('99001').selection).toEqual(DEFAULT_RACING_PLAN)
    expect(renderRacing('99001').data?.selection).toMatchObject(DEFAULT_RACING_PLAN)
  })
  it('aborts pending activation and clears timers on unmount, ignoring an eventual response', async () => {
    renderRacing(); await vi.advanceTimersByTimeAsync(0)
    let finish!: (detail: RacingPlanDetail) => void
    runtime.activateRacing.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    const next = { position: 3, plan_key: 'size-five-periods' }
    const request = renderRacing().activate(next)
    const signal = runtime.activateRacing.mock.lastCall?.[1] as AbortSignal
    runtime.hooks!.unmount()
    expect(signal.aborted).toBe(true)
    expect(vi.getTimerCount()).toBe(0)
    finish(racingPlanDetail(next))
    expect(await request).toBe(false)
  })
  it('reports activation timeout without accepting a late response or leaving controls busy', async () => {
    renderRacing(); await vi.advanceTimersByTimeAsync(0)
    let finish!: (detail: RacingPlanDetail) => void
    runtime.activateRacing.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    const next = { position: 4, plan_key: 'parity-five-periods' }
    const request = renderRacing().activate(next)
    await vi.advanceTimersByTimeAsync(15_000)
    expect(renderRacing()).toMatchObject({ activating: false, activationError: '切换计划超时，请稍后重试', selection: DEFAULT_RACING_PLAN })
    finish(racingPlanDetail(next))
    expect(await request).toBe(false)
    expect(renderRacing().selection).toEqual(DEFAULT_RACING_PLAN)
  })
  it('rejects mismatched stream reads and activation responses', async () => {
    runtime.racingDetail.mockResolvedValueOnce(racingPlanDetail({ position: 2, plan_key: 'size-five-periods' }))
    renderRacing(); await vi.advanceTimersByTimeAsync(0)
    expect(renderRacing().data).toBeNull()
    expect(renderRacing().error).toContain('当前选择不一致')
    runtime.activateRacing.mockResolvedValueOnce(racingPlanDetail())
    expect(await renderRacing().activate({ position: 6, plan_key: 'size-five-periods' })).toBe(false)
    expect(renderRacing().selection).toEqual(DEFAULT_RACING_PLAN)
  })
  it('preserves single-flight refreshes and the confirmed selection during outages', async () => {
    renderRacing(); await vi.advanceTimersByTimeAsync(0)
    const next = { position: 4, plan_key: 'three-period-seven-codes' }
    await renderRacing().activate(next)
    let finish!: (detail: RacingPlanDetail) => void
    runtime.racingDetail.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    renderRacing(); await vi.advanceTimersByTimeAsync(0)
    for (let index = 0; index < 15; index += 1) notify()
    await vi.advanceTimersByTimeAsync(100)
    expect(runtime.racingDetail).toHaveBeenCalledTimes(2)
    finish(racingPlanDetail(next)); await vi.advanceTimersByTimeAsync(0)
    runtime.racingDetail.mockRejectedValueOnce(new Error('连接恢复中'))
    await vi.advanceTimersByTimeAsync(100)
    expect(renderRacing()).toMatchObject({ selection: next, error: '连接恢复中' })
    expect(renderRacing().data?.selection).toMatchObject(next)
    expect(runtime.activateRacing).toHaveBeenCalledTimes(3)
  })
})
