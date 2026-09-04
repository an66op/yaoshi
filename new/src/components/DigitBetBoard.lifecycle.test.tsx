import { isValidElement, type ComponentProps, type ReactElement, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Game } from '../types'
import { HookHarness } from '../test/hookHarness'
import { controlSurfaceProps } from '../utils/controlSurface'
import { digitNumbers, digitPatterns, digitSides } from '../utils/digitBetSelection'
import { resolveLotteryTiming } from '../utils/lotteryTiming'
import { DigitBetBoard } from './DigitBetBoard'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))
type Props = ComponentProps<typeof DigitBetBoard>
type NodeProps = { children?: ReactNode; className?: string; role?: string; 'aria-label'?: string; 'aria-pressed'?: boolean; 'aria-invalid'?: boolean; 'data-choice'?: string; disabled?: boolean; onClick?: () => void; onChange?: (event: { target: { value: string } }) => void }
const timing = resolveLotteryTiming({ issue_status: 'accepting', source_healthy: true, next_draw_at: '2026-08-31T06:46:00Z', seal_seconds: 30 }, Date.parse('2026-08-31T06:45:00Z'))
const game: Game = { id: 'sg-ssc', title: 'SG时时彩', tag: '', category: 'ssc', lobbyCategory: 'lottery', online: '', period: '10001', latestIssue: '10000', due: timing.due, timing, balls: [0, 9, 2, 7, 4], color: '', issueStatus: 'accepting', sourceKind: 'platform', sourceName: '王者开奖', sourceHealthy: true, syncStatus: '', sourceError: '', rulesReady: true, ruleVersion: 'digits5-v3' }
function find(node: ReactNode, predicate: (node: ReactElement<NodeProps>) => boolean): ReactElement<NodeProps> | undefined {
  if (Array.isArray(node)) return node.map(child => find(child, predicate)).find(Boolean)
  if (!isValidElement<NodeProps>(node)) return
  return predicate(node) ? node : find(node.props.children, predicate)
}
function text(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(text).join('')
  return isValidElement<NodeProps>(node) ? text(node.props.children) : ''
}

