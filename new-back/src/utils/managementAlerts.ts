import type { ManagementWsEvent } from '../api'
import {
  chatTargetFromEvent,
  isInboundServiceChatEvent,
  shouldSuppressChatAlert,
  type ChatConversationTarget,
} from './chatNotifications'

export type ManagementAlertKind = 'application' | 'service'

export type ManagementAlert = {
  key: string
  groupKey: string
  kind: ManagementAlertKind
  title: string
  content: string
  path: '/applications' | '/chat' | '/lottery-chat'
  target?: ChatConversationTarget
  count: number
  audible: boolean
}

export type ManagementAlertContext = {
  role: string
  path: string
  visibility: DocumentVisibilityState
  focused: boolean
  activeChat?: ChatConversationTarget | null
}

const text = (value: unknown) => typeof value === 'string' ? value.trim() : ''
const positiveID = (value: unknown) => {
  const id = Number(value)
  return Number.isSafeInteger(id) && id > 0 ? id : 0
}

export function managementAlertFromEvent(
  event: ManagementWsEvent | null | undefined,
  context: ManagementAlertContext,
): ManagementAlert | null {
  if (!event) return null
  const data = event.data ?? {}

  if (event.type === 'application') {
    if (text(data.status) !== 'pending') return null
    if (context.role === 'admin' && text(data.request_type) === 'join') return null
    const applicationID = positiveID(data.application_id)
    if (!applicationID) return null
    return {
      key: `application:${applicationID}`,
      groupKey: 'applications',
      kind: 'application',
      title: '新申请待处理',
      content: `申请 #${applicationID} 正在等待审核`,
      path: '/applications',
      count: 1,
      audible: true,
    }
  }

  if (!isInboundServiceChatEvent(event)) return null
  const target = chatTargetFromEvent(event)
  const messageID = positiveID(data.message_id)
  if (!target || !messageID) return null
  if (shouldSuppressChatAlert(context.path, context.visibility, context.focused, context.activeChat, target)) return null

  return {
    key: `chat:${messageID}`,
    groupKey: `service:${target.scope}:${target.room_scope}:${target.game_id}`,
    kind: 'service',
    title: '客服新消息',
    content: '会员发来一条新消息',
    path: '/chat',
    target,
    count: 1,
    audible: true,
  }
}

export function mergeManagementAlertQueue(queue: ManagementAlert[], incoming: ManagementAlert, max = 6) {
  if (queue.some(item => item.key === incoming.key)) return queue
  const sameGroup = queue.findIndex(item => item.groupKey === incoming.groupKey)
  if (sameGroup >= 0) {
    const next = [...queue]
    next[sameGroup] = { ...incoming, count: queue[sameGroup].count + 1 }
    return next
  }
  return [...queue, incoming].slice(-Math.max(1, max))
}

export function shouldPlayManagementAlertSound(alert: ManagementAlert, soundEnabled: boolean, userInteracted: boolean) {
  return alert.audible && soundEnabled && userInteracted
}
