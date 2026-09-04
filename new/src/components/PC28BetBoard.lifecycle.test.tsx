import { isValidElement, type ComponentProps, type ReactElement, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AssistantBetResult } from '../api/bets'
import type { GameOdds } from '../api/portal'
import type { Game } from '../types'
import { HookHarness } from '../test/hookHarness'
import { pc28PricedPlayCodes } from '../utils/pc28BetSelection'
import { resolveLotteryTiming } from '../utils/lotteryTiming'
import { PC28BetBoard } from './PC28BetBoard'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))

type Props = ComponentProps<typeof PC28BetBoard>
type NodeProps = { children?: ReactNode; className?: string; role?: string; hidden?: boolean; disabled?: boolean; 'aria-label'?: string; 'aria-pressed'?: boolean; 'data-choice'?: string; onClick?: () => void }
const timing = resolveLotteryTiming({ issue_status: 'accepting', source_healthy: true, next_draw_at: '2026-09-02T06:46:00Z', seal_seconds: 30 }, Date.parse('2026-09-02T06:45:00Z'))
const game: Game = { id: 'canada-28', title: '加拿大28', tag: '', category: 'PC28', lobbyCategory: 'lottery', online: '', period: '20260902001', latestIssue: '20260902000', due: timing.due, timing, balls: [9, 1, 9], color: '', issueStatus: 'accepting', sourceKind: 'platform', sourceName: '王者开奖', sourceHealthy: true, syncStatus: '', sourceError: '', rulesReady: true, ruleVersion: 'pc28-v2' }
const quote = (play_code: string, odds = 2): GameOdds['items'][number] => ({ play_code, play_name: play_code, odds, min_bet: 1, max_bet: 500, max_user_period: 10000 })
const baseQuotes = [
  quote('pc28_sum_exact_0_27', 888), quote('pc28_package_three', 3),
  quote('pc28_position_number', 9.8), quote('pc28_position_two_sided', 1.98),
  quote('pc28_dragon_tiger', 1.98), quote('pc28_sum_size', 1.98), quote('pc28_sum_parity', 1.98),
  quote('pc28_combo_big_odd', 3.65), quote('pc28_extreme', 12), quote('pc28_leopard', 60), quote('pc28_color_red', 2.8),
]
const oddsInfo: GameOdds = { game_id: game.id, game_name: game.title, show_odds: true, rules_ready: true, rule_version: 'pc28-v2', items: baseQuotes }
const result: AssistantBetResult = { game_id: game.id, game_name: game.title, issue: game.period, content: '网投 1 注', lines: [], bet_count: 1, total: 20, balance: 980, accepted_at: '2026-09-02T06:45:02Z', rule_version: 'pc28-v2' }

function elements(node: ReactNode): ReactElement<NodeProps>[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  return isValidElement<NodeProps>(node) ? [node, ...elements(node.props.children)] : []
}
function text(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(text).join('')
  return isValidElement<NodeProps>(node) ? text(node.props.children) : ''
}

