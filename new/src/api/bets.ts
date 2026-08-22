import { request } from './client'

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
}

export type BetListResponse = {
  items: MemberBet[]
  total: number
  page: number
  page_size: number
}

export const betsApi = {
  list: (params?: { game_id?: string; issue?: string; status?: string; page?: number; page_size?: number }) => {
    const query = new URLSearchParams({
      game_id: params?.game_id ?? 'all',
      status: params?.status ?? 'all',
      page: String(params?.page ?? 1),
      page_size: String(params?.page_size ?? 20),
    })
    if (params?.issue) query.set('issue', params.issue)
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
    odds?: number
  }) => request<MemberBet>('/member/bets', { method: 'POST', body: JSON.stringify(payload) }),
  cancel: (id: number) => request<MemberBet>(`/member/bets/${id}/cancel`, { method: 'POST' }),
}
