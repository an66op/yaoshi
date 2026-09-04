import {
  markSixMarket,
  markSixNumbers,
  type MarkSixCategoryID,
  type MarkSixMarketSpec,
  type MarkSixOption,
} from './markSixBetSelection'
import { markSixWave, markSixZodiac, markSixZodiacOrder, type MarkSixWave } from './lotteryRules'

// A board may show several markets together without merging their play codes,
// positions or odds. The underlying market remains the source of every ticket.
const groupedMarketIds: Partial<Record<MarkSixCategoryID, readonly string[]>> = {
  special_a: ['special_a_number'],
  special_b: ['special_b_number'],
  two_sided: [
    'special_big_small', 'special_odd_even', 'special_sum_big_small', 'special_sum_odd_even',
    'special_heaven_earth', 'special_front_back', 'special_domestic_wild', 'special_tail_big_small',
    'total_odd_even', 'total_big_small', 'special_combo',
  ],
  head_tail: ['special_head', 'special_tail'],
  regular: ['regular_number', 'total_big_small', 'total_odd_even'],
  regular_1_6: [
    'regular_position_odd_even', 'regular_position_big_small',
    'regular_position_sum_odd_even', 'regular_position_sum_big_small',
    'regular_position_tail_big_small', 'regular_position_wave',
  ],
}

const tabMarketIds: Partial<Record<MarkSixCategoryID, readonly string[]>> = {
  color_wave: ['color_wave', 'half_wave', 'half_half_wave'],
  zodiac_tail: ['one_zodiac', 'one_tail'],
  link_zodiac: ['link_zodiac_2', 'link_zodiac_3', 'link_zodiac_4', 'link_zodiac_5'],
  link_tail: ['link_tail_2', 'link_tail_3', 'link_tail_4', 'link_tail_5'],
  link_number: ['combo_3_2', 'combo_2_special', 'combo_3_all', 'combo_2_all', 'combo_special_pair', 'combo_4_all'],
  regular_special: ['regular_special_number', 'regular_special_sides', 'regular_special_wave'],
  // Count-based families use one top-level entrance and a compact second row
  // for their exact count. This keeps the original seven “其他” entrances
  // readable while still exposing every configured contract.
  other: ['special_zodiac', 'combined_zodiac_2', 'five_element', 'proper_zodiac', 'total_zodiac', 'seven_color_wave', 'not_in_5'],
}

const marketsById = (ids: readonly string[]): MarkSixMarketSpec[] => ids.flatMap(id => {
  const spec = markSixMarket(id)
  return spec ? [spec] : []
})

export function markSixBoardTabs(category: MarkSixCategoryID): MarkSixMarketSpec[] {
  return marketsById(tabMarketIds[category] ?? [])
}

export function markSixBoardMarkets(category: MarkSixCategoryID, selectedMarketId: string): MarkSixMarketSpec[] {
  const grouped = groupedMarketIds[category]
  if (grouped) return marketsById(grouped)
  const selectedDirect = markSixMarket(selectedMarketId)
  if (selectedDirect?.category === category) return [selectedDirect]
  const tabs = markSixBoardTabs(category)
  const selected = tabs.find(spec => spec.id === selectedMarketId) ?? tabs[0]
  return selected ? [selected] : []
}

export function markSixBoardMarketFamily(marketId: string): string {
  if (/^combined_zodiac_\d+$/.test(marketId)) return 'combined_zodiac'
  if (/^not_in_\d+$/.test(marketId)) return 'not_in'
  return marketId
}

export function markSixBoardTabLabel(market: MarkSixMarketSpec): string {
  const family = markSixBoardMarketFamily(market.id)
  if (family === 'combined_zodiac') return '合肖'
  if (family === 'not_in') return '自选不中'
  return market.label
}

export function markSixBoardVariants(market: MarkSixMarketSpec | undefined): MarkSixMarketSpec[] {
  if (!market) return []
  const family = markSixBoardMarketFamily(market.id)
  if (family === 'combined_zodiac') return Array.from({ length: 10 }, (_, index) => markSixMarket(`combined_zodiac_${index + 2}`)).filter((item): item is MarkSixMarketSpec => Boolean(item))
  if (family === 'not_in') return Array.from({ length: 7 }, (_, index) => markSixMarket(`not_in_${index + 5}`)).filter((item): item is MarkSixMarketSpec => Boolean(item))
  return []
}

