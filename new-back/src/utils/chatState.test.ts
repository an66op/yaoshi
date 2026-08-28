import { describe, expect, it } from 'vitest'
import type { AdminChatConversation, AdminChatMessage } from '../api'
import { mergeAdminChatMessages, selectConversation } from './chatState'

const conversation = (gameID = 'service'): AdminChatConversation => ({
  scope: 'user:30', room_scope: 'agent:9', game_id: gameID, room_type: 'service',
  title: '在线客服', subtitle: '房间 8801', latest_text: '你好', latest_is_staff: false,
  message_count: 1, group_chat_enabled: true, enabled: true,
})

const message = (id: number, createdAt: string): AdminChatMessage => ({
  id, user_id: 30, username: 'member', nickname: '会员', room_type: 'service',
  scope: 'user:30', room_scope: 'agent:9', game_id: 'service', content: String(id),
  message_type: 'text', is_staff: false, created_at: createdAt,
})

describe('conversation state', () => {
  it('重复点击当前会话时保留右侧消息', () => {
    const current = conversation()
    const messages = [message(1, '2026-08-27T08:00:00Z')]
    const next = selectConversation(current, { ...current }, messages)
    expect(next.selected).toBe(current)
    expect(next.messages).toBe(messages)
  })

  it('切换到另一会话时清空上一会话消息', () => {
    const next = selectConversation(conversation(), conversation('lobby'), [message(1, '2026-08-27T08:00:00Z')])
    expect(next.selected?.game_id).toBe('lobby')
    expect(next.messages).toEqual([])
  })
})

describe('mergeAdminChatMessages', () => {
  it('重连快照合并后保持正序且不重复', () => {
    const result = mergeAdminChatMessages(
      [message(2, '2026-08-27T08:02:00Z'), message(1, '2026-08-27T08:01:00Z')],
      [message(2, '2026-08-27T08:02:00Z')],
    )
    expect(result.map((item) => item.id)).toEqual([1, 2])
  })

  it('加载更早一页时拼接在现有消息前且保持稳定正序', () => {
    const result = mergeAdminChatMessages(
      [message(3, '2026-08-27T08:03:00Z'), message(4, '2026-08-27T08:04:00Z')],
      [message(1, '2026-08-27T08:01:00Z'), message(2, '2026-08-27T08:02:00Z'), message(3, '2026-08-27T08:03:00Z')],
    )
    expect(result.map((item) => item.id)).toEqual([1, 2, 3, 4])
  })
})
