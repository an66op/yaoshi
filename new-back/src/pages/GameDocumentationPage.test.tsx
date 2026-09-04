import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import type { AdminGame, GameOddsLimits, PlayCatalogItem } from '../api'

vi.mock('../api', () => ({ adminApi: { games: vi.fn(), playCatalog: vi.fn(), oddsLimits: vi.fn() } }))

import { CurrentRules, GameDocumentationPage } from './GameDocumentationPage'

const game: AdminGame = {
  id: 'sg-ssc', code: 'SGSSC', name: 'SG时时彩', category: 'ssc', lobby_category: 'lottery', lobby_sort_order: 1,
  badge: '', badge_color: '', enabled: true, issue: '10001', next_draw_at: '2026-09-03T00:01:00Z', turnover: 0, profit: 0,
  source_kind: 'external', source_name: 'SG时时彩母源（163:64＋115校验）', source_url: 'http://23.97.72.253:50163/', sync_status: 'ok', last_sync_at: null, last_sync_error: '', schedule_mode: 'external-feed',
  rules_ready: true, rule_version: 'digits5-v3',
}
const catalog: PlayCatalogItem[] = [
  { play_code: 'two_sided', play_name: '球位两面', category: '球位', description: '第1–5球大小单双', example: '1/大/20', sort_order: 0 },
  { play_code: 'dragon_tiger_tie', play_name: '龙虎和局', category: '龙虎和', description: '第一球与第五球相同', example: '1/和/20', sort_order: 1 },
]
const limits: GameOddsLimits = {
  game_id: game.id, game_name: game.name, rules_ready: true, rule_version: 'digits5-v3',
  items: [{ play_code: 'two_sided', play_name: '球位两面', odds: 1.975, min_bet: 1, max_bet: 1000, max_user_period: 1000, max_period_total: 10000, sort_order: 0, configured: true, rule_version: 'digits5-v3', configuration_source: 'admin_save' }],
}

describe('GameDocumentationPage', () => {
  it('presents original, current and difference sources as separate read-only views', () => {
    const html = renderToStaticMarkup(<GameDocumentationPage />)
    expect(html).toContain('游戏说明')
    expect(html).toContain('原版说明')
    expect(html).toContain('当前所有规则')
    expect(html).toContain('与原版的差异')
    expect(html).toContain('本页面只读')
  })

  it('shows only current backend odds and leaves missing markets visibly unconfigured', () => {
    const html = renderToStaticMarkup(<CurrentRules game={game} catalog={catalog} limits={limits} loading={false} error="" />)
    expect(html).toContain('当前赔率')
    expect(html).not.toContain('默认赔率')
    expect(html).toContain('1.975')
    expect(html).toContain('待配置')
    const table = html.match(/<table[\s\S]*?<\/table>/)?.[0] ?? ''
    expect([...table.matchAll(/<th\b/g)]).toHaveLength(4)
    expect(html).toContain('163目录ID64是唯一号码母源')
    expect(html).toContain('115的sgssc产品只读校验')
    expect(html).toContain('最近连续24期')
    expect(html).toContain('不能替代或补写ID64缺失的号码')
    expect(html).toContain('暂停导入、投注及未核验期结算')
    expect(html).toContain('按匹配的注单来源快照幂等处理')
    expect(html).toContain('不保证上游独立')
    expect(html).toContain('ID169属于另一套开奖结果系统')
    expect(html).not.toContain('api.api168168.com')
    expect(html).not.toContain('王者开奖')
  })

  it('never borrows a removed catalog default when no backend quote exists', () => {
    const staleCatalog = catalog.map(item => ({ ...item, default_odds: 1234.567 }))
    const html = renderToStaticMarkup(<CurrentRules game={game} catalog={staleCatalog} limits={null} loading={false} error="" />)
    expect(html).not.toContain('默认赔率')
    expect(html).not.toContain('1234.567')
    expect(html).not.toContain('1.975')
    expect(html.match(/待配置/g)).toHaveLength(2)
  })
})
