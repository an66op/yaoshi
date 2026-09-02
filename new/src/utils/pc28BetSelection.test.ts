import { describe, expect, it } from 'vitest'
import type { GameOdds } from '../api/portal'
import {
  isPC28EnabledPlayCode,
  pc28BatchError,
  pc28BatchItems,
  pc28MarketsForCategory,
  pc28OddsItem,
  pc28PackageTicket,
  pc28SingleTicket,
  pc28SumExactPlayCode,
  pc28TicketAddError,
  togglePC28Draft,
} from './pc28BetSelection'

const quote = (playCode: string, odds = 2) => ({
  play_code: playCode,
  play_name: playCode,
  odds,
  min_bet: 1,
  max_bet: 500,
  max_user_period: 5000,
})

const odds = (items: ReturnType<typeof quote>[], ruleVersion = 'pc28-v2', showOdds = true): GameOdds => ({
  game_id: 'canada-28',
  game_name: '加拿大28',
  show_odds: showOdds,
  rules_ready: true,
  rule_version: ruleVersion,
  items,
})

describe('PC28 typed web-bet contract', () => {
  it('maps every sum to its own symmetric odds band without using a generic fallback', () => {
    expect(pc28SumExactPlayCode(0)).toBe('pc28_sum_exact_0_27')
    expect(pc28SumExactPlayCode(27)).toBe('pc28_sum_exact_0_27')
    expect(pc28SumExactPlayCode(13)).toBe('pc28_sum_exact_13_14')
    expect(pc28SumExactPlayCode(14)).toBe('pc28_sum_exact_13_14')
    expect(pc28SumExactPlayCode(-1)).toBeNull()
    expect(new Set(Array.from({ length: 28 }, (_, value) => pc28SumExactPlayCode(value))).size).toBe(14)
  })

  it('normalizes package-three to exactly three distinct ascending 0–27 values', () => {
    const market = pc28MarketsForCategory('package')[0]
    expect(pc28PackageTicket(market, ['13', '1', '7'])).toMatchObject({
      playCode: 'pc28_package_three', position: 0, selection: '1,7,13', selectionLabel: '1、7、13',
    })
    expect(pc28PackageTicket(market, ['1', '1', '2'])).toBeNull()
    expect(pc28PackageTicket(market, ['1', '2', '28'])).toBeNull()
    expect(togglePC28Draft(['1', '2', '3'], '4')).toEqual(['1', '2', '3'])
    expect(togglePC28Draft(['1', '2', '3'], '2')).toEqual(['1', '3'])
  })

  it('keeps dragon/tiger and tie on independent server odds codes', () => {
    const market = pc28MarketsForCategory('dragon')[0]
    expect(pc28SingleTicket(market, '龙', 3)).toMatchObject({ playCode: 'pc28_dragon_tiger', position: 1, selection: '龙' })
    expect(pc28SingleTicket(market, '虎', 2)).toMatchObject({ playCode: 'pc28_dragon_tiger', position: 1, selection: '虎' })
    expect(pc28SingleTicket(market, '和', 1)).toMatchObject({ playCode: 'pc28_dragon_tiger_tie', position: 1, selection: '和' })
    expect(isPC28EnabledPlayCode('pc28_dragon_tiger_tie')).toBe(true)
  })

  it('rejects a missing atomic quote, invalid odds and a mismatched financial version', () => {
    const response = odds([quote('pc28_dragon_tiger')])
    expect(pc28OddsItem('canada-28', 'pc28_dragon_tiger', response)).not.toBeNull()
    expect(pc28OddsItem('canada-28', 'pc28_dragon_tiger_tie', response)).toBeNull()
    expect(pc28OddsItem('canada-28', 'pc28_dragon_tiger', odds([quote('pc28_dragon_tiger', 1)]))).toBeNull()
    expect(pc28OddsItem('canada-28', 'pc28_dragon_tiger', odds([quote('pc28_dragon_tiger')], 'pc28-v1'))).toBeNull()
    expect(pc28OddsItem('canada-28', 'pc28_dragon_tiger', odds([quote('pc28_dragon_tiger', 0)], 'pc28-v2', false))).not.toBeNull()
  })

  it('serializes normalized rows and checks min/max against the exact returned quote', () => {
    const market = pc28MarketsForCategory('position')[0]
    const ticket = pc28SingleTicket(market, '8', 3)!
    expect(pc28BatchItems([ticket], '20')).toEqual([{
      play_code: 'pc28_position_number',
      play_name: '三球定位号码',
      position: 3,
      selection: '8',
      amount: 20,
    }])
    expect(pc28BatchError('canada-28', [ticket], '20', odds([quote('pc28_position_number')]))).toBe('')
    expect(pc28BatchError('canada-28', [ticket], '20', odds([]))).toContain('赔率待配置')
    expect(pc28BatchError('canada-28', [ticket], '0', odds([quote('pc28_position_number')]))).toContain('金额须大于')
  })

  it('caps exact points at ten and applies the v1/v2 reverse-bet restriction only to those versions', () => {
    const sum = pc28MarketsForCategory('sum')[0]
    const exact = Array.from({ length: 11 }, (_, value) => pc28SingleTicket(sum, String(value))!)
    expect(pc28TicketAddError(exact.slice(0, 10), exact[10], 'pc28-v2')).toContain('10个不同')
    const size = pc28MarketsForCategory('mixed').find(item => item.id === 'sum_size')!
    const big = pc28SingleTicket(size, '大')!
    const small = pc28SingleTicket(size, '小')!
    expect(pc28TicketAddError([big], small, 'pc28-v1')).toContain('反向下注')
    expect(pc28TicketAddError([big], small, 'pc28-v2')).toContain('反向下注')
    expect(pc28TicketAddError([big], small, 'pc28-v3')).toBe('')
  })
})
