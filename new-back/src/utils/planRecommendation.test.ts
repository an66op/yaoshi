import { describe, expect, it } from 'vitest'
import { buildPlanRecommendationPayload, isRacingPlanGame, isSupportedManualPlanGame, planRecommendationNumberError, planRecommendationNumberRule, planRecommendationSelection, RACING_MANUAL_PLAN_RULE, RACING_PLAN_GAME_IDS, type PlanRecommendationDraft } from './planRecommendation'

const draft = (patch: Partial<PlanRecommendationDraft> = {}): PlanRecommendationDraft => ({
  workspace_id: 12, game_id: 'speed-racing', issue: 'issue-1', master_name: '1号专家', master_title: '系统自动推荐', master_color: '#2aa9b3',
  numbersText: '1,3,5,7,10', size: '大', parity: '单', result: 'pending', note: '', enabled: true, sort_order: 10, ...patch,
})

describe('racing recommendations use only the rich automatic matrix', () => {
  it('rejects generic manual publication for all seven racing-v2 products', () => {
    expect(RACING_PLAN_GAME_IDS).toEqual(['speed-racing', 'speed-fly', 'sg-fly', 'fly-racing', 'au-lucky-10', 'bingo-racing-a', 'bingo-racing-b'])
    for (const gameId of RACING_PLAN_GAME_IDS) {
      expect(isRacingPlanGame(gameId)).toBe(true)
      expect(planRecommendationNumberError(gameId, '1,3,5,7,10')).toBe(RACING_MANUAL_PLAN_RULE)
      expect(() => buildPlanRecommendationPayload(draft({ game_id: gameId }), 37)).toThrow(RACING_MANUAL_PLAN_RULE)
    }
  })

  it('always sends an ungraded publication and does not send source metadata', () => {
	const payload = buildPlanRecommendationPayload(draft({ game_id: 'canada-28', source: 'demo', result: 'hit' }), 37)
	expect(payload.result).toBe('pending')
    expect(payload).not.toHaveProperty('source')
    expect(payload).not.toHaveProperty('master_hit_rate')
  })

  it('keeps legacy racing data visible in the administration archive', () => {
    expect(planRecommendationSelection({ game_id: 'speed-racing', numbers: [1, 4, 7], size: '大', parity: '单' })).toBe('1、4、7 大 单')
  })
})

describe('other games keep their existing recommendation rules', () => {
  it.each([
    ['canada-28', '0,12,27', '28'],
    ['speed-ssc', '0,5,9', '10'],
    ['hong-kong-mark-six', '1,25,49', '0'],
  ])('keeps the exact verified number range for %s', (gameId, valid, invalid) => {
    expect(isSupportedManualPlanGame(gameId)).toBe(true)
    expect(planRecommendationNumberError(gameId, valid)).toBe('')
    expect(planRecommendationNumberError(gameId, invalid)).toBe(planRecommendationNumberRule(gameId))
    const expected = valid.split(',').map(Number)
    expect(buildPlanRecommendationPayload(draft({ game_id: gameId, numbersText: valid, result: 'hit' }), 37)).toMatchObject({ numbers: expected, size: '大', parity: '单', result: 'pending' })
    expect(planRecommendationSelection({ game_id: gameId, numbers: expected, size: '大', parity: '单' })).toBe(`${expected.join('、')} 大 单`)
  })

  it('rejects unsupported games, duplicates, non-integers and oversized lists', () => {
    expect(isSupportedManualPlanGame('unknown')).toBe(false)
    expect(planRecommendationNumberError('unknown', '1,2,3')).toContain('尚未配置')
    expect(planRecommendationNumberError('speed-ssc', '1,1,2')).not.toBe('')
    expect(planRecommendationNumberError('speed-ssc', '1,2.5')).not.toBe('')
    expect(planRecommendationNumberError('speed-ssc', '0,1,2,3,4,5,6,7,8,9,0,1,2')).not.toBe('')
  })
})
