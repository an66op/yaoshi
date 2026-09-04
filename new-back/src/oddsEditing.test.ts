import { describe, expect, it } from 'vitest'
import type { PlayLimitItem } from './api'
import { applyOddsBatch, isOddsConfigurationConflict, oddsDirtyCodes, oddsDraftItems, parseOddsBatchValue, validateOddsDraft } from './oddsEditing'

const saved: PlayLimitItem[] = ['ball_1_5', 'two_sided', 'dragon_tiger'].map(play_code => ({
  play_code, play_name: play_code, odds: 1.99, min_bet: 1, max_bet: 100, max_user_period: 200, max_period_total: 500, sort_order: 0,
  configured: true, configuration_source: 'admin_save', configured_at: '2026-09-03T00:00:00Z', rule_version: 'racing-v2',
}))

describe('explicit odds draft editing', () => {
  it('batch edits only selected codes without mutating the loaded configuration', () => {
    const result = applyOddsBatch(saved, ['ball_1_5', 'dragon_tiger'], 'odds', '2.12345')
    expect(result.map(item => item.odds)).toEqual([2.1235, 1.99, 2.1235])
    expect(result[1]).toBe(saved[1])
    expect(saved.map(item => item.odds)).toEqual([1.99, 1.99, 1.99])
    expect(oddsDirtyCodes(result, saved)).toEqual(['ball_1_5', 'dragon_tiger'])
    const draft = oddsDraftItems(result, saved)
    expect(draft[0]).toMatchObject({ configured: false, configuration_source: 'pending_admin_save', configured_at: null })
    expect(draft[1]).toBe(saved[1])
  })

  it('restoring values restores the saved confirmation and clears dirty state', () => {
    const edited = oddsDraftItems(applyOddsBatch(saved, ['two_sided'], 'max_bet', '123.45'), saved)
    expect(oddsDirtyCodes(edited, saved)).toEqual(['two_sided'])
    const reverted = oddsDraftItems(applyOddsBatch(edited, ['two_sided'], 'max_bet', '100'), saved)
    expect(reverted).toEqual(saved)
    expect(oddsDirtyCodes(reverted, saved)).toEqual([])
    expect(oddsDirtyCodes([], saved)).toEqual(saved.map(item => item.play_code))
  })

  it('preserves pending row identity across repeated normalization and edits to another row', () => {
    const firstDraft = oddsDraftItems(applyOddsBatch(saved, ['ball_1_5'], 'odds', '3.72'), saved)
    const pending = firstDraft[0]
    expect(pending).not.toBe(saved[0])
    expect(pending).toMatchObject({ configured: false, configuration_source: 'pending_admin_save', configured_at: null })
    const repeated = oddsDraftItems(firstDraft, saved)
    expect(repeated[0]).toBe(pending)
    expect(repeated[1]).toBe(saved[1])
    expect(repeated[2]).toBe(saved[2])
    const otherEdited = oddsDraftItems(applyOddsBatch(repeated, ['two_sided'], 'max_bet', '125'), saved)
    expect(otherEdited[0]).toBe(pending)
    expect(otherEdited[1]).not.toBe(saved[1])
    expect(otherEdited[2]).toBe(saved[2])

    const reverted = oddsDraftItems(applyOddsBatch(otherEdited, ['ball_1_5'], 'odds', '1.99'), saved)
    expect(reverted[0]).toBe(saved[0])
    expect(reverted[1]).toBe(otherEdited[1])
    expect(reverted[2]).toBe(saved[2])
    expect(oddsDirtyCodes(reverted, saved)).toEqual(['two_sided'])
  })

  it('normalizes an unconfirmed added row once and retains its pending identity afterward', () => {
    const added = { ...saved[0], play_code: 'new_play', play_name: '新增玩法' }
    const normalized = oddsDraftItems([...saved, added], saved)
    expect(normalized[3]).not.toBe(added)
    expect(normalized[3]).toMatchObject({ configured: false, configuration_source: 'pending_admin_save', configured_at: null })
    const repeated = oddsDraftItems(normalized, saved)
    expect(repeated[3]).toBe(normalized[3])
    expect(repeated.slice(0, 3).every((item, index) => item === saved[index])).toBe(true)
  })

  it('allows explicit zero to close plays and never substitutes default prices', () => {
    const draft = oddsDraftItems(applyOddsBatch(saved, saved.map(item => item.play_code), 'odds', '0'), saved)
    expect(draft.every(item => item.odds === 0 && !item.configured && item.configuration_source === 'pending_admin_save')).toBe(true)
    expect(validateOddsDraft(draft)).toBe('')
    expect(parseOddsBatchValue('max_bet', '0')).toBe(0)
  })

  it.each(['', ' ', '-0.00001', 'NaN', 'Infinity', '1', '0.00001', '1.00001', '1e309'])('rejects invalid price %j', value => {
    expect(() => parseOddsBatchValue('odds', value)).toThrow()
  })

  it('validates currency precision and full per-play limit ordering', () => {
    expect(() => parseOddsBatchValue('max_bet', '1.001')).toThrow('两位小数')
    expect(validateOddsDraft(applyOddsBatch(saved, ['two_sided'], 'max_bet', '201'))).toContain('单注最高不能高于会员单期')
    expect(validateOddsDraft(applyOddsBatch(saved, ['two_sided'], 'min_bet', '0'))).toContain('单注最低必须大于 0')
    expect(validateOddsDraft([{ ...saved[0], odds: Number.NaN }])).toContain('非负有限数值')
    expect(validateOddsDraft([saved[0], saved[0]])).toContain('目录缺失或重复')
    expect(validateOddsDraft(saved)).toBe('')
  })

  it('recognizes revision and rules conflicts without treating ordinary errors as concurrent writes', () => {
    expect(isOddsConfigurationConflict({ code: 'ODDS_CONFIGURATION_CONFLICT' })).toBe(true)
    expect(isOddsConfigurationConflict({ code: 'RULE_VERSION_CONFLICT' })).toBe(true)
    expect(isOddsConfigurationConflict({ status: 409 })).toBe(true)
    expect(isOddsConfigurationConflict(new Error('network failed'))).toBe(false)
    expect(isOddsConfigurationConflict(undefined)).toBe(false)
  })
})
