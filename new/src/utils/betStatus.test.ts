import { describe, expect, it } from 'vitest'
import { betStatusText, betStatusTone, isSettlementPush } from './betStatus'

describe('member bet status labels', () => {
  it('distinguishes settled Mark Six pushes from operator cancellations', () => {
    expect(isSettlementPush('cancelled', '特码为49，和局返还本金')).toBe(true)
    expect(betStatusText('cancelled', '特码为49，和局返还本金')).toBe('和局返本')
    expect(betStatusTone('cancelled', '特码为49，和局返还本金')).toBe('push')
    expect(betStatusText('cancelled', '用户撤单')).toBe('已撤销')
    expect(betStatusTone('cancelled', '用户撤单')).toBe('cancelled')
  })

  it('also accepts a future explicit push status', () => {
    expect(betStatusText('push')).toBe('和局返本')
    expect(betStatusText('won')).toBe('已中奖')
  })
})
