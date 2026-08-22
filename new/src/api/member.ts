import { publicRequest, request } from './client'

export type LoginResult = {
  token: string
  user: {
    id: number
    username: string
    email: string
    nickname: string
    role: string
    status: number
  }
  message?: string
}

export type MemberProfile = LoginResult['user'] & {
  balance: number
  parent_agent_id?: number | null
  room_code?: string
  room_name?: string
}

export type WsTicket = {
  ticket: string
  expires_at: string
}

export type RoomResolve = {
  room_code: string
  room_name: string
  agent_id: number
  agent_username: string
  agent_nickname: string
}

export type MemberApplication = {
  id: number
  request_type: 'credit' | 'debit' | string
  payment_type: string
  requested_amount: number
  received_amount: number
  remark: string
  status: 'pending' | 'approved' | 'rejected' | string
  review_remark: string
  created_at: string
}

export type ApplicationListResponse = {
  items: MemberApplication[]
  total: number
  page: number
  page_size: number
}

export type BalanceRecord = {
  id: number
  amount: number
  before: number
  after: number
  type: string
  remark: string
  created_at: string
}

export type WalletChannel = {
  id: number
  provider: string
  name: string
  credit_type: string
  min_amount: number
  max_amount: number
  remark: string
}

export type WalletSummary = {
  today_turnover: number
  today_profit: number
  today_rebate: number
  pending_amount: number
  pending_count: number
  total_bet_count: number
}

export type RebatePreview = {
  biz_date: string
  enabled: boolean
  rate_percent: number
  today_turnover: number
  estimated: number
  credited: number
  pending_credit: number
}

export type InviteInfo = {
  invite_code: string
  username: string
  room_code: string
  title: string
  reward: number
  share_text: string
}

export type EntertainmentPlatform = {
  id: number
  code: string
  name: string
  category: string
  status: string
  remark: string
}

export type EntertainmentLaunch = {
  code: string
  name: string
  status: string
  message: string
  ready: boolean
  launch_url?: string
  expires_at?: number
}

export const memberApi = {
  login: (username: string, password: string) =>
    request<LoginResult>('/member/login', { method: 'POST', body: JSON.stringify({ username, password }) }),
  register: (payload: { username: string; password: string; nickname?: string; invite_code?: string; room_code?: string }) =>
    request<LoginResult>('/member/register', { method: 'POST', body: JSON.stringify(payload) }),
  me: () => request<MemberProfile>('/member/me'),
  wsTicket: () => request<WsTicket>('/member/ws-ticket', { method: 'POST' }),
  joinRoom: (room_code: string) =>
    request<RoomResolve>('/member/room/join', { method: 'POST', body: JSON.stringify({ room_code }) }),
  resolveRoom: (code: string) => publicRequest<RoomResolve>(`/public/rooms/${encodeURIComponent(code)}`),
  applications: (params?: { status?: string; request_type?: string; page?: number }) => {
    const query = new URLSearchParams({
      status: params?.status ?? 'all',
      request_type: params?.request_type ?? 'all',
      page: String(params?.page ?? 1),
      page_size: '20',
    })
    return request<ApplicationListResponse>(`/member/applications?${query}`)
  },
  createApplication: (payload: { request_type: 'credit' | 'debit'; amount: number; payment_type?: string; remark?: string }) =>
    request<MemberApplication>('/member/applications', { method: 'POST', body: JSON.stringify(payload) }),
  balanceHistory: (limit = 30) => request<BalanceRecord[]>(`/member/balance-history?limit=${limit}`),
  walletChannels: () => request<WalletChannel[]>('/member/wallet/channels'),
  walletSummary: () => request<WalletSummary>('/member/wallet/summary'),
  rebatePreview: () => request<RebatePreview>('/member/wallet/rebate'),
  inviteInfo: () => request<InviteInfo>('/member/invite'),
  entertainment: () => request<EntertainmentPlatform[]>('/member/entertainment'),
  launchEntertainment: (code: string) =>
    request<EntertainmentLaunch>(`/member/entertainment/${encodeURIComponent(code)}/launch`, { method: 'POST' }),
  changePassword: (old_password: string, new_password: string) =>
    request<null>('/member/password', { method: 'POST', body: JSON.stringify({ old_password, new_password }) }),
}
