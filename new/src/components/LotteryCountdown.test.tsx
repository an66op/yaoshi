import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { resolveLotteryTiming } from '../utils/lotteryTiming'
import { LotteryCountdown } from './LotteryCountdown'

const schedule = {
  source_healthy: true, issue_status: 'accepting', draw_interval: 75, seal_seconds: 30,
  accept_at: '2026-08-30T05:30:00Z', seal_at: '2026-08-30T05:30:45Z', next_draw_at: '2026-08-30T05:31:15Z',
}

describe('phase-labelled member countdown', () => {
  it('shows 45 seconds of accepting, then 30 seconds sealed, then waiting', () => {
    const accepting = renderToStaticMarkup(<LotteryCountdown timing={resolveLotteryTiming(schedule, Date.parse('2026-08-30T05:30:00Z'))} />)
    const sealed = renderToStaticMarkup(<LotteryCountdown timing={resolveLotteryTiming(schedule, Date.parse('2026-08-30T05:30:45Z'))} />)
    const waiting = renderToStaticMarkup(<LotteryCountdown timing={resolveLotteryTiming(schedule, Date.parse('2026-08-30T05:31:15Z'))} />)
    expect(accepting).toContain('aria-label="受理倒计时 00:45"')
    expect(sealed).toContain('aria-label="封盘倒计时 00:30"')
    expect(sealed).toContain('phase-sealed')
    expect(waiting).toContain('aria-label="开奖中 00:00"')
    expect(waiting).not.toContain('phase-accepting')
  })

  it('makes an unavailable schedule explicit rather than displaying a fake timer', () => {
    const html = renderToStaticMarkup(<LotteryCountdown timing={resolveLotteryTiming({ source_healthy: true, issue_status: 'accepting' }, Date.now())} />)
    expect(html).toContain('aria-label="时间待同步 --:--"')
  })

  it('omits the visible accepting caption in compact room headers', () => {
    const html = renderToStaticMarkup(<LotteryCountdown compact timing={resolveLotteryTiming(schedule, Date.parse('2026-08-30T05:30:00Z'))} />)
    expect(html).toContain('is-compact')
    expect(html).toContain('aria-label="受理倒计时 00:45"')
    expect(html).not.toContain('class="lottery-countdown-label"')
    expect(html.replace(/<[^>]*>/g, '')).toBe('00:45')
  })

  it('reserves an empty caption row in accepting lobby cards while retaining an accessible label', () => {
    const html = renderToStaticMarkup(<LotteryCountdown timing={resolveLotteryTiming(schedule, Date.parse('2026-08-30T05:30:00Z'))} />)
    expect(html).not.toContain('is-compact')
    expect(html).toContain('<span class="lottery-countdown-label" aria-hidden="true"></span><b class="lottery-countdown-digits"')
    expect(html.replace(/<[^>]*>/g, '')).toBe('00:45')
    expect(html).toContain('aria-label="受理倒计时 00:45"')
  })

  it.each([
    { phase: 'accepting', input: schedule, now: '2026-08-30T05:30:00Z', caption: '' },
    { phase: 'sealed', input: schedule, now: '2026-08-30T05:30:45Z', caption: '封盘倒计时' },
    { phase: 'awaiting_draw', input: schedule, now: '2026-08-30T05:31:15Z', caption: '开奖中' },
    { phase: 'settling', input: { ...schedule, issue_status: 'settling' }, now: '2026-08-30T05:31:15Z', caption: '正在结算' },
    { phase: 'settled', input: { ...schedule, issue_status: 'settled' }, now: '2026-08-30T05:31:15Z', caption: '等待下一期' },
    { phase: 'pending', input: schedule, now: '2026-08-30T05:29:50Z', caption: '距开始受理' },
    { phase: 'error', input: { ...schedule, source_healthy: false }, now: '2026-08-30T05:30:00Z', caption: '已停盘' },
    { phase: 'unavailable', input: { source_healthy: true, issue_status: 'accepting' }, now: '2026-08-30T05:30:00Z', caption: '时间待同步' },
  ])('keeps one caption slot before the digits for lobby phase $phase', ({ phase, input, now, caption }) => {
    const html = renderToStaticMarkup(<LotteryCountdown timing={resolveLotteryTiming(input, Date.parse(now))} />)
    expect(html).toContain(`phase-${phase}`)
    expect(html.match(/class="lottery-countdown-label"/g)).toHaveLength(1)
    expect(html).toMatch(new RegExp(`<span class="lottery-countdown-label"[^>]*>${caption}</span><b class="lottery-countdown-digits"`))
  })

  it('places 封盘中 after the seconds without an extra caption row', () => {
    const html = renderToStaticMarkup(<LotteryCountdown compact timing={resolveLotteryTiming(schedule, Date.parse('2026-08-30T05:30:45Z'))} />)
    expect(html).toContain('</b><span class="lottery-countdown-label">封盘中</span>')
    expect(html.replace(/<[^>]*>/g, '')).toBe('00:30封盘中')
  })

  it('keeps waiting and unavailable states visible beside compact timers', () => {
    const waiting = renderToStaticMarkup(<LotteryCountdown compact timing={resolveLotteryTiming(schedule, Date.parse('2026-08-30T05:31:15Z'))} />)
    const error = renderToStaticMarkup(<LotteryCountdown compact timing={resolveLotteryTiming({ ...schedule, source_healthy: false }, Date.parse('2026-08-30T05:30:00Z'))} />)
    expect(waiting.replace(/<[^>]*>/g, '')).toBe('00:00开奖中')
    expect(error.replace(/<[^>]*>/g, '')).toBe('--:--已停盘')
  })

  it('keeps the sealed caption visible in lobby cards', () => {
    const html = renderToStaticMarkup(<LotteryCountdown timing={resolveLotteryTiming(schedule, Date.parse('2026-08-30T05:30:45Z'))} />)
    expect(html).not.toContain('is-compact')
    expect(html.replace(/<[^>]*>/g, '')).toBe('封盘倒计时00:30')
  })

  it('switches from 封盘中 to 开奖中 at zero and keeps it beside the seconds', () => {
    const drawAt = Date.parse(schedule.next_draw_at)
    const before = renderToStaticMarkup(<LotteryCountdown compact timing={resolveLotteryTiming(schedule, drawAt - 1000)} />)
    const atZero = renderToStaticMarkup(<LotteryCountdown compact timing={resolveLotteryTiming(schedule, drawAt)} />)
    expect(before.replace(/<[^>]*>/g, '')).toBe('00:01封盘中')
    expect(atZero.replace(/<[^>]*>/g, '')).toBe('00:00开奖中')
    expect(atZero).toContain('</b><span class="lottery-countdown-label">开奖中</span>')
    expect(atZero).toContain('title="开奖中"')
  })
})
