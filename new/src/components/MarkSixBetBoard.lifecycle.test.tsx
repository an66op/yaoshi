import { isValidElement, type ComponentProps, type ReactElement, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AssistantBetResult } from '../api/bets'
import type { GameOdds } from '../api/portal'
import type { Game } from '../types'
import { HookHarness } from '../test/hookHarness'
import { resolveLotteryTiming } from '../utils/lotteryTiming'
import { MarkSixBetBoard } from './MarkSixBetBoard'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useMemo: <T,>(factory: () => T, dependencies: readonly unknown[]) => runtime.hooks!.useMemo(factory, dependencies),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))

type Props = ComponentProps<typeof MarkSixBetBoard>
type NodeProps = { children?: ReactNode; className?: string; role?: string; hidden?: boolean; disabled?: boolean; 'aria-label'?: string; 'aria-pressed'?: boolean; 'data-choice'?: string; onClick?: () => void }
const timing = resolveLotteryTiming({ issue_status: 'accepting', source_healthy: true, next_draw_at: '2026-09-01T06:46:00Z', seal_seconds: 30 }, Date.parse('2026-09-01T06:45:00Z'))
const game: Game = { id: 'bingo-mark-six', title: '宾果六合彩', tag: '', category: '六合彩', lobbyCategory: 'lottery', online: '', period: '115049456', latestIssue: '115049455', due: timing.due, timing, balls: [3, 8, 11, 14, 22, 24, 26], color: '', issueStatus: 'accepting', sourceKind: '', sourceName: '', sourceHealthy: true, syncStatus: '', sourceError: '', rulesReady: true, ruleVersion: 'mark6-v2' }
const quote = (play_code: string, odds = 48): GameOdds['items'][number] => ({ play_code, play_name: play_code, odds, min_bet: 1, max_bet: 200, max_user_period: 10000 })
const oddsInfo: GameOdds = { game_id: game.id, game_name: game.title, show_odds: true, rules_ready: true, rule_version: 'mark6-v2', items: [
  quote('marksix_special_a_number'), quote('marksix_special_b_number'), quote('marksix_special_big_small'), quote('marksix_special_odd_even'),
  quote('marksix_special_sum_big_small'), quote('marksix_special_sum_odd_even'), quote('marksix_special_heaven_earth'), quote('marksix_special_front_back'),
  quote('marksix_special_domestic_wild'), quote('marksix_special_tail_big_small'), quote('marksix_special_half'), quote('marksix_total_big_small'), quote('marksix_total_odd_even'),
  quote('marksix_regular_number'), quote('marksix_regular_position_number'), quote('marksix_regular_position_big_small'), quote('marksix_regular_position_odd_even'),
  quote('marksix_regular_position_sum_big_small'), quote('marksix_regular_position_sum_odd_even'), quote('marksix_regular_position_tail_big_small'),
  quote('marksix_regular_special_number'), quote('marksix_combo_4_all'), quote('marksix_combo_3_all'), quote('marksix_combo_2_all'), quote('marksix_combo_special_pair'), quote('marksix_not_in'),
] }
const result: AssistantBetResult = { game_id: game.id, game_name: game.title, issue: game.period, content: '网投 1 注', lines: [], bet_count: 1, total: 20, balance: 980, accepted_at: '2026-09-01T06:45:02Z' }

function elements(node: ReactNode): ReactElement<NodeProps>[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  return isValidElement<NodeProps>(node) ? [node, ...elements(node.props.children)] : []
}
function text(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(text).join('')
  return isValidElement<NodeProps>(node) ? text(node.props.children) : ''
}

