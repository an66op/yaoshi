import { describe, expect, it } from 'vitest'
import type { AdminGame, FeedStatus } from '../api'
import { buildInterfaceHealthLines, summarizeInterfaceHealthLines } from './interfaceHealth'

const game = (overrides: Partial<AdminGame> = {}): AdminGame => ({
  id: 'official-fc3d', code: 'FC3D', name: '福彩3D', category: '全国彩', lobby_category: '', lobby_sort_order: 0,
  badge: '', badge_color: '', enabled: true, issue: '2026001', next_draw_at: '', turnover: 0, profit: 0,
  source_kind: 'official', source_name: '中国福彩网', source_url: 'https://www.cwl.gov.cn/', sync_status: 'ok',
  last_sync_at: '2026-08-28T01:00:00Z', last_sync_error: '', schedule_mode: 'official-feed', source_healthy: true,
  ...overrides,
})

const feed = (overrides: Partial<FeedStatus> = {}): FeedStatus => ({
  running: true, server_time: '2026-08-28T02:00:00Z', server_time_ms: 0, timezone: 'Asia/Shanghai',
  jobs: [{
    id: 'china-welfare', name: '中国福利彩票开奖', group: 'china-welfare', game_ids: ['official-fc3d'], timezone: 'Asia/Shanghai',
    mode: 'normal', running: false, last_success_at: '2026-08-28T01:30:00Z', imported: 1, latest_issue: '2026001', consecutive_errors: 0,
  }],
  ...overrides,
})

describe('buildInterfaceHealthLines', () => {
  it('merges durable interface state with scheduler state', () => {
    const [line] = buildInterfaceHealthLines(feed(), [game()])
    expect(line.overallStatus).toBe('healthy')
    expect(line.interfaceStatus).toBe('ok')
    expect(line.schedulerStatus).toBe('scheduled')
    expect(line.lastSuccessAt).toBe('2026-08-28T01:30:00Z')
    expect(line.sourceURLs).toEqual(['https://www.cwl.gov.cn/'])
  })

  it('keeps consecutive scheduler failures visible even if the game row was previously healthy', () => {
    const failingFeed = feed({ jobs: [{ ...feed().jobs[0], consecutive_errors: 3, last_error: 'upstream timeout' }] })
    const [line] = buildInterfaceHealthLines(failingFeed, [game()])
    expect(line.overallStatus).toBe('error')
    expect(line.schedulerStatus).toBe('error')
    expect(line.consecutiveErrors).toBe(3)
    expect(line.lastError).toContain('upstream timeout')
  })

  it('shows enabled external games missing from the scheduler as an error line', () => {
    const [line] = buildInterfaceHealthLines(feed({ jobs: [] }), [game({ id: 'external-only', source_kind: 'external' })])
    expect(line.id).toBe('game:external-only')
    expect(line.schedulerStatus).toBe('missing')
    expect(line.overallStatus).toBe('error')
  })

  it('keeps the SG external adapter in its own scheduler health line', () => {
    const sg = game({ id: 'sg-ssc', name: 'SG时时彩', source_kind: 'external', source_name: 'SG时时彩双站校对', source_url: 'https://api.api168168.com/', schedule_mode: 'external-feed' })
    const sgJob = { ...feed().jobs[0], id: 'sg-ssc', name: 'SG时时彩双站校对', group: 'sg-ssc', game_ids: ['sg-ssc'] }
    const lines = buildInterfaceHealthLines(feed({ jobs: [...feed().jobs, sgJob] }), [game(), sg])
    const line = lines.find(item => item.id === 'sg-ssc')!
    expect(lines).toHaveLength(2)
    expect(line).toMatchObject({ group: 'sg-ssc', overallStatus: 'healthy', interfaceStatus: 'ok', schedulerStatus: 'scheduled' })
    expect(line.gameNames).toEqual(['SG时时彩'])
    expect(line.sourceKinds).toEqual(['external'])
    expect(line.sourceNames).toEqual(['SG时时彩双站校对'])
  })

  it('shows SG source disagreement as unhealthy even when the scheduler is waiting normally', () => {
    const sg = game({ id: 'sg-ssc', name: 'SG时时彩', source_kind: 'external', sync_status: 'paused', source_healthy: false, last_sync_error: 'SG双站同一期号码或开奖时间不一致，暂停导入' })
    const sgJob = { ...feed().jobs[0], id: 'sg-ssc', name: 'SG时时彩双站校对', group: 'sg-ssc', game_ids: ['sg-ssc'] }
    const [line] = buildInterfaceHealthLines(feed({ jobs: [sgJob] }), [sg])
    expect(line.interfaceStatus).toBe('error')
    expect(line.schedulerStatus).toBe('scheduled')
    expect(line.overallStatus).toBe('error')
    expect(line.lastError).toContain('SG双站同一期号码或开奖时间不一致，暂停导入')
  })

  it('does not report disabled integrations as outages', () => {
    const [line] = buildInterfaceHealthLines(feed(), [game({ enabled: false, sync_status: 'idle', last_sync_at: null })])
    expect(line.interfaceStatus).toBe('disabled')
    expect(line.overallStatus).toBe('disabled')
  })

  it('does not count disabled integrations as healthy enabled lines', () => {
    const healthy = buildInterfaceHealthLines(feed(), [game()])[0]
    const disabled = buildInterfaceHealthLines(feed(), [game({ enabled: false, sync_status: 'idle', last_sync_at: null })])[0]
    expect(summarizeInterfaceHealthLines([healthy, disabled])).toEqual({
      total: 2,
      enabled: 1,
      disabled: 1,
      healthy: 1,
      checking: 0,
      pending: 0,
      error: 0,
    })
  })
})
