export type SourceDiagnosticGame = {
  game_id: string
  name: string
  enabled: boolean
  category: string
  lobby_category: string
  rule_version: string
  rules_message: string
  source_kind: string
  source_name: string
  source_key: string | null
  source_163_status: 'current' | 'verified_candidate' | 'candidate_unverified' | 'unavailable' | 'not_found' | 'not_assessed'
  source_163_message: string
  sync_status: string
  last_sync_at: string | null
  last_sync_error: string
  next_issue: string
  next_draw_at: string | null
}

export type SourceDiagnosticRelation = 'production' | 'historical' | 'same_product_candidate' | 'different_product' | 'cross_check_only' | 'unverified_candidate' | 'unavailable' | 'catalog_only'

export type SourceDiagnosticSource = {
  key: string
  name: string
  provider: string
  groups: string[]
  candidate: boolean
  relation: SourceDiagnosticRelation
  game_ids: string[]
  game_relations?: Record<string, SourceDiagnosticRelation>
  endpoint: string
  upstream_game_id?: number
  warning: string
  warning_persistent: boolean
  warning_checked_at?: string
}

export type SourceDiagnostics = {
  games: SourceDiagnosticGame[]
  catalog: SourceDiagnosticSource[]
}

export type SourceProbeResult = {
  source_key: string
  status: 'success' | 'error' | 'empty' | 'stale'
  checked_at: string
  duration_ms: number
  http_status: number | null
  issue: string | null
  draw_at: string | null
  numbers: number[]
  history_count: number
  message: string
}

export type SourceProbeResults = Record<string, SourceProbeResult>
export type SourceDiagnosticFilter = 'all' | 'abnormal' | 'untested'
export const SOURCE_BATCH_SIZE = 12

export function sourceHasWarning(source: SourceDiagnosticSource, results: SourceProbeResults): boolean {
  const result = results[source.key]
  if (result && result.status !== 'success') return true
  if (!source.warning) return false
  // Connectivity cannot clear a product-identity or compatibility warning.
  // It may supersede only a dated availability observation such as stale/empty.
  return source.warning_persistent || !result
}

const sourceRelationOrder: Record<SourceDiagnosticRelation, number> = {
  production: 0,
  same_product_candidate: 1,
  cross_check_only: 2,
  unverified_candidate: 3,
  different_product: 4,
  unavailable: 5,
  historical: 6,
  catalog_only: 7,
}

export function sourceRelationForGame(source: SourceDiagnosticSource, gameID: string): SourceDiagnosticRelation {
  return source.game_relations?.[gameID] ?? source.relation
}

export function gameSources(game: SourceDiagnosticGame, catalog: SourceDiagnosticSource[]): SourceDiagnosticSource[] {
  return catalog.filter(source => source.key === game.source_key || source.game_ids.includes(game.game_id))
    .sort((a, b) => Number(b.key === game.source_key) - Number(a.key === game.source_key)
      || sourceRelationOrder[sourceRelationForGame(a, game.game_id)] - sourceRelationOrder[sourceRelationForGame(b, game.game_id)]
      || a.name.localeCompare(b.name, 'zh-CN'))
}

export function gameHasWarning(game: SourceDiagnosticGame, sources: SourceDiagnosticSource[], results: SourceProbeResults): boolean {
  const runtimeError = game.enabled && (Boolean(game.last_sync_error) || ['error', 'stale', 'paused'].includes(game.sync_status))
  const directoryGap = ['candidate_unverified', 'unavailable', 'not_found', 'not_assessed'].includes(game.source_163_status)
  return runtimeError || directoryGap || sources.some(source => sourceHasWarning(source, results))
}

export function filterDiagnosticSources(catalog: SourceDiagnosticSource[], results: SourceProbeResults, query: string, provider: string, filter: SourceDiagnosticFilter): SourceDiagnosticSource[] {
  const term = query.trim().toLocaleLowerCase()
  return catalog.filter(source => (!provider || source.provider === provider)
    && (!term || [source.key, source.name, ...source.groups, ...source.game_ids, String(source.upstream_game_id ?? '')].join(' ').toLocaleLowerCase().includes(term))
    && (filter === 'all' || (filter === 'abnormal' ? sourceHasWarning(source, results) : !results[source.key])))
}

export function filterDiagnosticGames(games: SourceDiagnosticGame[], catalog: SourceDiagnosticSource[], results: SourceProbeResults, query: string, provider: string, filter: SourceDiagnosticFilter): SourceDiagnosticGame[] {
  const term = query.trim().toLocaleLowerCase()
  return games.filter(game => {
    const sources = gameSources(game, catalog)
    return (!provider || sources.some(source => source.provider === provider))
      && (!term || [game.name, game.game_id, game.lobby_category, game.category, game.rule_version, game.rules_message, game.source_name, game.source_163_status, game.source_163_message, ...sources.map(source => source.name)].join(' ').toLocaleLowerCase().includes(term))
      && (filter === 'all' || (filter === 'abnormal' ? gameHasWarning(game, sources, results) : sources.some(source => !results[source.key])))
  })
}

export function diagnosticBatch<T>(items: T[], visibleCount: number): { items: T[]; total: number; hasMore: boolean } {
  const limit = Math.max(SOURCE_BATCH_SIZE, visibleCount)
  return { items: items.slice(0, limit), total: items.length, hasMore: limit < items.length }
}
