import { request } from './client'

export type ChatMessage = {
  id: number
  user_id: number
  public_id?: number
  nickname: string
  avatar?: string
  title?: string
  user_title?: string
  badge?: string
  room_type: string
  room_scope: string
  game_id: string
  content: string
  message_type: 'text' | 'redpacket' | string
  reference_id?: number
  red_packet_count?: number
  red_packet_total?: number
  red_packet_min_turnover?: number
  red_packet_cover?: 'classic' | 'celebration' | 'lucky' | string
  red_packet_status?: 'active' | 'empty' | 'expired' | 'closed' | string
  red_packet_funding_status?: 'reserved' | 'partially_released' | 'released' | 'refunded' | 'legacy_unfunded' | string
  red_packet_claimed_count?: number
  red_packet_remaining?: number
  red_packet_refunded?: number
  red_packet_expires_at?: string
  red_packet_closed_at?: string
  red_packet_close_reason?: string
  claimed?: boolean
  red_packet_reward?: number
  mine: boolean
  created_at: string
}

export type ChatPreview = {
  latest_message: string
  latest_at?: string
  can_chat: boolean
  min_chat_score: number
  chat_nickname: string
  balance: number
}

export type ChatMessagePage = {
  items: ChatMessage[]
  has_more: boolean
  next_before_id?: number
}

export const chatApi = {
  preview: () => request<ChatPreview>('/member/chat/preview'),
  availableRedPacket: () => request<ChatMessage | null>('/member/chat/redpackets/available'),
  messages: (room_type: 'group' | 'service', game_id = room_type === 'service' ? 'service' : 'lobby', limit = 20, cursor?: { before_id?: number; after_id?: number }) => {
    const query = new URLSearchParams({ room_type, game_id, limit: String(limit) })
    if (cursor?.before_id) query.set('before_id', String(cursor.before_id))
    if (cursor?.after_id) query.set('after_id', String(cursor.after_id))
    return request<ChatMessagePage>(`/member/chat/messages?${query}`)
  },
  send: (content: string, room_type: 'group' | 'service' = 'group', game_id = room_type === 'service' ? 'service' : 'lobby') =>
    request<ChatMessage>('/member/chat/messages', {
      method: 'POST',
      body: JSON.stringify({ content, room_type, game_id }),
    }),
  command: (content: string, game_id: string, options: { issue: string; request_id: string }) =>
    request<ChatMessage>('/member/chat/commands', {
      method: 'POST',
      body: JSON.stringify({ content, room_type: 'group', game_id, ...options }),
    }),
  claimRedPacket: (messageId: number) => request<{ reward: number; balance: number; message: string }>(`/member/chat/redpackets/${messageId}/claim`, { method: 'POST' }),
}
