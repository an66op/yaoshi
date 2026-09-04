import { describe, expect, it } from 'vitest'
import { markSixMarket, markSixMarkets, markSixSingleTicket } from './markSixBetSelection'
import {
  markSixBoardMarkets,
  markSixBoardMarketFamily,
  markSixBoardOptionLabel,
  markSixBoardOptions,
  markSixBoardTabLabel,
  markSixBoardTabs,
  markSixBoardVariants,
  markSixOptionNumbers,
} from './markSixBoardPresentation'

const horseDraw = '2026-09-03T10:00:00Z'
const optionCount = (category: Parameters<typeof markSixBoardMarkets>[0]) =>
  markSixBoardMarkets(category, '').reduce((total, market) => total + markSixBoardOptions(market).length, 0)

describe('Mark Six reference board layout', () => {
  it('shows all 24 two-sided options together without sub-tabs', () => {
    expect(markSixBoardTabs('two_sided')).toEqual([])
    expect(optionCount('two_sided')).toBe(24)
    expect(markSixBoardMarkets('two_sided', 'special_odd_even').map(market => market.id)).toEqual([
      'special_big_small', 'special_odd_even', 'special_sum_big_small', 'special_sum_odd_even',
      'special_heaven_earth', 'special_front_back', 'special_domestic_wild', 'special_tail_big_small',
      'total_odd_even', 'total_big_small', 'special_combo',
    ])
  })

  it('keeps head and tail together without the misplaced special zodiac', () => {
    expect(markSixBoardTabs('head_tail')).toEqual([])
    expect(markSixBoardMarkets('head_tail', '').map(market => market.id)).toEqual(['special_head', 'special_tail'])
    expect(optionCount('head_tail')).toBe(15)
  })

  it('shows 13 attributes in regular 1–6 and separates the three regular-special boards', () => {
    expect(markSixBoardMarkets('regular_1_6', 'regular_position_number').map(market => market.id)).toEqual([
      'regular_position_odd_even', 'regular_position_big_small', 'regular_position_sum_odd_even',
      'regular_position_sum_big_small', 'regular_position_tail_big_small', 'regular_position_wave',
    ])
    expect(optionCount('regular_1_6')).toBe(13)
    expect(markSixBoardTabs('regular_1_6')).toEqual([])
    expect(markSixBoardMarkets('regular_special', 'regular_special_sides').map(market => market.id)).toEqual(['regular_special_sides'])
    expect(optionCount('regular_special')).toBe(49)
    expect(markSixBoardTabs('regular_special').map(market => market.id)).toEqual(['regular_special_number', 'regular_special_sides', 'regular_special_wave'])
  })

  it('reuses the same total markets after the regular numbers', () => {
    const regular = markSixBoardMarkets('regular', '')
    const sides = markSixBoardMarkets('two_sided', '')
    expect(regular.map(market => market.id)).toEqual(['regular_number', 'total_big_small', 'total_odd_even'])
    expect(optionCount('regular')).toBe(53)
    for (const market of regular.slice(1)) {
      expect(market).toBe(sides.find(item => item.id === market.id))
      expect(markSixSingleTicket(market, market.options[0].value)?.position).toBe(0)
    }
  })

  it('keeps A/B independent and their 49-number boards unchanged', () => {
    expect(markSixBoardMarkets('special_a', '').map(market => market.id)).toEqual(['special_a_number'])
    expect(markSixBoardMarkets('special_b', '').map(market => market.id)).toEqual(['special_b_number'])
    expect(optionCount('special_a')).toBe(49)
    expect(optionCount('special_b')).toBe(49)
  })

  it('uses the reference tab order and falls back within the selected category', () => {
    expect(markSixBoardTabs('other').map(market => market.id)).toEqual([
      'special_zodiac', 'combined_zodiac_2', 'five_element', 'proper_zodiac', 'total_zodiac', 'seven_color_wave', 'not_in_5',
    ])
    expect(markSixBoardTabs('link_number').map(market => market.id)).toEqual([
      'combo_3_2', 'combo_2_special', 'combo_3_all', 'combo_2_all', 'combo_special_pair', 'combo_4_all',
    ])
    expect(markSixBoardMarkets('color_wave', 'half_wave').map(market => market.id)).toEqual(['half_wave'])
    expect(markSixBoardMarkets('color_wave', 'special_a_number').map(market => market.id)).toEqual(['color_wave'])
    expect(markSixBoardMarkets('zodiac_tail', 'one_tail').map(market => market.id)).toEqual(['one_tail'])
    expect(markSixBoardTabs('link_zodiac').map(market => market.pickCount)).toEqual([2, 3, 4, 5])
    expect(markSixBoardTabs('link_tail').map(market => market.pickCount)).toEqual([2, 3, 4, 5])
    expect(markSixBoardMarketFamily('combined_zodiac_11')).toBe('combined_zodiac')
    expect(markSixBoardTabLabel(markSixMarket('combined_zodiac_2')!)).toBe('合肖')
    expect(markSixBoardVariants(markSixMarket('combined_zodiac_2')).map(market => market.pickCount)).toEqual([2, 3, 4, 5, 6, 7, 8, 9, 10, 11])
    expect(markSixBoardVariants(markSixMarket('not_in_5')).map(market => market.pickCount)).toEqual([5, 6, 7, 8, 9, 10, 11])
  })

  it('formats labels and ordering without changing canonical values or market definitions', () => {
    const before = JSON.stringify(markSixMarkets)
    expect(markSixBoardOptions(markSixMarket('special_combo')!).map(option => option.value)).toEqual(['大单', '小单', '大双', '小双'])
    expect(markSixBoardOptions(markSixMarket('regular_position_wave')!).map(option => option.value)).toEqual(['红波', '绿波', '蓝波'])
    expect(markSixBoardOptionLabel('special_big_small', '大')).toBe('特大')
    expect(markSixBoardOptionLabel('special_sum_odd_even', '合双')).toBe('特合双')
    expect(markSixBoardOptionLabel('special_heaven_earth', '天肖')).toBe('特天肖')
    expect(markSixBoardOptionLabel('special_tail_big_small', '尾大')).toBe('特大尾')
    expect(markSixBoardOptionLabel('special_tail_big_small', '尾小')).toBe('特小尾')
    expect(markSixBoardOptionLabel('regular_position_big_small', '大')).toBe('大码')
    expect(markSixBoardOptionLabel('regular_position_odd_even', '单')).toBe('单码')
    expect(markSixBoardOptionLabel('special_a_number', '1')).toBe('01')
    expect(markSixBoardOptionLabel('unknown', 'invalid')).toBe('invalid')
    expect(JSON.stringify(markSixMarkets)).toBe(before)
  })
})

