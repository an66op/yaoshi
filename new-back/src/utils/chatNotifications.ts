import type { AdminChatConversation, ManagementWsEvent } from '../api'

export type ChatConversationTarget = Pick<AdminChatConversation, 'scope' | 'room_scope' | 'game_id' | 'room_type'>

export const CHAT_OPEN_CONVERSATION_EVENT = 'wangzhe-chat-open-conversation'
export const CHAT_UNREAD_CHANGED_EVENT = 'wangzhe-chat-unread-changed'
export const CHAT_ACTIVE_CONVERSATION_EVENT = 'wangzhe-chat-active-conversation'
const CHAT_TARGET_STORAGE_KEY = 'wangzhe-chat-open-target'

let activeConversation: ChatConversationTarget | null = null

export function sameChatTarget(left: ChatConversationTarget | null | undefined, right: ChatConversationTarget | null | undefined) {
  return Boolean(left && right
    && left.scope === right.scope
    && left.room_scope === right.room_scope
    && left.game_id === right.game_id
    && left.room_type === right.room_type)
}

export function shouldAutoReadChat(
  path: string,
  visibility: DocumentVisibilityState,
  focused: boolean,
  active: ChatConversationTarget | null | undefined,
  target: ChatConversationTarget | null | undefined,
) {
  return path === '/chat' && visibility === 'visible' && focused && sameChatTarget(active, target)
}

export function chatPageForTarget(target: ChatConversationTarget) {
  return target.room_type === 'group' && target.game_id !== 'lobby' ? '/lottery-chat' : '/chat'
}

export function shouldSuppressChatAlert(
  path: string,
  visibility: DocumentVisibilityState,
  focused: boolean,
  active: ChatConversationTarget | null | undefined,
  target: ChatConversationTarget | null | undefined,
) {
  return Boolean(target
    && path === chatPageForTarget(target)
    && visibility === 'visible'
    && focused
    && sameChatTarget(active, target))
}

export function setActiveChatConversation(target: ChatConversationTarget | null) {
  activeConversation = target
  window.dispatchEvent(new CustomEvent<ChatConversationTarget | null>(CHAT_ACTIVE_CONVERSATION_EVENT, { detail: target }))
}

export function getActiveChatConversation() {
  return activeConversation
}

export function isInboundServiceChatEvent(event: ManagementWsEvent | null | undefined) {
  const data = event?.data ?? {}
  return event?.type === 'chat_message'
    && data.operation === 'created'
    && data.sender_kind === 'member'
    && data.room_type === 'service'
}

export function isInboundMemberChatEvent(event: ManagementWsEvent | null | undefined) {
  const data = event?.data ?? {}
  return event?.type === 'chat_message'
    && data.operation === 'created'
    && data.sender_kind === 'member'
    && (data.room_type === 'service' || data.room_type === 'group')
}

export function chatTargetFromEvent(event: ManagementWsEvent): ChatConversationTarget | null {
  const data = event.data ?? {}
  if (typeof data.scope !== 'string' || typeof data.room_scope !== 'string' || typeof data.game_id !== 'string' || (data.room_type !== 'service' && data.room_type !== 'group')) return null
  return { scope: data.scope, room_scope: data.room_scope, game_id: data.game_id, room_type: data.room_type }
}

export function requestOpenChatConversation(target: ChatConversationTarget) {
  try { window.sessionStorage.setItem(CHAT_TARGET_STORAGE_KEY, JSON.stringify(target)) } catch { /* Storage can be unavailable in hardened browsers. */ }
  window.dispatchEvent(new CustomEvent<ChatConversationTarget>(CHAT_OPEN_CONVERSATION_EVENT, { detail: target }))
}

export function consumePendingChatConversation(): ChatConversationTarget | null {
  try {
    const value = window.sessionStorage.getItem(CHAT_TARGET_STORAGE_KEY)
    if (!value) return null
    window.sessionStorage.removeItem(CHAT_TARGET_STORAGE_KEY)
    const parsed = JSON.parse(value) as Partial<ChatConversationTarget>
    if (typeof parsed.scope !== 'string' || typeof parsed.room_scope !== 'string' || typeof parsed.game_id !== 'string' || (parsed.room_type !== 'service' && parsed.room_type !== 'group')) return null
    return parsed as ChatConversationTarget
  } catch {
    return null
  }
}

export function reportChatUnreadChanged() {
  window.dispatchEvent(new CustomEvent(CHAT_UNREAD_CHANGED_EVENT))
}
