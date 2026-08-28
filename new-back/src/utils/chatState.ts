import type { AdminChatConversation, AdminChatMessage } from '../api'

export function sameConversation(left: AdminChatConversation | null, right: AdminChatConversation) {
  return Boolean(left
    && left.scope === right.scope
    && left.room_scope === right.room_scope
    && left.game_id === right.game_id
    && left.room_type === right.room_type)
}
export function selectConversation(
  current: AdminChatConversation | null,
  next: AdminChatConversation,
  messages: AdminChatMessage[],
) {
  if (sameConversation(current, next)) return { selected: current, messages }
  return { selected: next, messages: [] as AdminChatMessage[] }
}

export function mergeAdminChatMessages(...groups: AdminChatMessage[][]) {
  const byID = new Map<number, AdminChatMessage>()
  for (const message of groups.flat()) byID.set(message.id, message)
  return [...byID.values()].sort((left, right) => {
    const leftTime = new Date(left.created_at).getTime()
    const rightTime = new Date(right.created_at).getTime()
    const byTime = (Number.isFinite(leftTime) ? leftTime : left.id) - (Number.isFinite(rightTime) ? rightTime : right.id)
    return byTime || left.id - right.id
  })
}
