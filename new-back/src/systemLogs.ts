import type { AuditLog, SystemLogItem } from './api'

export type SourceLogStatusFilter = '' | 'error' | 'ok' | 'standby' | 'started' | 'stopped'

export const systemLogStatusLabel = (status: string) => ({
  error: '异常',
  ok: '已恢复',
  standby: '待命',
  started: '已启动',
  stopped: '已停止',
}[status] ?? (status || '未知'))

export const systemLogEventLabel = (eventType: string) => ({
  sync_error: '开奖源异常',
  sync_recovered: '开奖源恢复',
  scheduler_error: '调度异常',
  scheduler_recovered: '调度恢复',
  scheduler_started: '调度启动',
  scheduler_stopped: '调度停止',
  scheduler_standby: '调度待命',
  standby: '调度待命',
}[eventType] ?? (eventType || '系统事件'))

export const mergeLogPage = <T extends { id: number }>(current: T[], incoming: T[], reset: boolean): T[] => {
  const rows = reset ? incoming : [...current, ...incoming]
  const byID = new Map<number, T>()
  for (const row of rows) if (!byID.has(row.id)) byID.set(row.id, row)
  return [...byID.values()].sort((a, b) => b.id - a.id)
}

export const filterAuditLogs = (rows: AuditLog[], query: string, status: string): AuditLog[] => {
  const term = query.trim().toLocaleLowerCase()
  return rows.filter(row => {
    const matchesStatus = !status || (status === 'success' ? row.status_code >= 200 && row.status_code < 400 : row.status_code >= 400)
    const haystack = [row.actor_name, row.actor_role, row.method, row.path, row.request_id, row.ip, row.status_code].join(' ').toLocaleLowerCase()
    return matchesStatus && (!term || haystack.includes(term))
  })
}

export const sourceLogMatchesQuery = (row: SystemLogItem, query: string, gameName = '') => {
  const term = query.trim().toLocaleLowerCase()
  if (!term) return true
  return [row.game_id, gameName, row.source_group, row.job_id, row.message, row.latest_issue, systemLogEventLabel(row.event_type), systemLogStatusLabel(row.status)]
    .join(' ').toLocaleLowerCase().includes(term)
}

export const dateBoundaryISO = (value: string, end = false) => {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return ''
  const date = new Date(`${value}T00:00:00`)
  if (Number.isNaN(date.getTime())) return ''
  if (end) date.setDate(date.getDate() + 1)
  return date.toISOString()
}
