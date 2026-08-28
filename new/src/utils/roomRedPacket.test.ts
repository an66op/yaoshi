import { describe, expect, it } from 'vitest'
import type { ChatMessage } from '../api/chat'
import { isClaimableRoomRedPacket, latestClaimableRoomRedPacket } from './roomRedPacket'

function packet(id: number, overrides: Partial<ChatMessage> = {}): ChatMessage {
  return {
    id,
    user_id: 0,
    nickname: '房间客服',
    room_type: 'group',
    room_scope: 'room:88001',
    game_id: 'lobby',
    content: '恭喜发财',
    message_type: 'redpacket',
    mine: false,
    created_at: `2026-08-28T00:00:0${id}Z`,
    red_packet_status: 'active',
    red_packet_count: 10,
    red_packet_claimed_count: 0,
    red_packet_remaining: 100,
    ...overrides,
  }
}

describe('room red-packet prompt', () => {
  it('selects the newest claimable packet', () => {
    expect(latestClaimableRoomRedPacket([packet(1), packet(2)])?.id).toBe(2)
  })

  it('hides packets already claimed by the current member', () => {
    expect(isClaimableRoomRedPacket(packet(1, { claimed: true }))).toBe(false)
  })

  it.each(['empty', 'expired', 'closed'])(
    'hides a packet with %s status',
    (status) => expect(isClaimableRoomRedPacket(packet(1, { red_packet_status: status }))).toBe(false),
  )

  it('hides packets whose count or balance is exhausted', () => {
    expect(isClaimableRoomRedPacket(packet(1, { red_packet_claimed_count: 10 }))).toBe(false)
    expect(isClaimableRoomRedPacket(packet(2, { red_packet_remaining: 0 }))).toBe(false)
  })

  it('hides packets after their expiration time', () => {
    expect(isClaimableRoomRedPacket(packet(1, { red_packet_expires_at: '2026-08-28T01:00:00Z' }), Date.parse('2026-08-28T01:00:01Z'))).toBe(false)
  })
})
