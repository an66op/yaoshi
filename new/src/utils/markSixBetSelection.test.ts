import { describe, expect, it } from 'vitest'
import type { GameOdds } from '../api/portal'
import {
  canSubmitMarkSixBatchItemWithOddsResponse,
  markSixBatchError,
  markSixBatchItemPricingCodes,
  markSixBatchItems,
  markSixCategories,
  markSixComboTicket,
  markSixMarket,
  markSixMarkets,
  markSixEnabledPlayCodes,
  markSixNumberFilterValues,
  markSixNumberSelectionValues,
  markSixPricingCodes,
  markSixSingleTicket,
  toggleMarkSixFilterSelection,
  toggleMarkSixManualSelection,
  type MarkSixNumberSelection,
} from './markSixBetSelection'

const coreEnabledCodes = [
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
  'marksix_regular_number',
  'marksix_regular_position_number',
  'marksix_regular_position_big_small',
  'marksix_regular_position_odd_even',
  'marksix_regular_position_sum_big_small',
  'marksix_regular_position_sum_odd_even',
  'marksix_regular_position_tail_big_small',
  'marksix_regular_special_number',
  ...[2, 3, 4, 5].map(count => `marksix_link_zodiac_${count}`),
  ...[2, 3, 4, 5].map(count => `marksix_link_tail_${count}`),
  'marksix_combo_4_all',
  'marksix_combo_3_all',
  'marksix_combo_3_2',
  'marksix_combo_2_all',
  'marksix_combo_2_special',
  'marksix_combo_special_pair',
  ...Array.from({ length: 10 }, (_, index) => `marksix_combined_zodiac_${index + 2}`),
  'marksix_not_in',
  ...Array.from({ length: 6 }, (_, index) => `marksix_not_in_${index + 6}`),
]

const colors = ['red', 'blue', 'green'] as const
const sides = ['big', 'small', 'odd', 'even'] as const
const sizes = ['big', 'small'] as const
const parities = ['odd', 'even'] as const
const atomicEnabledCodes = [
  ...colors.map(color => `marksix_color_wave_${color}`),
  ...colors.flatMap(color => sides.map(side => `marksix_half_wave_${color}_${side}`)),
  ...colors.flatMap(color => sizes.flatMap(size => parities.map(parity => `marksix_halfhalf_${color}_${size}_${parity}`))),
  ...colors.map(color => `marksix_regular_color_${color}`),
  ...Array.from({ length: 5 }, (_, index) => `marksix_special_head_${index}`),
  ...Array.from({ length: 10 }, (_, index) => `marksix_special_tail_${index}`),
  ...['metal', 'wood', 'water', 'fire', 'earth'].map(element => `marksix_five_element_${element}`),
  ...['rat', 'ox', 'tiger', 'rabbit', 'dragon', 'snake', 'horse', 'goat', 'monkey', 'rooster', 'dog', 'pig'].map(zodiac => `marksix_special_zodiac_${zodiac}`),
  ...['rat', 'ox', 'tiger', 'rabbit', 'dragon', 'snake', 'horse', 'goat', 'monkey', 'rooster', 'dog', 'pig'].map(zodiac => `marksix_one_zodiac_${zodiac}`),
  ...Array.from({ length: 10 }, (_, index) => `marksix_one_tail_${index}`),
  ...['rat', 'ox', 'tiger', 'rabbit', 'dragon', 'snake', 'horse', 'goat', 'monkey', 'rooster', 'dog', 'pig'].map(zodiac => `marksix_regular_zodiac_${zodiac}`),
  ...[2, 3, 4, 5, 6, 7].map(count => `marksix_total_zodiac_${count}`),
  'marksix_total_zodiac_odd', 'marksix_total_zodiac_even',
  ...['red', 'blue', 'green', 'draw'].map(color => `marksix_seven_color_${color}`),
]
const enabledCodes = [...coreEnabledCodes, ...atomicEnabledCodes]

const odds = (playCode = 'marksix_special_a_number', value = 48): GameOdds => ({
  game_id: 'bingo-mark-six', game_name: '宾果六合彩', show_odds: true, rules_ready: true,
  items: [{ play_code: playCode, play_name: '玩法', odds: value, min_bet: 10, max_bet: 200, max_user_period: 1000 }],
})