describe('PC28 detailed web board lifecycle', () => {
  let props: Props
  const render = () => {
    runtime.hooks!.render(() => PC28BetBoard(props))
    runtime.hooks!.flushEffects()
    return runtime.hooks!.render(() => PC28BetBoard(props))
  }
  const find = (predicate: (node: ReactElement<NodeProps>) => boolean) => elements(render()).find(predicate)
  const button = (label: string) => find(node => node.type === 'button' && text(node) === label)!
  const choice = (value: string) => find(node => node.props['data-choice'] === value)!
  const settle = async () => { for (let pass = 0; pass < 10; pass += 1) await Promise.resolve() }

  beforeEach(() => {
    runtime.hooks = new HookHarness()
    props = { game, ruleVersion: 'pc28-v2', oddsInfo, rulesReady: true, rulesMessage: '', onClose: vi.fn(), onConfirm: vi.fn(async () => result) }
  })

  it('renders every PC-specific market and reuses the room collapse interaction', () => {
    const tree = render()
    expect(find(node => node.props['aria-label'] === 'PC28详细网投面板')).toBeDefined()
    for (const label of ['和值', '包三', '三球定位', '龙虎和', '混合', '形态', '色波']) expect(text(tree)).toContain(label)
    const collapse = find(node => node.props.className === 'detail-panel-collapse')!
    collapse.props.onClick!()
    expect(props.onClose).toHaveBeenCalledOnce()
  })

  it('enables exact sums only when their actual symmetric play code is returned', () => {
    expect(text(render())).toContain('赔率目录 11/32')
    expect(choice('0').props.disabled).toBe(false)
    expect(choice('27').props.disabled).toBe(false)
    expect(choice('1').props.disabled).toBe(true)
    expect(text(choice('1'))).toContain('待配置')
  })

  it.each([
    ['pc-canada', 'PC加拿大', 'pc28-v1'],
    ['canada-28', '加拿大28', 'pc28-v2'],
    ['canada-20', '加拿大2.0', 'pc28-v3'],
  ])('binds all 32 current price rows to the exact product identity for %s', (gameId, title, ruleVersion) => {
    props.game = { ...game, id: gameId, title, ruleVersion }
    props.ruleVersion = ruleVersion
    props.oddsInfo = {
      ...oddsInfo,
      game_id: gameId,
      game_name: title,
      rule_version: ruleVersion,
      items: pc28PricedPlayCodes.map(playCode => quote(playCode, 2)),
    }
    expect(text(render())).toContain('赔率目录 32/32')
    expect(choice('0').props.disabled).toBe(false)

    props.oddsInfo = { ...props.oddsInfo, game_id: 'another-game' }
    expect(text(render())).toContain('赔率目录 0/32')
    expect(text(render())).toContain('赔率目录身份或规则版本不匹配')
    expect(choice('0').props.disabled).toBe(true)
    expect(find(node => node.props.className === 'full-bet-confirm')!.props.disabled).toBe(true)
  })

  it('keeps the tie disabled until its independent quote exists', () => {
    button('龙虎和').props.onClick!()
    expect(choice('龙').props.disabled).toBe(false)
    expect(choice('虎').props.disabled).toBe(false)
    expect(choice('和').props.disabled).toBe(true)
    props.oddsInfo = { ...oddsInfo, items: [...oddsInfo.items, quote('pc28_dragon_tiger_tie', 8.5)] }
    expect(choice('和').props.disabled).toBe(false)
    expect(text(choice('和'))).toContain('8.50')
  })

  it('fails closed inside the board when the draw source becomes unhealthy', () => {
    props.game = { ...game, sourceHealthy: false }
    props.rulesMessage = '开奖同步暂时暂停，当前可查看已公布结果和聊天，投注已暂停。'
    expect(choice('0').props.disabled).toBe(true)
    expect(text(render())).toContain(props.rulesMessage)
    expect(text(find(node => node.props.className === 'full-bet-confirm'))).toContain('开奖暂停')
  })

  it('submits package-three as one sorted typed row and clears the accepted cart', async () => {
    button('包三').props.onClick!()
    choice('13').props.onClick!()
    choice('1').props.onClick!()
    choice('7').props.onClick!()
    button('加入包三清单').props.onClick!()
    const confirm = find(node => node.props.className === 'full-bet-confirm')!
    expect(confirm.props.disabled).toBe(false)
    confirm.props.onClick!()
    await settle()
    expect(props.onConfirm).toHaveBeenCalledWith([{ play_code: 'pc28_package_three', play_name: '特码包三', position: 0, selection: '1,7,13', amount: 20 }])
    expect(text(render())).toContain('已受理 1 注')
    expect(text(render())).toContain('已选 0 组 · 0 注')
  })

  it('edits all three positions independently and includes each position in the typed row', async () => {
    button('三球定位').props.onClick!()
    button('第三球').props.onClick!()
    choice('8').props.onClick!()
    const confirm = find(node => node.props.className === 'full-bet-confirm')!
    confirm.props.onClick!()
    await settle()
    expect(props.onConfirm).toHaveBeenCalledWith([{ play_code: 'pc28_position_number', play_name: '三球定位号码', position: 3, selection: '8', amount: 20 }])
  })

  it('documents the original circular straight boundary and disables all prices on a version mismatch', () => {
    button('形态').props.onClick!()
    expect(text(render())).toContain('890、901及同组不同排列均算顺子')
    expect(choice('豹子').props.disabled).toBe(false)
    props.oddsInfo = { ...oddsInfo, rule_version: 'pc28-v1' }
    expect(choice('豹子').props.disabled).toBe(true)
    expect(find(node => node.props.className === 'full-bet-confirm')!.props.disabled).toBe(true)
  })

  it.each([
    ['pc-canada', 'pc28-v1', ['全部总注严格大于1且开13/14时', '和值大小/单双按1.5倍', '全部总注严格大于9999', '和值组合按1倍', '有效流水为0']],
    ['canada-28', 'pc28-v2', ['全部总注严格大于1且开13/14时', '和值大小/单双按1.5倍', '全部总注严格大于9999', '和值组合庄家通吃']],
    ['canada-20', 'pc28-v3', ['全部总注严格大于1且开13/14时', '和值大小/单双按1.98倍', '和值组合按3.65倍']],
  ])('discloses the exact 13/14 dynamic settlement terms for %s / %s on every affected market', (gameId, ruleVersion, expected) => {
    props.game = { ...game, id: gameId, title: gameId, ruleVersion }
    props.ruleVersion = ruleVersion
    props.oddsInfo = { ...oddsInfo, game_id: gameId, game_name: gameId, rule_version: ruleVersion }
    render()
    button('混合').props.onClick!()
    for (const market of ['和值大小', '和值单双', '和值组合']) {
      button(market).props.onClick!()
      const disclosure = text(render())
      for (const phrase of expected) expect(disclosure).toContain(phrase)
      expect(disclosure).toContain('当前房间、当前彩种、当前期')
      expect(disclosure).toContain('页面展示的是当前房间基础赔率')
      expect(disclosure).toContain('按当前规则版本及本期总注由服务端结算')
    }
    button('三球定位').props.onClick!()
    expect(text(render())).not.toContain('页面展示的是当前房间基础赔率')
  })

  it('drops an unsubmitted cart when the issue or financial version changes', () => {
    choice('0').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 1 注')
    props.game = { ...game, period: '20260902002' }
    render()
    expect(text(render())).toContain('已选 0 组 · 0 注')
  })
})
