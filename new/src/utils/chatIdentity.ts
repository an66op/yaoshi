import type { ChatMessage } from '../api/chat'

export function memberFacingServiceName(value: string) {
  const name = value.trim()
  return !name || name === '群主' ? '客服' : name
}

export function serviceMessageIdentity(message: ChatMessage, serviceName: string) {
  const name = message.mine ? message.nickname : memberFacingServiceName(serviceName || message.nickname)
  const rawTitle = message.title?.trim() || message.user_title?.trim() || ''
  const title = memberFacingServiceName(rawTitle)
  return {
    name,
    title: !rawTitle || title === name ? '' : title,
    badge: message.badge?.trim() || '',
  }
}
