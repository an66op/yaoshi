import type { ChatMessage } from '../api/chat'
import type { DrawResult } from '../api/lottery'
import type { GameFeedItem } from '../api/portal'
import { recentGameTimelineItems } from './gameTimelineBudget'

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
  | { kind: 'feed'; key: string; at: number; value: GameFeedItem; index: number }
  | { kind: 'ticket'; key: string; at: number; value: AcceptedTicket }

export function ticketsForGame(tickets: AcceptedTicket[], gameId: string) {
  return tickets.filter(ticket => ticket.gameId === gameId)
}

const incompleteBetPattern = /^(?:买)?(?:冠军|亚军|第[三四五六七八九十]名|前三|中三|后三|前五|后五|冠亚(?:和)?)?[0-9大小单双龙虎和豹子顺对半杂六#\s,，.]*$/
const applicationCommandPattern = /^(?:申请)?\s*(?:上分|下分)\s*[/：:]?\s*([0-9]+(?:\.[0-9]{1,2})?)(?:\s+.*)?$/

/** Repeat is an editable draft, not another server command. Keep financial
 * applications and query/cancel/chat text out of the remembered bet input. */
export function isRepeatableBetInput(content: string) {
  const normalized = content.trim()
  if (!normalized || /^(?:申请)?\s*(?:上分|下分)/.test(normalized)) return false
  return isRoomCommandContent(normalized) && (normalized.includes('/') || normalized.includes('梭哈'))
}

export function latestBetInput(messages: ChatMessage[], tickets: AcceptedTicket[], gameId: string) {
  const inputs = [
    ...messages.filter(message => message.mine && message.game_id === gameId).map(message => ({ content: message.content, at: timelineTime(message.created_at), id: message.id })),
    ...tickets.filter(ticket => ticket.gameId === gameId).map(ticket => ({ content: ticket.content, at: timelineTime(ticket.acceptedAt), id: 0 })),
  ].filter(input => isRepeatableBetInput(input.content))
  inputs.sort((left, right) => right.at - left.at || right.id - left.id)
  return inputs[0]?.content.trim() ?? ''
}

export type RoomKeyboardShortcut = 'all-in' | 'cancel' | 'credit' | 'check' | 'debit' | 'repeat'

/** Shortcuts only change the editable input. Sending remains a separate,
 * explicit action, including for repeat and credit/debit applications. */
export function keyboardShortcutInput(action: RoomKeyboardShortcut, current: string, previous: string) {
  if (action === 'cancel') return '取消'
  if (action === 'credit') return '上分 '
  if (action === 'debit') return '下分 '
  if (action === 'check') return '查'
  if (action === 'repeat') return isRepeatableBetInput(previous) ? previous : current
  return current.endsWith('梭哈') ? current : `${current}梭哈`
}

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
    || (/[0-9大小单双龙虎和冠亚军第名豹子顺对半杂六]/.test(normalized) && incompleteBetPattern.test(normalized))
}

/** Query/cancel/financial-service commands remain available in read-only games. */
export function isBettingCommandContent(content: string) {
  const normalized = content.trim()
  if (normalized === '查' || normalized === '取消' || applicationCommandPattern.test(normalized)) return false
  return isRoomCommandContent(normalized)
}

/** Keep durable audit text untouched; compact only accepted-ticket display. */
export function compactAcceptedReceiptContent(content: string) {
  const lines = content.split('\n')
  const titleIndex = lines[0]?.startsWith('@') ? 1 : 0
  if (!/^【[^】]+】下单成功$/.test(lines[titleIndex] ?? '')) return content
  return lines.map((line, index) => {
    if (index <= titleIndex) return line
    return line.replace(/[ \t]*·[ \t]*赔率[ \t]+\d+(?:\.\d+)?[ \t]*$/, '')
      .replace(/\[[^\]\n]*\]/g, group => group.replace(/(\/\d+)\.00(?=[\s\]])/g, '$1'))
      .replace(/^(使用[：:]\s*\d+)\.00(\s*)$/, '$1$2')
  }).join('\n')
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

/** A range image belongs to its own announcement. Never include a later
 * result just because the room has advanced since this message was sent. */
export function drawHistoryAtIssue(draws: DrawResult[], draw: DrawResult) {
  const byIssue = new Map<string, DrawResult>()
  for (const row of draws) {
    if (row.game_id !== draw.game_id || !row.numbers.length) continue
    if (timelineTime(row.draw_at) > timelineTime(draw.draw_at)) continue
    if (!byIssue.has(row.issue)) byIssue.set(row.issue, row)
  }
  byIssue.set(draw.issue, draw)
  return [...byIssue.values()].sort((left, right) => timelineTime(right.draw_at) - timelineTime(left.draw_at) || right.issue.localeCompare(left.issue, 'en', { numeric: true }))
}

export function buildGameTimelineEntries({ gameId, messages, draws = [], feed, tickets, startAt, anchorIssue }: {
  gameId: string
  messages: ChatMessage[]
  draws?: DrawResult[]
  feed: GameFeedItem[]
  tickets: AcceptedTicket[]
  startAt?: number
  anchorIssue?: string
}) {
  const entries: GameTimelineEntry[] = []
  messages.filter(message => message.game_id === gameId).forEach(message => entries.push({ kind: 'chat', key: `chat:${message.id}`, at: timelineTime(message.created_at), value: message }))
  feed.forEach((item, index) => entries.push({ kind: 'feed', key: `feed:${item.issue ?? ''}:${item.created_at}:${item.nickname}:${item.detail}:${item.amount}`, at: timelineTime(item.created_at), value: item, index }))
  ticketsForGame(tickets, gameId).forEach(ticket => entries.push({ kind: 'ticket', key: `ticket:${ticket.issue}:${ticket.acceptedAt}:${ticket.content}`, at: timelineTime(ticket.acceptedAt), value: ticket }))
  const byIssue = new Map<string, DrawResult>()
  draws.forEach(draw => {
    if (draw.game_id === gameId && draw.numbers.length && !byIssue.has(draw.issue)) byIssue.set(draw.issue, draw)
  })
  byIssue.forEach(draw => entries.push({ kind: 'draw', key: `draw:${gameId}:${draw.issue}`, at: timelineTime(draw.draw_at), value: draw }))
  // An announcement is first when settlement/chat shares its timestamp. The
  // fixed entry boundary applies to every message source, not only draw cards.
  const priority: Record<GameTimelineEntry['kind'], number> = { draw: 0, chat: 1, feed: 2, ticket: 3 }
  const isAnchor = (entry: GameTimelineEntry) => entry.kind === 'draw' && entry.value.issue === anchorIssue
  const ordered = entries.filter(entry => isAnchor(entry) || startAt === undefined || entry.at >= startAt)
    .sort((left, right) => Number(isAnchor(right)) - Number(isAnchor(left)) || left.at - right.at || priority[left.kind] - priority[right.kind]
      || (left.kind === 'chat' && right.kind === 'chat' ? left.value.id - right.value.id : left.key.localeCompare(right.key)))
  const recent = recentGameTimelineItems(ordered)
  if (recent === ordered) return ordered
  // A busy room must not lose its most recent confirmed result just because
  // the next issue receives hundreds of messages. Reserve one display slot
  // for that announcement, inside (not in addition to) the shared budget.
  const latestDraw = ordered.reduce<GameTimelineEntry | undefined>((latest, entry) => entry.kind === 'draw' && (!latest || entry.at >= latest.at) ? entry : latest, undefined)
  return latestDraw && !recent.includes(latestDraw) ? [latestDraw, ...recent.slice(1)] : recent
}
