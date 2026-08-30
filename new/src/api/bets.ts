import { request } from './client'
import { createRequestId } from '../utils/requestId'
import type { LotteryBettingWindow } from '../utils/lotteryTiming'

export type MemberBet = {
  id: number
  game_id: string
  issue: string
  user_id: number
  username: string
  play_code: string
  play_name: string
  position: number
  selection: string
  amount: number
  odds: number
  status: string
  payout: number
  created_at: string
	request_id?: string
	deducted?: number
	balance?: number
}

export type BetListResponse = {
  items: MemberBet[]
  total: number
  page: number
  page_size: number
  has_more: boolean
  next_before_id?: number
}

export type AssistantBetLine = {
  position: number
  selection: string
  play_code: string
  play_name: string
  amount: number
  odds: number
  label: string
}

export type AssistantBetResult = {
  game_id: string
  game_name: string
  issue: string
  content: string
  lines: AssistantBetLine[]
  bet_count: number
  total: number
  balance: number
  accepted_at: string
}

export type AssistantDrawStatus = {
  game_id: string
  game_name: string
  issue: string
  accepting: boolean
  betting_window?: LotteryBettingWindow | null
  next_draw_at: string
  latest_issue?: string
  latest_numbers?: number[]
  latest_draw_at?: string
  source_name?: string
	issue_status: string
	source_healthy: boolean
	source_error?: string
}

export type CancelIssueResult = {
  game_id: string
  issue: string
  count: number
  refund: number
  balance: number
}

export const betsApi = {
  list: (params?: { game_id?: string; issue?: string; status?: string; page?: number; page_size?: number; before_id?: number }) => {
    const query = new URLSearchParams({
      game_id: params?.game_id ?? 'all',
      status: params?.status ?? 'all',
      page: String(params?.page ?? 1),
      page_size: String(params?.page_size ?? 20),
    })
    if (params?.issue) query.set('issue', params.issue)
    if (params?.before_id) query.set('before_id', String(params.before_id))
    return request<BetListResponse>(`/member/bets?${query}`)
  },
  place: (payload: {
    game_id: string
    issue?: string
    play_code?: string
    play_name?: string
    position: number
    selection: string
    amount: number
    request_id?: string
  }) => request<MemberBet>('/member/bets', {
    method: 'POST',
    // Serialize the accepted contract explicitly. In particular, a caller can
    // never inject a client-supplied odds value into the placement request.
    body: JSON.stringify({
      game_id: payload.game_id,
      issue: payload.issue,
      play_code: payload.play_code,
      play_name: payload.play_name,
      position: payload.position,
      selection: payload.selection,
      amount: payload.amount,
      request_id: payload.request_id ?? createRequestId(),
    }),
  }),
  assistantStatus: (gameId: string) => request<AssistantDrawStatus>(`/member/games/${encodeURIComponent(gameId)}/assistant`),
  assistantHistory: (gameId: string, limit = 20) => request<AssistantBetResult[]>(`/member/games/${encodeURIComponent(gameId)}/assistant/history?limit=${limit}`),
  assistantPlace: (gameId: string, payload: { issue?: string; content: string; request_id?: string }) =>
    request<AssistantBetResult>(`/member/games/${encodeURIComponent(gameId)}/assistant/bets`, { method: 'POST', body: JSON.stringify(payload) }),
  cancelCurrent: (gameId: string, issue?: string) => request<CancelIssueResult>('/member/bets/cancel-current', {
    method: 'POST',
    body: JSON.stringify({ game_id: gameId, issue }),
  }),
  cancel: (id: number) => request<MemberBet>(`/member/bets/${id}/cancel`, { method: 'POST' }),
}
