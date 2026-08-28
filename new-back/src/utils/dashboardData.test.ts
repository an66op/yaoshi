import { describe, expect, it } from 'vitest'
import { normalizeDashboardData } from './dashboardData'

describe('normalizeDashboardData', () => {
  it('replaces nullable response collections', () => {
    const result = normalizeDashboardData({ overview: null, stats: null, games: null } as never)
    expect(result).toEqual({ overview: {}, stats: {}, games: [] })
  })

  it('preserves valid dashboard values', () => {
    const game = { id: 'speed-racing' }
    const result = normalizeDashboardData({ overview: { member_count: 2 }, stats: { today_turnover: 8 }, games: [game] } as never)
    expect(result.overview.member_count).toBe(2)
    expect(result.stats.today_turnover).toBe(8)
    expect(result.games).toEqual([game])
  })
})
