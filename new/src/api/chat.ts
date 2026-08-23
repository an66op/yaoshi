import { request } from './client'

export type ChatMessage = {
  id: number
  user_id: number
  public_id?: number
  nickname: string
  room_type: string
  content: string
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
  messages: (room_type: 'group' | 'service', limit = 20, cursor?: { before_id?: number; after_id?: number }) => {
    const query = new URLSearchParams({ room_type, limit: String(limit) })
    if (cursor?.before_id) query.set('before_id', String(cursor.before_id))
    if (cursor?.after_id) query.set('after_id', String(cursor.after_id))
    return request<ChatMessagePage>(`/member/chat/messages?${query}`)
  },
  send: (content: string, room_type: 'group' | 'service' = 'group') =>
    request<ChatMessage>('/member/chat/messages', {
      method: 'POST',
      body: JSON.stringify({ content, room_type }),
    }),
}
