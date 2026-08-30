import type { RacingPlanDetail, RacingPlanKind, RacingPlanRecommendation, RacingPlanSelection } from '../api/plans'
import { recentPlanHistory } from './planPresentation'

export const DEFAULT_RACING_PLAN: RacingPlanSelection = { position: 1, plan_key: 'four-period-five-codes' }
export const RACING_PLAN_GROUPS: Array<{ kind: RacingPlanKind; label: string }> = [
  { kind: 'numbers', label: '号码计划' }, { kind: 'size', label: '大小计划' },
  { kind: 'parity', label: '单双计划' }, { kind: 'dragon_tiger', label: '龙虎计划' },
]

export const racingPlanKey = (selection: RacingPlanSelection) => `${selection.position}:${selection.plan_key}`
export const sameRacingPlan = (left: RacingPlanSelection, right: RacingPlanSelection) => left.position === right.position && left.plan_key === right.plan_key
export const racingPlanPositionLabel = (position: number) => position === 1 ? '冠军' : position === 2 ? '亚军' : `第${['', '', '', '三', '四', '五', '六', '七', '八', '九', '十'][position] ?? position}名`

export function racingPlanAllowed(detail: RacingPlanDetail, selection: RacingPlanSelection) {
  return detail.allowed_positions.includes(selection.position) && detail.allowed_plan_keys.includes(selection.plan_key)
}

/** The response and every row must match the requested stream. Legacy/manual
 * publications are deliberately separate, even when their expert name matches. */
export function isRacingStreamRow(row: RacingPlanRecommendation, selection: RacingPlanSelection) {
  return row.game_id === 'speed-racing' && row.source === 'demo' && sameRacingPlan(row, selection)
}

export function racingPlanMasters(detail: RacingPlanDetail | null, selection: RacingPlanSelection) {
  if (!detail || !sameRacingPlan(detail.selection, selection)) return []
  const rows = new Map<string, RacingPlanRecommendation>()
  for (const row of [...detail.latest_recommendations, ...detail.recommendations]) {
    if (isRacingStreamRow(row, selection) && row.kind === detail.selection.kind) rows.set(row.master_name, row)
  }
  return [...rows.values()].sort((a, b) => a.sort_order - b.sort_order || a.master_name.localeCompare(b.master_name))
}

export function racingPlanIsCurrent(detail: RacingPlanDetail | null, selection: RacingPlanSelection, row?: RacingPlanRecommendation) {
  return Boolean(row && detail && sameRacingPlan(detail.selection, selection) && isRacingStreamRow(row, selection)
    && detail.current_issue && row.issue === detail.current_issue && detail.recommendations.some(item => item.id === row.id && isRacingStreamRow(item, selection)))
}

export function racingPlanHistory(detail: RacingPlanDetail | null, selection: RacingPlanSelection, master?: RacingPlanRecommendation) {
  if (!detail || !master || !sameRacingPlan(detail.selection, selection)) return []
  return recentPlanHistory(detail.history.filter(row => isRacingStreamRow(row, selection) && row.kind === master.kind && row.master_name === master.master_name && row.source === master.source))
}

export function racingPlanProgress(row: RacingPlanRecommendation) {
  if (!Number.isInteger(row.cycle_period) || !Number.isInteger(row.cycle_periods) || row.cycle_period < 1 || row.cycle_period > row.cycle_periods) return '周期进度待更新'
  return `第 ${row.cycle_period} / ${row.cycle_periods} 期`
}

export const racingPlanCycleStatus = (row: RacingPlanRecommendation) => row.cycle_status === 'completed' ? '本轮已全部发布' : row.cycle_status === 'interrupted' ? '本轮已中断' : '本轮发布中'

export function racingPlanDirection(row: RacingPlanRecommendation) {
  return row.kind === 'size' ? row.size : row.kind === 'parity' ? row.parity : row.kind === 'dragon_tiger' ? row.dragon_tiger : ''
}
