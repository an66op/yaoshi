import { describe, expect, it } from 'vitest'
import { racingPlanDetail, racingPlanRow } from '../test/racingPlanFixtures'
import { DEFAULT_RACING_PLAN, racingPlanAllowed, racingPlanCycleStatus, racingPlanDirection, racingPlanHistory, racingPlanIsCurrent, racingPlanMasters, racingPlanPositionLabel, racingPlanProgress } from './racingPlans'

describe('racing plan stream presentation', () => {
  it('defaults to champion / four-period-five-codes and labels all ten positions', () => {
    expect(DEFAULT_RACING_PLAN).toEqual({ position: 1, plan_key: 'four-period-five-codes' })
    expect(Array.from({ length: 10 }, (_, index) => racingPlanPositionLabel(index + 1))).toEqual(['冠军', '亚军', '第三名', '第四名', '第五名', '第六名', '第七名', '第八名', '第九名', '第十名'])
  })
  it('filters wrong position/type/game/source rows even if issue and expert name match', () => {
    const detail = racingPlanDetail()
    const wrongRows = [racingPlanRow({ id: 80, position: 2 }), racingPlanRow({ id: 81, plan_key: 'three-period-five-codes' }), racingPlanRow({ id: 82, game_id: 'speed-fly' }), racingPlanRow({ id: 83, source: 'manual' })]
    detail.recommendations.push(...wrongRows)
    detail.history.push(...wrongRows)
    expect(racingPlanMasters(detail, DEFAULT_RACING_PLAN)).toHaveLength(3)
    expect(racingPlanHistory(detail, DEFAULT_RACING_PLAN, detail.recommendations[0]).map(row => row.id)).toEqual([1])
    for (const wrong of wrongRows) expect(racingPlanIsCurrent(detail, DEFAULT_RACING_PLAN, wrong)).toBe(false)
  })
  it('rejects an entire response for a different selected stream', () => {
    const detail = racingPlanDetail({ position: 2, plan_key: 'three-period-six-codes' })
    expect(racingPlanMasters(detail, DEFAULT_RACING_PLAN)).toEqual([])
    expect(racingPlanHistory(detail, DEFAULT_RACING_PLAN, detail.recommendations[0])).toEqual([])
    expect(racingPlanIsCurrent(detail, DEFAULT_RACING_PLAN, detail.recommendations[0])).toBe(false)
  })
  it('keeps old rows historical even when their cycle remains active', () => {
    const detail = racingPlanDetail(DEFAULT_RACING_PLAN, { current_issue: '101', recommendations: [] })
    expect(racingPlanMasters(detail, DEFAULT_RACING_PLAN)).toHaveLength(3)
    expect(racingPlanIsCurrent(detail, DEFAULT_RACING_PLAN, detail.latest_recommendations[0])).toBe(false)
  })
  it('does not mix legacy recommendations into the new stream', () => {
    const detail = racingPlanDetail(DEFAULT_RACING_PLAN, { recommendations: [], latest_recommendations: [], history: [], legacy_history: [racingPlanRow({ numbers: [1, 5, 9] })] })
    expect(racingPlanMasters(detail, DEFAULT_RACING_PLAN)).toEqual([])
  })
  it('uses persisted cycle progress, never fabricated future periods', () => {
    expect(racingPlanProgress(racingPlanRow())).toBe('第 2 / 4 期')
    expect(racingPlanProgress(racingPlanRow({ cycle_period: 0 }))).toBe('周期进度待更新')
    expect(racingPlanProgress(racingPlanRow({ cycle_period: 5 }))).toBe('周期进度待更新')
    expect(racingPlanCycleStatus(racingPlanRow({ cycle_status: 'completed' }))).toBe('本轮已全部发布')
    expect(racingPlanCycleStatus(racingPlanRow({ cycle_status: 'active' }))).toBe('本轮发布中')
  })
  it.each([['size', '小'], ['parity', '双'], ['dragon_tiger', '虎']] as const)('uses only the %s stream direction', (kind, expected) => {
    expect(racingPlanDirection(racingPlanRow({ kind, size: '小', parity: '双', dragon_tiger: '虎' }))).toBe(expected)
  })
  it('requires both administrator allowlists to match', () => {
    const detail = racingPlanDetail(DEFAULT_RACING_PLAN, { allowed_positions: [1], allowed_plan_keys: ['four-period-five-codes'] })
    expect(racingPlanAllowed(detail, DEFAULT_RACING_PLAN)).toBe(true)
    expect(racingPlanAllowed(detail, { position: 2, plan_key: 'four-period-five-codes' })).toBe(false)
    expect(racingPlanAllowed(detail, { position: 1, plan_key: 'three-period-five-codes' })).toBe(false)
  })
})
