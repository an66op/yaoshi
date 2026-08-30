import type { PlanRecommendation, PlanRecommendationPayload } from '../api'

export type PlanRecommendationDraft = Omit<PlanRecommendationPayload, 'numbers'> & {
  id?: number
  source?: PlanRecommendation['source']
  numbersText: string
}

export const SPEED_RACING_PLAN_RULE = '极速赛车需填写 5 个不重复的 1–10 整数号码，使用逗号分隔。'
export const isSpeedRacingPlan = (gameId: string) => gameId === 'speed-racing'

const parseNumbers = (text: string) => text.split(/[，,\s]+/).filter(Boolean).map(Number)

export function planRecommendationNumberError(gameId: string, numbersText: string) {
  if (!isSpeedRacingPlan(gameId)) return ''
  const numbers = parseNumbers(numbersText)
  return numbers.length === 5 && numbers.every(value => Number.isInteger(value) && value >= 1 && value <= 10) && new Set(numbers).size === 5
    ? '' : SPEED_RACING_PLAN_RULE
}

export function buildPlanRecommendationPayload(draft: PlanRecommendationDraft, workspaceId: number): PlanRecommendationPayload {
  const error = planRecommendationNumberError(draft.game_id, draft.numbersText)
  if (error) throw new Error(error)
  const racing = isSpeedRacingPlan(draft.game_id)
  return {
    workspace_id: workspaceId,
    game_id: draft.game_id, issue: draft.issue, master_name: draft.master_name,
    master_title: draft.master_title, master_color: draft.master_color,
    numbers: parseNumbers(draft.numbersText).filter(Number.isFinite),
    size: racing ? '' : draft.size, parity: racing ? '' : draft.parity,
    result: draft.source === 'demo' ? 'pending' : draft.result, note: draft.note,
    enabled: draft.enabled, sort_order: draft.sort_order,
  }
}

export function planRecommendationSelection(item: Pick<PlanRecommendation, 'game_id' | 'numbers' | 'size' | 'parity'>) {
  const numbers = item.numbers.join('、')
  return isSpeedRacingPlan(item.game_id) ? numbers : [numbers, item.size, item.parity].filter(Boolean).join(' ')
}
