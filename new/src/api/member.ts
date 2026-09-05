import { apiBase, publicRequest, request } from './client'
import { createRequestId } from '../utils/requestId'

export type LoginCaptcha = { id: string; image: string; expires_in: number }
export type LoginCaptchaInput = { captcha_id: string; captcha_code: string }

export type LoginResult = {
  user: {
    id: number
    public_id: number
    username: string
    email: string
    nickname: string
    avatar?: string
    public_title?: string
    badge?: string
    room_logo?: string
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
  room_logo?: string
}

export type WsTicket = {
  ticket: string
  expires_at: string
}

export type RoomResolve = {
  room_code: string
  room_name: string
  room_logo?: string
  status: 'joined' | 'pending'
  application_id?: number
}

export type MemberRoomHistoryItem = {
  room_code: string
  room_name: string
  room_logo?: string
  status: 'current' | 'available' | 'pending' | 'disabled'
  current: boolean
  last_entered_at: string
}

export type MemberApplication = {
  id: number
  request_type: 'credit' | 'debit' | string
  payment_type: string

  payment_account_id: number
  payment_account_label: string
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

export type BalanceHistoryPage = {
  items: BalanceRecord[]
  has_more: boolean
  next_before_id: number
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

export type MemberPaymentAccount = {
  id: number
  account_type: 'wechat' | 'alipay' | 'bank' | 'usdt' | string
  label: string
  account_name: string
  account_no: string
  holder_name: string
  is_default: boolean
  qr_code_url?: string
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
  invited_count: number
  total_reward: number
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
  loginCaptcha: (signal?: AbortSignal) =>
    publicRequest<LoginCaptcha>('/member/login/captcha', { cache: 'no-store', signal }),
  login: (username: string, password: string, captcha: LoginCaptchaInput) =>
    request<LoginResult>('/member/login', {
      method: 'POST',
      body: JSON.stringify({ username, password, captcha_id: captcha.captcha_id, captcha_code: captcha.captcha_code }),
    }),
  register: (payload: { username: string; password: string; invite_code?: string; room_code?: string }) =>
    request<LoginResult>('/member/register', { method: 'POST', body: JSON.stringify(payload) }),
	logout: () => request<null>('/member/logout', { method: 'POST' }),
	refreshSession: () => request<{ expires_in: number }>('/member/session/refresh', { method: 'POST' }),
  me: () => request<MemberProfile>('/member/me'),
  wsTicket: () => request<WsTicket>('/member/ws-ticket', { method: 'POST' }),
  joinRoom: (room_code: string, request_id = `join-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`) =>
    request<RoomResolve>('/member/room/join', { method: 'POST', body: JSON.stringify({ room_code, request_id }) }),
  roomHistory: (limit = 8) => request<MemberRoomHistoryItem[]>(`/member/room/history?limit=${Math.min(10, Math.max(1, limit))}`),
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
  createApplication: (payload: { request_type: 'credit' | 'debit'; amount: number; payment_type?: string; payment_account_id?: number; remark?: string; request_id?: string }) =>
    request<MemberApplication>('/member/applications', { method: 'POST', body: JSON.stringify({ ...payload, request_id: payload.request_id ?? createRequestId() }) }),
  balanceHistory: (limit = 30, beforeID?: number) => {
    const query = new URLSearchParams({ limit: String(limit) })
    if (beforeID) query.set('before_id', String(beforeID))
    return request<BalanceHistoryPage>(`/member/balance-history?${query}`)
  },
  walletChannels: () => request<WalletChannel[]>('/member/wallet/channels'),
  paymentAccounts: () => request<MemberPaymentAccount[]>('/member/payment-accounts'),
  createPaymentAccount: (payload: { account_type: string; label?: string; account_name: string; account_no: string; holder_name?: string; is_default?: boolean; qr_code?: File | null }) => {
    if (!payload.qr_code) {
      const { qr_code: _qrCode, ...account } = payload
      return request<MemberPaymentAccount>('/member/payment-accounts', { method: 'POST', body: JSON.stringify(account) })
    }
    const form = new FormData()
    form.set('account_type', payload.account_type)
    form.set('label', payload.label ?? '')
    form.set('account_name', payload.account_name)
    form.set('account_no', payload.account_no)
    form.set('holder_name', payload.holder_name ?? '')
    form.set('is_default', String(Boolean(payload.is_default)))
    // The server ignores this synthetic name and derives its own random name.
    form.set('qr_code', payload.qr_code, 'qr-code-upload')
    return request<MemberPaymentAccount>('/member/payment-accounts', { method: 'POST', body: form })
  },
  paymentAccountQRCodeURL: (id: number) => `${apiBase}/member/payment-accounts/${encodeURIComponent(String(id))}/qr-code`,
  deletePaymentAccount: (id: number) => request<null>(`/member/payment-accounts/${id}`, { method: 'DELETE' }),
  walletSummary: () => request<WalletSummary>('/member/wallet/summary'),
  rebatePreview: () => request<RebatePreview>('/member/wallet/rebate'),
  inviteInfo: () => request<InviteInfo>('/member/invite'),
  entertainment: () => request<EntertainmentPlatform[]>('/member/entertainment'),
  launchEntertainment: (code: string) =>
    request<EntertainmentLaunch>(`/member/entertainment/${encodeURIComponent(code)}/launch`, { method: 'POST' }),
  changePassword: (old_password: string, new_password: string) =>
    request<null>('/member/password', { method: 'POST', body: JSON.stringify({ old_password, new_password }) }),
  updateNickname: (nickname: string) =>
    request<MemberProfile>('/member/nickname', { method: 'PATCH', body: JSON.stringify({ nickname }) }),
  updateAvatar: (avatar: string) =>
    request<MemberProfile>('/member/avatar', { method: 'PATCH', body: JSON.stringify({ avatar }) }),
}
