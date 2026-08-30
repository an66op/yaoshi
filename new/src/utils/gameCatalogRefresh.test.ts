import { describe, expect, it } from 'vitest'
import type { LotteryGame } from '../api/lottery'
import { gameCatalogRefreshDelay } from './gameCatalogRefresh'
import { resolveLotteryTiming } from './lotteryTiming'

const drawAt = Date.parse('2026-08-30T14:48:03+08:00')
const game: LotteryGame = {
  id: 'speed-racing', code: 'speed-racing', name: '赛车', category: 'racing', lobby_category: '彩票', lobby_sort_order: 0,
  badge: '', badge_color: '', enabled: true, issue: '34136854', current_issue: '34136855', latest_numbers: [],
  source_kind: 'external', source_name: '', sync_status: 'ok', source_healthy: true, issue_status: 'accepting',
  draw_interval: 75, seal_seconds: 30,
  accept_at: new Date(drawAt - 75_000).toISOString(), seal_at: new Date(drawAt - 30_000).toISOString(), next_draw_at: new Date(drawAt).toISOString(),
}

describe('server-authoritative game rollover refresh', () => {
  it('keeps a recovery poll even while websocket is connected', () => {
    expect(gameCatalogRefreshDelay([game], drawAt - 70_000, true, game.id)).toBe(30_000)
    expect(gameCatalogRefreshDelay([game], drawAt - 70_000, false, game.id)).toBe(10_000)
  })

  it('refreshes at sealing and drawing boundaries, not at the next unrelated event', () => {
    expect(gameCatalogRefreshDelay([game], drawAt - 35_000, true, game.id)).toBe(5150)
    expect(gameCatalogRefreshDelay([game], drawAt - 5000, true, game.id)).toBe(5150)
    expect(gameCatalogRefreshDelay([game], drawAt + 100, true, game.id)).toBe(2000)
  })

  it.each(['awaiting_draw', 'settling', 'settled'])('rechecks %s without locally opening the next period', issue_status => {
    const stale = { ...game, issue_status }
    expect(gameCatalogRefreshDelay([stale], drawAt + 14_000, true, game.id)).toBe(2000)
    expect(resolveLotteryTiming(stale, drawAt + 14_000).accepting).toBe(false)
    expect(stale.current_issue).toBe('34136855')
  })

  it('slows down a long outage instead of polling at 2 seconds indefinitely', () => {
    expect(gameCatalogRefreshDelay([{ ...game, issue_status: 'settled' }], drawAt + 600_000, true, game.id)).toBe(10_000)
    expect(gameCatalogRefreshDelay([{ ...game, source_healthy: false }], drawAt + 100, true, game.id)).toBe(10_000)
  })

  it('does not let another game waiting for a draw accelerate an active room', () => {
    const other = { ...game, id: 'other', issue_status: 'settled', next_draw_at: new Date(drawAt - 70_000).toISOString() }
    expect(gameCatalogRefreshDelay([game, other], drawAt - 70_000, true, game.id)).toBe(30_000)
    expect(gameCatalogRefreshDelay([game, other], drawAt - 70_000, true, null)).toBe(2000)
  })

  it('preserves pending state while requesting an updated server decision', () => {
    const pending = { ...game, issue_status: 'pending' }
    expect(gameCatalogRefreshDelay([pending], drawAt - 80_000, true, game.id)).toBe(5150)
    expect(gameCatalogRefreshDelay([pending], drawAt - 70_000, true, game.id)).toBe(2000)
    expect(resolveLotteryTiming(pending, drawAt - 70_000).accepting).toBe(false)
  })

  it('does not fast-poll indefinitely when a pending source omits the acceptance time', () => {
    const pending = { ...game, issue_status: 'pending', accept_at: null }
    expect(gameCatalogRefreshDelay([pending], drawAt - 70_000, true, game.id)).toBe(10_000)
    expect(gameCatalogRefreshDelay([pending], drawAt - 70_000, true, null)).toBe(30_000)
  })

  it('backs off network failures independently from the displayed countdown', () => {
    expect(gameCatalogRefreshDelay([game], drawAt, true, game.id, 1)).toBe(2000)
    expect(gameCatalogRefreshDelay([game], drawAt, true, game.id, 3)).toBe(8000)
    expect(gameCatalogRefreshDelay([game], drawAt, true, game.id, 20)).toBe(30_000)
    expect(gameCatalogRefreshDelay(undefined, null, true, game.id)).toBe(5000)
  })
})
