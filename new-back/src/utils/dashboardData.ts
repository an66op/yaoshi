import type { DashboardData } from '../api'

function record(value: unknown): Record<string, number> {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, number>
    : {}
}

/** Keep an empty or partially migrated dashboard response renderable. */
export function normalizeDashboardData(value: DashboardData | null | undefined): DashboardData {
  return {
    overview: record(value?.overview),
    stats: record(value?.stats),
    games: Array.isArray(value?.games) ? value.games : [],
  }
}
