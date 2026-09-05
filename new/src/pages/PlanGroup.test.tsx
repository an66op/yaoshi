import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PlanDetail as PlanDetailData, PlanGameSummary, PlanRecommendation, RacingPlanDetail, RacingPlanSelection } from '../api/plans'
import type { Game } from '../types'
import { resolveLotteryTiming } from '../utils/lotteryTiming'
import { PlanDetail, PlanLobby } from './PlanGroup'
import { racingPlanDetail } from '../test/racingPlanFixtures'
import { DEFAULT_RACING_PLAN } from '../utils/racingPlans'

const feed = vi.hoisted(() => ({ catalog: [] as PlanGameSummary[], detail: null as PlanDetailData | null, racing: null as RacingPlanDetail | null, selection: { position: 1, plan_key: 'four-period-five-codes' } as RacingPlanSelection, error: '' }))
vi.mock('../hooks/usePlanFeed', () => ({
  usePlanCatalog: () => ({ data: feed.catalog, loading: false, error: feed.error }),
  usePlanDetail: () => ({ data: feed.detail, loading: false, error: feed.error }),
  useRacingPlanStream: () => ({ data: feed.racing, selection: feed.selection, loading: false, error: feed.error, activating: false, activationError: '', activate: vi.fn(), clearActivationError: vi.fn() }),
}))
const game: Game = {
  id: 'speed-racing', title: '极速赛车', tag: '赛车', category: '赛车', lobbyCategory: '彩票', online: '—', period: '101', latestIssue: '100', due: '00:30',
  timing: resolveLotteryTiming({ issue_status: 'awaiting_draw', source_healthy: true }, Date.now()),
  color: '#289fac', balls: [1, 2, 3], issueStatus: 'awaiting_draw', sourceKind: 'external', sourceName: '', sourceHealthy: true, syncStatus: 'ok', sourceError: '',
}
const master: PlanRecommendation = {
  id: 1, workspace_id: 2, game_id: game.id, issue: '100', master_name: '1号专家', master_title: '系统自动推荐', master_color: '#2aa9b3',
  numbers: [1, 3, 5, 9, 10], size: '', parity: '', result: 'pending', source: 'demo', note: '系统自动生成，仅供娱乐参考，不保证命中。', enabled: true, sort_order: 10,
  master_hit_rate: null, master_sample_count: 0, created_at: '2026-08-30T08:00:00Z', updated_at: '2026-08-30T08:00:00Z',
}
const masters = [master, { ...master, id: 2, master_name: '2号专家', sort_order: 20 }, { ...master, id: 3, master_name: '3号专家', sort_order: 30 }]

