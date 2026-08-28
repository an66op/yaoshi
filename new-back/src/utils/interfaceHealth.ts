import type { AdminGame, FeedJobStatus, FeedStatus } from '../api'

export type SourceInterfaceStatus = 'ok' | 'syncing' | 'idle' | 'error' | 'disabled' | 'missing'
export type SourceSchedulerStatus = 'running' | 'retrying' | 'scheduled' | 'standby' | 'error' | 'stopped' | 'missing'
export type SourceOverallStatus = 'healthy' | 'checking' | 'pending' | 'error' | 'disabled'

export type InterfaceHealthLine = {
  id: string
  group: string
  name: string
  games: AdminGame[]
  gameNames: string[]
  sourceKinds: string[]
  sourceNames: string[]
  sourceURLs: string[]
  enabledCount: number
  interfaceStatus: SourceInterfaceStatus
  schedulerStatus: SourceSchedulerStatus
  overallStatus: SourceOverallStatus
  lastSuccessAt?: string
  lastError: string
  consecutiveErrors: number
  mode: string
  latestIssue: string
}

export type InterfaceHealthSummary = {
  total: number
  enabled: number
  disabled: number
  healthy: number
  checking: number
  pending: number
  error: number
}

const failureStatuses = new Set(['error', 'stale', 'paused'])

function validTimestamp(value?: string | null) {
  if (!value) return 0
  const parsed = new Date(value).getTime()
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0
}

function latestTimestamp(values: Array<string | null | undefined>) {
  let latest = ''
  let latestMs = 0
  for (const value of values) {
    const timestamp = validTimestamp(value)
    if (timestamp > latestMs) {
      latest = value ?? ''
      latestMs = timestamp
    }
  }
  return latest || undefined
}

function unique(values: string[]) {
  return [...new Set(values.map(value => String(value ?? '').trim()).filter(Boolean))]
}

function interfaceStatus(games: AdminGame[]): SourceInterfaceStatus {
  if (games.length === 0) return 'missing'
  const enabled = games.filter(game => game.enabled)
  if (enabled.length === 0) return 'disabled'
  if (enabled.some(game => game.source_healthy === false || failureStatuses.has(String(game.sync_status).toLowerCase()) || String(game.last_sync_error ?? '').trim() !== '')) return 'error'
  if (enabled.some(game => String(game.sync_status).toLowerCase() === 'syncing')) return 'syncing'
  if (enabled.every(game => String(game.sync_status).toLowerCase() === 'ok' && validTimestamp(game.last_sync_at) > 0)) return 'ok'
  return 'idle'
}

function schedulerStatus(feed: FeedStatus, job?: FeedJobStatus): SourceSchedulerStatus {
  if (!job) return 'missing'
  if (!feed.running) return 'stopped'
  if (job.running && job.consecutive_errors > 0) return 'retrying'
  if (job.running) return 'running'
  if (job.last_error || job.consecutive_errors > 0) return 'error'
  if (job.mode === 'standby') return 'standby'
  return 'scheduled'
}

function overallStatus(apiStatus: SourceInterfaceStatus, scheduleStatus: SourceSchedulerStatus): SourceOverallStatus {
  if (apiStatus === 'disabled') return 'disabled'
  if (apiStatus === 'error' || apiStatus === 'missing' || scheduleStatus === 'error' || scheduleStatus === 'stopped' || scheduleStatus === 'missing') return 'error'
  if (apiStatus === 'syncing' || scheduleStatus === 'running' || scheduleStatus === 'retrying') return 'checking'
  if (apiStatus === 'idle') return 'pending'
  return 'healthy'
}

function lineFor(feed: FeedStatus, job: FeedJobStatus | undefined, games: AdminGame[], fallbackID?: string): InterfaceHealthLine {
  const apiStatus = interfaceStatus(games)
  const scheduleStatus = schedulerStatus(feed, job)
  const gameErrors = games
    .filter(game => String(game.last_sync_error ?? '').trim())
    .map(game => `${game.name}：${String(game.last_sync_error ?? '').trim()}`)
  const lastError = unique([job?.last_error ?? '', ...gameErrors]).join('；')
  return {
    id: job?.id ?? fallbackID ?? games[0]?.id ?? 'unknown',
    group: job?.group ?? '',
    name: job?.name ?? games[0]?.source_name ?? games[0]?.name ?? '未命名线路',
    games,
    gameNames: unique(games.map(game => game.name)),
    sourceKinds: unique(games.map(game => game.source_kind)),
    sourceNames: unique(games.map(game => game.source_name)),
    sourceURLs: unique(games.map(game => game.source_url)),
    enabledCount: games.filter(game => game.enabled).length,
    interfaceStatus: apiStatus,
    schedulerStatus: scheduleStatus,
    overallStatus: overallStatus(apiStatus, scheduleStatus),
    lastSuccessAt: latestTimestamp([job?.last_success_at, ...games.map(game => game.last_sync_at)]),
    lastError,
    consecutiveErrors: job?.consecutive_errors ?? 0,
    mode: job?.mode ?? 'unassigned',
    latestIssue: job?.latest_issue ?? games.find(game => game.issue)?.issue ?? '',
  }
}

/** Merge the scheduler's provider-level runtime state with durable per-game source state. */
export function buildInterfaceHealthLines(feed: FeedStatus, games: AdminGame[]): InterfaceHealthLine[] {
  const gameByID = new Map(games.map(game => [game.id, game]))
  const scheduledGameIDs = new Set<string>()
  const lines = feed.jobs.map(job => {
    const matched = job.game_ids.flatMap(id => {
      scheduledGameIDs.add(id)
      const game = gameByID.get(id)
      return game ? [game] : []
    })
    return lineFor(feed, job, matched)
  })
  for (const game of games) {
    if (scheduledGameIDs.has(game.id) || !['official', 'external'].includes(String(game.source_kind).toLowerCase())) continue
    lines.push(lineFor(feed, undefined, [game], `game:${game.id}`))
  }
  return lines.sort((left, right) => left.name.localeCompare(right.name, 'zh-CN'))
}

/** Count only enabled integrations as healthy so disabled sources never inflate readiness. */
export function summarizeInterfaceHealthLines(lines: InterfaceHealthLine[]): InterfaceHealthSummary {
  const summary: InterfaceHealthSummary = {
    total: lines.length,
    enabled: 0,
    disabled: 0,
    healthy: 0,
    checking: 0,
    pending: 0,
    error: 0,
  }
  for (const line of lines) {
    if (line.overallStatus === 'disabled') {
      summary.disabled += 1
      continue
    }
    summary.enabled += 1
    summary[line.overallStatus] += 1
  }
  return summary
}
