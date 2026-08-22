import { publicRequest } from './client'

export type LotteryGame = {
  id: string
  code: string
  name: string
  category: string
  badge: string
  badge_color: string
  enabled: boolean
  issue: string
  current_issue?: string
  bettor_count?: number
  latest_numbers?: number[]
  next_draw_at: string
  source_kind: string
}

export type ServerClock = {
  server_time: string
  server_time_ms: number
  timezone: string
}

export const lotteryApi = {
  enabledGames: () => publicRequest<LotteryGame[]>('/public/lottery/games/enabled'),
  clock: () => publicRequest<ServerClock>('/public/clock'),
  draws: (gameId: string, limit = 30) => publicRequest<DrawResult[]>(`/public/lottery/games/${encodeURIComponent(gameId)}/draws?limit=${limit}`),
}

export type DrawResult = {
  id: number
  game_id: string
  issue: string
  numbers: number[]
  draw_at: string
}