describe('Bingo Mark Six board lifecycle', () => {
  let props: Props
  const render = () => {
    runtime.hooks!.render(() => MarkSixBetBoard(props))
    runtime.hooks!.flushEffects()
    return runtime.hooks!.render(() => MarkSixBetBoard(props))
  }
  const find = (predicate: (node: ReactElement<NodeProps>) => boolean) => elements(render()).find(predicate)
  const button = (label: string) => find(node => node.type === 'button' && text(node) === label)!
  const choice = (value: string) => find(node => node.props['data-choice'] === value)!
  const settle = async () => { for (let pass = 0; pass < 10; pass += 1) await Promise.resolve() }

  beforeEach(() => {
    runtime.hooks = new HookHarness()
    props = { game, oddsInfo, rulesReady: true, rulesMessage: '', onConfirm: vi.fn(async () => result) }
  })

  it('shows every contracted entrance without any control that collapses to chat', () => {
    const tree = render()
    expect(find(node => node.props['aria-label'] === '宾果六合彩网投面板')).toBeDefined()
    for (const label of ['特码A', '特码B', '两面', '头尾数', '正码', '正码1–6', '正码特', '色波', '一肖尾数', '连肖', '连尾', '连码', '其他']) expect(text(tree)).toContain(label)
    expect(elements(tree).some(node => (node.props['aria-label'] ?? '').includes('返回聊天'))).toBe(false)
    expect(elements(tree).some(node => node.props.className === 'detail-panel-collapse')).toBe(false)
  })

  it('submits normalized special-number rows and clears the cart with an in-board receipt', async () => {
    expect(choice('18').props.disabled).toBe(false)
    choice('18').props.onClick!()
    const confirm = find(node => node.props.className === 'full-bet-confirm')!
    expect(text(confirm)).toContain('立即投注')
    confirm.props.onClick!()
    await settle()
    expect(props.onConfirm).toHaveBeenCalledWith([{ play_code: 'marksix_special_a_number', play_name: '特码A', position: 7, selection: '18', amount: 20 }])
    expect(text(render())).toContain('已受理 1 注')
    expect(text(render())).toContain('已选 0 组 · 0 注')
  })

  it('enables 家禽/野兽 shortcuts from the server-confirmed target draw date', () => {
    expect(button('家禽').props.disabled).toBe(false)
    expect(button('野兽').props.disabled).toBe(false)
    button('家禽').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 25 注')
    expect(choice('1').props['aria-pressed']).toBe(true)
    expect(choice('49').props['aria-pressed']).toBe(true)
    expect(choice('2').props['aria-pressed']).toBe(false)
  })

  it('enables reviewed two-sided markets while keeping unpriced markets disabled', () => {
    props.oddsInfo = { ...oddsInfo, items: [...oddsInfo.items, quote('marksix_color_wave')] }
    button('两面').props.onClick!()
    expect(choice('大').props.disabled).toBe(false)
    choice('大').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 1 注')
    button('色波').props.onClick!()
    expect(choice('红波').props.disabled).toBe(true)
    expect(props.onConfirm).not.toHaveBeenCalled()
  })

  it('shows the dynamic special-zodiac market but requires an explicit server price', () => {
    button('头尾数').props.onClick!()
    button('特码生肖').props.onClick!()
    expect(choice('马').props.disabled).toBe(true)
    expect(text(render())).toContain('赔率待配置')
    props.oddsInfo = { ...oddsInfo, items: [...oddsInfo.items, quote('marksix_special_zodiac')] }
    expect(choice('马').props.disabled).toBe(false)
    choice('马').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 1 注')
  })

  it('prices and submits each atomic option independently while zero-price siblings stay disabled', async () => {
    props.oddsInfo = { ...oddsInfo, items: [
      ...oddsInfo.items,
      quote('marksix_color_wave_red', 2.7),
      quote('marksix_color_wave_blue', 0),
      quote('marksix_color_wave_green', 0),
    ] }
    button('色波').props.onClick!()
    expect(choice('红波').props.disabled).toBe(false)
    expect(text(choice('红波'))).toContain('2.70')
    expect(choice('蓝波').props.disabled).toBe(true)
    expect(text(choice('蓝波'))).toContain('待配置')
    expect(choice('绿波').props.disabled).toBe(true)

    choice('红波').props.onClick!()
    const confirm = find(node => node.props.className === 'full-bet-confirm')!
    expect(confirm.props.disabled).toBe(false)
    confirm.props.onClick!()
    await settle()
    expect(props.onConfirm).toHaveBeenCalledWith([{ play_code: 'marksix_color_wave_red', play_name: '红波', position: 7, selection: '红波', amount: 20 }])
  })

  it('clears an unsubmitted cart when the server-confirmed betting issue changes', () => {
    choice('18').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 1 注')
    props.game = { ...game, period: '115049457' }
    render()
    expect(text(render())).toContain('第 115049457 期')
    expect(text(render())).toContain('已选 0 组 · 0 注')
  })
})
