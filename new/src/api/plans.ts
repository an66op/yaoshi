import { request } from './client'
import { PLAN_HISTORY_LIMIT } from '../utils/planLimits'

export type PlanGameSummary = {
  game_id: string
  current_issue: string
  latest_issue: string
  history_only: boolean
  master_count: number
  updated_at: string
}

export type PlanRecommendation = {
  id: number
  workspace_id: number
  game_id: string
  issue: string
  master_name: string
  master_title: string
  master_color: string
  numbers: number[]
  size: '' | '大' | '小'
  parity: '' | '单' | '双'
  result: 'pending' | 'hit' | 'miss'
  source: 'manual' | 'demo'
  note: string
  enabled: boolean
  sort_order: number
  master_hit_rate: number | null
  created_at: string
  updated_at: string
}

export type PlanDetail = {
  game_id: string
  current_issue: string
  recommendations: PlanRecommendation[]
  latest_recommendations: PlanRecommendation[]
  history: PlanRecommendation[]
  generation_mode?: 'on_visit'
  automation_enabled?: boolean
  history_limit?: number
  refresh_seconds?: number
}

export type RacingPlanKind = 'numbers' | 'size' | 'parity' | 'dragon_tiger'
export type RacingPlanSelection = { position: number; plan_key: string }
export type RacingPlanOption = { key: string; label: string; kind: RacingPlanKind; periods: number; number_count: number }
export type RacingPlanPosition = { position: number; label: string; opponent_position: number }
export type RacingPlanRecommendation = PlanRecommendation & {
  position: number
  plan_key: string
  kind: RacingPlanKind
  dragon_tiger: '' | '龙' | '虎'
  cycle_id: number
  cycle_period: number
  cycle_periods: number
  cycle_start_issue: string
  cycle_status: 'active' | 'completed' | 'interrupted'
  draw_numbers?: number[]
  draw_at?: string
}
export type RacingPlanDetail = Omit<PlanDetail, 'recommendations' | 'latest_recommendations' | 'history'> & {
  recommendations: RacingPlanRecommendation[]
  latest_recommendations: RacingPlanRecommendation[]
  history: RacingPlanRecommendation[]
  legacy_history: PlanRecommendation[]
  options: RacingPlanOption[]
  positions: RacingPlanPosition[]
  allowed_positions: number[]
  allowed_plan_keys: string[]
  selection: RacingPlanSelection & { kind: RacingPlanKind; periods: number; number_count: number }
  stream: { id: number; allowed: boolean; active: boolean; activation_required: boolean; active_until: string | null; active_count: number; max_active: number }
  notice: string
}

const historyQuery = `history_limit=${PLAN_HISTORY_LIMIT}`
const racingPlanQuery = (selection: RacingPlanSelection) => new URLSearchParams({ position: String(selection.position), plan_key: selection.plan_key, history_limit: String(PLAN_HISTORY_LIMIT) }).toString()

export const planApi = {
  catalog: (signal?: AbortSignal) => request<PlanGameSummary[]>('/member/plans', { signal }),
  detail: (gameId: string, signal?: AbortSignal) => request<PlanDetail>(`/member/plans/${encodeURIComponent(gameId)}?${historyQuery}`, { signal }),
  activate: (gameId: string, signal?: AbortSignal) => request<PlanDetail>(`/member/plans/${encodeURIComponent(gameId)}/activate?${historyQuery}`, { method: 'POST', body: '{}', signal }),
  racingDetail: (selection: RacingPlanSelection, signal?: AbortSignal) => request<RacingPlanDetail>(`/member/plans/speed-racing?${racingPlanQuery(selection)}`, { signal }),
  activateRacing: (selection: RacingPlanSelection, signal?: AbortSignal) => request<RacingPlanDetail>(`/member/plans/speed-racing/activate?${historyQuery}`, { method: 'POST', body: JSON.stringify(selection), signal }),
}