describe('plan group publications', () => {
  beforeEach(() => {
    feed.error = ''
    feed.catalog = [{ game_id: game.id, current_issue: '', latest_issue: '100', history_only: true, master_count: 3, updated_at: master.updated_at }]
    feed.detail = { game_id: game.id, current_issue: '', recommendations: [], latest_recommendations: masters, history: masters }
    feed.selection = { ...DEFAULT_RACING_PLAN }
    feed.racing = racingPlanDetail(DEFAULT_RACING_PLAN, { current_issue: '', recommendations: [] })
  })
  it('keeps a published game on the shelf after sealing', () => {
    const html = renderToStaticMarkup(<PlanLobby games={[game]} onBack={() => {}} onSelect={() => {}} />)
    expect(html).toContain('极速赛车')
    expect(html).toContain('位专家')
    expect(html).toContain('查看最近发布')
    expect(html).not.toContain('暂无计划推荐')
  })
  it('offers an unpublished allowed game without invented expert counts or issue labels', () => {
    feed.catalog = [{ game_id: game.id, current_issue: '', latest_issue: '', history_only: true, master_count: 0, updated_at: '' }]
    const html = renderToStaticMarkup(<PlanLobby games={[game]} onBack={() => {}} onSelect={() => {}} />)
    expect(html).toContain('可切换计划 · 等待首次发布')
    expect(html).toContain('选择计划')
    expect(html).not.toContain('0 位专家')
    expect(html).not.toContain('最近 — 期')
    expect(html).not.toContain('查看最近发布')
  })
  it('shows all three experts and distinguishes historical plans from current picks', () => {
    const html = renderToStaticMarkup(<PlanDetail games={[game]} gameId={game.id} onBack={() => {}} />)
    for (const row of masters) expect(html).toContain(row.master_name)
    expect(html).toContain('历史计划，非本期推荐')
    expect(html).not.toContain('系统自动生成，仅供娱乐参考，不保证命中。')
    expect(html).toContain('暂无开奖样本')
    expect(html).not.toContain('历史命中率 100%')
    expect(html).toContain('冠军 · 四期五码')
    expect(html).not.toContain('演示')
  })
  it('keeps a transient refresh error alongside the last publication', () => {
    feed.error = '连接恢复中'
    const html = renderToStaticMarkup(<PlanDetail games={[game]} gameId={game.id} onBack={() => {}} />)
    expect(html).toContain('连接恢复中')
    expect(html).toContain(master.master_name)
    expect(html).toContain('第 100 期')
    expect(html).toContain('历史计划，非本期推荐')
  })
  it('marks a real current publication as current without fabricated statistics', () => {
    feed.racing = racingPlanDetail()
    const html = renderToStaticMarkup(<PlanDetail games={[game]} gameId={game.id} onBack={() => {}} />)
    expect(html).toContain('本期计划')
    expect(html).not.toContain('历史计划，非本期推荐')
    expect(html).not.toContain('undefined%')
  })
  it('labels an unavailable selection as read-only rather than claiming the entire automation is off', () => {
    const detail = racingPlanDetail(DEFAULT_RACING_PLAN, { automation_enabled: false })
    detail.stream.allowed = false
    feed.racing = detail
    const html = renderToStaticMarkup(<PlanDetail games={[game]} gameId={game.id} onBack={() => {}} />)
    expect(html).toContain('当前计划暂未开放 · 只读记录')
    expect(html).not.toContain('自动推荐已关闭')
    expect(html).toContain('最近 6 期发布记录')
  })
  it('renders the selected position/type and persisted cycle progress', () => {
    feed.racing = racingPlanDetail()
    const html = renderToStaticMarkup(<PlanDetail games={[game]} gameId={game.id} onBack={() => {}} />)
    expect(html).toContain('切换计划')
    expect(html).toContain('冠军 · 四期五码')
    expect(html).toContain('第 2 / 4 期')
    expect(html).toContain('5码推荐')
    expect(html).toContain('aria-label="推荐号码 1、2、3、4、5"')
    expect(html).not.toContain('plan-mode-tabs')
    expect(html).not.toContain('大小')
    expect(html).not.toContain('单双')
    expect(html).not.toContain('演示')
  })
  it('does not promote legacy three-number publications into the selected cycle stream', () => {
    feed.racing = racingPlanDetail(DEFAULT_RACING_PLAN, { current_issue: '', recommendations: [], latest_recommendations: [], history: [], legacy_history: [{ ...master, numbers: [1, 5, 9] }] })
    const html = renderToStaticMarkup(<PlanDetail games={[game]} gameId={game.id} onBack={() => {}} />)
    expect(html).not.toContain('aria-label="推荐号码 1、5、9"')
    expect(html).not.toContain('5码推荐')
    expect(html).toContain('当前计划正在准备中')
  })
  it.each([['four-period-six-codes', 6], ['four-period-seven-codes', 7], ['two-period-eight-codes', 8]] as const)('renders every number in %s without extra direction columns', (plan_key, count) => {
    feed.selection = { position: 10, plan_key }
    feed.racing = racingPlanDetail(feed.selection)
    const html = renderToStaticMarkup(<PlanDetail games={[game]} gameId={game.id} onBack={() => {}} />)
    expect(html).toContain(`aria-label="推荐号码 ${Array.from({ length: count }, (_, index) => index + 1).join('、')}"`)
    expect(html).toContain(`${count}码推荐`)
    expect(html).toContain('第十名')
  })
  it.each([['size-five-periods', '大'], ['parity-four-periods', '单'], ['dragon-tiger-three-periods', '龙']] as const)('renders a prominent genuine direction for %s', (plan_key, direction) => {
    feed.selection = { position: 6, plan_key }
    feed.racing = racingPlanDetail(feed.selection)
    const html = renderToStaticMarkup(<PlanDetail games={[game]} gameId={game.id} onBack={() => {}} />)
    expect(html).toContain(`class="racing-plan-direction">${direction}`)
    expect(html).not.toContain('aria-label="推荐号码')
    if (plan_key.startsWith('dragon')) expect(html).toContain('第六名 vs 第五名')
  })
  it('does not render another stream returned by a stale response', () => {
    feed.selection = { position: 3, plan_key: 'four-period-six-codes' }
    feed.racing = racingPlanDetail()
    const html = renderToStaticMarkup(<PlanDetail games={[game]} gameId={game.id} onBack={() => {}} />)
    expect(html).not.toContain('aria-label="推荐号码')
    expect(html).not.toContain('本期计划</em>')
  })
  it('opens a configured non-racing plan with its actual publication', () => {
		const otherGame = { ...game, id: 'speed-ssc', title: '极速时时彩' }
    const otherRow = { ...master, game_id: otherGame.id, numbers: [1, 3, 10] }
    feed.detail = { game_id: otherGame.id, current_issue: '100', recommendations: [otherRow], latest_recommendations: [otherRow], history: [otherRow] }
    const html = renderToStaticMarkup(<PlanDetail games={[otherGame]} gameId={otherGame.id} onBack={() => {}} />)
		expect(html).toContain('极速时时彩')
    expect(html).not.toContain('plan-mode-tabs')
    expect(html).toContain('aria-label="推荐号码 1、3、10"')
  })
  it('keeps same-named manual and automatic experts as separate tabs and statistics', () => {
    const otherGame = { ...game, id: 'speed-ssc', title: '极速时时彩' }
    const automatic = { ...master, game_id: otherGame.id, master_hit_rate: 100, master_sample_count: 1 }
    const manual = { ...automatic, id: 77, source: 'manual' as const, master_hit_rate: 0 }
    feed.detail = { game_id: otherGame.id, current_issue: '100', recommendations: [automatic, manual], latest_recommendations: [automatic, manual], history: [automatic, manual] }
    const html = renderToStaticMarkup(<PlanDetail games={[otherGame]} gameId={otherGame.id} onBack={() => {}} />)
    expect((html.match(/<b>1号专家<\/b>/g) || []).length).toBe(2)
    expect(html).toContain('命中 100% · 1期')
    expect(html).toContain('命中 0% · 1期')
  })
  it('shows only six actual racing periods and never mixes another selection into history', () => {
    const detail = racingPlanDetail()
    const row = detail.history[0]
    detail.history = Array.from({ length: 12 }, (_, index) => ({ ...row, id: 100 + index, issue: `saved-${20 - index}` }))
    detail.history.unshift({ ...row, id: 999, position: 2, issue: 'wrong-position' })
    feed.racing = detail
    const html = renderToStaticMarkup(<PlanDetail games={[game]} gameId={game.id} onBack={() => {}} />)
    expect(html).toContain('最近 6 期发布记录')
    expect(html).toContain('saved-15')
    expect(html).not.toContain('saved-14')
    expect(html).not.toContain('wrong-position')
    expect((html.match(/class="plan-history-row"/g) || []).length).toBe(6)
  })
	it('opens another racing-v2 product through its rich deep link and keeps only its saved stream', () => {
		const otherGame = { ...game, id: 'speed-fly', title: '极速飞艇' }
		const detail = racingPlanDetail(DEFAULT_RACING_PLAN, { game_id: otherGame.id })
		const row = detail.history[0]
		detail.history = Array.from({ length: 12 }, (_, index) => ({ ...row, id: 100 + index, issue: `saved-${20 - index}` }))
		detail.history.unshift({ ...row, id: 999, game_id: 'speed-racing', issue: 'wrong-game' })
		detail.history.unshift({ ...row, id: 998, source: 'manual', issue: 'wrong-source' })
		feed.racing = detail
		const html = renderToStaticMarkup(<PlanDetail games={[otherGame]} gameId={otherGame.id} onBack={() => {}} />)
    expect(html).toContain('极速飞艇')
    expect(html).toContain('saved-15')
    expect(html).not.toContain('saved-14')
    expect(html).not.toContain('wrong-game')
    expect(html).not.toContain('wrong-source')
    expect((html.match(/class="plan-history-row"/g) || []).length).toBe(6)
	})
	it('shows only verified hit statistics with their real sample size', () => {
		const detail = racingPlanDetail()
		for (const row of [...detail.recommendations, ...detail.latest_recommendations, ...detail.history]) {
			row.master_hit_rate = 50
			row.master_sample_count = 2
		}
		feed.racing = detail
		const html = renderToStaticMarkup(<PlanDetail games={[game]} gameId={game.id} onBack={() => {}} />)
		expect(html).toContain('命中 50% · 2期')
		expect(html).not.toContain('80%')
	})
  it('lists every configured game returned by the room catalog', () => {
    const otherGame = { ...game, id: 'speed-fly', title: '极速飞艇' }
    feed.catalog.push({ ...feed.catalog[0], game_id: otherGame.id })
    const html = renderToStaticMarkup(<PlanLobby games={[game, otherGame]} onBack={() => {}} onSelect={() => {}} />)
    expect(html).toContain('极速赛车')
    expect(html).toContain('极速飞艇')
    expect((html.match(/class="plan-game-card"/g) || []).length).toBe(2)
  })
  it('uses draw ball colors and real hit/miss states independently from cycle progress', () => {
    const detail = racingPlanDetail()
    detail.recommendations[0].result = 'hit'
    detail.recommendations[0].draw_numbers = [1, 6, 3, 4, 5, 2, 7, 8, 9, 10]
    detail.history = [detail.recommendations[0], { ...detail.recommendations[0], id: 900, issue: '99', result: 'miss', cycle_status: 'completed' }]
    feed.racing = detail
    const html = renderToStaticMarkup(<PlanDetail games={[game]} gameId={game.id} onBack={() => {}} />)
    expect(html).toContain('class="lottery-ball ball-1"')
    expect(html).toContain('class="lottery-ball ball-10"')
    expect(html).toContain('class="plan-outcome hit">中<')
    expect(html).toContain('class="plan-outcome miss">不中<')
    expect(html).toContain('aria-label="开奖号码 1、6、3、4、5、2、7、8、9、10"')
    expect(html).not.toContain('本轮已全部发布')
    expect(html).toContain('<span>切换计划</span>')
    expect(html).toContain('aria-haspopup="dialog"')
  })
})
