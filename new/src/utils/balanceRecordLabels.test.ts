import { describe, expect, it } from 'vitest'
import { balanceRecordLabel } from './balanceRecordLabels'

describe('member balance record labels', () => {
  it.each([['bet', '投注扣款'], ['bet_cancel', '撤单退款'], ['settlement', '注单结算'], ['credit', '上分到账']])('labels %s in Chinese without altering the ledger type', (type, label) => {
    expect(balanceRecordLabel(type)).toBe(label)
  })
  it('does not expose an unknown internal type as English UI', () => {
    expect(balanceRecordLabel('future_internal_kind')).toBe('其他账变')
  })
})
