import { describe, expect, it } from 'vitest'
import { formatLotteryCountdown, readServerClock, resolveLotteryTiming, sampleServerClock, type LotteryTimingInput } from './lotteryTiming'

const drawMs = Date.parse('2026-08-30T13:31:10+08:00')
const beforeDraw = (seconds: number) => drawMs - seconds * 1000
const base: LotteryTimingInput = {
  next_draw_at: new Date(drawMs).toISOString(),
  seal_at: new Date(beforeDraw(15)).toISOString(),
  accept_at: new Date(beforeDraw(90)).toISOString(),
  draw_interval: 90,
  seal_seconds: 15,
  issue_status: 'accepting',
  source_healthy: true,
  enabled: true,
}

describe('resolveLotteryTiming', () => {
  it('uses separate accepting and sealed countdowns at their exact boundaries', () => {
    const accepting = resolveLotteryTiming(base, beforeDraw(45))
    expect(accepting).toMatchObject({ phase: 'accepting', accepting: true, phaseLabel: '受理倒计时', due: '00:30', remainingSeconds: 30 })
    expect(resolveLotteryTiming(base, beforeDraw(15) - 1)).toMatchObject({ phase: 'accepting', due: '00:01' })
    expect(resolveLotteryTiming(base, beforeDraw(15))).toMatchObject({ phase: 'sealed', accepting: false, phaseLabel: '封盘倒计时', due: '00:15' })
    expect(resolveLotteryTiming(base, drawMs - 1)).toMatchObject({ phase: 'sealed', due: '00:01' })
    expect(resolveLotteryTiming(base, drawMs)).toMatchObject({ phase: 'awaiting_draw', accepting: false, phaseLabel: '开奖中', statusLabel: '开奖中', due: '00:00' })
  })

  it('uses 开奖中 for server draw-waiting state without changing settlement or acceptance', () => {
    expect(resolveLotteryTiming({ ...base, issue_status: 'awaiting_draw' }, drawMs)).toMatchObject({ phaseLabel: '开奖中', statusLabel: '开奖中', due: '00:00', accepting: false })
    expect(resolveLotteryTiming({ ...base, issue_status: 'settling' }, drawMs).phaseLabel).toBe('正在结算')
    expect(resolveLotteryTiming({ ...base, issue_status: 'settled' }, drawMs).phaseLabel).toBe('等待下一期')
  })

  it('never fabricates the next period when the advertised draw is overdue', () => {
    for (const secondsLate of [1, 90, 180, 86_400]) {
      expect(resolveLotteryTiming(base, drawMs + secondsLate * 1000)).toMatchObject({ phase: 'awaiting_draw', accepting: false, due: '00:00', drawAtMs: drawMs })
    }
  })

  it('uses the explicit seal boundary over a differing configuration value', () => {
    expect(resolveLotteryTiming({ ...base, seal_seconds: 30 }, beforeDraw(45))).toMatchObject({ due: '00:30', sealAtMs: beforeDraw(15), sealSeconds: 15 })
  })

  it.each([5, 12, 45, 60])('supports an effective %i-second seal without a game-specific constant', (sealSeconds) => {
    const timing = resolveLotteryTiming({ ...base, seal_at: null, seal_seconds: sealSeconds }, beforeDraw(75))
    expect(timing).toMatchObject({ phase: 'accepting', sealAtMs: beforeDraw(sealSeconds), remainingSeconds: 75 - sealSeconds, sealSeconds })
    expect(resolveLotteryTiming({ ...base, seal_at: null, seal_seconds: sealSeconds }, beforeDraw(sealSeconds))).toMatchObject({ phase: 'sealed', remainingSeconds: sealSeconds })
  })

  it('supports an explicitly configured zero-second seal without using a fallback', () => {
    const input = { ...base, seal_at: null, seal_seconds: 0 }
    expect(resolveLotteryTiming(input, beforeDraw(10))).toMatchObject({ accepting: true, due: '00:10', sealSeconds: 0 })
    expect(resolveLotteryTiming(input, drawMs)).toMatchObject({ phase: 'awaiting_draw', accepting: false, due: '00:00' })
  })

  it.each(['error', 'sealed', 'awaiting_draw', 'settling', 'settled', 'pending'])('respects non-accepting server state %s even before sealing', (issue_status) => {
    const timing = resolveLotteryTiming({ ...base, issue_status }, beforeDraw(45))
    expect(timing.accepting).toBe(false)
    expect(timing.phase).toBe(issue_status)
  })

  it('uses the draw deadline when the server seals a period early', () => {
    expect(resolveLotteryTiming({ ...base, issue_status: 'sealed' }, beforeDraw(45))).toMatchObject({ phase: 'sealed', phaseLabel: '封盘倒计时', due: '00:45' })
  })

  it('does not reopen source failures or unknown server states', () => {
    expect(resolveLotteryTiming({ ...base, source_healthy: false }, beforeDraw(45))).toMatchObject({ phase: 'error', accepting: false, statusLabel: '开奖源异常 · 已停盘' })
    expect(resolveLotteryTiming({ ...base, enabled: false }, beforeDraw(45))).toMatchObject({ phase: 'unavailable', accepting: false })
    for (const issue_status of [undefined, '', 'future-state', 'ACCEPTING']) {
      expect(resolveLotteryTiming({ ...base, issue_status }, beforeDraw(45))).toMatchObject({ phase: 'unavailable', accepting: false, due: '--:--' })
    }
  })

  it('does not trust an accepting state before its actual acceptance window', () => {
    expect(resolveLotteryTiming(base, beforeDraw(100))).toMatchObject({ phase: 'pending', accepting: false, due: '00:10' })
    expect(resolveLotteryTiming(base, beforeDraw(90))).toMatchObject({ phase: 'accepting', due: '01:15' })
    expect(resolveLotteryTiming({ ...base, issue_status: 'pending' }, beforeDraw(90))).toMatchObject({ phase: 'pending', accepting: false, due: '--:--' })
  })

  it('requires a synchronized, healthy server clock before allowing bets', () => {
    for (const now of [0, Number.NaN, Number.POSITIVE_INFINITY]) {
      expect(resolveLotteryTiming(base, now)).toMatchObject({ phase: 'unavailable', accepting: false, due: '--:--' })
    }
    expect(resolveLotteryTiming({ ...base, source_healthy: undefined }, beforeDraw(45))).toMatchObject({ accepting: false })
  })

  it('does not substitute an arbitrary 30-second window for missing settings', () => {
    expect(resolveLotteryTiming({ ...base, seal_at: null, seal_seconds: undefined }, beforeDraw(45))).toMatchObject({ phase: 'unavailable', accepting: false, due: '--:--', sealSeconds: null })
  })

  it.each([
    { next_draw_at: 'not-a-date' },
    { next_draw_at: '0001-01-01T00:00:00Z' },
    { next_draw_at: '2026/08/30 13:31:10' },
    { seal_at: 'broken', seal_seconds: 15 },
    { seal_at: new Date(drawMs + 1000).toISOString() },
    { seal_at: null, seal_seconds: -5 },
    { seal_at: null, seal_seconds: Number.NaN },
    { accept_at: 'invalid' },
    { accept_at: new Date(drawMs).toISOString() },
  ])('fails closed for invalid timing %j', (override) => {
    expect(resolveLotteryTiming({ ...base, ...override }, beforeDraw(45))).toMatchObject({ phase: 'unavailable', accepting: false, due: '--:--' })
  })

  it('preserves server timing settings without estimating a 75-second racing period', () => {
    expect(resolveLotteryTiming(base, beforeDraw(45)).intervalSeconds).toBe(90)
    expect(resolveLotteryTiming({ ...base, draw_interval: undefined }, beforeDraw(45)).intervalSeconds).toBeNull()
    expect(resolveLotteryTiming({ ...base, draw_interval: -10 }, beforeDraw(45)).intervalSeconds).toBeNull()
  })
})

