import { describe, expect, it } from 'vitest'
import { gameAvailability } from './gameAvailability'

describe('member-facing game availability', () => {
  it('shows a results-only state without leaking internal rule details', () => {
    const state = gameAvailability({ id: 'hong-kong-mark-six', sourceHealthy: true, rulesReady: false })
    expect(state).toMatchObject({ kind: 'results-only', label: '仅开奖', cardText: '仅开奖 · 投注未开放' })
    expect(JSON.stringify(state)).not.toMatch(/rules|source|接口|母源/i)
  })

  it('prioritizes a friendly draw pause over rule readiness', () => {
    const state = gameAvailability({ id: 'speed-racing', sourceHealthy: false, rulesReady: true })
    expect(state).toMatchObject({ kind: 'source-paused', label: '开奖暂停', cardText: '开奖暂停 · 投注暂停' })
    expect(`${state?.label}${state?.cardText}${state?.roomMessage}${state?.detailText}`).not.toMatch(/source|接口|母源/i)
  })

  it('returns no restriction for a healthy exact contract', () => {
    expect(gameAvailability({ id: 'bingo-ssc-2', sourceHealthy: true, rulesReady: true, ruleVersion: 'digits5-v3' })).toBeNull()
  })
})
