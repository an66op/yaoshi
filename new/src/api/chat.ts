import { request } from './client'

export type ChatMessage = {
  id: number
  user_id: number
  username: string
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

export const chatApi = {
  preview: () => request<ChatPreview>('/member/chat/preview'),
  messages: (room_type: 'group' | 'service', limit = 50) =>
    request<ChatMessage[]>(`/member/chat/messages?room_type=${room_type}&limit=${limit}`),
  send: (content: string, room_type: 'group' | 'service' = 'group') =>
    request<ChatMessage>('/member/chat/messages', {
      method: 'POST',
      body: JSON.stringify({ content, room_type }),
    }),
}
