import { describe, expect, it } from 'vitest'
import { GAME_TIMELINE_LIMIT, recentGameTimelineItems } from './gameTimelineBudget'

describe('game-room display cache budget', () => {
  it.each([0, 1, GAME_TIMELINE_LIMIT])('does not copy or trim a cache of %i entries', count => {
    const rows = Array.from({ length: count }, (_, index) => index)
    expect(recentGameTimelineItems(rows)).toBe(rows)
  })

  it('drops only the oldest display entry at the boundary without mutating the source', () => {
    const rows = Array.from({ length: GAME_TIMELINE_LIMIT + 1 }, (_, index) => index)
    expect(recentGameTimelineItems(rows)).toEqual(rows.slice(1))
    expect(rows).toHaveLength(GAME_TIMELINE_LIMIT + 1)
    expect(rows[0]).toBe(0)
  })

  it('retains only the newest window after a long catch-up', () => {
    const rows = Array.from({ length: GAME_TIMELINE_LIMIT * 10 }, (_, index) => index)
    const recent = recentGameTimelineItems(rows)
    expect(recent).toHaveLength(GAME_TIMELINE_LIMIT)
    expect(recent[0]).toBe(rows.length - GAME_TIMELINE_LIMIT)
    expect(recent.at(-1)).toBe(rows.length - 1)
  })
})
