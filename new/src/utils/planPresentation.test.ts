import { describe, expect, it } from 'vitest'
import type { PlanDetail, PlanRecommendation } from '../api/plans'
import { displayedPlanMasters, planIsCurrent, planResultLabel, recentPlanHistory } from './planPresentation'

const row: PlanRecommendation = {
  id: 1, workspace_id: 2, game_id: 'speed-racing', issue: '100', master_name: '1号专家',
  master_title: '系统自动推荐', master_color: '#2aa9b3', numbers: [1, 3, 5, 9, 10], size: '', parity: '',
  result: 'pending', source: 'demo', note: '系统自动生成', enabled: true, sort_order: 10, master_hit_rate: null,
  created_at: '2026-08-30T08:00:00Z', updated_at: '2026-08-30T08:00:00Z',
}
const detail: PlanDetail = { game_id: row.game_id, current_issue: '101', recommendations: [], latest_recommendations: [row], history: [row] }

describe('honest current and historical plan presentation', () => {
  it('shows only six actual periods by default, with a hard maximum of ten', () => {
    const rows = Array.from({ length: 30 }, (_, index) => ({ ...row, id: index, issue: String(100 - Math.floor(index / 3)) }))
    expect(recentPlanHistory(rows)).toHaveLength(18)
    expect(new Set(recentPlanHistory(rows).map(item => item.issue)).size).toBe(6)
    expect(recentPlanHistory(rows, 999)).toHaveLength(30)
    expect(recentPlanHistory(rows, 1)).toHaveLength(3)
    expect(recentPlanHistory(rows, Number.NaN)).toHaveLength(18)
    expect(recentPlanHistory([row])).toEqual([row])
    expect(recentPlanHistory([])).toEqual([])
    expect(recentPlanHistory([{ ...row, issue: '' }])).toEqual([])
  })
  it('keeps masters visible after their issue closes without presenting them as current', () => {
    expect(displayedPlanMasters(detail)).toEqual([row])
    expect(planIsCurrent(detail, row)).toBe(false)
    expect(planIsCurrent({ ...detail, current_issue: '' }, row)).toBe(false)
  })
  it('prefers the new current publication while preserving other master tabs', () => {
    const other = { ...row, id: 2, master_name: '2号专家', sort_order: 20 }
    const current = { ...row, id: 3, issue: '101' }
    const updated = { ...detail, recommendations: [current], latest_recommendations: [other, row] }
    expect(displayedPlanMasters(updated)).toEqual([current, other])
    expect(planIsCurrent(updated, current)).toBe(true)
    expect(planIsCurrent(updated, other)).toBe(false)
  })
  it('never displays a claimed win or loss for a demo row', () => {
    expect(planResultLabel({ ...row, result: 'hit' })).toBe('未统计')
    expect(planResultLabel({ ...row, result: 'miss' })).toBe('未统计')
    expect(planResultLabel({ ...row, source: 'manual', result: 'hit' })).toBe('中')
    expect(planResultLabel({ ...row, source: 'manual', result: 'pending' })).toBe('待开奖')
  })
})
