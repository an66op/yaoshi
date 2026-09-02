import type { WebBetBatchItem } from '../api/bets'
import type { GameOdds, OddsItem } from '../api/portal'
import { boardAmountCents } from './fullBetSelection'
import { isPC28RuleVersion, pc28RuleVersionForGame } from './lotteryRules'

export type PC28CategoryID = 'sum' | 'package' | 'position' | 'dragon' | 'mixed' | 'shape' | 'color'
export type PC28PositionMode = 'none' | 'ball' | 'dragon'
export type PC28OptionKind = 'number' | 'value'

export type PC28Option = Readonly<{
  value: string
  label: string
  playCode?: string
  playName?: string
}>

export type PC28MarketSpec = Readonly<{
  id: string
  category: PC28CategoryID
  label: string
  playCode: string | null
  playName: string
  positionMode: PC28PositionMode
  optionKind: PC28OptionKind
  options: readonly PC28Option[]
  pickCount: number
}>

export type PC28Ticket = Readonly<{
  marketId: string
  marketLabel: string
  playCode: string
  playName: string
  position: number
  selection: string
  selectionLabel: string
}>

export const pc28Categories: readonly Readonly<{ id: PC28CategoryID; label: string }>[] = [
  { id: 'sum', label: '和值' },
  { id: 'package', label: '包三' },
  { id: 'position', label: '三球定位' },
  { id: 'dragon', label: '龙虎和' },
  { id: 'mixed', label: '混合' },
  { id: 'shape', label: '形态' },
  { id: 'color', label: '色波' },
]

const values = (...items: string[]): readonly PC28Option[] => items.map(value => ({ value, label: value }))
const atomic = (value: string, playCode: string, playName = value): PC28Option => ({ value, label: value, playCode, playName })

/** 0/27 through 13/14 share the fourteen symmetric odds bands. */
export function pc28SumExactPlayCode(value: number): string | null {
  if (!Number.isInteger(value) || value < 0 || value > 27) return null
  return `pc28_sum_exact_${Math.min(value, 27 - value)}_${Math.max(value, 27 - value)}`
}

export const pc28SumOptions: readonly PC28Option[] = Array.from({ length: 28 }, (_, value) => ({
  value: String(value),
  label: String(value),
  playCode: pc28SumExactPlayCode(value)!,
  playName: `和值${value}`,
}))

export const pc28DigitOptions: readonly PC28Option[] = Array.from({ length: 10 }, (_, value) => ({ value: String(value), label: String(value) }))
export const pc28PackageOptions: readonly PC28Option[] = Array.from({ length: 28 }, (_, value) => ({ value: String(value), label: String(value) }))

const market = (
  id: string,
  category: PC28CategoryID,
  label: string,
  playCode: string | null,
  playName: string,
  options: readonly PC28Option[],
  extras: Partial<Pick<PC28MarketSpec, 'positionMode' | 'optionKind' | 'pickCount'>> = {},
): PC28MarketSpec => ({
  id,
  category,
  label,
  playCode,
  playName,
  options,
  positionMode: extras.positionMode ?? 'none',
  optionKind: extras.optionKind ?? 'value',
  pickCount: extras.pickCount ?? 1,
})

export const pc28Markets: readonly PC28MarketSpec[] = [
  market('sum_exact', 'sum', '和值0–27', null, '和值', pc28SumOptions, { optionKind: 'number' }),
  market('package_three', 'package', '特码包三', 'pc28_package_three', '特码包三', pc28PackageOptions, { optionKind: 'number', pickCount: 3 }),
  market('position_number', 'position', '球位号码', 'pc28_position_number', '三球定位号码', pc28DigitOptions, { positionMode: 'ball', optionKind: 'number' }),
  market('position_two_sided', 'position', '球位两面', 'pc28_position_two_sided', '三球定位两面', values('大', '小', '单', '双'), { positionMode: 'ball' }),
  market('dragon_tiger', 'dragon', '第一球 ↔ 第三球', 'pc28_dragon_tiger', '第一球对第三球龙虎和', [
    { value: '龙', label: '龙' },
    { value: '虎', label: '虎' },
    atomic('和', 'pc28_dragon_tiger_tie', '第一球对第三球和'),
  ], { positionMode: 'dragon' }),
  market('sum_size', 'mixed', '和值大小', 'pc28_sum_size', '和值大小', values('大', '小')),
  market('sum_parity', 'mixed', '和值单双', 'pc28_sum_parity', '和值单双', values('单', '双')),
  market('sum_combo', 'mixed', '和值组合', null, '和值组合', [
    atomic('大单', 'pc28_combo_big_odd'),
    atomic('大双', 'pc28_combo_big_even'),
    atomic('小单', 'pc28_combo_small_odd'),
    atomic('小双', 'pc28_combo_small_even'),
  ]),
  market('extreme', 'mixed', '极值', 'pc28_extreme', '极值', values('极大', '极小')),
  market('shape', 'shape', '三球形态', null, '三球形态', [
    atomic('豹子', 'pc28_leopard'),
    atomic('对子', 'pc28_pair'),
    atomic('顺子', 'pc28_straight'),
  ]),
  market('color', 'color', '色波', null, '色波', [
    atomic('红波', 'pc28_color_red'),
    atomic('绿波', 'pc28_color_green'),
    atomic('蓝波', 'pc28_color_blue'),
  ]),
]