describe('formatLotteryCountdown', () => {
  it('formats seconds, long periods, and missing time without NaN digits', () => {
    expect(formatLotteryCountdown(5)).toBe('00:05')
    expect(formatLotteryCountdown(75)).toBe('01:15')
    expect(formatLotteryCountdown(3661)).toBe('01:01:01')
    expect(formatLotteryCountdown(-1)).toBe('00:00')
    expect(formatLotteryCountdown(null)).toBe('--:--')
    expect(formatLotteryCountdown(Number.NaN)).toBe('--:--')
  })
})

describe('member server clock', () => {
  it('accounts for half the clock HTTP round trip rather than the catalog request duration', () => {
    const sample = sampleServerClock(drawMs, 1000, 1200)
    expect(sample).toEqual({ serverTimeMs: drawMs + 100, monotonicAtMs: 1200, roundTripMs: 200 })
    expect(readServerClock(sample, 5200)).toBe(drawMs + 4100)
  })

  it('advances from monotonic elapsed time without a dependency on the device wall clock', () => {
    const sample = sampleServerClock(drawMs, 8000, 8000)
    expect(readServerClock(sample, 10_000)).toBe(drawMs + 2000)
    expect(readServerClock(sample, 70_000)).toBe(drawMs + 62_000)
    expect(readServerClock(sample, 7999)).toBeNull()
  })

  it('rejects invalid clock samples', () => {
    expect(sampleServerClock(Number.NaN, 1, 2)).toBeNull()
    expect(sampleServerClock(0, 1, 2)).toBeNull()
    expect(sampleServerClock(drawMs, 2, 1)).toBeNull()
    expect(sampleServerClock(drawMs, 1, Number.POSITIVE_INFINITY)).toBeNull()
    expect(readServerClock(null, 2000)).toBeNull()
    expect(readServerClock(sampleServerClock(drawMs, 1, 2), Number.NaN)).toBeNull()
  })
})
