import type { ChatMessage } from '../api/chat'

/**
 * 房间红包提示只展示当前会员仍可领取的持久化红包。
 * 状态字段为空用于兼容迁移前的红包消息；明确关闭、领完或过期的红包不再提示。
 */
export function isClaimableRoomRedPacket(message: ChatMessage, now = Date.now()) {
  if (message.message_type !== 'redpacket' || message.claimed) return false
  if (message.red_packet_status && message.red_packet_status !== 'active') return false
  if (message.red_packet_claimed_count != null && message.red_packet_count != null
    && message.red_packet_claimed_count >= message.red_packet_count) return false
  if (message.red_packet_remaining != null && message.red_packet_remaining <= 0) return false
  if (message.red_packet_expires_at) {
    const expiresAt = new Date(message.red_packet_expires_at).getTime()
    if (Number.isFinite(expiresAt) && expiresAt <= now) return false
  }
  return true
}

export function latestClaimableRoomRedPacket(messages: ChatMessage[], now = Date.now()) {
  return [...messages]
    .reverse()
    .find((message) => isClaimableRoomRedPacket(message, now)) ?? null
}
