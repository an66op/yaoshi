import type { WebBetBatchItem } from '../api/bets'
import type { GameOdds, OddsItem } from '../api/portal'
import { boardAmountCents } from './fullBetSelection'
import { markSixWave, markSixZodiac } from './lotteryRules'

export type MarkSixCategoryID =
  | 'special_a' | 'special_b' | 'two_sided' | 'head_tail' | 'regular'
  | 'regular_1_6' | 'regular_special' | 'color_wave' | 'zodiac_tail'
  | 'link_zodiac' | 'link_tail' | 'link_number' | 'other'

export type MarkSixPositionMode = 'special' | 'regular' | 'regular-position' | 'none'
export type MarkSixOptionKind = 'number' | 'value'

export type MarkSixOption = Readonly<{
  value: string
  label: string
  playCode?: string
  playName?: string
}>
export type MarkSixMarketSpec = Readonly<{
  id: string
  category: MarkSixCategoryID
  label: string
  playCode: string | null
  playName: string
  positionMode: MarkSixPositionMode
  optionKind: MarkSixOptionKind
  options: readonly MarkSixOption[]
  pickCount: number
  blockedReason?: string
}>

export type MarkSixTicket = Readonly<{
  marketId: string
  marketLabel: string
  playCode: string
  playName: string
  position: number
  selection: string
  selectionLabel: string
}>

export const markSixCategories: readonly Readonly<{ id: MarkSixCategoryID; label: string }>[] = [
  { id: 'special_a', label: '特码A' },
  { id: 'special_b', label: '特码B' },
  { id: 'two_sided', label: '两面' },
  { id: 'head_tail', label: '头尾数' },
  { id: 'regular', label: '正码' },
  { id: 'regular_1_6', label: '正码1–6' },
  { id: 'regular_special', label: '正码特' },
  { id: 'color_wave', label: '色波' },
  { id: 'zodiac_tail', label: '一肖尾数' },
  { id: 'link_zodiac', label: '连肖' },
  { id: 'link_tail', label: '连尾' },
  { id: 'link_number', label: '连码' },
  { id: 'other', label: '其他' },
]

export const markSixNumbers: readonly MarkSixOption[] = Array.from({ length: 49 }, (_, index) => ({
  value: String(index + 1),
  label: String(index + 1).padStart(2, '0'),
}))

export const markSixZodiacs: readonly MarkSixOption[] = [...'鼠牛虎兔龙蛇马羊猴鸡狗猪'].map(value => ({ value, label: value }))
export const markSixTails: readonly MarkSixOption[] = Array.from({ length: 10 }, (_, index) => ({ value: String(index), label: `${index}尾` }))

const values = (...items: string[]): readonly MarkSixOption[] => items.map(value => ({ value, label: value }))
type MarkSixAtomicOption = MarkSixOption & Readonly<{ playCode: string; playName: string }>
const atomicOption = (value: string, playCode: string, playName: string): MarkSixAtomicOption => ({ value, label: value, playCode, playName })
const markSixWaveColors = [
  { code: 'red', label: '红' },
  { code: 'blue', label: '蓝' },
  { code: 'green', label: '绿' },
] as const
const markSixWaveSides = [
  { code: 'big', label: '大' },
  { code: 'small', label: '小' },
  { code: 'odd', label: '单' },
  { code: 'even', label: '双' },
] as const
const markSixWaveSizes = markSixWaveSides.slice(0, 2)
const markSixWaveParities = markSixWaveSides.slice(2)

