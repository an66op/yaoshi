import { describe, expect, it } from 'vitest'
import type { AuditLog, SystemLogItem } from './api'
import { dateBoundaryISO, filterAuditLogs, mergeLogPage, sourceLogMatchesQuery, systemLogEventLabel, systemLogStatusLabel } from './systemLogs'

const sourceLog = (changes: Partial<SystemLogItem> = {}): SystemLogItem => ({
  id: 10, category: 'source', event_type: 'sync_error', level: 'error', status: 'error', source_group: 'pc28-163',
  game_id: 'pc-canada', job_id: '', message: '163母源超过预期时间未更新', imported: 0, latest_issue: '3477901', consecutive_errors: 3,
  created_at: '2026-09-04T11:00:00Z', ...changes,
})
const auditLog = (changes: Partial<AuditLog> = {}): AuditLog => ({
  id: 8, actor_id: 1, actor_name: '系统管理员', actor_role: 'admin', method: 'PUT', path: '/api/admin/games/pc-canada',
  status_code: 200, request_id: 'request-1', ip: '127.0.0.1', created_at: '2026-09-04T12:00:00Z', ...changes,
})

describe('system log presentation', () => {
  it('uses operator-facing labels for source and scheduler transitions', () => {
    expect(systemLogEventLabel('sync_error')).toBe('开奖源异常')
    expect(systemLogEventLabel('sync_recovered')).toBe('开奖源恢复')
    expect(systemLogEventLabel('scheduler_started')).toBe('调度启动')
    expect(systemLogStatusLabel('error')).toBe('异常')
    expect(systemLogStatusLabel('ok')).toBe('已恢复')
  })

  it('deduplicates cursor overlap and keeps newest-first order', () => {
    expect(mergeLogPage([sourceLog({ id: 10 }), sourceLog({ id: 9 })], [sourceLog({ id: 9 }), sourceLog({ id: 8 })], false).map(row => row.id)).toEqual([10, 9, 8])
    expect(mergeLogPage([sourceLog({ id: 10 })], [sourceLog({ id: 4 })], true).map(row => row.id)).toEqual([4])
  })

  it('searches source logs by translated status, game name, issue and reason', () => {
    const row = sourceLog()
    for (const query of ['开奖源异常', '异常', '加拿大', '3477901', '母源']) expect(sourceLogMatchesQuery(row, query, 'PC加拿大')).toBe(true)
    expect(sourceLogMatchesQuery(row, 'SG飞艇', 'PC加拿大')).toBe(false)
  })

  it('filters loaded operation logs without changing cursor data', () => {
    const rows = [auditLog(), auditLog({ id: 7, actor_name: '运营员', path: '/api/admin/system', status_code: 500 })]
    expect(filterAuditLogs(rows, '管理员', 'success').map(row => row.id)).toEqual([8])
    expect(filterAuditLogs(rows, '/system', 'error').map(row => row.id)).toEqual([7])
    expect(rows).toHaveLength(2)
  })

  it('converts local day filters to an exclusive RFC3339 end boundary', () => {
    const start = dateBoundaryISO('2026-09-04')
    const end = dateBoundaryISO('2026-09-04', true)
    expect(new Date(end).getTime() - new Date(start).getTime()).toBe(24 * 60 * 60 * 1000)
    expect(dateBoundaryISO('bad-date')).toBe('')
  })
})
