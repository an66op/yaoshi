import { describe, expect, it } from 'vitest'
import {
  describeGameSchedule,
  describeIssueState,
  formatBeijingDateTime,
  formatDurationSeconds,
  formatFeedCountdown,
} from './drawTiming'

describe('Beijing draw timestamps', () => {
  it('includes the year and exact seconds for display and CSV', () => {
    expect(formatBeijingDateTime('2026-08-30T05:31:10Z')).toBe('2026/08/30 13:31:10')
    expect(formatBeijingDateTime('2020-10-31T23:50:40+08:00')).toBe('2020/10/31 23:50:40')
  })

  it('uses Asia/Shanghai across a UTC year boundary and midnight', () => {
    expect(formatBeijingDateTime('2026-12-31T16:00:00Z')).toBe('2027/01/01 00:00:00')
    expect(formatBeijingDateTime('2026-08-30T05:31:10-07:00')).toBe('2026/08/30 20:31:10')
  })

  it('also formats the numeric server clock without local timezone dependence', () => {
    expect(formatBeijingDateTime(Date.parse('2026-08-30T05:31:10Z'))).toBe('2026/08/30 13:31:10')
    expect(formatBeijingDateTime(0)).toBe('1970/01/01 08:00:00')
  })

  it('does not crash or print bogus times for missing or invalid backend values', () => {
    for (const value of [undefined, null, '', 'not-a-date', '0001-01-01T00:00:00Z', Number.NaN, Infinity, 1e20]) {
      expect(formatBeijingDateTime(value)).toBe('—')
    }
    expect(formatBeijingDateTime(undefined, '等待时间')).toBe('等待时间')
  })
})

describe('collector countdown', () => {
  const now = Date.parse('2026-08-30T05:31:10Z')

  it('rounds up subsecond time and never gives a negative wait', () => {
    expect(formatFeedCountdown(now + 19_001, now)).toBe('20 秒后检查')
    expect(formatFeedCountdown(now, now)).toBe('0 秒后检查')
    expect(formatFeedCountdown(now - 30_000, now)).toBe('0 秒后检查')
  })

  it('waits for both a valid scheduled attempt and a calibrated clock', () => {
    expect(formatFeedCountdown('not-a-date', now)).toBe('等待调度')
    expect(formatFeedCountdown(undefined, now)).toBe('等待调度')
    expect(formatFeedCountdown(now, 0)).toBe('等待调度')
    expect(formatFeedCountdown(now, Number.NaN)).toBe('等待调度')
  })
})

describe('per-game cadence and actual closing settings', () => {
  it('formats non-round periods and long intervals without calling them polling times', () => {
    expect(formatDurationSeconds(75)).toBe('1分15秒')
    expect(formatDurationSeconds(180)).toBe('3分00秒')
    expect(formatDurationSeconds(3601)).toBe('1小时00分01秒')
    expect(formatDurationSeconds(259200)).toBe('3天')
    expect(formatDurationSeconds(0)).toBe('0秒')
  })

  it('displays values received for each game instead of hardcoded racing or seal defaults', () => {
    expect(describeGameSchedule({ schedule_mode: 'external-feed', draw_interval: 75, seal_seconds: 10, timing_source: 'upstream' })).toEqual({
      interval: '每期 1分15秒', seal: '提前 10秒 封盘', source: '源站时序',
    })
    expect(describeGameSchedule({ schedule_mode: 'interval', draw_interval: 180, seal_seconds: 45, timing_source: 'configured' })).toEqual({
      interval: '每期 3分00秒', seal: '提前 45秒 封盘', source: '配置周期',
    })
  })

  it('keeps an explicit zero-second seal and does not silently assume 30 seconds', () => {
    expect(describeGameSchedule({ schedule_mode: 'interval', draw_interval: 60, seal_seconds: 0 }).seal).toBe('开奖时封盘')
    expect(describeGameSchedule(undefined)).toEqual({ interval: '开奖周期未配置', seal: '封盘提前量未配置', source: '' })
  })

  it('handles missing, fractional or invalid intervals as unknown', () => {
    for (const value of [undefined, -1, 1.5, Number.NaN, Infinity]) {
      expect(formatDurationSeconds(value)).toBeNull()
      expect(describeGameSchedule({ schedule_mode: 'interval', draw_interval: value, seal_seconds: value }).interval).toBe('开奖周期未配置')
      expect(describeGameSchedule({ schedule_mode: 'interval', draw_interval: value, seal_seconds: value }).seal).toBe('封盘提前量未配置')
    }
    expect(describeGameSchedule({ schedule_mode: 'interval', draw_interval: 0 }).interval).toBe('开奖周期未配置')
  })

  it('does not present an approximate official-calendar seed as an exact period', () => {
    expect(describeGameSchedule({ schedule_mode: 'official-feed', draw_interval: 259200, seal_seconds: 30, timing_source: 'configured' }).interval).toBe('按官方日程开奖')
    expect(describeGameSchedule({ schedule_mode: 'official-feed', draw_interval: 259200, seal_seconds: 30, timing_source: 'upstream' }).interval).toBe('按官方日程开奖')
    expect(describeGameSchedule({ schedule_mode: 'official-feed', draw_interval: 300, seal_seconds: 30, timing_source: 'observed' }).interval).toBe('每期 5分00秒')
    expect(describeGameSchedule({ schedule_mode: 'official-feed', draw_interval: 300, seal_seconds: 30, timing_source: 'upstream' }).interval).toBe('每期 5分00秒')
  })
})

describe('current-period status', () => {
  const game = {
    issue_status: 'accepting',
    accept_at: '2026-08-30T05:30:00Z',
    seal_at: '2026-08-30T05:31:00Z',
    next_draw_at: '2026-08-30T05:31:15Z',
  }

  it('separates the acceptance, sealed and awaiting-result phases at exact boundaries', () => {
    expect(describeIssueState(game, Date.parse('2026-08-30T05:29:59Z'))).toBe('未开始受理')
    expect(describeIssueState(game, Date.parse('2026-08-30T05:30:00Z'))).toBe('受理中')
    expect(describeIssueState(game, Date.parse('2026-08-30T05:31:00Z'))).toBe('封盘中')
    expect(describeIssueState(game, Date.parse('2026-08-30T05:31:15Z'))).toBe('等待开奖')
  })

  it('never lets a wall clock reopen an error or settled lifecycle', () => {
    const now = Date.parse('2026-08-30T05:30:20Z')
    expect(describeIssueState({ ...game, issue_status: 'error' }, now)).toBe('开奖异常')
    expect(describeIssueState({ ...game, issue_status: 'settling' }, now)).toBe('结算中')
    expect(describeIssueState({ ...game, issue_status: 'settled' }, now)).toBe('已结算')
  })

  it('uses the server lifecycle until time is calibrated and tolerates absent timing', () => {
    expect(describeIssueState({ ...game, issue_status: 'sealed' }, 0)).toBe('封盘中')
    expect(describeIssueState({ ...game, next_draw_at: 'invalid', seal_at: undefined, accept_at: undefined }, Number.NaN)).toBe('受理中')
    expect(describeIssueState(undefined, 0)).toBe('等待期号')
  })
})
