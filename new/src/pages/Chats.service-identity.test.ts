import { describe, expect, it } from 'vitest'
import type { ChatMessage } from '../api/chat'
import { memberFacingServiceName, serviceMessageIdentity } from '../utils/chatIdentity'

const staffMessage = (overrides: Partial<ChatMessage> = {}): ChatMessage => ({
  id: 1,
  user_id: 0,
  nickname: '客服小七',
  title: '群主',
  badge: '官方',
  room_type: 'service',
  room_scope: 'agent:1',
  game_id: 'service',
  content: '您好',
  message_type: 'text',
  mine: false,
  created_at: '2026-09-05T00:00:00Z',
  ...overrides,
})

describe('member-facing customer-service identity', () => {
  it('normalizes the legacy group-owner label and avoids a duplicate title', () => {
    expect(memberFacingServiceName('群主')).toBe('客服')
    expect(serviceMessageIdentity(staffMessage(), '群主')).toEqual({ name: '客服', title: '', badge: '官方' })
  })

  it('preserves an explicitly configured custom service name without repeating it', () => {
    expect(serviceMessageIdentity(staffMessage({ title: '客服小七' }), '客服小七')).toEqual({ name: '客服小七', title: '', badge: '官方' })
  })
})
