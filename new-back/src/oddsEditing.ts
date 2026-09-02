import type { PlayLimitItem } from './api'

export type OddsEditableField = 'odds' | 'min_bet' | 'max_bet' | 'max_user_period' | 'max_period_total'

export const oddsEditableFields: Array<{ key: OddsEditableField; label: string; step: number }> = [
  { key: 'odds', label: '平台赔率（0关闭）', step: .0001 },
  { key: 'min_bet', label: '单注最低', step: .01 },
  { key: 'max_bet', label: '单注最高', step: .01 },
  { key: 'max_user_period', label: '会员单期', step: .01 },
  { key: 'max_period_total', label: '全房单期', step: .01 },
]

export function sameOddsValues(left: PlayLimitItem, right: PlayLimitItem) {
  return left.play_code === right.play_code && oddsEditableFields.every(field => Object.is(left[field.key], right[field.key]))
}

export function oddsDirtyCodes(items: PlayLimitItem[], saved: PlayLimitItem[]) {
  const baseline = new Map(saved.map(item => [item.play_code, item]))
  const currentCodes = new Set(items.map(item => item.play_code))
  return [...items.filter(item => !baseline.has(item.play_code) || !sameOddsValues(item, baseline.get(item.play_code)!)).map(item => item.play_code), ...saved.filter(item => !currentCodes.has(item.play_code)).map(item => item.play_code)]
}

export function oddsDraftItems(items: PlayLimitItem[], saved: PlayLimitItem[]) {
  const baseline = new Map(saved.map(item => [item.play_code, item]))
  return items.map(item => {
    const original = baseline.get(item.play_code)
    if (original && sameOddsValues(item, original)) return original
    return { ...item, configured: false, configuration_source: 'pending_admin_save', configured_at: null }
  })
}

export function parseOddsBatchValue(field: OddsEditableField, raw: string) {
  if (!raw.trim()) throw new Error('请填写批量设置的数值')
  const value = Number(raw)
  if (!Number.isFinite(value) || value < 0) throw new Error('赔率和限额必须是非负有限数值')
  if (field === 'odds') {
    const rounded = Math.round(value * 10000) / 10000
    if (!Number.isFinite(rounded) || (value !== 0 && rounded <= 1)) throw new Error('赔率必须大于 1，填 0 表示关闭')
    return rounded
  }
  if (Math.abs(Math.round(value * 100) / 100 - value) > .0000001) throw new Error('限额最多保留两位小数')
  return value
}

export function applyOddsBatch(items: PlayLimitItem[], codes: readonly string[], field: OddsEditableField, raw: string) {
  const value = parseOddsBatchValue(field, raw)
  const selected = new Set(codes)
  return items.map(item => selected.has(item.play_code) ? { ...item, [field]: value } : item)
}

export function validateOddsDraft(items: PlayLimitItem[]) {
  const seen = new Set<string>()
  for (const item of items) {
    if (!item.play_code || seen.has(item.play_code)) return '玩法目录缺失或重复，请刷新后重试'
    seen.add(item.play_code)
    try {
      for (const field of oddsEditableFields) parseOddsBatchValue(field.key, String(item[field.key]))
    } catch (reason) {
      return `${item.play_name}：${reason instanceof Error ? reason.message : '赔率或限额无效'}`
    }
    if (item.odds > 1 && item.min_bet <= 0) return `${item.play_name}：开启时单注最低必须大于 0`
    if (item.max_bet > 0 && item.min_bet > item.max_bet) return `${item.play_name}：单注最低不能高于单注最高`
    if (item.max_user_period > 0 && item.max_bet > item.max_user_period) return `${item.play_name}：单注最高不能高于会员单期限额`
    if (item.max_period_total > 0 && item.max_user_period > item.max_period_total) return `${item.play_name}：会员单期限额不能高于全房单期限额`
  }
  return ''
}

export function isOddsConfigurationConflict(reason: unknown) {
  if (!reason || typeof reason !== 'object') return false
  const error = reason as { status?: number; code?: string }
  return error.status === 409 || error.code === 'ODDS_CONFIGURATION_CONFLICT' || error.code === 'RULE_VERSION_CONFLICT'
}
