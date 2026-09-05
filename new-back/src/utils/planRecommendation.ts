import type { PlanRecommendation, PlanRecommendationPayload } from '../api'

export type PlanRecommendationDraft = Omit<PlanRecommendationPayload, 'numbers'> & {
  id?: number
  source?: PlanRecommendation['source']
  numbersText: string
}

export const RACING_MANUAL_PLAN_RULE = '赛车类彩种必须使用上方自动计划配置名次和方案，不能发布通用手工推荐。'
export const RACING_PLAN_GAME_IDS = [
  'speed-racing', 'speed-fly', 'sg-fly', 'fly-racing', 'au-lucky-10',
  'bingo-racing-a', 'bingo-racing-b',
] as const
export const isRacingPlanGame = (gameId: string) => (RACING_PLAN_GAME_IDS as readonly string[]).includes(gameId)

const MANUAL_PLAN_NUMBER_RANGES: Readonly<Record<string, readonly [number, number]>> = {
  'speed-ssc': [0, 9], 'sg-ssc': [0, 9], 'au-lucky-5': [0, 9],
  'bingo-ssc-1': [0, 9], 'bingo-ssc-2': [0, 9], 'bingo-ssc-3': [0, 9], 'bingo-ssc-4': [0, 9],
  'official-fc3d': [0, 9], 'official-pl3': [0, 9],
  'pc-canada': [0, 27], 'canada-28': [0, 27], 'canada-20': [0, 27],
  'bingo-mark-six': [1, 49], 'hong-kong-mark-six': [1, 49], 'happy8-mark-six': [1, 49],
  'new-macau-mark-six': [1, 49], 'old-macau-mark-six': [1, 49],
}

export const isSupportedManualPlanGame = (gameId: string) => Object.prototype.hasOwnProperty.call(MANUAL_PLAN_NUMBER_RANGES, gameId)

const parseNumbers = (text: string) => text.split(/[，,\s]+/).filter(Boolean).map(Number)

export function planRecommendationNumberRule(gameId: string) {
  if (isRacingPlanGame(gameId)) return RACING_MANUAL_PLAN_RULE
  const range = MANUAL_PLAN_NUMBER_RANGES[gameId]
  if (!range) return '该彩种尚未配置可验证的推荐号码规则。'
  return `需填写 1 至 12 个不重复的 ${range[0]}–${range[1]} 整数号码，使用逗号分隔。`
}

export function planRecommendationNumberError(gameId: string, numbersText: string) {
  if (isRacingPlanGame(gameId)) return RACING_MANUAL_PLAN_RULE
  const range = MANUAL_PLAN_NUMBER_RANGES[gameId]
  if (!range) return planRecommendationNumberRule(gameId)
  const numbers = parseNumbers(numbersText)
  return numbers.length >= 1 && numbers.length <= 12 && numbers.every(value => Number.isInteger(value) && value >= range[0] && value <= range[1]) && new Set(numbers).size === numbers.length
    ? '' : planRecommendationNumberRule(gameId)
}

export function buildPlanRecommendationPayload(draft: PlanRecommendationDraft, workspaceId: number): PlanRecommendationPayload {
  const error = planRecommendationNumberError(draft.game_id, draft.numbersText)
  if (error) throw new Error(error)
  return {
    workspace_id: workspaceId,
    game_id: draft.game_id, issue: draft.issue, master_name: draft.master_name,
    master_title: draft.master_title, master_color: draft.master_color,
    numbers: parseNumbers(draft.numbersText).filter(Number.isFinite),
    size: draft.size, parity: draft.parity,
    // Published outcomes are derived by the server from verified draws. The
    // administration form can only submit an ungraded forecast.
    result: 'pending', note: draft.note,
    enabled: draft.enabled, sort_order: draft.sort_order,
  }
}

export function planRecommendationSelection(item: Pick<PlanRecommendation, 'game_id' | 'numbers' | 'size' | 'parity'>) {
  const numbers = item.numbers.join('、')
  return [numbers, item.size, item.parity].filter(Boolean).join(' ')
}
