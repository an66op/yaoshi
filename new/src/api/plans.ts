import { request } from './client'

export type PlanGameSummary = {
  game_id: string
  current_issue: string
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
  history: PlanRecommendation[]
}

export const planApi = {
  catalog: () => request<PlanGameSummary[]>('/member/plans'),
  detail: (gameId: string) => request<PlanDetail>(`/member/plans/${encodeURIComponent(gameId)}`),
}
