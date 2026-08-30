import { describe, expect, it } from 'vitest'
import { buildPlanAutomationPayload, canManagePlanAutomation, hasPlanAutomationChanges } from './planAutomation'

describe('plan automation permission boundary', () => {
  it('permits only the authenticated platform administrator', () => {
    expect(canManagePlanAutomation('admin')).toBe(true)
    for (const role of ['tenant', 'agent', 'member', 'superadmin', '', undefined]) expect(canManagePlanAutomation(role)).toBe(false)
  })
})

describe('plan automation configuration payload', () => {
  it('allows an explicitly disabled empty configuration without seeding games or masters', () => {
    expect(buildPlanAutomationPayload(37, false, [])).toEqual({ workspace_id: 37, enabled: false, mode: 'demo', game_ids: [] })
  })

  it('requires an explicit game selection before enabling', () => {
    expect(() => buildPlanAutomationPayload(37, true, [])).toThrow('至少选择一个彩种')
    expect(() => buildPlanAutomationPayload(37, true, ['  '])).toThrow('至少选择一个彩种')
  })

  it.each([0, -1, 1.5, Number.NaN, Number.POSITIVE_INFINITY])('rejects an invalid workspace %s', workspace => {
    expect(() => buildPlanAutomationPayload(workspace, true, ['speed-racing'])).toThrow('选择配置房间')
  })

  it('scopes and deduplicates selected games and never supplies generated recommendations or hit rates', () => {
    expect(buildPlanAutomationPayload(82, true, ['speed-racing', ' canada-28 ', 'speed-racing', ''])).toEqual({
      workspace_id: 82, enabled: true, mode: 'demo', game_ids: ['speed-racing', 'canada-28'],
    })
  })

  it('detects unsaved edits without treating selection order as a change', () => {
    const saved = { enabled: true, game_ids: ['speed-racing', 'speed-fly'] }
    expect(hasPlanAutomationChanges(saved, true, ['speed-fly', 'speed-racing'])).toBe(false)
    expect(hasPlanAutomationChanges(saved, false, saved.game_ids)).toBe(true)
    expect(hasPlanAutomationChanges(saved, true, ['speed-racing'])).toBe(true)
    expect(saved.game_ids).toEqual(['speed-racing', 'speed-fly'])
  })

  it('saves selected racing positions and variants without manufacturing recommendations', () => {
    const payload = buildPlanAutomationPayload(37, true, ['speed-racing'], {
      positions: [10, 1, 1], plan_keys: ['four-period-five-codes', ' size-three-periods ', 'four-period-five-codes'],
    })
    expect(payload.positions).toEqual([1, 10])
    expect(payload.plan_keys).toEqual(['four-period-five-codes', 'size-three-periods'])
    expect(payload).not.toHaveProperty('numbers')
    expect(payload).not.toHaveProperty('issue')
  })

  it.each([0, 11, 1.5, Number.NaN])('rejects an invalid position %s', position => {
    expect(() => buildPlanAutomationPayload(37, true, ['speed-racing'], { positions: [position], plan_keys: ['four-period-five-codes'] })).toThrow('名次')
  })

  it('requires both dimensions only when racing is enabled', () => {
    const empty = { positions: [], plan_keys: [] }
    expect(() => buildPlanAutomationPayload(37, true, ['speed-racing'], empty)).toThrow('至少选择一个名次和一种计划')
    expect(() => buildPlanAutomationPayload(37, true, ['speed-racing'], { positions: [1], plan_keys: [' '] })).toThrow('至少选择一个名次和一种计划')
    expect(() => buildPlanAutomationPayload(37, true, ['speed-fly'], empty)).not.toThrow()
    expect(() => buildPlanAutomationPayload(37, false, ['speed-racing'], empty)).not.toThrow()
  })

  it('detects racing configuration edits and compares sets without mutating saved order', () => {
    const saved = { enabled: true, game_ids: ['speed-racing'], positions: [10, 1], plan_keys: ['size-three-periods', 'four-period-five-codes'] }
    expect(hasPlanAutomationChanges(saved, true, saved.game_ids, { positions: [1, 10], plan_keys: [...saved.plan_keys].reverse() })).toBe(false)
    expect(hasPlanAutomationChanges(saved, true, saved.game_ids, { positions: [1], plan_keys: saved.plan_keys })).toBe(true)
    expect(hasPlanAutomationChanges(saved, true, saved.game_ids, { positions: saved.positions, plan_keys: ['four-period-five-codes'] })).toBe(true)
    expect(saved.positions).toEqual([10, 1])
  })
})
