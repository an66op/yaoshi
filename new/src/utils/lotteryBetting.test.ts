import { describe, expect, it } from 'vitest'
import { resolveLotteryBetting, resolveLotteryTiming, type LotteryBettingWindow, type LotteryTimingInput } from './lotteryTiming'

const drawAt = Date.parse('2026-08-30T13:31:10+08:00')
const at = (offset: number) => new Date(drawAt + offset * 1000).toISOString()
const next: LotteryBettingWindow = {
  issue: '34137174', issue_status: 'accepting', accept_at: at(0),
  seal_at: at(45), next_draw_at: at(75), draw_interval: 75, seal_seconds: 30,
}
const source = {
  current_issue: '34137173', issue_status: 'awaiting_draw', enabled: true, source_healthy: true,
  accept_at: at(-75), seal_at: at(-30), next_draw_at: at(0), draw_interval: 75, seal_seconds: 30,
  betting_window: next,
}

describe('server-issued next-issue betting target', () => {
  it('leaves the drawing issue unchanged but accepts the explicit next target', () => {
    expect(resolveLotteryTiming(source, drawAt + 1000)).toMatchObject({ phase: 'awaiting_draw', accepting: false, due: '00:00' })
    expect(resolveLotteryBetting(source, drawAt + 1000)).toMatchObject({
      issue: '34137174', timing: { phase: 'accepting', accepting: true, due: '00:44' },
    })
  })

  it('uses exact next-window seal/draw boundaries without rolling it forward again', () => {
    expect(resolveLotteryBetting(source, drawAt + 44_999)?.timing.accepting).toBe(true)
    expect(resolveLotteryBetting(source, drawAt + 45_000)?.timing).toMatchObject({ phase: 'sealed', accepting: false, due: '00:30' })
    expect(resolveLotteryBetting(source, drawAt + 75_000)?.timing).toMatchObject({ phase: 'awaiting_draw', accepting: false })
    expect(resolveLotteryBetting(source, drawAt + 300_000)?.issue).toBe('34137174')
  })

  it('does not use a next window during current acceptance/sealing or before the current draw instant', () => {
    expect(resolveLotteryBetting(source, drawAt - 1)).toBeUndefined()
    expect(resolveLotteryBetting({ ...source, issue_status: 'accepting' }, drawAt - 40_000)).toBeUndefined()
    expect(resolveLotteryBetting({ ...source, issue_status: 'sealed' }, drawAt - 15_000)).toBeUndefined()
  })

  it.each<Partial<LotteryTimingInput>>([
    { enabled: false }, { source_healthy: false }, { source_healthy: undefined },
    { issue_status: 'error' }, { issue_status: 'settled' }, { issue_status: 'settling' },
    { next_draw_at: '0001-01-01T00:00:00Z' },
  ])('does not override a disabled, unhealthy, or incompatible parent snapshot: %j', changes => {
    expect(resolveLotteryBetting({ ...source, ...changes }, drawAt + 1000)).toBeUndefined()
  })

  it.each([undefined, null])('does not invent a betting period without a window: %j', betting_window => {
    expect(resolveLotteryBetting({ ...source, betting_window }, drawAt + 1000)).toBeUndefined()
  })

  it.each<Partial<LotteryBettingWindow>>([
    { issue: '' }, { issue: '34137173' }, { issue: ' 34137174' },
    { next_draw_at: at(0) }, { accept_at: at(-1) },
  ])('rejects same-issue and overlapping window targets: %j', changes => {
    expect(resolveLotteryBetting({ ...source, betting_window: { ...next, ...changes } }, drawAt + 1000)).toBeUndefined()
  })

  it.each<Partial<LotteryBettingWindow>>([
    { issue_status: 'sealed' }, { issue_status: 'pending' }, { issue_status: 'error' },
    { issue_status: 'awaiting_draw' }, { issue_status: 'unexpected' },
    { seal_at: 'bad' }, { seal_at: at(76) }, { accept_at: at(60) },
  ])('cannot accept an explicitly non-open or malformed next window: %j', changes => {
    expect(resolveLotteryBetting({ ...source, betting_window: { ...next, ...changes } }, drawAt + 1000)?.timing.accepting ?? false).toBe(false)
  })

  it('waits for a valid server clock sample', () => {
    for (const now of [0, NaN, Infinity]) expect(resolveLotteryBetting(source, now)).toBeUndefined()
  })

  it('does not override an explicitly unhealthy next window', () => {
    expect(resolveLotteryBetting({ ...source, betting_window: { ...next, source_healthy: false } }, drawAt + 1000)).toBeUndefined()
  })
})