const optionOrders: Partial<Record<string, readonly string[]>> = {
  special_combo: ['大单', '小单', '大双', '小双'],
  regular_position_wave: ['红波', '绿波', '蓝波'],
}

export function markSixBoardOptions(market: MarkSixMarketSpec): readonly MarkSixOption[] {
  const order = optionOrders[market.id]
  if (!order) return market.options
  return [...market.options].sort((left, right) => {
    const leftIndex = order.indexOf(left.value)
    const rightIndex = order.indexOf(right.value)
    return (leftIndex < 0 ? order.length : leftIndex) - (rightIndex < 0 ? order.length : rightIndex)
  })
}

const specialLabelMarkets = new Set([
  'special_big_small', 'special_odd_even', 'special_sum_big_small', 'special_sum_odd_even',
  'special_heaven_earth', 'special_front_back', 'special_domestic_wild', 'special_combo',
])

export function markSixBoardOptionLabel(marketId: string, value: string): string {
  const option = markSixMarket(marketId)?.options.find(item => item.value === value)
  if (!option) return value
  if (specialLabelMarkets.has(marketId)) return `特${option.label}`
  if (marketId === 'special_tail_big_small') return value === '尾大' ? '特大尾' : '特小尾'
  if (marketId === 'regular_position_big_small' || marketId === 'regular_position_odd_even') return `${option.label}码`
  return option.label
}

const numbers = markSixNumbers.map(option => Number(option.value))
const waveLabels: Record<string, MarkSixWave> = { 红: 'red', 蓝: 'blue', 绿: 'green' }
const zodiacMarketIds = new Set([
  'special_zodiac', 'one_zodiac', 'proper_zodiac',
  'link_zodiac_2', 'link_zodiac_3', 'link_zodiac_4', 'link_zodiac_5',
])
const tailMarketIds = new Set(['one_tail', 'link_tail_2', 'link_tail_3', 'link_tail_4', 'link_tail_5'])

// These groups are tied to the current mark6-v2 settlement table in
// backend/services/mark_six_rules.go. A reference screenshot has a different
// five-element mapping; changing only the visual groups would misstate wins.
const markSixV2FiveElements: Record<string, readonly number[]> = {
  金: [6, 7, 20, 21, 28, 29, 36, 37],
  木: [2, 3, 10, 11, 18, 19, 32, 33, 40, 41, 48, 49],
  水: [8, 9, 16, 17, 24, 25, 38, 39, 46, 47],
  火: [4, 5, 12, 13, 26, 27, 34, 35, 42, 43],
  土: [1, 14, 15, 22, 23, 30, 31, 44, 45],
}

function matchesSide(number: number, side: string): boolean {
  if (side === '大') return number >= 25
  if (side === '小') return number <= 24
  if (side === '单') return number % 2 === 1
  return number % 2 === 0
}

/**
 * Informational number groups only: null means the compact option needs no
 * group; [] means a grouped option cannot currently resolve any valid numbers.
 * This function does not enable a market or determine its odds/settlement.
 */
export function markSixOptionNumbers(marketId: string, value: string, drawAt?: string | number | Date | null): number[] | null {
  if (zodiacMarketIds.has(marketId) || marketId.startsWith('combined_zodiac_')) {
    if (!markSixZodiacOrder.some(zodiac => zodiac === value)) return []
    return numbers.filter(number => markSixZodiac(number, drawAt) === value)
  }
  if (tailMarketIds.has(marketId)) {
    const match = /^([0-9])尾$/.exec(value)
    if (!match) return []
    return numbers.filter(number => number % 10 === Number(match[1]))
  }
  if (marketId === 'five_element') return [...(markSixV2FiveElements[value] ?? [])]
  if (marketId === 'color_wave') {
    const match = /^([红蓝绿])波$/.exec(value)
    return match ? numbers.filter(number => markSixWave(number) === waveLabels[match[1]]) : []
  }
  if (marketId === 'half_wave' || marketId === 'half_half_wave') {
    const match = (marketId === 'half_wave' ? /^([红蓝绿])([大小单双])$/ : /^([红蓝绿])([大小])([单双])$/).exec(value)
    if (!match) return []
    // 49 is a push for both half-wave variants, not part of a winning group.
    return numbers.filter(number => number !== 49
      && markSixWave(number) === waveLabels[match[1]]
      && match.slice(2).every(side => matchesSide(number, side)))
  }
  return null
}
