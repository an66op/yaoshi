import { describe, expect, it } from 'vitest'
import { diagnosticBatch, filterDiagnosticGames, filterDiagnosticSources, gameHasWarning, gameSources, SOURCE_BATCH_SIZE, sourceHasWarning, sourceRelationForGame, type SourceDiagnosticGame, type SourceDiagnosticSource, type SourceProbeResult } from './sourceDiagnostics'

const game: SourceDiagnosticGame = {
  game_id: 'sg-ssc', name: 'SG时时彩', enabled: true, category: 'ssc', lobby_category: '彩票', rule_version: 'digits5-v3', rules_message: '前三、中三、后三、龙虎和',
  source_kind: 'external', source_name: 'SG双站', source_key: 'sg-ssc-verified', source_163_status: 'verified_candidate', source_163_message: 'ID64同款，ID169不同产品', sync_status: 'ok', last_sync_at: '2026-09-04T00:00:00Z', last_sync_error: '', next_issue: '20260904097', next_draw_at: '2026-09-04T00:05:00Z',
}
const source: SourceDiagnosticSource = { key: '163:169', name: 'SG时时彩', provider: '163', groups: ['163官方彩'], candidate: true, relation: 'different_product', game_ids: ['sg-ssc'], endpoint: 'http://example.test/api/draw', upstream_game_id: 169, warning: '同期号码与现源不同', warning_persistent: true }
const current: SourceDiagnosticSource = { ...source, key: 'sg-ssc-verified', name: 'SG双站', provider: '163＋115', candidate: false, relation: 'production', warning: '', warning_persistent: false, upstream_game_id: undefined }
const success: SourceProbeResult = { source_key: source.key, status: 'success', checked_at: '2026-09-04T00:00:01Z', duration_ms: 12, http_status: 200, issue: '20260904096', draw_at: '2026-09-04T00:00:00Z', numbers: [1, 2, 3, 4, 5], history_count: 3, message: '只读样本通过' }

describe('source diagnostic presentation selectors', () => {
  it('starts with 12 items and appends batches without replacing or duplicating earlier entries', () => {
    const items = Array.from({ length: 88 }, (_, index) => index)
    expect(SOURCE_BATCH_SIZE).toBe(12)
    expect(diagnosticBatch(items, SOURCE_BATCH_SIZE)).toEqual({ items: items.slice(0, 12), total: 88, hasMore: true })
    for (const count of [24, 36, 48, 60, 72, 84, 96]) {
      const batch = diagnosticBatch(items, count)
      expect(batch.items).toEqual(items.slice(0, count))
      expect(new Set(batch.items).size).toBe(batch.items.length)
      expect(batch.hasMore).toBe(count < items.length)
    }
    expect(diagnosticBatch(items, 0).items).toEqual(items.slice(0, 12))
    expect(diagnosticBatch([1], 24)).toEqual({ items: [1], total: 1, hasMore: false })
    expect(diagnosticBatch([], 24)).toEqual({ items: [], total: 0, hasMore: false })
  })
  it('keeps current and candidate identities distinct while putting the actual binding first', () => {
    expect(gameSources(game, [source, current]).map(item => item.key)).toEqual([current.key, source.key])
    expect(gameSources({ ...game, source_key: null }, [source, current])).toHaveLength(2)
    expect(gameSources({ ...game, game_id: 'pc-canada', source_key: null }, [source, current])).toEqual([])
  })
  it('uses a game-specific role when one upstream is production for derivatives but only a cross-check elsewhere', () => {
    const bingo: SourceDiagnosticSource = {
      ...source, key: '163:135', name: '台湾宾果', relation: 'production', game_ids: ['bingo-ssc-2', 'official-tw-bingo'],
      game_relations: { 'official-tw-bingo': 'cross_check_only' },
    }
    expect(sourceRelationForGame(bingo, 'bingo-ssc-2')).toBe('production')
    expect(sourceRelationForGame(bingo, 'official-tw-bingo')).toBe('cross_check_only')
  })
  it('keeps identity warnings after a successful fetch but lets fresh evidence supersede availability history', () => {
    expect(sourceHasWarning(source, {})).toBe(true)
    expect(sourceHasWarning(current, {})).toBe(false)
    expect(sourceHasWarning(source, { [source.key]: success })).toBe(true)
    expect(sourceHasWarning({ ...source, warning_persistent: false }, { [source.key]: success })).toBe(false)
    expect(source.warning).toBe('同期号码与现源不同')
    for (const status of ['error', 'empty', 'stale'] as const) expect(sourceHasWarning(current, { [current.key]: { ...success, status } })).toBe(true)
  })
  it('never hides a current sync error just because a candidate probe succeeded', () => {
    expect(gameHasWarning({ ...game, last_sync_error: '数据不一致' }, [source], { [source.key]: success })).toBe(true)
    expect(gameHasWarning({ ...game, sync_status: 'stale' }, [], {})).toBe(true)
    expect(gameHasWarning(game, [source], { [source.key]: success })).toBe(true)
    expect(gameHasWarning({ ...game, source_163_status: 'candidate_unverified' }, [], {})).toBe(true)
  })
  it('searches game rules and source IDs with independent provider and warning filters', () => {
    const catalog = [source, current]
    expect(filterDiagnosticGames([game], catalog, {}, '龙虎和', '', 'all')).toEqual([game])
    expect(filterDiagnosticGames([game], catalog, {}, '', '不存在', 'all')).toEqual([])
    expect(filterDiagnosticSources(catalog, {}, '169', '163', 'all')).toEqual([source])
    expect(filterDiagnosticSources(catalog, {}, '', '', 'abnormal')).toEqual([source])
    expect(filterDiagnosticSources(catalog, { [source.key]: success }, '', '', 'abnormal')).toEqual([source])
    expect(filterDiagnosticSources(catalog, { [source.key]: success }, '', '', 'untested')).toEqual([current])
    expect(filterDiagnosticGames([game], catalog, { [source.key]: success }, '', '', 'untested')).toEqual([game])
  })
})
