import { Alert, Chip, MenuItem } from '@mui/material'
import { isValidElement, type ReactElement, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AdminGame } from '../api'
import { SGHistoryRecoveryPanel } from '../components/SGHistoryRecoveryPanel'

const runtime = vi.hoisted(() => ({ states: [] as unknown[], cursor: 0 }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T) => [runtime.states[runtime.cursor++] ?? initial, vi.fn()],
  useMemo: <T,>(factory: () => T) => factory(),
  useEffect: () => undefined,
}))
vi.mock('../api', () => ({ adminApi: {} }))
vi.mock('../components/feedback', () => ({ useFeedback: () => ({ showMessage: vi.fn() }) }))
vi.mock('../hooks/useServerClock', () => ({ useServerClock: () => ({ now: Date.parse('2026-09-03T00:00:00Z'), synced: true, latency: 0 }) }))

import { ResultsPage } from './ResultsPage'

type Props = { children?: ReactNode; actions?: ReactNode; value?: string; severity?: string; label?: string; color?: string; eyebrow?: string }
function elements(node: ReactNode): ReactElement<Props>[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  if (!isValidElement<Props>(node)) return []
  return [node, ...elements(node.props.children), ...elements(node.props.actions)]
}
function text(node: ReactNode): string {
  if (Array.isArray(node)) return node.map(text).join('')
  if (isValidElement<Props>(node)) return text(node.props.children)
  return typeof node === 'string' || typeof node === 'number' ? String(node) : ''
}

const sg: AdminGame = {
  id: 'sg-ssc', code: 'SGSSC', name: 'SG时时彩', category: 'ssc', lobby_category: '彩票', lobby_sort_order: 1,
  badge: '', badge_color: '', enabled: true, issue: '20260903095', current_issue: '20260903096', issue_status: 'accepting',
  accept_at: '2026-09-02T23:55:00Z', seal_at: '2026-09-03T00:00:30Z', next_draw_at: '2026-09-03T00:01:00Z', turnover: 0, profit: 0,
  source_kind: 'external', source_name: 'SG时时彩双站校对', source_url: 'https://api.api168168.com/',
  sync_status: 'ok', source_healthy: true, last_sync_at: '2026-09-03T00:00:00Z', last_sync_error: '', schedule_mode: 'external-feed',
  rules_ready: true, rule_version: 'digits5-v3',
}
const official: AdminGame = { ...sg, id: 'official-fc3d', name: '福彩3D', source_kind: 'official', source_name: '中国福彩网' }
const platform: AdminGame = { ...sg, id: 'speed-ssc', name: '极速时时彩', source_kind: 'platform', source_name: '王者开奖' }
function render(current = sg) {
  runtime.cursor = 0
  runtime.states = [[current, official, platform], current.id, [], false, '', '', 0, 10, false, false, '', null]
  return ResultsPage()
}
const sourceAlert = (root: ReactNode) => elements(root).find(element => element.type === Alert && text(element).includes('外部数据'))!

beforeEach(() => { runtime.cursor = 0; runtime.states = [] })

describe('ResultsPage source identity', () => {
  it('mounts historical recovery only for the selected SG game', () => {
    expect(elements(render()).filter(element => element.type === SGHistoryRecoveryPanel)).toHaveLength(1)
    expect(elements(render(official)).filter(element => element.type === SGHistoryRecoveryPanel)).toHaveLength(0)
    expect(elements(render(platform)).filter(element => element.type === SGHistoryRecoveryPanel)).toHaveLength(0)
  })
  it('labels SG as external without changing official or platform labels', () => {
    const root = render()
    const options = elements(root).filter(element => element.type === MenuItem)
    expect(text(options.find(option => option.props.value === sg.id))).toBe('【外部】SG时时彩')
    expect(text(options.find(option => option.props.value === official.id))).toBe('【官方】福彩3D')
    expect(text(options.find(option => option.props.value === platform.id))).toBe('【平台】极速时时彩')
    const alert = sourceAlert(root)
    expect(text(alert)).toContain('外部数据 · SG时时彩双站校对')
    expect(alert.props.severity).toBe('success')
    expect(elements(root).find(element => element.props.eyebrow)?.props.eyebrow).toBe('游戏运营 / 开奖来源')
    expect(text(root)).not.toContain('官方开奖同步服务')
  })

  it.each(['error', 'stale', 'paused'])('shows SG %s source status as a warning with the upstream reason', sync_status => {
    const alert = sourceAlert(render({ ...sg, sync_status, last_sync_error: 'SG双站同一期号码或开奖时间不一致，已暂停' }))
    expect(alert.props.severity).toBe('warning')
    expect(text(alert)).toContain('SG双站同一期号码或开奖时间不一致，已暂停')
  })

  it('does not paint an unhealthy SG source green when its last sync status was ok', () => {
    const root = render({ ...sg, source_healthy: false })
    const alert = sourceAlert(root)
    expect(alert.props.severity).toBe('warning')
    const chips = elements(root).filter(element => element.type === Chip)
    expect(chips.some(chip => chip.props.label === '开奖源异常 · 已停盘' && chip.props.color !== 'success')).toBe(true)
    expect(chips.some(chip => chip.props.label === '受理中')).toBe(false)
    expect(elements(render()).some(element => element.type === Chip && element.props.label === '受理中')).toBe(true)
  })

  it('keeps the latest trusted result separate when SG has no confirmed current issue', () => {
    const root = render({ ...sg, current_issue: '', issue_status: 'awaiting_draw', source_healthy: false })
    expect(text(root)).toContain('最近开奖期号：20260903095')
    expect(text(root).match(/20260903095/g)).toHaveLength(1)
  })
})
