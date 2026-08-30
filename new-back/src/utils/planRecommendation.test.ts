import { describe, expect, it } from 'vitest'
import { buildPlanRecommendationPayload, planRecommendationNumberError, planRecommendationSelection, SPEED_RACING_PLAN_RULE, type PlanRecommendationDraft } from './planRecommendation'

const draft = (patch: Partial<PlanRecommendationDraft> = {}): PlanRecommendationDraft => ({
  workspace_id: 12, game_id: 'speed-racing', issue: 'issue-1', master_name: '1号专家', master_title: '系统自动推荐', master_color: '#2aa9b3',
  numbersText: '1,3,5,7,10', size: '大', parity: '单', result: 'pending', note: '', enabled: true, sort_order: 10, ...patch,
})

describe('speed racing five-number recommendation', () => {
  it.each(['1,3,5,7,10', '10，8，6，4，2', '01 02 03 04 05'])('accepts five unique integers from 1 through 10: %s', value => {
    expect(planRecommendationNumberError('speed-racing', value)).toBe('')
  })

  it.each(['', '1,2,3', '1,2,3,4', '1,2,3,4,5,6', '1,2,3,4,4', '0,1,2,3,4', '1,2,3,4,11', '1,2,3,4,5.5', '1,2,3,4,no', '1,2,3,4,5,no'])('rejects invalid racing selections without dropping or generating numbers: %s', value => {
    expect(planRecommendationNumberError('speed-racing', value)).toBe(SPEED_RACING_PLAN_RULE)
    expect(() => buildPlanRecommendationPayload(draft({ numbersText: value }), 37)).toThrow(SPEED_RACING_PLAN_RULE)
  })

  it('clears legacy size and parity in the saved racing payload without mutating the draft', () => {
    const input = draft()
    const payload = buildPlanRecommendationPayload(input, 37)
    expect(payload).toMatchObject({ workspace_id: 37, numbers: [1, 3, 5, 7, 10], size: '', parity: '' })
    expect(input).toMatchObject({ workspace_id: 12, size: '大', parity: '单' })
  })

  it('keeps the internal automatic-source result guard and does not send source metadata', () => {
    const payload = buildPlanRecommendationPayload(draft({ source: 'demo', result: 'hit' }), 37)
    expect(payload.result).toBe('pending')
    expect(payload).not.toHaveProperty('source')
    expect(payload).not.toHaveProperty('master_hit_rate')
  })

  it('hides legacy racing size/parity without rewriting historical numbers', () => {
    expect(planRecommendationSelection({ game_id: 'speed-racing', numbers: [1, 4, 7], size: '大', parity: '单' })).toBe('1、4、7')
  })
})

describe('other games keep their existing recommendation rules', () => {
  it.each(['speed-fly', 'canada-28', 'lucky-racing'])('does not apply the speed-racing-only five-number restriction to %s', gameId => {
    const input = draft({ game_id: gameId, numbersText: '0,12,27', result: 'hit' })
    expect(planRecommendationNumberError(gameId, input.numbersText)).toBe('')
    expect(buildPlanRecommendationPayload(input, 37)).toMatchObject({ numbers: [0, 12, 27], size: '大', parity: '单', result: 'hit' })
    expect(planRecommendationSelection({ game_id: gameId, numbers: [0, 12, 27], size: '大', parity: '单' })).toBe('0、12、27 大 单')
  })
})