const enabledPlayCodes = new Set<string>([
  ...pc28SumOptions.map(option => option.playCode!),
  'pc28_package_three',
  'pc28_position_number',
  'pc28_position_two_sided',
  'pc28_dragon_tiger',
  'pc28_dragon_tiger_tie',
  'pc28_sum_size',
  'pc28_sum_parity',
  'pc28_combo_big_odd',
  'pc28_combo_big_even',
  'pc28_combo_small_odd',
  'pc28_combo_small_even',
  'pc28_extreme',
  'pc28_color_red',
  'pc28_color_green',
  'pc28_color_blue',
  'pc28_leopard',
  'pc28_pair',
  'pc28_straight',
])

export function isPC28EnabledPlayCode(playCode: string) {
  return enabledPlayCodes.has(playCode)
}

export function pc28MarketsForCategory(category: PC28CategoryID) {
  return pc28Markets.filter(item => item.category === category)
}

export function pc28OptionPlayCode(marketSpec: PC28MarketSpec, optionValue: string): string | null {
  const option = marketSpec.options.find(item => item.value === optionValue)
  return option?.playCode ?? marketSpec.playCode
}

function pc28Position(marketSpec: PC28MarketSpec, requestedPosition: number): number | null {
  if (marketSpec.positionMode === 'none') return 0
  if (marketSpec.positionMode === 'dragon') return 1
  return Number.isInteger(requestedPosition) && requestedPosition >= 1 && requestedPosition <= 3 ? requestedPosition : null
}

export function pc28TicketKey(ticket: Pick<PC28Ticket, 'playCode' | 'position' | 'selection'>) {
  return `${ticket.playCode}:${ticket.position}:${ticket.selection}`
}

export function pc28SingleTicket(marketSpec: PC28MarketSpec, optionValue: string, requestedPosition = 0): PC28Ticket | null {
  if (marketSpec.pickCount !== 1) return null
  const option = marketSpec.options.find(item => item.value === optionValue)
  const playCode = option?.playCode ?? marketSpec.playCode
  const position = pc28Position(marketSpec, requestedPosition)
  if (!option || !playCode || position === null || !isPC28EnabledPlayCode(playCode)) return null
  return {
    marketId: marketSpec.id,
    marketLabel: marketSpec.label,
    playCode,
    playName: option.playName ?? marketSpec.playName,
    position,
    selection: option.value,
    selectionLabel: option.label,
  }
}

export function pc28PackageTicket(marketSpec: PC28MarketSpec, valuesToPack: readonly string[]): PC28Ticket | null {
  if (marketSpec.id !== 'package_three' || marketSpec.pickCount !== 3 || !marketSpec.playCode) return null
  const normalized = [...new Set(valuesToPack.map(Number))].sort((left, right) => left - right)
  if (normalized.length !== 3 || normalized.some(value => !Number.isInteger(value) || value < 0 || value > 27)) return null
  const selection = normalized.join(',')
  return {
    marketId: marketSpec.id,
    marketLabel: marketSpec.label,
    playCode: marketSpec.playCode,
    playName: marketSpec.playName,
    position: 0,
    selection,
    selectionLabel: selection.replaceAll(',', '、'),
  }
}

export function togglePC28Draft(current: readonly string[], value: string, maximum = 3): string[] {
  if (current.includes(value)) return current.filter(item => item !== value)
  if (current.length >= maximum) return [...current]
  return [...current, value].sort((left, right) => Number(left) - Number(right))
}

export function togglePC28Ticket(current: readonly PC28Ticket[], ticket: PC28Ticket): PC28Ticket[] {
  const key = pc28TicketKey(ticket)
  return current.some(item => pc28TicketKey(item) === key)
    ? current.filter(item => pc28TicketKey(item) !== key)
    : [...current, ticket]
}

