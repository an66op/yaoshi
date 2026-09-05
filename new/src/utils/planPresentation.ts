import type { PlanDetail, PlanRecommendation } from '../api/plans'
import { PLAN_HISTORY_LIMIT, PLAN_HISTORY_MAX } from './planLimits'

/** A period limit, not a row limit: never pad missing historical publications. */
export function recentPlanHistory<T extends { issue: string }>(rows: T[], requestedLimit = PLAN_HISTORY_LIMIT): T[] {
  const limit = Number.isFinite(requestedLimit) ? Math.min(PLAN_HISTORY_MAX, Math.max(1, Math.floor(requestedLimit))) : PLAN_HISTORY_LIMIT
  const issues = new Set<string>()
  return rows.filter(row => {
    if (!row.issue || (!issues.has(row.issue) && issues.size >= limit)) return false
    issues.add(row.issue)
    return true
  })
}

export function displayedPlanMasters(detail: PlanDetail | null) {
  if (!detail) return []
  // A newly published master can appear while others are still on the previous
  // issue. Retain their tabs, but never promote their old rows to this period.
  const byIdentity = new Map<string, PlanRecommendation>()
  for (const row of detail.latest_recommendations ?? []) byIdentity.set(planMasterIdentity(row), row)
  for (const row of detail.recommendations ?? []) byIdentity.set(planMasterIdentity(row), row)
  return [...byIdentity.values()].sort((left, right) => left.sort_order - right.sort_order || left.master_name.localeCompare(right.master_name) || left.source.localeCompare(right.source))
}

export const planMasterIdentity = (row: Pick<PlanRecommendation, 'source' | 'master_name'>) => `${row.source}\u0000${row.master_name}`

export function planIsCurrent(detail: PlanDetail | null, row?: PlanRecommendation) {
  return Boolean(row && detail?.current_issue && row.issue === detail.current_issue
    && detail.recommendations.some(item => item.id === row.id))
}

export function planResultLabel(row: PlanRecommendation) {
  return row.result === 'hit' ? '中' : row.result === 'miss' ? '未中' : '待开奖'
}
