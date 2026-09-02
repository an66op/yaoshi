import { describe, expect, it } from 'vitest'
import type { GameOdds } from '../api/portal'
import {
  markSixBatchError,
  markSixBatchItems,
  markSixCategories,
  markSixComboTicket,
  markSixMarket,
  markSixMarkets,
  markSixEnabledPlayCodes,
  markSixNumberFilterValues,
  markSixSingleTicket,
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
    expect([...optionPlayCodes].sort()).toEqual([...atomicEnabledCodes].sort())
    expect(new Set(optionPlayCodes).size).toBe(50)
    expect([...markSixEnabledPlayCodes]).toEqual(enabledCodes)
    for (const id of ['regular_special_sides', 'regular_special_wave', 'one_zodiac', 'one_tail', 'combo_3_2', 'combo_2_special', 'total_zodiac', 'proper_zodiac', 'seven_color_wave']) {
      expect(markSixMarket(id)).toMatchObject({ playCode: null, blockedReason: expect.stringContaining('待核验') })
    }
    expect(markSixMarket('special_zodiac')).toMatchObject({ playCode: 'marksix_special_zodiac', blockedReason: undefined, positionMode: 'special' })
    expect(markSixMarket('special_combo')).toMatchObject({ playCode: 'marksix_special_half', blockedReason: undefined })
    expect(markSixMarket('regular_position_big_small')).toMatchObject({ playCode: 'marksix_regular_position_big_small', blockedReason: undefined })
    expect(markSixMarket('regular_position_wave')).toMatchObject({ playCode: null, blockedReason: undefined })
  })

  it('uses the contracted positions and canonical ascending selections', () => {
    expect(markSixSingleTicket(markSixMarket('special_a_number')!, '18')).toMatchObject({ playCode: 'marksix_special_a_number', position: 7, selection: '18' })
    expect(markSixSingleTicket(markSixMarket('regular_number')!, '18')).toMatchObject({ playCode: 'marksix_regular_number', position: 0, selection: '18' })
    expect(markSixSingleTicket(markSixMarket('regular_position_number')!, '18', 6)).toMatchObject({ playCode: 'marksix_regular_position_number', position: 6, selection: '18' })
    expect(markSixSingleTicket(markSixMarket('special_domestic_wild')!, '家肖')).toMatchObject({ playCode: 'marksix_special_domestic_wild', position: 7, selection: '家肖' })
    expect(markSixSingleTicket(markSixMarket('special_zodiac')!, '马')).toMatchObject({ playCode: 'marksix_special_zodiac', playName: '特码生肖', position: 7, selection: '马' })
    expect(markSixSingleTicket(markSixMarket('total_big_small')!, '总和大')).toMatchObject({ playCode: 'marksix_total_big_small', position: 0, selection: '总和大' })
    expect(markSixSingleTicket(markSixMarket('regular_position_tail_big_small')!, '尾小', 4)).toMatchObject({ playCode: 'marksix_regular_position_tail_big_small', position: 4, selection: '尾小' })
    expect(markSixSingleTicket(markSixMarket('regular_special_number')!, '18', 0)).toBeNull()
    expect(markSixComboTicket(markSixMarket('combo_3_all')!, ['18', '1', '7'])).toMatchObject({ position: 0, selection: '1,7,18' })
    expect(markSixComboTicket(markSixMarket('not_in')!, ['49', '1', '18', '7', '30'])).toMatchObject({ selection: '1,7,18,30,49' })
    expect(markSixComboTicket(markSixMarket('not_in')!, ['1', '7', '18', '30'])).toBeNull()
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

  it('never creates tickets for a displayed-only market even if a stale quote is supplied', () => {
    const unsafe = markSixMarket('combo_3_2')!
    expect(unsafe).toMatchObject({ playCode: null })
    expect(unsafe.blockedReason).toContain('待核验')
    expect(markSixComboTicket(unsafe, ['1', '2', '3'])).toBeNull()
    const forged = { ...markSixSingleTicket(markSixMarket('special_a_number')!, '18')!, playCode: 'marksix_combo_3_2' }
    expect(markSixBatchItems([forged], '20')).toEqual([])
    expect(markSixBatchError([forged], '20', odds('marksix_combo_3_2'))).toContain('未通过规则核验')
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
