import { describe, expect, it } from 'vitest'
import type { ChatMessage } from '../api/chat'
import { mergeChatMessages } from './chatMessages'

const message = (id: number, createdAt: string, content = String(id)): ChatMessage => ({
  id,
  user_id: 1,
  nickname: '测试会员',
  room_type: 'group',
  room_scope: 'agent:9',
  game_id: 'lobby',
  content,
  message_type: 'text',
  mine: false,
  created_at: createdAt,
})
describe('mergeChatMessages', () => {
  it('按服务端时间和消息 ID 生成稳定的正序时间线', () => {
    const result = mergeChatMessages([
      message(3, '2026-08-27T08:02:00Z'),
      message(1, '2026-08-27T08:00:00Z'),
      message(2, '2026-08-27T08:00:00Z'),
    ])
    expect(result.map((item) => item.id)).toEqual([1, 2, 3])
  })

  it('合并历史、发送回执和重连补拉时不会重复消息', () => {
    const first = message(8, '2026-08-27T08:00:00Z', '旧内容')
    const updated = message(8, '2026-08-27T08:00:00Z', '服务端内容')
    const result = mergeChatMessages([first], [updated, message(9, '2026-08-27T08:01:00Z')])
    expect(result.map((item) => item.id)).toEqual([8, 9])
    expect(result[0].content).toBe('服务端内容')
  })
})