describe('Bingo Mark Six typed web selections', () => {
  it('keeps all requested entrances while exposing play codes only for the reviewed whitelist', () => {
    expect(markSixCategories.map(item => item.label)).toEqual([
      '特码A', '特码B', '两面', '头尾数', '正码', '正码1–6', '正码特', '色波', '一肖尾数', '连肖', '连尾', '连码', '其他',
    ])
    const optionPlayCodes = markSixMarkets.flatMap(item => item.options.flatMap(option => option.playCode ? [option.playCode] : []))
    expect(markSixMarkets.filter(item => item.playCode !== null).map(item => item.playCode)).toEqual(coreEnabledCodes)
    for (const code of atomicEnabledCodes) expect(optionPlayCodes).toContain(code)
    for (const code of optionPlayCodes) expect(markSixEnabledPlayCodes).toContain(code)
    expect(new Set(optionPlayCodes).size).toBe(113)
    expect(markSixEnabledPlayCodes).toHaveLength(160)
    expect(new Set(markSixEnabledPlayCodes)).toEqual(new Set(enabledCodes))
    for (const id of ['regular_special_sides', 'regular_special_wave', 'one_tail', 'proper_zodiac', 'special_zodiac', 'one_zodiac', 'total_zodiac', 'seven_color_wave']) {
      expect(markSixMarket(id)).toMatchObject({ playCode: null, blockedReason: undefined })
    }
    expect(markSixMarket('combo_3_2')).toMatchObject({ playCode: 'marksix_combo_3_2', blockedReason: undefined })
    expect(markSixMarket('combo_2_special')).toMatchObject({ playCode: 'marksix_combo_2_special', blockedReason: undefined })
    expect(markSixMarket('special_zodiac')).toMatchObject({ positionMode: 'special' })
    expect(markSixMarket('special_combo')).toMatchObject({ playCode: 'marksix_special_half', blockedReason: undefined })
    expect(markSixMarket('regular_position_big_small')).toMatchObject({ playCode: 'marksix_regular_position_big_small', blockedReason: undefined })
    expect(markSixMarket('regular_position_wave')).toMatchObject({ playCode: null, blockedReason: undefined })
  })

  it('uses the contracted positions and canonical ascending selections', () => {
    expect(markSixSingleTicket(markSixMarket('special_a_number')!, '18')).toMatchObject({ playCode: 'marksix_special_a_number', position: 7, selection: '18' })
    expect(markSixSingleTicket(markSixMarket('regular_number')!, '18')).toMatchObject({ playCode: 'marksix_regular_number', position: 0, selection: '18' })
    expect(markSixSingleTicket(markSixMarket('regular_position_number')!, '18', 6)).toMatchObject({ playCode: 'marksix_regular_position_number', position: 6, selection: '18' })
    expect(markSixSingleTicket(markSixMarket('special_domestic_wild')!, '家肖')).toMatchObject({ playCode: 'marksix_special_domestic_wild', position: 7, selection: '家肖' })
    expect(markSixSingleTicket(markSixMarket('special_zodiac')!, '马')).toMatchObject({ playCode: 'marksix_special_zodiac_horse', playName: '特肖马', position: 7, selection: '马' })
    expect(markSixSingleTicket(markSixMarket('one_zodiac')!, '马')).toMatchObject({ playCode: 'marksix_one_zodiac_horse', playName: '一肖马', position: 0, selection: '马' })
    expect(markSixSingleTicket(markSixMarket('total_zodiac')!, '5肖')).toMatchObject({ playCode: 'marksix_total_zodiac_5', playName: '总肖5肖', position: 0, selection: '5肖' })
    expect(markSixSingleTicket(markSixMarket('total_zodiac')!, '总肖单')).toMatchObject({ playCode: 'marksix_total_zodiac_odd', playName: '总肖单', position: 0, selection: '总肖单' })
    expect(markSixSingleTicket(markSixMarket('seven_color_wave')!, '和局')).toMatchObject({ playCode: 'marksix_seven_color_draw', playName: '七色波和局', position: 0, selection: '和局' })
    expect(markSixSingleTicket(markSixMarket('total_big_small')!, '总和大')).toMatchObject({ playCode: 'marksix_total_big_small', position: 0, selection: '总和大' })
    expect(markSixSingleTicket(markSixMarket('regular_position_tail_big_small')!, '尾小', 4)).toMatchObject({ playCode: 'marksix_regular_position_tail_big_small', position: 4, selection: '尾小' })
    expect(markSixSingleTicket(markSixMarket('regular_special_number')!, '18', 0)).toBeNull()
    expect(markSixComboTicket(markSixMarket('combo_3_all')!, ['18', '1', '7'])).toMatchObject({ position: 0, selection: '1,7,18' })
    expect(markSixComboTicket(markSixMarket('not_in_5')!, ['49', '1', '18', '7', '30'])).toMatchObject({ selection: '1,7,18,30,49' })
    expect(markSixComboTicket(markSixMarket('not_in_5')!, ['1', '7', '18', '30'])).toBeNull()
    expect(markSixComboTicket(markSixMarket('combined_zodiac_3')!, ['马', '鼠', '牛'])).toMatchObject({ playCode: 'marksix_combined_zodiac_3', position: 7, selection: '鼠,牛,马' })
    expect(markSixComboTicket(markSixMarket('link_tail_2')!, ['9尾', '0尾'])).toMatchObject({ playCode: 'marksix_link_tail_2', selection: '0尾,9尾' })
  })

  it('maps every atomic option to its exact server code, name, position and selection token', () => {
    expect(markSixSingleTicket(markSixMarket('color_wave')!, '红波')).toMatchObject({ playCode: 'marksix_color_wave_red', playName: '红波', position: 7, selection: '红波' })
    expect(markSixSingleTicket(markSixMarket('half_wave')!, '绿双')).toMatchObject({ playCode: 'marksix_half_wave_green_even', playName: '绿双', position: 7, selection: '绿双' })
    expect(markSixSingleTicket(markSixMarket('half_half_wave')!, '蓝小单')).toMatchObject({ playCode: 'marksix_halfhalf_blue_small_odd', playName: '蓝小单', position: 7, selection: '蓝小单' })
    expect(markSixSingleTicket(markSixMarket('regular_position_wave')!, '绿波', 6)).toMatchObject({ playCode: 'marksix_regular_color_green', playName: '正码1-6绿波', position: 6, selection: '绿波' })
    expect(markSixSingleTicket(markSixMarket('regular_position_wave')!, '绿波', 0)).toBeNull()
    expect(markSixSingleTicket(markSixMarket('special_head')!, '0头')).toMatchObject({ playCode: 'marksix_special_head_0', playName: '特码0头', position: 7, selection: '0头' })
    expect(markSixSingleTicket(markSixMarket('special_tail')!, '9尾')).toMatchObject({ playCode: 'marksix_special_tail_9', playName: '特码9尾', position: 7, selection: '9尾' })
    expect(markSixSingleTicket(markSixMarket('five_element')!, '水')).toMatchObject({ playCode: 'marksix_five_element_water', playName: '五行水', position: 7, selection: '水' })
    expect(markSixSingleTicket(markSixMarket('special_zodiac')!, '猪')).toMatchObject({ playCode: 'marksix_special_zodiac_pig', playName: '特肖猪', position: 7, selection: '猪' })
    expect(markSixSingleTicket(markSixMarket('one_zodiac')!, '牛')).toMatchObject({ playCode: 'marksix_one_zodiac_ox', playName: '一肖牛', position: 0, selection: '牛' })
    expect(markSixSingleTicket(markSixMarket('one_tail')!, '0尾')).toMatchObject({ playCode: 'marksix_one_tail_0', playName: '一尾0尾', position: 0, selection: '0尾' })
    expect(markSixSingleTicket(markSixMarket('proper_zodiac')!, '马')).toMatchObject({ playCode: 'marksix_regular_zodiac_horse', playName: '正肖马', position: 0, selection: '马' })
    expect(markSixSingleTicket(markSixMarket('seven_color_wave')!, '红波')).toMatchObject({ playCode: 'marksix_seven_color_red', playName: '七色波红波', position: 0, selection: '红波' })
  })

  it('treats special quick actions as batch filters and resolves animal groups from the target draw date', () => {
    expect(markSixNumberFilterValues('red')).toContain('46')
    expect(markSixNumberFilterValues('red')).not.toContain('48')
    expect(markSixNumberFilterValues('green')).toContain('49')
    expect(markSixNumberFilterValues('big')).toEqual(Array.from({ length: 24 }, (_, index) => String(index + 25)))
    for (const filter of ['big', 'small', 'odd', 'even', 'sum_odd', 'sum_even'] as const) expect(markSixNumberFilterValues(filter)).not.toContain('49')
    const horseYear = '2026-09-01T12:00:00+08:00'
    expect(markSixNumberFilterValues('domestic', horseYear)).toEqual(expect.arrayContaining(['1', '6', '8', '9', '10', '12', '49']))
    expect(markSixNumberFilterValues('wild', horseYear)).toEqual(expect.arrayContaining(['2', '3', '4', '5', '7', '11']))
    expect(markSixNumberFilterValues('domestic', horseYear)).toHaveLength(25)
    expect(markSixNumberFilterValues('wild', horseYear)).toHaveLength(24)
    expect(markSixNumberFilterValues('domestic')).toEqual([])
  })

  it('keeps composite tickets atomic while requiring every internal price row', () => {
    const market = markSixMarket('combo_3_2')!
    const ticket = markSixComboTicket(market, ['1', '2', '3'])!
    expect(markSixBatchItems([ticket], '20')).toEqual([{ play_code: 'marksix_combo_3_2', play_name: '三中二', position: 0, selection: '1,2,3', amount: 20 }])
    expect(markSixPricingCodes(market)).toEqual(['marksix_combo_3_2_exact2', 'marksix_combo_3_2_exact3'])
    expect(markSixBatchError([ticket], '20', odds('marksix_combo_3_2_exact2', 20.1))).toContain('赔率待配置')
    const both: GameOdds = { ...odds(), items: [
      { ...odds().items[0], play_code: 'marksix_combo_3_2_exact2', odds: 20.1 },
      { ...odds().items[0], play_code: 'marksix_combo_3_2_exact3', odds: 125 },
    ] }
    expect(markSixBatchError([ticket], '20', both)).toBe('')
    expect(markSixPricingCodes(markSixMarket('link_zodiac_2')!, ['鼠', '马'])).toEqual(['marksix_link_zodiac_2_rat', 'marksix_link_zodiac_2_horse'])
  })

  it('preflights public composite tickets against their exact private price rows and contract version', () => {
    const linked = { play_code: 'marksix_link_zodiac_2', position: 0, selection: '鼠,马' }
    const tiered = { play_code: 'marksix_combo_3_2', position: 0, selection: '1,2,3' }
    expect(markSixBatchItemPricingCodes(linked)).toEqual(['marksix_link_zodiac_2_rat', 'marksix_link_zodiac_2_horse'])
    expect(markSixBatchItemPricingCodes(tiered)).toEqual(['marksix_combo_3_2_exact2', 'marksix_combo_3_2_exact3'])

    const linkedOdds: GameOdds = { ...odds(), rule_version: 'mark6-v2', items: [
      { ...odds().items[0], play_code: 'marksix_link_zodiac_2_rat', odds: 4.2 },
      { ...odds().items[0], play_code: 'marksix_link_zodiac_2_horse', odds: 3.55 },
    ] }
    expect(canSubmitMarkSixBatchItemWithOddsResponse(linked, linkedOdds, 'bingo-mark-six', 'mark6-v2')).toBe(true)
    expect(canSubmitMarkSixBatchItemWithOddsResponse(linked, { ...linkedOdds, items: linkedOdds.items.slice(0, 1) }, 'bingo-mark-six', 'mark6-v2')).toBe(false)
    expect(canSubmitMarkSixBatchItemWithOddsResponse(linked, { ...linkedOdds, rule_version: 'mark6-v1' }, 'bingo-mark-six', 'mark6-v2')).toBe(false)
    expect(canSubmitMarkSixBatchItemWithOddsResponse(linked, { ...linkedOdds, items: linkedOdds.items.map((row, index) => index ? { ...row, odds: 0 } : row) }, 'bingo-mark-six', 'mark6-v2')).toBe(false)
  })

  it.each([
    ['hong-kong-mark-six', 'hk-mark6-v1'],
    ['happy8-mark-six', 'happy8-mark6-v1'],
    ['new-macau-mark-six', 'new-macau-mark6-v1'],
    ['old-macau-mark-six', 'old-macau-mark6-v1'],
  ])('applies the same composite preflight to direct product %s without borrowing another contract', (gameID, ruleVersion) => {
    const item = { play_code: 'marksix_link_tail_2', position: 0, selection: '0尾,9尾' }
    const response: GameOdds = {
      ...odds(), game_id: gameID, rule_version: ruleVersion,
      items: [
        { ...odds().items[0], play_code: 'marksix_link_tail_2_0', odds: 7.5 },
        { ...odds().items[0], play_code: 'marksix_link_tail_2_9', odds: 3 },
      ],
    }
    expect(canSubmitMarkSixBatchItemWithOddsResponse(item, response, gameID, ruleVersion)).toBe(true)
    expect(canSubmitMarkSixBatchItemWithOddsResponse(item, response, 'bingo-mark-six', ruleVersion)).toBe(false)
    expect(canSubmitMarkSixBatchItemWithOddsResponse(item, response, gameID, 'mark6-v2')).toBe(false)
  })

  it('resolves every linked count and both tiered parent contracts without requiring a parent quote', () => {
    const zodiacValues = ['鼠', '牛', '虎', '兔', '龙']
    const zodiacCodes = ['rat', 'ox', 'tiger', 'rabbit', 'dragon']
    const tailValues = ['0尾', '1尾', '2尾', '3尾', '4尾']
    for (const count of [2, 3, 4, 5]) {
      expect(markSixBatchItemPricingCodes({ play_code: `marksix_link_zodiac_${count}`, position: 0, selection: zodiacValues.slice(0, count).join(',') }))
        .toEqual(zodiacCodes.slice(0, count).map(code => `marksix_link_zodiac_${count}_${code}`))
      expect(markSixBatchItemPricingCodes({ play_code: `marksix_link_tail_${count}`, position: 0, selection: tailValues.slice(0, count).join(',') }))
        .toEqual(Array.from({ length: count }, (_, index) => `marksix_link_tail_${count}_${index}`))
    }
    expect(markSixBatchItemPricingCodes({ play_code: 'marksix_combo_3_2', position: 0, selection: '1,2,3' }))
      .toEqual(['marksix_combo_3_2_exact2', 'marksix_combo_3_2_exact3'])
    expect(markSixBatchItemPricingCodes({ play_code: 'marksix_combo_2_special', position: 0, selection: '1,2' }))
      .toEqual(['marksix_combo_2_special_mixed', 'marksix_combo_2_special_regular'])
  })

  it('serializes atomic rows only after that exact option has a configured quote', () => {
    const ticket = markSixSingleTicket(markSixMarket('color_wave')!, '红波')!
    expect(markSixBatchItems([ticket], '20')).toEqual([{ play_code: 'marksix_color_wave_red', play_name: '红波', position: 7, selection: '红波', amount: 20 }])
    expect(markSixBatchError([ticket], '20', odds('marksix_color_wave_red', 0))).toContain('赔率待配置')
    expect(markSixBatchError([ticket], '20', odds('marksix_color_wave_red', 2.7))).toBe('')
    expect(markSixBatchError([ticket], '20', odds('marksix_color_wave_blue', 2.7))).toContain('赔率待配置')
  })

  it('serializes exact typed rows and enforces the current quote limits', () => {
    const ticket = markSixSingleTicket(markSixMarket('special_a_number')!, '18')!
    expect(markSixBatchItems([ticket], '20')).toEqual([{ play_code: 'marksix_special_a_number', play_name: '特码A', position: 7, selection: '18', amount: 20 }])
    expect(markSixBatchError([ticket], '20', odds())).toBe('')
    expect(markSixBatchError([ticket], '1', odds())).toContain('单注最低 10')
    expect(markSixBatchError([ticket], '201', odds())).toContain('单注最高 200')
    expect(markSixBatchError([ticket], '20', null)).toContain('赔率待配置')
    expect(markSixBatchItems([ticket], '0.001')).toEqual([])
  })

  it('accepts at most 200 unique tickets in one atomic web batch', () => {
    const base = markSixSingleTicket(markSixMarket('special_a_number')!, '18')!
    const twoHundred = Array.from({ length: 200 }, (_, index) => ({ ...base, selection: String(index + 1) }))
    expect(markSixBatchItems(twoHundred, '20')).toHaveLength(200)
    const tooMany = [...twoHundred, { ...base, selection: '201' }]
    expect(markSixBatchItems(tooMany, '20')).toEqual([])
    expect(markSixBatchError(tooMany, '20', odds())).toBe('每张最多200注。')
  })
})

