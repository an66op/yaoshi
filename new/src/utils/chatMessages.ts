import type { ChatMessage } from '../api/chat'

function messageTimestamp(message: ChatMessage) {
  const value = new Date(message.created_at).getTime()
  return Number.isFinite(value) ? value : message.id
}
/**
 * Return one stable, oldest-to-newest timeline. The API, an optimistic send,
 * and a reconnect catch-up can all contain the same message, so ID based
 * de-duplication happens before sorting.
 */
export function mergeChatMessages(...groups: ChatMessage[][]) {
  const byID = new Map<number, ChatMessage>()
  for (const message of groups.flat()) byID.set(message.id, message)
  return [...byID.values()].sort((left, right) => {
    const byTime = messageTimestamp(left) - messageTimestamp(right)
    return byTime || left.id - right.id
  })
}