const markSixColorWaveOptions: readonly MarkSixAtomicOption[] = markSixWaveColors.map(color => {
  const value = `${color.label}波`
  return atomicOption(value, `marksix_color_wave_${color.code}`, value)
})
const markSixHalfWaveOptions: readonly MarkSixAtomicOption[] = markSixWaveColors.flatMap(color => markSixWaveSides.map(side => {
  const value = `${color.label}${side.label}`
  return atomicOption(value, `marksix_half_wave_${color.code}_${side.code}`, value)
}))
const markSixHalfHalfWaveOptions: readonly MarkSixAtomicOption[] = markSixWaveColors.flatMap(color => markSixWaveSizes.flatMap(size => markSixWaveParities.map(parity => {
  const value = `${color.label}${size.label}${parity.label}`
  return atomicOption(value, `marksix_halfhalf_${color.code}_${size.code}_${parity.code}`, value)
})))
const markSixRegularColorOptions: readonly MarkSixAtomicOption[] = markSixWaveColors.map(color => {
  const value = `${color.label}波`
  return atomicOption(value, `marksix_regular_color_${color.code}`, `正码1-6${value}`)
})
const markSixSpecialHeadOptions: readonly MarkSixAtomicOption[] = Array.from({ length: 5 }, (_, index) => {
  const value = `${index}头`
  return atomicOption(value, `marksix_special_head_${index}`, `特码${value}`)
})
const markSixSpecialTailOptions: readonly MarkSixAtomicOption[] = Array.from({ length: 10 }, (_, index) => {
  const value = `${index}尾`
  return atomicOption(value, `marksix_special_tail_${index}`, `特码${value}`)
})
const markSixFiveElementOptions: readonly MarkSixAtomicOption[] = [
  { code: 'metal', label: '金' },
  { code: 'wood', label: '木' },
  { code: 'water', label: '水' },
  { code: 'fire', label: '火' },
  { code: 'earth', label: '土' },
].map(element => atomicOption(element.label, `marksix_five_element_${element.code}`, `五行${element.label}`))
const markSixAtomicOptions: readonly MarkSixAtomicOption[] = [
  ...markSixColorWaveOptions,
  ...markSixHalfWaveOptions,
  ...markSixHalfHalfWaveOptions,
  ...markSixRegularColorOptions,
  ...markSixSpecialHeadOptions,
  ...markSixSpecialTailOptions,
  ...markSixFiveElementOptions,
]
const market = (
  id: string,
  category: MarkSixCategoryID,
  label: string,
  playCode: string | null,
  playName: string,
  options: readonly MarkSixOption[],
  extras: Partial<Pick<MarkSixMarketSpec, 'positionMode' | 'optionKind' | 'pickCount' | 'blockedReason'>> = {},
): MarkSixMarketSpec => ({
  id, category, label, playCode, playName, options,
  positionMode: extras.positionMode ?? 'none',
  optionKind: extras.optionKind ?? 'value',
  pickCount: extras.pickCount ?? 1,
  blockedReason: extras.blockedReason,
})

const unverified = '赔率维度/结算规则待核验'

export const markSixEnabledPlayCodes = [
  'marksix_special_a_number',
  'marksix_special_b_number',
  'marksix_special_big_small',
  'marksix_special_odd_even',
  'marksix_special_sum_big_small',
  'marksix_special_sum_odd_even',
  'marksix_special_heaven_earth',
  'marksix_special_front_back',
  'marksix_special_domestic_wild',
  'marksix_special_tail_big_small',
  'marksix_special_half',
  'marksix_total_big_small',
  'marksix_total_odd_even',
  'marksix_special_zodiac',
  'marksix_regular_number',
  'marksix_regular_position_number',
  'marksix_regular_position_big_small',
  'marksix_regular_position_odd_even',
  'marksix_regular_position_sum_big_small',
  'marksix_regular_position_sum_odd_even',
  'marksix_regular_position_tail_big_small',
  'marksix_regular_special_number',
  'marksix_combo_4_all',
  'marksix_combo_3_all',
  'marksix_combo_2_all',
  'marksix_combo_special_pair',
  'marksix_not_in',
  ...markSixAtomicOptions.map(option => option.playCode),
] as const
const markSixEnabledPlayCodeSet = new Set<string>(markSixEnabledPlayCodes)

export function isMarkSixEnabledPlayCode(playCode: string): boolean {
  return markSixEnabledPlayCodeSet.has(playCode)
}