describe('digit lottery board lifecycle and submission gates', () => {
  let props: Props
  const render = () => {
    runtime.hooks!.render(() => DigitBetBoard(props))
    runtime.hooks!.flushEffects()
    return runtime.hooks!.render(() => DigitBetBoard(props))
  }
  const label = (value: string) => find(render(), node => node.props['aria-label'] === value)!
  const choice = (value: string) => find(render(), node => node.props['data-choice'] === value)!
  const confirm = () => find(render(), node => node.props.className === 'full-bet-confirm')!
  const clickChoice = (value: string) => { expect(choice(value).props.disabled).toBe(false); choice(value).props.onClick!() }
  const tab = (value: string) => find(render(), node => node.type === 'button' && text(node) === value)!.props.onClick!()
  const amount = (value: string) => label('自定义单注金额').props.onChange!({ target: { value } })

  beforeEach(() => {
    runtime.hooks = new HookHarness()
    props = { game, ballCount: 5, odds: { ball_1_5: 9.9, two_sided: 1.993, sum: 1.993, dragon_tiger: 1.85, leopard: 50, straight: 15, pair: 8, half_straight: 6, mixed: 4 }, oddsHidden: false, oddsResponseReady: true, onConfirm: vi.fn(), onClose: vi.fn() }
  })

  it('uses the same upper-right arrow to return an embedded digit board to chat', () => {
    props.embedded = true
    const collapse = label('收起详细投注，返回聊天')
    expect(collapse.props.className).toBe('detail-panel-collapse')
    collapse.props.onClick!()
    expect(props.onClose).toHaveBeenCalledOnce()
  })

  it.each(['speed-ssc', 'sg-ssc', 'bingo-ssc-1', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4', 'au-lucky-5'])('offers five independent balls numbered 0–9 for %s', id => {
    props.game = { ...game, id, ruleVersion: 'digits5-v3' }
    props.ruleVersion = 'digits5-v3'
    expect(label('数字彩投注面板').props).toMatchObject(controlSurfaceProps)
    expect(label('编辑第五球')).toBeDefined()
    expect(label('编辑第六球')).toBeUndefined()
    for (const value of digitNumbers) expect(choice(value)).toBeDefined()
    expect(choice('10')).toBeUndefined()
    expect(choice('龙')).toBeUndefined()
    expect(choice('和')).toBeUndefined()
    clickChoice('0')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenCalledWith('1/0/20')
  })

  it.each(['official-fc3d', 'official-pl3'])('limits %s to three balls while retaining first-three shapes', id => {
    props.game = { ...game, id, balls: [0, 4, 9], ruleVersion: 'digits3-v2' }
    props.ballCount = 3
    expect(label('编辑第三球')).toBeDefined()
    expect(label('编辑第四球')).toBeUndefined()
    expect(text(label('最近开奖号码'))).toBe('049')
    label('编辑第三球').props.onClick!()
    clickChoice('0')
    tab('前三形态')
    for (const item of digitPatterns) expect(choice(item.selection).props.disabled).toBe(false)
    clickChoice('杂六')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenCalledWith('3/0/20#前三/杂六/20')
    expect(text(render())).not.toContain('中三')
    expect(text(render())).not.toContain('后三')
  })

  it('preserves different balls without selecting a ball that was merely visited', () => {
    label('编辑第三球').props.onClick!()
    clickChoice('2')
    clickChoice('3')
    label('编辑第四球').props.onClick!()
    expect(choice('2').props['aria-pressed']).toBe(false)
    label('编辑第五球').props.onClick!()
    clickChoice('4')
    expect(text(render())).toContain('已选 2 组 · 3 注')
    expect(text(label('已选投注'))).toBe('第三球 2、3第五球 4')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenLastCalledWith('3/2/20#3/3/20#5/4/20')
    label('编辑第三球').props.onClick!()
    expect(choice('2').props['aria-pressed']).toBe(true)
    expect(choice('3').props['aria-pressed']).toBe(true)
    expect(choice('4').props['aria-pressed']).toBe(false)
    clickChoice('2')
    expect(text(confirm())).toBe('立即投注 ¥ 40')
  })

  it('keeps the sole five-ball comparison separate from individual ball choices', () => {
    label('编辑第五球').props.onClick!()
    clickChoice('2')
    tab('龙虎')
    expect(label('编辑第一球 vs 第五球').props['aria-pressed']).toBe(true)
    expect(label('编辑第二球 vs 第四球')).toBeUndefined()
    expect(text(choice('龙'))).toBe('龙1.85')
    expect(choice('和').props.disabled).toBe(true)
    expect(choice('大')).toBeUndefined()
    clickChoice('龙')
    clickChoice('虎')
    label('编辑第一球 vs 第五球').props.onClick!()
    expect(choice('龙').props['aria-pressed']).toBe(true)
    expect(choice('虎').props['aria-pressed']).toBe(true)
    expect(text(label('已选投注'))).toBe('第五球 2第一球 vs 第五球 龙、虎')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenCalledWith('5/2/20#1/龙/20#1/虎/20')
    tab('单球')
    expect(label('编辑第五球').props['aria-pressed']).toBe(true)
    expect(choice('2').props['aria-pressed']).toBe(true)
  })

  it.each(['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4'])('offers three independent shape segments and one independently-priced tie group for exact v3 %s', id => {
    props.game = { ...game, id, ruleVersion: 'digits5-v3' }
    props.ruleVersion = 'digits5-v3'
    props.odds = { ...props.odds, dragon_tiger_tie: 8.75 }
    expect(find(render(), node => node.type === 'button' && text(node) === '总和')).toBeUndefined()
    expect(find(render(), node => node.type === 'button' && text(node) === '总和尾')).toBeUndefined()
    tab('三段形态')
    expect(label('编辑前三形态').props['aria-pressed']).toBe(true)
    clickChoice('豹子')
    label('编辑中三形态').props.onClick!()
    clickChoice('顺子')
    label('编辑后三形态').props.onClick!()
    clickChoice('对子')
    tab('龙虎')
    expect(label('编辑第一球 vs 第五球')).toBeDefined()
    expect(label('编辑第二球 vs 第四球')).toBeUndefined()
    expect(text(choice('和'))).toBe('和8.75')
    clickChoice('和')
    expect(text(label('已选投注'))).toBe('前三形态 豹子中三形态 顺子后三形态 对子第一球 vs 第五球 和')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenCalledWith('前三/豹子/20#中三/顺子/20#后三/对子/20#1/和/20')
  })

  it.each(['speed-ssc', 'sg-ssc', 'au-lucky-5'])('opens every newly priced v3 shape and tie option from the authoritative %s response', id => {
    props.game = { ...game, id, ruleVersion: 'digits5-v3' }
    props.ruleVersion = 'digits5-v3'
    props.odds = {
      ball_1_5: 9.9, two_sided: 1.993, dragon_tiger: 1.993, dragon_tiger_tie: 8.7,
      leopard: 80, straight: 15.08, pair: 3.38, half_straight: 2.58, mixed: 3.08,
    }
    props.oddsInfo = {
      game_id: id, game_name: props.game.title, show_odds: true, rules_ready: true, rule_version: 'digits5-v3',
      items: Object.entries(props.odds).map(([play_code, value]) => ({
        play_code, play_name: play_code, odds: value, min_bet: 1, max_bet: 1000, max_user_period: 5000,
      })),
    }
    tab('三段形态')
    for (const item of digitPatterns) {
      expect(choice(item.selection).props.disabled).toBe(false)
      clickChoice(item.selection)
    }
    tab('龙虎')
    expect(choice('和').props.disabled).toBe(false)
    clickChoice('和')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenCalledWith('前三/豹子/20#前三/顺子/20#前三/对子/20#前三/半顺/20#前三/杂六/20#1/和/20')
  })

  it('disables a missing v3 tie price without disabling the configured dragon price', () => {
    props.game = { ...game, id: 'speed-ssc', rulesReady: true, ruleVersion: 'digits5-v3' }
    props.ruleVersion = 'digits5-v3'
    props.odds = { dragon_tiger: 1.92 }
    tab('龙虎')
    expect(choice('龙').props.disabled).toBe(false)
    expect(choice('和').props.disabled).toBe(true)
    clickChoice('龙')
    expect(confirm().props.disabled).toBe(false)
  })

  it.each(['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4'])('requires the exact current version before %s offers choices', id => {
    for (const ruleVersion of [undefined, '', 'digits5-v2', 'digits5-v4']) {
      props.game = { ...game, id, ruleVersion }
      props.ruleVersion = ruleVersion
      expect(choice('0')).toBeUndefined()
      expect(confirm().props.disabled).toBe(true)
      expect(text(render())).toContain('该彩种规则尚未就绪')
      confirm().props.onClick!()
      expect(props.onConfirm).not.toHaveBeenCalled()
    }
  })

  it('never opens a five-ball board for Bingo Racing B', () => {
    const id = 'bingo-racing-b'
    props.game = { ...game, id, ruleVersion: 'digits5-v3' }
    props.ruleVersion = 'digits5-v3'
    expect(choice('0')).toBeUndefined()
    tab('龙虎')
    expect(label('编辑第一球 vs 第五球')).toBeUndefined()
    expect(choice('和')).toBeUndefined()
    expect(confirm().props.disabled).toBe(true)
    confirm().props.onClick!()
    expect(props.onConfirm).not.toHaveBeenCalled()
  })

  it('rejects a five-ball model or response version that does not match the current game', () => {
    props.ballCount = 3
    expect(choice('0').props.disabled).toBe(true)
    props.ballCount = 5
    props.ruleVersion = 'digits5-v2'
    expect(choice('0')).toBeUndefined()
    expect(confirm().props.disabled).toBe(true)
    props.ruleVersion = 'digits5-v3'
    props.game = { ...game, ruleVersion: undefined }
    expect(choice('0').props.disabled).toBe(true)
    props.game = game
    props.oddsInfo = {
      game_id: game.id, game_name: game.title, show_odds: true, rules_ready: true, rule_version: 'digits5-v2',
      items: [{ play_code: 'ball_1_5', play_name: '号码', odds: 9.9, min_bet: 1, max_bet: 1000, max_user_period: 5000 }],
    }
    expect(choice('0').props.disabled).toBe(true)
    props.oddsInfo = { ...props.oddsInfo, game_id: 'au-lucky-5', rule_version: 'digits5-v3' }
    expect(choice('0').props.disabled).toBe(true)
    props.oddsInfo = { ...props.oddsInfo, game_id: game.id }
    clickChoice('0')
    expect(confirm().props.disabled).toBe(false)
  })

  it('limits three-ball games to first vs third and clears a prior five-ball choice on game switch', () => {
    tab('龙虎')
    clickChoice('虎')
    props.game = { ...game, id: 'official-fc3d', balls: [0, 4, 9], ruleVersion: 'digits3-v2' }
    props.ballCount = 3
    render()
    tab('龙虎')
    expect(label('编辑第一球 vs 第三球')).toBeDefined()
    expect(label('编辑第二球 vs 第四球')).toBeUndefined()
    expect(choice('虎').props['aria-pressed']).toBe(false)
    expect(confirm().props.disabled).toBe(true)
    clickChoice('龙')
    expect(text(confirm())).toBe('立即投注 ¥ 20')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenCalledWith('1/龙/20')
  })

  it('gates dragon/tiger choices on their own odds and freezes pair editing while submitting', () => {
    tab('龙虎')
    props.odds = { two_sided: 1.993, ball_1_5: 9.9 }
    expect(choice('龙').props.disabled).toBe(true)
    choice('龙').props.onClick!()
    expect(text(render())).toContain('已选 0 组 · 0 注')
    props.odds = { dragon_tiger: 1.92 }
    clickChoice('龙')
    expect(text(choice('龙'))).toBe('龙1.92')
    props.odds = {}
    expect(confirm().props.disabled).toBe(true)
    props.oddsHidden = true
    expect(confirm().props.disabled).toBe(false)
    expect(text(choice('龙'))).toContain('已隐藏')
    props.submitting = true
    expect(label('编辑第一球 vs 第五球').props.disabled).toBe(true)
    expect(choice('虎').props.disabled).toBe(true)
    expect(confirm().props.disabled).toBe(true)
  })

  it('keeps ball, total, tail and shape selections separate with uniform per-option stakes', () => {
    props.game = { ...game, id: 'official-fc3d', balls: [0, 4, 9], ruleVersion: 'digits3-v2' }
    props.ballCount = 3
    clickChoice('大')
    tab('总和')
    expect(choice('大').props['aria-pressed']).toBe(false)
    clickChoice('大')
    tab('总和尾')
    clickChoice('7')
    tab('前三形态')
    clickChoice('豹子')
    amount('1.25')
    expect(text(confirm())).toBe('立即投注 ¥ 5')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenCalledWith('1/大/1.25#总和/大/1.25#总和尾/7/1.25#前三/豹子/1.25')
    find(render(), node => node.props.className === 'full-bet-selection-toggle')!.props.onClick!()
    label('移除总和大').props.onClick!()
    expect(text(confirm())).toBe('立即投注 ¥ 3.75')
    tab('单球')
    expect(choice('大').props['aria-pressed']).toBe(true)
    tab('总和')
    expect(choice('大').props['aria-pressed']).toBe(false)
  })

  it.each(['0', '0.001', '20.', '1e2', '-20', '90071992547409.92'])('blocks invalid money %s without calling confirm', value => {
    clickChoice('0')
    amount(value)
    expect(label('自定义单注金额').props['aria-invalid']).toBe(true)
    expect(confirm().props.disabled).toBe(true)
    expect(text(render())).toContain('最多 2 位小数')
    confirm().props.onClick!()
    expect(props.onConfirm).not.toHaveBeenCalled()
  })

  it('accepts the four amount shortcuts and preserves two decimal places', () => {
    clickChoice('0')
    clickChoice('1')
    for (const value of [20, 50, 100, 200]) {
      tab(String(value))
      expect(text(confirm())).toBe(`立即投注 ¥ ${value * 2}`)
    }
    amount('0.11')
    expect(text(confirm())).toBe('立即投注 ¥ 0.22')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenCalledWith('1/0/0.11#1/1/0.11')
  })

  it('requires loaded odds, permits explicit hidden odds and disables unavailable shape odds', () => {
    props.oddsResponseReady = false
    expect(choice('0').props.disabled).toBe(true)
    choice('0').props.onClick!()
    expect(text(render())).toContain('已选 0 组 · 0 注')
    props.oddsResponseReady = true
    clickChoice('0')
    props.odds = {}
    expect(confirm().props.disabled).toBe(true)
    tab('三段形态')
    expect(choice('豹子').props.disabled).toBe(true)
    props.oddsHidden = true
    expect(confirm().props.disabled).toBe(false)
    clickChoice('豹子')
    expect(text(choice('豹子'))).toContain('已隐藏')
    props.oddsResponseReady = false
    expect(confirm().props.disabled).toBe(true)
  })

  it('does not expose a missing digit market merely because numeric odds are hidden', () => {
    props.odds = {}
    props.oddsHidden = true
    props.oddsInfo = {
      game_id: game.id, game_name: game.title, show_odds: false, rules_ready: true, rule_version: 'digits5-v3',
      items: [{ play_code: 'ball_1_5', play_name: '号码', odds: 0, min_bet: 1, max_bet: 1000, max_user_period: 5000 }],
    }
    expect(choice('0').props.disabled).toBe(false)
    tab('三段形态')
    expect(choice('豹子').props.disabled).toBe(true)
  })

  it('gates submission on ready rules, server-confirmed betting timing and in-flight state', () => {
    clickChoice('0')
    props.game = { ...game, rulesReady: false }
    expect(confirm().props.disabled).toBe(true)
    expect(text(confirm())).toContain('规则待配置')
    confirm().props.onClick!()
    expect(props.onConfirm).not.toHaveBeenCalled()
    props.game = { ...game, timing: { ...timing, accepting: false, statusLabel: '封盘中' } }
    expect(confirm().props.disabled).toBe(true)
    expect(text(confirm())).toContain('封盘中')
    props.game = { ...props.game, betting: { issue: '10002', timing } }
    expect(text(render())).toContain('第 10002 期')
    expect(confirm().props.disabled).toBe(false)
    props.submitting = true
    expect(confirm().props.disabled).toBe(true)
    expect(choice('0').props.disabled).toBe(true)
    expect(label('编辑第二球').props.disabled).toBe(true)
    expect(label('自定义单注金额').props.disabled).toBe(true)
    expect(label('关闭投注面板').props.disabled).toBe(true)
    confirm().props.onClick!()
    expect(props.onConfirm).not.toHaveBeenCalled()
  })

  it('does not carry a cart into a different game or ball-count model', () => {
    label('编辑第五球').props.onClick!()
    clickChoice('9')
    props.game = { ...game, id: 'official-fc3d', ruleVersion: 'digits3-v2' }
    props.ballCount = 3
    // The new context is blocked synchronously, before a state-reset effect can run.
    const pendingTree = runtime.hooks!.render(() => DigitBetBoard(props))
    expect(find(pendingTree, node => node.props.className === 'full-bet-confirm')!.props.disabled).toBe(true)
    expect(text(render())).toContain('已选 0 组 · 0 注')
    expect(label('编辑第一球').props['aria-pressed']).toBe(true)
    clickChoice('0')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenCalledWith('1/0/20')
  })

  it('drops a three-ball total tab and cart synchronously when switching to five-ball rules', () => {
    props.game = { ...game, id: 'official-fc3d', balls: [0, 4, 9], ruleVersion: 'digits3-v2' }
    props.ballCount = 3
    tab('总和')
    clickChoice('大')
    props.game = game
    props.ballCount = 5
    const pending = runtime.hooks!.render(() => DigitBetBoard(props))
    expect(find(pending, node => node.props.className === 'full-bet-confirm')!.props.disabled).toBe(true)
    expect(find(pending, node => node.type === 'button' && text(node) === '总和')).toBeUndefined()
    expect(text(render())).toContain('已选 0 组 · 0 注')
    clickChoice('0')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenCalledWith('1/0/20')
  })

  it('shows the 400-codepoint limit and never submits or silently splits an oversized cart', () => {
    const labels = ['第一球', '第二球', '第三球', '第四球', '第五球']
    for (const value of labels) {
      label(`编辑${value}`).props.onClick!()
      for (const selection of [...digitNumbers, ...digitSides]) clickChoice(selection)
    }
    expect(text(render())).toContain('已选 5 组 · 70 注')
    expect(text(confirm())).toContain('¥ 1400')
    expect(confirm().props.disabled).toBe(true)
    expect(text(render())).toContain('超过单次 400 字上限')
    expect(text(render())).toContain('不会自动拆单')
    confirm().props.onClick!()
    expect(props.onConfirm).not.toHaveBeenCalled()
    tab('清空选择')
    clickChoice('0')
    expect(confirm().props.disabled).toBe(false)
    expect(text(render())).not.toContain('超过单次 400 字上限')
  })
})