const oppositePC28Selection = (marketId: string, selection: string) => {
  if (marketId === 'sum_size') return selection === '大' ? '小' : selection === '小' ? '大' : ''
  if (marketId === 'sum_parity') return selection === '单' ? '双' : selection === '双' ? '单' : ''
  return ''
}

/** Client-side guidance for constraints the server rechecks across the period. */
export function pc28TicketAddError(current: readonly PC28Ticket[], ticket: PC28Ticket, ruleVersion: string): string {
  if (ticket.marketId === 'sum_exact') {
    const points = new Set(current.filter(item => item.marketId === 'sum_exact').map(item => item.selection))
    if (!points.has(ticket.selection) && points.size >= 10) return '同一会员每期最多投注10个不同的PC28单点和值。'
  }
  if (ruleVersion === 'pc28-v1' || ruleVersion === 'pc28-v2') {
    const opposite = oppositePC28Selection(ticket.marketId, ticket.selection)
    if (opposite && current.some(item => item.marketId === ticket.marketId && item.selection === opposite)) {
      return '玩法一、二禁止同一期在和值大小或和值单双市场反向下注；球位定位两面不在此限制内。'
    }
  }
  return ''
}

export function pc28OddsItem(gameId: string, playCode: string, response: GameOdds | null | undefined): OddsItem | null {
  if (!response || response.rules_ready === false || !isPC28RuleVersion(gameId, response.rule_version) || !Array.isArray(response.items)) return null
  const item = response.items.find(row => row.play_code === playCode)
  if (!item) return null
  if (response.show_odds !== false && (typeof item.odds !== 'number' || !Number.isFinite(item.odds) || item.odds <= 1)) return null
  return item
}

export function pc28BatchItems(tickets: readonly PC28Ticket[], amount: string): WebBetBatchItem[] {
  const cents = boardAmountCents(amount)
  if (cents === null || !tickets.length || tickets.length > 200 || !Number.isSafeInteger(cents * tickets.length)) return []
  if (tickets.some(ticket => !isPC28EnabledPlayCode(ticket.playCode))) return []
  if (new Set(tickets.map(pc28TicketKey)).size !== tickets.length) return []
  return tickets.map(ticket => ({
    play_code: ticket.playCode,
    play_name: ticket.playName,
    position: ticket.position,
    selection: ticket.selection,
    amount: cents / 100,
  }))
}

export function pc28BatchError(gameId: string, tickets: readonly PC28Ticket[], amount: string, response: GameOdds | null | undefined): string {
  const cents = boardAmountCents(amount)
  if (cents === null) return '金额须大于 0，最多 2 位小数。'
  if (!tickets.length) return ''
  if (tickets.length > 200) return '每张最多200注。'
  if (tickets.some(ticket => !isPC28EnabledPlayCode(ticket.playCode))) return '本次清单含有未通过规则核验的玩法。'
  if (new Set(tickets.map(pc28TicketKey)).size !== tickets.length) return '本次清单含有重复注单。'
  if (!Number.isSafeInteger(cents * tickets.length)) return '本次投注金额超出安全范围。'
  const ruleVersion = pc28RuleVersionForGame(gameId) ?? ''
  const accepted: PC28Ticket[] = []
  for (const ticket of tickets) {
    const constraintError = pc28TicketAddError(accepted, ticket, ruleVersion)
    if (constraintError) return constraintError
    accepted.push(ticket)
  }
  for (const ticket of tickets) {
    const quote = pc28OddsItem(gameId, ticket.playCode, response)
    if (!quote) return `${ticket.marketLabel}赔率待配置。`
    const amountValue = cents / 100
    if (quote.min_bet > 0 && amountValue < quote.min_bet) return `${ticket.marketLabel}单注最低 ${quote.min_bet}。`
    if (quote.max_bet > 0 && amountValue > quote.max_bet) return `${ticket.marketLabel}单注最高 ${quote.max_bet}。`
  }
  return ''
}

export function pc28TicketGroups(tickets: readonly PC28Ticket[]) {
  const groups = new Map<string, PC28Ticket[]>()
  for (const ticket of tickets) {
    const key = `${ticket.marketId}:${ticket.position}`
    groups.set(key, [...(groups.get(key) ?? []), ticket])
  }
  return [...groups.entries()].map(([key, choices]) => ({
    key,
    label: choices[0].position >= 1 && choices[0].position <= 3 && choices[0].marketId.startsWith('position_')
      ? `第${['一', '二', '三'][choices[0].position - 1]}球 · ${choices[0].marketLabel}`
      : choices[0].marketLabel,
    choices,
  }))
}