export const markSixMarkets: readonly MarkSixMarketSpec[] = [
  market('special_a_number', 'special_a', '特码A号码', 'marksix_special_a_number', '特码A', markSixNumbers, { positionMode: 'special', optionKind: 'number' }),
  market('special_b_number', 'special_b', '特码B号码', 'marksix_special_b_number', '特码B', markSixNumbers, { positionMode: 'special', optionKind: 'number' }),

  market('special_big_small', 'two_sided', '特码大小', 'marksix_special_big_small', '特码大小', values('大', '小'), { positionMode: 'special' }),
  market('special_odd_even', 'two_sided', '特码单双', 'marksix_special_odd_even', '特码单双', values('单', '双'), { positionMode: 'special' }),
  market('special_sum_big_small', 'two_sided', '特合大小', 'marksix_special_sum_big_small', '特合大小', values('合大', '合小'), { positionMode: 'special' }),
  market('special_sum_odd_even', 'two_sided', '特合单双', 'marksix_special_sum_odd_even', '特合单双', values('合单', '合双'), { positionMode: 'special' }),
  market('special_heaven_earth', 'two_sided', '特天地肖', 'marksix_special_heaven_earth', '特天地肖', values('天肖', '地肖'), { positionMode: 'special' }),
  market('special_front_back', 'two_sided', '特前后肖', 'marksix_special_front_back', '特前后肖', values('前肖', '后肖'), { positionMode: 'special' }),
  market('special_domestic_wild', 'two_sided', '特家野肖', 'marksix_special_domestic_wild', '特家野肖', values('家肖', '野肖'), { positionMode: 'special' }),
  market('special_tail_big_small', 'two_sided', '特尾大小', 'marksix_special_tail_big_small', '特尾大小', values('尾大', '尾小'), { positionMode: 'special' }),
  market('special_combo', 'two_sided', '特码半特', 'marksix_special_half', '特码半特', values('大单', '大双', '小单', '小双'), { positionMode: 'special' }),
  market('total_big_small', 'two_sided', '总和大小', 'marksix_total_big_small', '总和大小', values('总和大', '总和小')),
  market('total_odd_even', 'two_sided', '总和单双', 'marksix_total_odd_even', '总和单双', values('总和单', '总和双')),

  market('special_head', 'head_tail', '头数', null, '特码头数', markSixSpecialHeadOptions, { positionMode: 'special' }),
  market('special_tail', 'head_tail', '尾数', null, '特码尾数', markSixSpecialTailOptions, { positionMode: 'special' }),
  market('special_zodiac', 'head_tail', '特码生肖', 'marksix_special_zodiac', '特码生肖', markSixZodiacs, { positionMode: 'special' }),
  market('regular_number', 'regular', '正码号码', 'marksix_regular_number', '正码', markSixNumbers, { positionMode: 'regular', optionKind: 'number' }),
  market('regular_position_number', 'regular_1_6', '正码1–6号码', 'marksix_regular_position_number', '正码定位', markSixNumbers, { positionMode: 'regular-position', optionKind: 'number' }),
  market('regular_position_big_small', 'regular_1_6', '正码1–6大小', 'marksix_regular_position_big_small', '正码定位大小', values('大', '小'), { positionMode: 'regular-position' }),
  market('regular_position_odd_even', 'regular_1_6', '正码1–6单双', 'marksix_regular_position_odd_even', '正码定位单双', values('单', '双'), { positionMode: 'regular-position' }),
  market('regular_position_sum_big_small', 'regular_1_6', '正码1–6合大小', 'marksix_regular_position_sum_big_small', '正码定位合大小', values('合大', '合小'), { positionMode: 'regular-position' }),
  market('regular_position_sum_odd_even', 'regular_1_6', '正码1–6合单双', 'marksix_regular_position_sum_odd_even', '正码定位合单双', values('合单', '合双'), { positionMode: 'regular-position' }),
  market('regular_position_tail_big_small', 'regular_1_6', '正码1–6尾大小', 'marksix_regular_position_tail_big_small', '正码定位尾大小', values('尾大', '尾小'), { positionMode: 'regular-position' }),
  market('regular_position_wave', 'regular_1_6', '正码1–6色波', null, '正码定位色波', markSixRegularColorOptions, { positionMode: 'regular-position' }),
  market('regular_special_number', 'regular_special', '正码特号码', 'marksix_regular_special_number', '正码特', markSixNumbers, { positionMode: 'regular-position', optionKind: 'number' }),
  market('regular_special_sides', 'regular_special', '正码特两面', null, '正码特两面', values('大', '小', '单', '双', '合大', '合小', '合单', '合双', '尾大', '尾小'), { positionMode: 'regular-position', blockedReason: unverified }),
  market('regular_special_wave', 'regular_special', '正码特色波', null, '正码特色波', values('红波', '蓝波', '绿波'), { positionMode: 'regular-position', blockedReason: unverified }),

  market('color_wave', 'color_wave', '色波', null, '特码色波', markSixColorWaveOptions, { positionMode: 'special' }),
  market('half_wave', 'color_wave', '半波', null, '特码半波', markSixHalfWaveOptions, { positionMode: 'special' }),
  market('half_half_wave', 'color_wave', '半半波', null, '特码半半波', markSixHalfHalfWaveOptions, { positionMode: 'special' }),

  market('one_zodiac', 'zodiac_tail', '一肖', null, '一肖', markSixZodiacs, { blockedReason: unverified }),
  market('one_tail', 'zodiac_tail', '尾数', null, '一尾', markSixTails, { blockedReason: unverified }),
  ...([2, 3, 4, 5] as const).map(count => market(`link_zodiac_${count}`, 'link_zodiac', `${count}连肖`, null, `${count}连肖`, markSixZodiacs, { pickCount: count, blockedReason: unverified })),
  ...([2, 3, 4, 5] as const).map(count => market(`link_tail_${count}`, 'link_tail', `${count}连尾`, null, `${count}连尾`, markSixTails, { pickCount: count, blockedReason: unverified })),

  market('combo_4_all', 'link_number', '四全中', 'marksix_combo_4_all', '四全中', markSixNumbers, { optionKind: 'number', pickCount: 4 }),
  market('combo_3_all', 'link_number', '三全中', 'marksix_combo_3_all', '三全中', markSixNumbers, { optionKind: 'number', pickCount: 3 }),
  market('combo_3_2', 'link_number', '三中二', null, '三中二', markSixNumbers, { optionKind: 'number', pickCount: 3, blockedReason: unverified }),
  market('combo_2_all', 'link_number', '二全中', 'marksix_combo_2_all', '二全中', markSixNumbers, { optionKind: 'number', pickCount: 2 }),
  market('combo_2_special', 'link_number', '二中特', null, '二中特', markSixNumbers, { optionKind: 'number', pickCount: 2, blockedReason: unverified }),
  market('combo_special_pair', 'link_number', '特串', 'marksix_combo_special_pair', '特串', markSixNumbers, { optionKind: 'number', pickCount: 2 }),

  market('five_element', 'other', '五行', null, '特码五行', markSixFiveElementOptions, { positionMode: 'special' }),
  market('total_zodiac', 'other', '总肖', null, '总肖', values('2肖', '3肖', '4肖', '5肖', '6肖', '7肖', '总肖单', '总肖双'), { blockedReason: unverified }),
  market('proper_zodiac', 'other', '正肖', null, '正肖', markSixZodiacs, { blockedReason: unverified }),
  market('seven_color_wave', 'other', '七色波', null, '七色波', values('红波', '蓝波', '绿波', '和局'), { blockedReason: unverified }),
  market('not_in', 'other', '五不中', 'marksix_not_in', '五不中', markSixNumbers, { optionKind: 'number', pickCount: 5 }),
]