describe('Mark Six multi-select number filters', () => {
  it('unions red and big without duplicate numbers and preserves red when big is removed', () => {
    const red = toggleMarkSixFilterSelection(undefined, 'red')
    const redAndBig = toggleMarkSixFilterSelection(red, 'big')
    const selected = markSixNumberSelectionValues(redAndBig)
    expect(redAndBig.filters).toEqual(['red', 'big'])
    expect(selected).toHaveLength(34)
    expect(new Set(selected).size).toBe(34)
    expect(selected).toEqual(expect.arrayContaining(['1', '25', '29', '46', '48']))
    expect(selected.filter(value => value === '29')).toHaveLength(1)
    const redOnly = toggleMarkSixFilterSelection(redAndBig, 'big')
    expect(redOnly.filters).toEqual(['red'])
    expect(markSixNumberSelectionValues(redOnly)).toEqual(markSixNumberFilterValues('red'))
    expect(markSixNumberSelectionValues(redOnly)).toHaveLength(17)
  })

  it('keeps the explicitly selected red group after all is deselected', () => {
    const all = toggleMarkSixFilterSelection(undefined, 'all')
    const allAndRed = toggleMarkSixFilterSelection(all, 'red')
    expect(allAndRed.filters).toEqual(['all', 'red'])
    expect(markSixNumberSelectionValues(allAndRed)).toHaveLength(49)
    const redOnly = toggleMarkSixFilterSelection(allAndRed, 'all')
    expect(redOnly.filters).toEqual(['red'])
    expect(markSixNumberSelectionValues(redOnly)).toEqual(markSixNumberFilterValues('red'))
    expect(markSixNumberSelectionValues(redOnly)).toHaveLength(17)
  })

  it('does not infer active filters from overlapping or complete number coverage', () => {
    const blue = toggleMarkSixFilterSelection(undefined, 'blue')
    expect(blue.filters).toEqual(['blue'])
    const withRed = toggleMarkSixFilterSelection(blue, 'red')
    const allColors = toggleMarkSixFilterSelection(withRed, 'green')
    expect(markSixNumberSelectionValues(allColors)).toHaveLength(49)
    expect(allColors.filters).toEqual(['blue', 'red', 'green'])
    expect(allColors.filters).not.toContain('all')
  })

  it('keeps manual exclusions through shortcut changes until the number is selected again', () => {
    const red = toggleMarkSixFilterSelection(undefined, 'red')
    const without29 = toggleMarkSixManualSelection(red, '29')
    expect(without29.filters).toEqual(['red'])
    expect(without29.excluded).toEqual(['29'])
    expect(markSixNumberSelectionValues(without29)).not.toContain('29')
    const withBig = toggleMarkSixFilterSelection(without29, 'big')
    const withAll = toggleMarkSixFilterSelection(withBig, 'all')
    expect(markSixNumberSelectionValues(withAll)).toHaveLength(48)
    expect(markSixNumberSelectionValues(withAll)).not.toContain('29')
    const restored = toggleMarkSixManualSelection(withAll, '29')
    expect(restored.excluded).toEqual([])
    expect(restored.included).toEqual(['29'])
    expect(markSixNumberSelectionValues(restored)).toHaveLength(49)
  })

  it('keeps a manually selected number after a covering filter is added and removed', () => {
    const manual = toggleMarkSixManualSelection(undefined, '29')
    const withRed = toggleMarkSixFilterSelection(manual, 'red')
    expect(markSixNumberSelectionValues(withRed)).toHaveLength(17)
    expect(withRed.included).toEqual(['29'])
    const withoutRed = toggleMarkSixFilterSelection(withRed, 'red')
    expect(markSixNumberSelectionValues(withoutRed)).toEqual(['29'])
    const removed = toggleMarkSixManualSelection(withoutRed, '29')
    expect(removed.included).toEqual([])
    expect(removed.excluded).toEqual(['29'])
    expect(markSixNumberSelectionValues(removed)).toEqual([])
  })

  it('honors a removed manual number when an overlapping group is subsequently enabled', () => {
    const manual = toggleMarkSixManualSelection(undefined, '29')
    const withRed = toggleMarkSixFilterSelection(manual, 'red')
    const removed = toggleMarkSixManualSelection(withRed, '29')
    expect(removed.included).toEqual([])
    const withBig = toggleMarkSixFilterSelection(removed, 'big')
    expect(markSixNumberSelectionValues(withBig)).not.toContain('29')
    expect(withBig.filters).toEqual(['red', 'big'])
  })

  it('cannot select animal-group numbers without a valid draw date', () => {
    const domestic = toggleMarkSixFilterSelection(undefined, 'domestic')
    expect(markSixNumberSelectionValues(domestic)).toEqual([])
    expect(markSixNumberSelectionValues(domestic, 'not-a-date')).toEqual([])
    const withWild = toggleMarkSixFilterSelection(domestic, 'wild')
    expect(markSixNumberSelectionValues(withWild)).toEqual([])
    expect(markSixNumberSelectionValues(domestic, '2026-09-01T12:00:00+08:00')).toEqual(markSixNumberFilterValues('domestic', '2026-09-01T12:00:00+08:00'))
    expect(markSixNumberSelectionValues(withWild, '2026-09-01T12:00:00+08:00')).toHaveLength(49)
  })

  it('returns canonical ascending numbers independently of click order or duplicate inputs', () => {
    const selection: MarkSixNumberSelection = { filters: ['big', 'red', 'red'], included: ['49', '1', '29', '1', '0', '50', '01'], excluded: ['26'] }
    const selected = markSixNumberSelectionValues(selection)
    expect(selected).toEqual([...new Set(selected)].sort((a, b) => Number(a) - Number(b)))
    expect(selected[0]).toBe('1')
    expect(selected.at(-1)).toBe('49')
    for (const invalid of ['0', '50', '01', '26']) expect(selected).not.toContain(invalid)
    const reversed: MarkSixNumberSelection = { ...selection, filters: [...selection.filters].reverse(), included: [...selection.included].reverse() }
    expect(markSixNumberSelectionValues(reversed)).toEqual(selected)
  })

  it.each(['', '0', '50', '-1', '1.5', '01', '1 ', 'NaN', 'Infinity'])('rejects non-canonical manual value %j without changing selection', value => {
    const selection: MarkSixNumberSelection = { filters: ['red'], included: ['49'], excluded: ['29'] }
    expect(toggleMarkSixManualSelection(selection, value)).toEqual(selection)
    expect(toggleMarkSixManualSelection(undefined, value)).toEqual({ filters: [], included: [], excluded: [] })
  })

  it('treats missing state as an empty selection', () => {
    expect(markSixNumberSelectionValues(undefined)).toEqual([])
    expect(toggleMarkSixFilterSelection(undefined, 'all')).toEqual({ filters: ['all'], included: [], excluded: [] })
    expect(toggleMarkSixManualSelection(undefined, '49')).toEqual({ filters: [], included: ['49'], excluded: [] })
  })

  it('does not mutate the source object or any of its arrays', () => {
    const selection: MarkSixNumberSelection = { filters: ['red'], included: ['49'], excluded: ['29'] }
    const original = structuredClone(selection)
    Object.freeze(selection.filters)
    Object.freeze(selection.included)
    Object.freeze(selection.excluded)
    Object.freeze(selection)
    const filterResult = toggleMarkSixFilterSelection(selection, 'big')
    const manualResult = toggleMarkSixManualSelection(selection, '1')
    markSixNumberSelectionValues(selection)
    expect(selection).toEqual(original)
    for (const result of [filterResult, manualResult]) {
      expect(result).not.toBe(selection)
      expect(result.filters).not.toBe(selection.filters)
      expect(result.included).not.toBe(selection.included)
      expect(result.excluded).not.toBe(selection.excluded)
    }
  })
})
