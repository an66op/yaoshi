import { describe, expect, it } from 'vitest'
import type { ManagementWsEvent } from '../api'
import {
  chatPageForTarget, chatTargetFromEvent, isInboundMemberChatEvent, isInboundServiceChatEvent,
  sameChatTarget, shouldAutoReadChat, shouldSuppressChatAlert,
} from './chatNotifications'

const incoming = (overrides: Record<string, unknown> = {}): ManagementWsEvent => ({
  event_id: 'evt-1', type: 'chat_message', data: {
    operation: 'created', sender_kind: 'member', room_type: 'service',
    scope: 'user:7', room_scope: 'agent:3', game_id: 'service', message_id: 99,
    ...overrides,
  },
})

describe('chat notification classification', () => {
  it('only accepts newly-created member service messages', () => {
    expect(isInboundServiceChatEvent(incoming())).toBe(true)
    expect(isInboundServiceChatEvent(incoming({ sender_kind: 'staff' }))).toBe(false)
    expect(isInboundServiceChatEvent(incoming({ operation: 'deleted' }))).toBe(false)
    expect(isInboundServiceChatEvent(incoming({ room_type: 'group' }))).toBe(false)
  })

  it('uses the complete conversation key', () => {
    const target = chatTargetFromEvent(incoming())
    expect(target).toEqual({ scope: 'user:7', room_scope: 'agent:3', game_id: 'service', room_type: 'service' })
    expect(sameChatTarget(target, { ...target!, room_scope: 'agent:4' })).toBe(false)
    expect(sameChatTarget(target, { ...target! })).toBe(true)
  })

  it('accepts group targets and routes them to the matching management page', () => {
    const lobby = chatTargetFromEvent(incoming({ room_type: 'group', scope: 'agent:3', game_id: 'lobby' }))!
    const lottery = chatTargetFromEvent(incoming({ room_type: 'group', scope: 'agent:3', game_id: 'speed-racing' }))!
    expect(isInboundMemberChatEvent(incoming({ room_type: 'group' }))).toBe(true)
    expect(chatPageForTarget(lobby)).toBe('/chat')
    expect(chatPageForTarget(lottery)).toBe('/lottery-chat')
    expect(shouldSuppressChatAlert('/lottery-chat', 'visible', true, lottery, { ...lottery })).toBe(true)
    expect(shouldSuppressChatAlert('/chat', 'visible', true, lottery, lottery)).toBe(false)
  })

  it('auto-reads only the focused visible copy of the exact open conversation', () => {
    const target = chatTargetFromEvent(incoming())
    expect(shouldAutoReadChat('/chat', 'visible', true, target, { ...target! })).toBe(true)
    expect(shouldAutoReadChat('/chat', 'visible', false, target, target)).toBe(false)
    expect(shouldAutoReadChat('/chat', 'hidden', true, target, target)).toBe(false)
    expect(shouldAutoReadChat('/chat', 'visible', true, target, { ...target!, scope: 'user:8' })).toBe(false)
    expect(shouldAutoReadChat('/', 'visible', true, target, target)).toBe(false)
  })
})
