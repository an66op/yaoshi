import type { RacingPlanDetail, RacingPlanOption, RacingPlanPosition, RacingPlanRecommendation, RacingPlanSelection } from '../api/plans'
import { DEFAULT_RACING_PLAN, racingPlanPositionLabel } from '../utils/racingPlans'

export const racingPlanOptions: RacingPlanOption[] = [
  { key: 'four-period-five-codes', label: '四期五码', kind: 'numbers', periods: 4, number_count: 5 },
  { key: 'three-period-five-codes', label: '三期五码', kind: 'numbers', periods: 3, number_count: 5 },
  { key: 'four-period-six-codes', label: '四期六码', kind: 'numbers', periods: 4, number_count: 6 },
  { key: 'three-period-six-codes', label: '三期六码', kind: 'numbers', periods: 3, number_count: 6 },
  { key: 'four-period-seven-codes', label: '四期七码', kind: 'numbers', periods: 4, number_count: 7 },
  { key: 'three-period-seven-codes', label: '三期七码', kind: 'numbers', periods: 3, number_count: 7 },
  { key: 'two-period-eight-codes', label: '二期八码', kind: 'numbers', periods: 2, number_count: 8 },
  { key: 'one-period-eight-codes', label: '一期八码', kind: 'numbers', periods: 1, number_count: 8 },
  ...[5, 4, 3].map(periods => ({ key: `size-${periods === 5 ? 'five' : periods === 4 ? 'four' : 'three'}-periods`, label: `大小${periods === 5 ? '五' : periods === 4 ? '四' : '三'}期`, kind: 'size' as const, periods, number_count: 0 })),
  ...[5, 4, 3].map(periods => ({ key: `parity-${periods === 5 ? 'five' : periods === 4 ? 'four' : 'three'}-periods`, label: `单双${periods === 5 ? '五' : periods === 4 ? '四' : '三'}期`, kind: 'parity' as const, periods, number_count: 0 })),
  ...[5, 4, 3].map(periods => ({ key: `dragon-tiger-${periods === 5 ? 'five' : periods === 4 ? 'four' : 'three'}-periods`, label: `龙虎${periods === 5 ? '五' : periods === 4 ? '四' : '三'}期`, kind: 'dragon_tiger' as const, periods, number_count: 0 })),
]
export const racingPlanPositions: RacingPlanPosition[] = Array.from({ length: 10 }, (_, index) => ({ position: index + 1, label: racingPlanPositionLabel(index + 1), opponent_position: 10 - index }))

export function racingPlanRow(patch: Partial<RacingPlanRecommendation> = {}): RacingPlanRecommendation {
  return {
    id: 1, workspace_id: 2, game_id: 'speed-racing', issue: '100', master_name: '1号专家', master_title: '系统自动推荐', master_color: '#2aa9b3',
    numbers: [1, 3, 5, 9, 10], size: '', parity: '', dragon_tiger: '', result: 'pending', source: 'demo', note: '系统自动生成，仅供娱乐参考，不保证命中。', enabled: true, sort_order: 10,
	master_hit_rate: null, master_sample_count: 0, created_at: '2026-08-30T08:00:00Z', updated_at: '2026-08-30T08:00:00Z', position: 1, plan_key: 'four-period-five-codes', kind: 'numbers',
    cycle_id: 1, cycle_period: 2, cycle_periods: 4, cycle_start_issue: '99', cycle_status: 'active', ...patch,
  }
}

export function racingPlanDetail(selection: RacingPlanSelection = DEFAULT_RACING_PLAN, patch: Partial<RacingPlanDetail> = {}): RacingPlanDetail {
	const option = racingPlanOptions.find(item => item.key === selection.plan_key)!
	const gameId = patch.game_id ?? 'speed-racing'
	const rows = [1, 2, 3].map(index => racingPlanRow({
		...selection, id: index, game_id: gameId, master_name: `${index}号专家`, sort_order: index * 10, kind: option.kind,
    numbers: option.kind === 'numbers' ? Array.from({ length: option.number_count }, (_, number) => number + 1) : [],
    size: option.kind === 'size' ? '大' : '', parity: option.kind === 'parity' ? '单' : '', dragon_tiger: option.kind === 'dragon_tiger' ? '龙' : '',
    cycle_period: Math.min(2, option.periods), cycle_periods: option.periods,
  }))
  return {
		game_id: gameId, current_issue: '100', recommendations: rows, latest_recommendations: rows, history: rows, legacy_history: [],
    generation_mode: 'on_visit', automation_enabled: true, history_limit: 6, refresh_seconds: 15,
    options: racingPlanOptions, positions: racingPlanPositions, allowed_positions: racingPlanPositions.map(item => item.position), allowed_plan_keys: racingPlanOptions.map(item => item.key),
    selection: { ...selection, kind: option.kind, periods: option.periods, number_count: option.number_count },
    stream: { id: 1, allowed: true, active: true, activation_required: false, active_until: null, active_count: 1, max_active: 20 },
    notice: '系统自动生成，仅供娱乐参考，不保证命中。', ...patch,
  }
}
