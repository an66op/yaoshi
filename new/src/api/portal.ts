import { request } from './client'

export type RoomSettings = {
  room_name: string
  room_notice: string
  show_odds: boolean
  sound_enabled: boolean
  min_credit_amount: number
  min_debit_amount: number
  min_chat_score: number
  chat_nickname?: string
  game: Record<string, unknown>
  quick_replies: string[] | Record<string, unknown>
}

export type OddsItem = {
  play_code: string
  play_name: string
  odds: number
  min_bet: number
  max_bet: number
  max_user_period: number
}

export type GameOdds = {
  game_id: string
  game_name: string
  show_odds: boolean
  items: OddsItem[]
}

export type ActivityItem = {
  id: number
  type: string
  title: string
  subtitle: string
  cover: string
  status: string
  reward: number
  participants: number
  config?: Record<string, unknown>
}

export type ActivityStatus = {
  activity_id: number
  type: string
  title: string
  checked_in: boolean
  claimed: boolean
  streak: number
  reward: number
  participants: number
  config?: Record<string, unknown>
}

export type ActivityActionResult = {
  reward: number
  streak: number
  balance: number
  message: string
}

export type MemberNotification = {
  id: number
  title: string
  content: string
  level: string
  category: string
  link: string
  read: boolean
  created_at: string
}

export type MemberNotificationPage = {
  items: MemberNotification[]
  has_more: boolean
  next_before_id?: number
}

export type GameFeedItem = {
  nickname: string
  detail: string
  amount: number
  created_at: string
}

export const portalApi = {
  roomSettings: () => request<RoomSettings>('/member/room/settings'),
  gameOdds: (gameId: string) => request<GameOdds>(`/member/games/${encodeURIComponent(gameId)}/odds`),
  gameFeed: (gameId: string, issue?: string, limit = 20) => {
    const query = new URLSearchParams({ limit: String(limit) })
    if (issue) query.set('issue', issue)
    return request<GameFeedItem[]>(`/member/games/${encodeURIComponent(gameId)}/feed?${query}`)
  },
  activities: (type?: string) => request<ActivityItem[]>(`/member/activities${type ? `?type=${type}` : ''}`),
  activityStatus: (id: number) => request<ActivityStatus>(`/member/activities/${id}/status`),
  checkIn: (id: number) => request<ActivityActionResult>(`/member/activities/${id}/checkin`, { method: 'POST' }),
  claimRedPacket: (id: number) => request<ActivityActionResult>(`/member/activities/${id}/redpacket`, { method: 'POST' }),
  notifications: (limit = 20, cursor?: { before_id?: number; category?: 'system' | 'activity' | 'winning' | 'all' }) => {
    const query = new URLSearchParams({ limit: String(limit), category: cursor?.category ?? 'all' })
    if (cursor?.before_id) query.set('before_id', String(cursor.before_id))
    return request<MemberNotificationPage>(`/member/notifications?${query}`)
  },
  unreadCount: () => request<{ unread: number }>('/member/notifications/unread'),
  markRead: (id: number) => request<null>(`/member/notifications/${id}/read`, { method: 'POST' }),
  markAllRead: () => request<null>('/member/notifications/read-all', { method: 'POST' }),
}
