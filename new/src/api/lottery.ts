import { publicRequest, request } from './client'
import type { LotteryBettingWindow } from '../utils/lotteryTiming'

export type LotteryGame = {
  id: string
  code: string
  name: string
  category: string
  lobby_category: string
  lobby_sort_order: number
  badge: string
  badge_color: string
  enabled: boolean
  issue: string
  current_issue?: string
  bettor_count?: number
  latest_numbers?: number[]
  next_draw_at: string
  /** Effective per-game period and room seal settings, in seconds. */
  draw_interval?: number | null
  seal_seconds?: number | null
  accept_at?: string | null
  source_kind: string
  source_name: string
  sync_status: string
  last_sync_at?: string | null
  last_sync_error?: string
  issue_status: string
  seal_at?: string | null
  source_healthy: boolean
  rules_ready?: boolean
  rule_version?: string
  rules_message?: string
  betting_window?: LotteryBettingWindow | null
}

export type ServerClock = {
  server_time: string
  server_time_ms: number
  timezone: string
}

export const lotteryApi = {
  enabledGames: (signal?: AbortSignal) => request<LotteryGame[]>('/member/games', signal ? { signal } : undefined),
  clock: (signal?: AbortSignal) => publicRequest<ServerClock>('/public/clock', signal ? { signal } : undefined),
  draws: (gameId: string, limit = 30, signal?: AbortSignal) => publicRequest<DrawResult[]>(`/public/lottery/games/${encodeURIComponent(gameId)}/draws?limit=${limit}`, signal ? { signal } : undefined),
}

export type DrawResult = {
  id: number
  game_id: string
  issue: string
  numbers: number[]
  draw_at: string
}
