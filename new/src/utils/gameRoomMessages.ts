import type { ChatMessage } from '../api/chat'
import type { DrawResult } from '../api/lottery'
import type { GameFeedItem, MemberNotification } from '../api/portal'

export type AcceptedTicket = {
  gameId: string
  content: string
  lines: string[]
  total: number
  balance: number
  issue: string
  acceptedAt: string
}

export type GameTimelineEntry =
  | { kind: 'chat'; key: string; at: number; value: ChatMessage }
  | { kind: 'draw'; key: string; at: number; value: DrawResult }
  | { kind: 'settlement'; key: string; at: number; value: MemberNotification }
  | { kind: 'feed'; key: string; at: number; value: GameFeedItem; index: number }
  | { kind: 'ticket'; key: string; at: number; value: AcceptedTicket }

export function ticketsForGame(tickets: AcceptedTicket[], gameId: string) {
  return tickets.filter(ticket => ticket.gameId === gameId)
}

const incompleteBetPattern = /^(?:买)?(?:冠军|亚军|第[三四五六七八九十]名|冠亚(?:和)?)?[0-9大小单双龙虎#\s,，.]*$/
const applicationCommandPattern = /^(?:申请)?\s*(?:上分|下分)\s*[/：:]?\s*([0-9]+(?:\.[0-9]{1,2})?)(?:\s+.*)?$/

/** Match the server's command boundary; incomplete bet fragments still need a
 * durable parsing failure, not silent treatment as a successful chat/bet. */
export function isRoomCommandContent(content: string) {
  const normalized = content.trim()
  if (!normalized) return false
  const application = normalized.match(applicationCommandPattern)
  return ['取消', '查', '重复'].includes(normalized)
    || Boolean(application && Number(application[1]) > 0)
    || normalized.includes('/')
    || normalized.includes('梭哈')
    || (/[0-9大小单双龙虎冠亚军第名]/.test(normalized) && incompleteBetPattern.test(normalized))
}

/** Old durable receipts retain their audit text in storage. Only compact the
 * odds suffix in an accepted-ticket presentation; never rewrite other chat. */
export function compactAcceptedReceiptContent(content: string) {
  const lines = content.split('\n')
  const titleIndex = lines[0]?.startsWith('@') ? 1 : 0
  if (!/^【[^】]+】下单成功$/.test(lines[titleIndex] ?? '')) return content
  return lines.map((line, index) => index > titleIndex
    ? line.replace(/[ \t]*·[ \t]*赔率[ \t]+\d+(?:\.\d+)?[ \t]*$/, '')
    : line).join('\n')
}

export function formatGameMessageTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '刚刚'
  return date.toLocaleTimeString('zh-CN', {
    timeZone: 'Asia/Shanghai', hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
  })
}

function timelineTime(value: string) {
  const parsed = new Date(value).getTime()
  return Number.isFinite(parsed) ? parsed : 0
}

/** The timeline contains actual messages/receipts, not an automatically
 * generated second summary of the same member bet rows. Explicit querying and
 * the orders dialog keep using the member-bets API independently. */
export function buildGameTimelineEntries({ gameId, messages, draw, notices, feed, tickets }: {
  gameId: string
  messages: ChatMessage[]
  draw?: DrawResult
  notices: MemberNotification[]
  feed: GameFeedItem[]
  tickets: AcceptedTicket[]
}) {
  const entries: GameTimelineEntry[] = []
  messages.forEach(message => entries.push({ kind: 'chat', key: `chat:${message.id}`, at: timelineTime(message.created_at), value: message }))
  feed.forEach((item, index) => entries.push({ kind: 'feed', key: `feed:${item.created_at}:${item.nickname}:${item.detail}`, at: timelineTime(item.created_at), value: item, index }))
  tickets.forEach(ticket => entries.push({ kind: 'ticket', key: `ticket:${ticket.issue}:${ticket.acceptedAt}:${ticket.content}`, at: timelineTime(ticket.acceptedAt), value: ticket }))
  notices.filter(notice => notice.game_id === gameId).slice(-8).forEach(notice => entries.push({ kind: 'settlement', key: `settlement:${notice.id}`, at: timelineTime(notice.created_at), value: notice }))
  if (draw) entries.push({ kind: 'draw', key: `draw:${draw.id}`, at: timelineTime(draw.draw_at), value: draw })
  const priority: Record<GameTimelineEntry['kind'], number> = { chat: 0, feed: 1, ticket: 2, draw: 3, settlement: 4 }
  return entries.sort((left, right) => left.at - right.at || priority[left.kind] - priority[right.kind] || left.key.localeCompare(right.key))
}