describe('Mark Six informational number groups', () => {
  it('lists all 49 numbers in exactly one of the three wave groups', () => {
    expect(markSixOptionNumbers('color_wave', '红波')).toEqual([1, 2, 7, 8, 12, 13, 18, 19, 23, 24, 29, 30, 34, 35, 40, 45, 46])
    expect(markSixOptionNumbers('color_wave', '蓝波')).toEqual([3, 4, 9, 10, 14, 15, 20, 25, 26, 31, 36, 37, 41, 42, 47, 48])
    expect(markSixOptionNumbers('color_wave', '绿波')).toContain(49)
    const all = ['红波', '蓝波', '绿波'].flatMap(value => markSixOptionNumbers('color_wave', value)!)
    expect(all).toHaveLength(49)
    expect([...new Set(all)].sort((left, right) => left - right)).toEqual(Array.from({ length: 49 }, (_, index) => index + 1))
  })

  it('matches half-wave combinations and never lists neutral 49 as a win', () => {
    expect(markSixOptionNumbers('half_wave', '绿大')).toEqual([27, 28, 32, 33, 38, 39, 43, 44])
    expect(markSixOptionNumbers('half_wave', '绿单')).not.toContain(49)
    expect(markSixOptionNumbers('half_half_wave', '红小单')).toEqual([1, 7, 13, 19, 23])
    expect(markSixOptionNumbers('half_half_wave', '蓝大双')).toEqual([26, 36, 42, 48])
    for (const marketId of ['half_wave', 'half_half_wave']) {
      for (const option of markSixMarket(marketId)!.options) expect(markSixOptionNumbers(marketId, option.value)).not.toContain(49)
    }
  })

  it('maps every zodiac-style market using the draw date, including 49', () => {
    for (const marketId of ['special_zodiac', 'combined_zodiac_2', 'combined_zodiac_11', 'one_zodiac', 'proper_zodiac', 'link_zodiac_2', 'link_zodiac_3', 'link_zodiac_4', 'link_zodiac_5']) {
      expect(markSixOptionNumbers(marketId, '马', horseDraw)).toEqual([1, 13, 25, 37, 49])
      expect(markSixOptionNumbers(marketId, '鼠', horseDraw)).toEqual([7, 19, 31, 43])
    }
    expect(markSixOptionNumbers('special_zodiac', '马', '2026-02-16T15:59:59Z')).not.toContain(1)
    expect(markSixOptionNumbers('special_zodiac', '马', '2026-02-16T16:00:00Z')).toContain(1)
    for (const drawAt of [undefined, null, '', 'not-a-date', new Date(Number.NaN)]) {
      expect(markSixOptionNumbers('one_zodiac', '马', drawAt)).toEqual([])
    }
  })

  it('uses real tail digits rather than the incorrect reference 2-tail row', () => {
    for (const marketId of ['one_tail', 'link_tail_2', 'link_tail_3', 'link_tail_4', 'link_tail_5']) {
      expect(markSixOptionNumbers(marketId, '0尾')).toEqual([10, 20, 30, 40])
      expect(markSixOptionNumbers(marketId, '2尾')).toEqual([2, 12, 22, 32, 42])
      expect(markSixOptionNumbers(marketId, '9尾')).toEqual([9, 19, 29, 39, 49])
    }
  })

  it('keeps the five-element groups aligned to current mark6-v2 settlement', () => {
    expect(markSixOptionNumbers('five_element', '金')).toEqual([6, 7, 20, 21, 28, 29, 36, 37])
    expect(markSixOptionNumbers('five_element', '木')).toEqual([2, 3, 10, 11, 18, 19, 32, 33, 40, 41, 48, 49])
    expect(markSixOptionNumbers('five_element', '水')).toEqual([8, 9, 16, 17, 24, 25, 38, 39, 46, 47])
    expect(markSixOptionNumbers('five_element', '火')).toEqual([4, 5, 12, 13, 26, 27, 34, 35, 42, 43])
    expect(markSixOptionNumbers('five_element', '土')).toEqual([1, 14, 15, 22, 23, 30, 31, 44, 45])
    const all = ['金', '木', '水', '火', '土'].flatMap(value => markSixOptionNumbers('five_element', value)!)
    expect(all).toHaveLength(49)
    expect(new Set(all).size).toBe(49)
    const copy = markSixOptionNumbers('five_element', '金')!
    copy.push(49)
    expect(markSixOptionNumbers('five_element', '金')).not.toContain(49)
  })

  it('distinguishes compact options from unresolved or invalid grouped options', () => {
    for (const marketId of ['special_a_number', 'special_big_small', 'special_head', 'special_tail', 'regular_position_wave', 'total_zodiac', 'seven_color_wave', 'not_in_5', 'unknown']) {
      expect(markSixOptionNumbers(marketId, '1', horseDraw)).toBeNull()
    }
    expect(markSixOptionNumbers('color_wave', '红')).toEqual([])
    expect(markSixOptionNumbers('half_wave', '绿大单')).toEqual([])
    expect(markSixOptionNumbers('half_half_wave', '红大')).toEqual([])
    expect(markSixOptionNumbers('one_zodiac', '猫', horseDraw)).toEqual([])
    expect(markSixOptionNumbers('link_tail_2', '02')).toEqual([])
    expect(markSixOptionNumbers('five_element', 'unknown')).toEqual([])
  })
})