export function markSixMarketsForCategory(category: MarkSixCategoryID) {
  return markSixMarkets.filter(item => item.category === category)
}

export function markSixMarket(id: string) {
  return markSixMarkets.find(item => item.id === id)
}

export function markSixPosition(marketSpec: MarkSixMarketSpec, requestedPosition = 0): number | null {
  if (marketSpec.positionMode === 'special') return 7
  if (marketSpec.positionMode === 'regular' || marketSpec.positionMode === 'none') return 0
  return Number.isInteger(requestedPosition) && requestedPosition >= 1 && requestedPosition <= 6 ? requestedPosition : null
}

const optionOrder = (marketSpec: MarkSixMarketSpec, value: string) => marketSpec.options.findIndex(option => option.value === value)

export function markSixOptionPlayCode(marketSpec: MarkSixMarketSpec, optionValue: string): string | null {
  const option = marketSpec.options.find(item => item.value === optionValue)
  return option?.playCode ?? marketSpec.playCode
}

export function markSixTicketKey(ticket: Pick<MarkSixTicket, 'playCode' | 'position' | 'selection'>) {
  return `${ticket.playCode}:${ticket.position}:${ticket.selection}`
}

export function markSixSingleTicket(marketSpec: MarkSixMarketSpec, optionValue: string, requestedPosition = 0): MarkSixTicket | null {
  const option = marketSpec.options.find(item => item.value === optionValue)
  const playCode = option?.playCode ?? marketSpec.playCode
  if (marketSpec.pickCount !== 1 || marketSpec.blockedReason || !option || !playCode || !isMarkSixEnabledPlayCode(playCode)) return null
  const position = markSixPosition(marketSpec, requestedPosition)
  if (position === null) return null
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

export function markSixComboTicket(marketSpec: MarkSixMarketSpec, optionValues: readonly string[], requestedPosition = 0): MarkSixTicket | null {
  if (marketSpec.pickCount <= 1 || marketSpec.blockedReason || !marketSpec.playCode || !isMarkSixEnabledPlayCode(marketSpec.playCode)) return null
  const position = markSixPosition(marketSpec, requestedPosition)
  const unique = [...new Set(optionValues)]
  if (position === null || unique.length !== marketSpec.pickCount || unique.some(value => optionOrder(marketSpec, value) < 0)) return null
  unique.sort((left, right) => optionOrder(marketSpec, left) - optionOrder(marketSpec, right))
  const labels = unique.map(value => marketSpec.options.find(option => option.value === value)!.label)
  return {
    marketId: marketSpec.id,
    marketLabel: marketSpec.label,
    playCode: marketSpec.playCode,
    playName: marketSpec.playName,
    position,
    selection: unique.join(','),
    selectionLabel: labels.join('、'),
  }
}

export function toggleMarkSixTicket(items: readonly MarkSixTicket[], ticket: MarkSixTicket): MarkSixTicket[] {
  const key = markSixTicketKey(ticket)
  return items.some(item => markSixTicketKey(item) === key)
    ? items.filter(item => markSixTicketKey(item) !== key)
    : [...items, ticket]
}

export function toggleMarkSixTickets(items: readonly MarkSixTicket[], tickets: readonly MarkSixTicket[]): MarkSixTicket[] {
  const keys = new Set(tickets.map(markSixTicketKey))
  if (tickets.length && tickets.every(ticket => items.some(item => markSixTicketKey(item) === markSixTicketKey(ticket)))) {
    return items.filter(item => !keys.has(markSixTicketKey(item)))
  }
  const existing = new Set(items.map(markSixTicketKey))
  return [...items, ...tickets.filter(ticket => !existing.has(markSixTicketKey(ticket)))]
}

export function toggleMarkSixDraft(values: readonly string[], optionValue: string, marketSpec: MarkSixMarketSpec) {
  if (!marketSpec.options.some(option => option.value === optionValue)) return [...values]
  if (values.includes(optionValue)) return values.filter(value => value !== optionValue)
  if (values.length >= marketSpec.pickCount) return [...values]
  return [...values, optionValue].sort((left, right) => optionOrder(marketSpec, left) - optionOrder(marketSpec, right))
}

export type MarkSixNumberFilterID = 'all' | 'red' | 'blue' | 'green' | 'big' | 'small' | 'odd' | 'even' | 'sum_odd' | 'sum_even' | 'domestic' | 'wild'
export const markSixNumberFilters: readonly Readonly<{ id: MarkSixNumberFilterID; label: string }>[] = [
  { id: 'all', label: '全选' }, { id: 'red', label: '红波' }, { id: 'blue', label: '蓝波' }, { id: 'green', label: '绿波' },
  { id: 'big', label: '大' }, { id: 'small', label: '小' }, { id: 'odd', label: '单' }, { id: 'even', label: '双' },
  { id: 'sum_odd', label: '合单' }, { id: 'sum_even', label: '合双' },
  { id: 'domestic', label: '家禽' }, { id: 'wild', label: '野兽' },
]

const domesticZodiacs = new Set(['牛', '马', '羊', '鸡', '狗', '猪'])
const wildZodiacs = new Set(['鼠', '虎', '兔', '龙', '蛇', '猴'])

export function markSixNumberFilterValues(filter: MarkSixNumberFilterID, drawAt?: string | number | Date | null): string[] {
  return markSixNumbers.filter(option => {
    const number = Number(option.value)
    if (filter === 'all') return true
    if (filter === 'red' || filter === 'blue' || filter === 'green') return markSixWave(number) === filter
    if (filter === 'domestic' || filter === 'wild') {
      const zodiac = markSixZodiac(number, drawAt)
      return zodiac !== null && (filter === 'domestic' ? domesticZodiacs.has(zodiac) : wildZodiacs.has(zodiac))
    }
    // 49 is the neutral special number for all two-sided attributes. It stays
    // available under “全选” and “绿波”, but those attribute shortcuts must
    // never imply that it is big/small, odd/even or sum odd/even.
    if (number === 49) return false
    if (filter === 'big') return number >= 25
    if (filter === 'small') return number <= 24
    if (filter === 'odd') return number % 2 === 1
    if (filter === 'even') return number % 2 === 0
    const digitSum = Math.floor(number / 10) + number % 10
    return filter === 'sum_odd' ? digitSum % 2 === 1 : digitSum % 2 === 0
  }).map(option => option.value)
}

export function markSixOddsItem(playCode: string, response: GameOdds | null | undefined): OddsItem | null {
  if (!response || response.rules_ready === false || !Array.isArray(response.items)) return null
  const item = response.items.find(row => row.play_code === playCode)
  if (!item) return null
  if (response.show_odds !== false && (!(typeof item.odds === 'number') || !Number.isFinite(item.odds) || item.odds <= 1)) return null
  return item
}

export function markSixBatchItems(tickets: readonly MarkSixTicket[], amount: string): WebBetBatchItem[] {
  const cents = boardAmountCents(amount)
  if (cents === null || !tickets.length || tickets.length > 200 || !Number.isSafeInteger(cents * tickets.length)) return []
  if (tickets.some(ticket => !isMarkSixEnabledPlayCode(ticket.playCode))) return []
  if (new Set(tickets.map(markSixTicketKey)).size !== tickets.length) return []
  return tickets.map(ticket => ({
    play_code: ticket.playCode,
    play_name: ticket.playName,
    position: ticket.position,
    selection: ticket.selection,
    amount: cents / 100,
  }))
}

export function markSixBatchError(tickets: readonly MarkSixTicket[], amount: string, response: GameOdds | null | undefined): string {
  const cents = boardAmountCents(amount)
  if (cents === null) return '金额须大于 0，最多 2 位小数。'
  if (!tickets.length) return ''
  if (tickets.length > 200) return '每张最多200注。'
  if (tickets.some(ticket => !isMarkSixEnabledPlayCode(ticket.playCode))) return '本次清单含有未通过规则核验的玩法。'
  if (!Number.isSafeInteger(cents * tickets.length)) return '本次投注金额超出安全范围。'
  for (const ticket of tickets) {
    const quote = markSixOddsItem(ticket.playCode, response)
    if (!quote) return `${ticket.marketLabel}赔率待配置。`
    const amountValue = cents / 100
    if (quote.min_bet > 0 && amountValue < quote.min_bet) return `${ticket.marketLabel}单注最低 ${quote.min_bet}。`
    if (quote.max_bet > 0 && amountValue > quote.max_bet) return `${ticket.marketLabel}单注最高 ${quote.max_bet}。`
  }
  return ''
}

export function markSixTicketGroups(tickets: readonly MarkSixTicket[]) {
  const groups = new Map<string, MarkSixTicket[]>()
  for (const ticket of tickets) {
    const key = `${ticket.marketId}:${ticket.position}`
    groups.set(key, [...(groups.get(key) ?? []), ticket])
  }
  return [...groups.entries()].map(([key, choices]) => ({
    key,
    label: choices[0].position >= 1 && choices[0].position <= 6 ? `正${choices[0].position} · ${choices[0].marketLabel}` : choices[0].marketLabel,
    choices,
  }))
}
