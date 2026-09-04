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
type NodeProps = { children?: ReactNode; className?: string; role?: string; hidden?: boolean; disabled?: boolean; 'aria-label'?: string; 'aria-pressed'?: boolean; 'data-choice'?: string; 'data-market-id'?: string; onClick?: () => void }
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
  const choices = (marketId?: string) => elements(render()).filter(node => node.type === 'button' && node.props['data-choice'] !== undefined && (!marketId || node.props['data-market-id'] === marketId))
  const choice = (value: string, marketId?: string) => choices(marketId).find(node => node.props['data-choice'] === value)!
  const numberBallLabels = (node: ReactNode) => elements(node).filter(item => /\bmark-six-ball\b/.test(item.props.className ?? '')).map(item => text(item))
  const expectNoMarketTabs = () => expect(find(node => /\bmark-six-market-tabs\b/.test(node.props.className ?? ''))).toBeUndefined()
  const editPosition = (position: number) => find(node => node.props['aria-label'] === `编辑正${position}`)!.props.onClick!()
  const expectShortcutSelection = (selected: string | readonly string[] | null) => {
    const activeLabels = typeof selected === 'string' ? [selected] : selected ?? []
    for (const label of ['全选', '红波', '蓝波', '绿波', '大', '小', '单', '双', '合单', '合双', '家禽', '野兽']) {
      const shortcut = button(label)
      expect(shortcut.props['aria-pressed'], label).toBe(activeLabels.includes(label))
      if (activeLabels.includes(label)) expect(shortcut.props.className, label).toMatch(/\bselected\b/)
      else expect(shortcut.props.className ?? '', label).not.toMatch(/\bselected\b/)
    }
  }
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

  it.each([
    ['hong-kong-mark-six', '香港六合彩', 'hk-mark6-v1'],
    ['happy8-mark-six', '快乐8六合彩', 'happy8-mark6-v1'],
    ['new-macau-mark-six', '新澳门六合彩', 'new-macau-mark6-v1'],
    ['old-macau-mark-six', '老澳门六合彩', 'old-macau-mark6-v1'],
  ])('opens the independent %s web board and emits the same normalized ticket contract', async (gameId, title, ruleVersion) => {
    const productGame = { ...game, id: gameId, title, ruleVersion }
    const productOdds = { ...oddsInfo, game_id: gameId, game_name: title, rule_version: ruleVersion }
    props = {
      game: productGame,
      oddsInfo: productOdds,
      rulesReady: true,
      rulesMessage: '',
      onConfirm: vi.fn(async items => ({ ...result, game_id: gameId, game_name: title, content: `网投 ${items.length} 注` })),
    }

    expect(find(node => node.props['aria-label'] === `${title}网投面板`)).toBeDefined()
    choice('18').props.onClick!()
    find(node => node.props.className === 'full-bet-confirm')!.props.onClick!()
    await settle()
    expect(props.onConfirm).toHaveBeenCalledWith([{ play_code: 'marksix_special_a_number', play_name: '特码A', position: 7, selection: '18', amount: 20 }])
  })

  it('submits normalized special-number rows and clears the cart with an in-board receipt', async () => {
    expect(choice('18').props.disabled).toBe(false)
    choice('18').props.onClick!()
    expectShortcutSelection(null)
    const confirm = find(node => node.props.className === 'full-bet-confirm')!
    expect(text(confirm)).toContain('立即投注')
    confirm.props.onClick!()
    await settle()
    expect(props.onConfirm).toHaveBeenCalledWith([{ play_code: 'marksix_special_a_number', play_name: '特码A', position: 7, selection: '18', amount: 20 }])
    expect(text(render())).toContain('已受理 1 注')
    expect(text(render())).toContain('已选 0 组 · 0 注')
    expect(button('红波').props['aria-pressed']).toBe(false)
    expect(button('全选').props['aria-pressed']).toBe(false)
  })

  it('unions overlapping shortcuts without duplicate tickets and preserves the remaining active group', () => {
    expectShortcutSelection(null)
    button('红波').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 17 注')
    expectShortcutSelection('红波')

    button('大').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 34 注')
    expectShortcutSelection(['红波', '大'])
    expect(choice('29').props['aria-pressed']).toBe(true)
    expect(choice('25').props['aria-pressed']).toBe(true)

    button('大').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 17 注')
    expectShortcutSelection('红波')
    expect(choice('29').props['aria-pressed']).toBe(true)
    expect(choice('25').props['aria-pressed']).toBe(false)

    button('红波').props.onClick!()
    expect(text(render())).toContain('已选 0 组 · 0 注')
    expectShortcutSelection(null)
    expect(props.onConfirm).not.toHaveBeenCalled()
  })

  it('records a contained shortcut as active even when all its numbers were already selected', () => {
    button('全选').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 49 注')
    expectShortcutSelection('全选')
    button('红波').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 49 注')
    expectShortcutSelection(['全选', '红波'])

    button('全选').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 17 注')
    expectShortcutSelection('红波')
    expect(choice('1').props['aria-pressed']).toBe(true)
    expect(choice('3').props['aria-pressed']).toBe(false)
    expect(choice('49').props['aria-pressed']).toBe(false)

    button('红波').props.onClick!()
    expect(text(render())).toContain('已选 0 组 · 0 注')
    expectShortcutSelection(null)
    expect(props.onConfirm).not.toHaveBeenCalled()
  })

  it('keeps independent manual additions when their overlapping shortcut is removed', () => {
    choice('1').props.onClick!()
    choice('3').props.onClick!()
    expectShortcutSelection(null)
    button('红波').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 18 注')
    expectShortcutSelection('红波')

    button('红波').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 2 注')
    expect(choice('1').props['aria-pressed']).toBe(true)
    expect(choice('3').props['aria-pressed']).toBe(true)
    expect(choice('2').props['aria-pressed']).toBe(false)
    expectShortcutSelection(null)
    expect(props.onConfirm).not.toHaveBeenCalled()
  })

  it('retains manual exclusions across shortcut changes without changing the active button set', () => {
    button('红波').props.onClick!()
    choice('29').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 16 注')
    expectShortcutSelection('红波')

    button('大').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 33 注')
    expect(choice('29').props['aria-pressed']).toBe(false)
    expectShortcutSelection(['红波', '大'])

    button('大').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 16 注')
    expect(choice('29').props['aria-pressed']).toBe(false)
    expectShortcutSelection('红波')

    button('红波').props.onClick!()
    expect(text(render())).toContain('已选 0 组 · 0 注')
    expectShortcutSelection(null)
    button('红波').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 16 注')
    expect(choice('29').props['aria-pressed']).toBe(false)
    expectShortcutSelection('红波')

    choice('29').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 17 注')
    expect(choice('29').props['aria-pressed']).toBe(true)
    expectShortcutSelection('红波')
    expect(props.onConfirm).not.toHaveBeenCalled()
  })

  it('stores independent shortcut unions and charges the same number separately for special A and B', () => {
    button('红波').props.onClick!()
    button('大').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 34 注')
    button('特码B').props.onClick!()
    expectShortcutSelection(null)
    expect(choice('1').props['aria-pressed']).toBe(false)

    button('红波').props.onClick!()
    button('大').props.onClick!()
    expectShortcutSelection(['红波', '大'])
    expect(text(render())).toContain('已选 2 组 · 68 注')
    expect(choice('29').props['aria-pressed']).toBe(true)
    button('大').props.onClick!()
    expectShortcutSelection('红波')
    expect(text(render())).toContain('已选 2 组 · 51 注')

    button('特码A').props.onClick!()
    expectShortcutSelection(['红波', '大'])
    expect(choice('25').props['aria-pressed']).toBe(true)
    expect(choice('29').props['aria-pressed']).toBe(true)

    button('特码B').props.onClick!()
    expectShortcutSelection('红波')
    expect(choice('25').props['aria-pressed']).toBe(false)
    expect(choice('29').props['aria-pressed']).toBe(true)
    expect(props.onConfirm).not.toHaveBeenCalled()
  })

  it('removes a list item as a manual exclusion in its own market without affecting a matching number elsewhere', () => {
    button('红波').props.onClick!()
    button('特码B').props.onClick!()
    button('红波').props.onClick!()
    expect(text(render())).toContain('已选 2 组 · 34 注')

    find(node => node.props.className === 'full-bet-selection-toggle')!.props.onClick!()
    find(node => node.props['aria-label'] === '移除特码A号码29')!.props.onClick!()
    expect(text(render())).toContain('已选 2 组 · 33 注')
    expectShortcutSelection('红波')
    expect(choice('29').props['aria-pressed']).toBe(true)

    button('特码A').props.onClick!()
    expectShortcutSelection('红波')
    expect(choice('29').props['aria-pressed']).toBe(false)
    button('大').props.onClick!()
    expectShortcutSelection(['红波', '大'])
    expect(choice('29').props['aria-pressed']).toBe(false)
    expect(props.onConfirm).not.toHaveBeenCalled()
  })

  it('clears all shortcuts, manual additions and exclusions for every market with the cart', () => {
    button('红波').props.onClick!()
    choice('29').props.onClick!()
    choice('3').props.onClick!()
    button('特码B').props.onClick!()
    button('蓝波').props.onClick!()
    choice('25').props.onClick!()
    choice('1').props.onClick!()
    expectShortcutSelection('蓝波')

    button('清空选择').props.onClick!()
    expect(text(render())).toContain('已选 0 组 · 0 注')
    expectShortcutSelection(null)
    button('特码A').props.onClick!()
    expectShortcutSelection(null)
    button('红波').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 17 注')
    expect(choice('29').props['aria-pressed']).toBe(true)
    expect(choice('3').props['aria-pressed']).toBe(false)
    button('红波').props.onClick!()
    expect(text(render())).toContain('已选 0 组 · 0 注')
    button('特码B').props.onClick!()
    button('蓝波').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 16 注')
    expect(choice('25').props['aria-pressed']).toBe(true)
    expect(choice('1').props['aria-pressed']).toBe(false)
    button('蓝波').props.onClick!()
    expect(text(render())).toContain('已选 0 组 · 0 注')
    expect(props.onConfirm).not.toHaveBeenCalled()
  })

  it('allows clearing persisted manual exclusions even when no tickets remain', () => {
    button('红波').props.onClick!()
    choice('29').props.onClick!()
    button('红波').props.onClick!()
    expect(text(render())).toContain('已选 0 组 · 0 注')
    expectShortcutSelection(null)
    expect(button('清空选择').props.disabled).toBe(false)

    button('清空选择').props.onClick!()
    button('红波').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 17 注')
    expect(choice('29').props['aria-pressed']).toBe(true)
    expect(props.onConfirm).not.toHaveBeenCalled()
  })

  it('clears all shortcut and manual state for every market after a successful submission', async () => {
    props.onConfirm = vi.fn(async () => ({ ...result, bet_count: 33, total: 660, balance: 340 }))
    button('红波').props.onClick!()
    choice('29').props.onClick!()
    choice('3').props.onClick!()
    button('特码B').props.onClick!()
    button('蓝波').props.onClick!()
    choice('25').props.onClick!()
    choice('1').props.onClick!()
    expectShortcutSelection('蓝波')

    find(node => node.props.className === 'full-bet-confirm')!.props.onClick!()
    await settle()
    expect(props.onConfirm).toHaveBeenCalledTimes(1)
    expect(vi.mocked(props.onConfirm).mock.calls[0][0]).toHaveLength(33)
    expect(text(render())).toContain('已受理 33 注')
    expect(text(render())).toContain('已选 0 组 · 0 注')
    expectShortcutSelection(null)
    button('特码A').props.onClick!()
    expectShortcutSelection(null)
    button('红波').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 17 注')
    expect(choice('29').props['aria-pressed']).toBe(true)
    expect(choice('3').props['aria-pressed']).toBe(false)
    button('红波').props.onClick!()
    expect(text(render())).toContain('已选 0 组 · 0 注')
    button('特码B').props.onClick!()
    button('蓝波').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 16 注')
    expect(choice('25').props['aria-pressed']).toBe(true)
    expect(choice('1').props['aria-pressed']).toBe(false)
    button('蓝波').props.onClick!()
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

  it('prices and highlights each 特肖 independently despite grouped number previews', async () => {
    button('其他').props.onClick!()
    expect(button('特肖').props['aria-pressed']).toBe(true)
    expect(choice('马', 'special_zodiac').props.disabled).toBe(true)
    expect(numberBallLabels(choice('马', 'special_zodiac'))).toEqual(['01', '13', '25', '37', '49'])
    expect(text(render())).toContain('赔率待配置')
    props.oddsInfo = { ...oddsInfo, items: [...oddsInfo.items, quote('marksix_special_zodiac_horse', 11.8)] }
    expect(choice('马', 'special_zodiac').props.disabled).toBe(false)
    expect(choice('羊', 'special_zodiac').props.disabled).toBe(true)
    choice('马', 'special_zodiac').props.onClick!()
    expect(choice('马', 'special_zodiac').props['aria-pressed']).toBe(true)
    expect(choice('羊', 'special_zodiac').props['aria-pressed']).toBe(false)
    expect(text(choice('马', 'special_zodiac'))).toContain('11.80')
    expect(text(render())).toContain('已选 1 组 · 1 注')
    find(node => node.props.className === 'full-bet-confirm')!.props.onClick!()
    await settle()
    expect(props.onConfirm).toHaveBeenCalledWith([{ play_code: 'marksix_special_zodiac_horse', play_name: '特肖马', position: 7, selection: '马', amount: 20 }])
  })

  it('prices and submits each atomic option independently while zero-price siblings stay disabled', async () => {
    props.oddsInfo = { ...oddsInfo, items: [
      ...oddsInfo.items,
      quote('marksix_color_wave_red', 2.7),
      quote('marksix_color_wave_blue', 0),
      quote('marksix_color_wave_green', 0),
    ] }
    button('色波').props.onClick!()
    expect(choices('color_wave')).toHaveLength(3)
    expect(choice('红波').props.disabled).toBe(false)
    expect(numberBallLabels(choice('红波'))).toEqual(['01', '02', '07', '08', '12', '13', '18', '19', '23', '24', '29', '30', '34', '35', '40', '45', '46'])
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

  it('shows all 24 two-sided choices together without submarket tabs and prices each market independently', async () => {
    props.oddsInfo = { ...oddsInfo, items: [quote('marksix_special_odd_even', 1.97)] }
    button('两面').props.onClick!()
    expectNoMarketTabs()
    expect(choices()).toHaveLength(24)
    expect(new Set(choices().map(node => node.props['data-market-id'])).size).toBe(11)
    expect(choice('大', 'special_big_small').props.disabled).toBe(true)
    expect(choice('小', 'special_big_small').props.disabled).toBe(true)
    expect(choice('单', 'special_odd_even').props.disabled).toBe(false)
    expect(choice('双', 'special_odd_even').props.disabled).toBe(false)
    expect(choice('合大', 'special_sum_big_small').props.disabled).toBe(true)
    expect(choice('总和大', 'total_big_small').props.disabled).toBe(true)
    expect(choice('总和双', 'total_odd_even').props.disabled).toBe(true)
    expect(text(choice('单', 'special_odd_even'))).toContain('1.97')
    expect(text(choice('大', 'special_big_small'))).toContain('待配置')

    choice('单', 'special_odd_even').props.onClick!()
    find(node => node.props.className === 'full-bet-confirm')!.props.onClick!()
    await settle()
    expect(props.onConfirm).toHaveBeenCalledWith([{ play_code: 'marksix_special_odd_even', play_name: '特码单双', position: 7, selection: '单', amount: 20 }])
  })

  it.each(['大单', '小单', '大双', '小双'])('uses the configured half-special price for %s without sending client odds or rule authority', async selection => {
    props.oddsInfo = { ...oddsInfo, rule_version: 'mark6-v2', items: [quote('marksix_special_half', 3.72)] }
    props.onConfirm = vi.fn(async () => ({ ...result, rule_version: 'mark6-v2' }))
    button('两面').props.onClick!()
    expect(props.game.ruleVersion).toBe('mark6-v2')
    expect(choices('special_combo')).toHaveLength(4)
    for (const value of ['大单', '小单', '大双', '小双']) {
      const option = choice(value, 'special_combo')
      expect(option.props.disabled).toBe(false)
      expect(text(option)).toContain(`特${value}`)
      expect(text(option)).toContain('3.72')
      expect(option.props['aria-pressed']).toBe(false)
    }
    const unpriced = choices().filter(node => node.props['data-market-id'] !== 'special_combo')
    expect(unpriced).toHaveLength(20)
    expect(unpriced.every(node => node.props.disabled && text(node).includes('待配置'))).toBe(true)

    choice(selection, 'special_combo').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 1 注')
    for (const value of ['大单', '小单', '大双', '小双']) {
      expect(choice(value, 'special_combo').props['aria-pressed']).toBe(value === selection)
    }
    const confirm = find(node => node.props.className === 'full-bet-confirm')!
    expect(confirm.props.disabled).toBe(false)
    confirm.props.onClick!()
    await settle()
    expect(props.onConfirm).toHaveBeenCalledExactlyOnceWith([
      { play_code: 'marksix_special_half', play_name: '特码半特', position: 7, selection, amount: 20 },
    ])
    const submitted = vi.mocked(props.onConfirm).mock.calls[0][0][0]
    expect(Object.keys(submitted).sort()).toEqual(['amount', 'play_code', 'play_name', 'position', 'selection'])
    expect(submitted).not.toHaveProperty('odds')
    expect(submitted).not.toHaveProperty('rule_version')
  })

  it('uses ordinary special size and parity prices without enabling unquoted half-special or other markets', async () => {
    props.oddsInfo = { ...oddsInfo, rule_version: 'mark6-v2', items: [
      quote('marksix_special_big_small', 1.98),
      quote('marksix_special_odd_even', 1.98),
    ] }
    props.onConfirm = vi.fn(async () => ({ ...result, rule_version: 'mark6-v2', bet_count: 4, total: 80, balance: 920 }))
    const ordinaryOptions = [
      { marketId: 'special_big_small', play_code: 'marksix_special_big_small', play_name: '特码大小', selection: '大' },
      { marketId: 'special_big_small', play_code: 'marksix_special_big_small', play_name: '特码大小', selection: '小' },
      { marketId: 'special_odd_even', play_code: 'marksix_special_odd_even', play_name: '特码单双', selection: '单' },
      { marketId: 'special_odd_even', play_code: 'marksix_special_odd_even', play_name: '特码单双', selection: '双' },
    ]
    button('两面').props.onClick!()
    for (const item of ordinaryOptions) {
      const option = choice(item.selection, item.marketId)
      expect(option.props.disabled).toBe(false)
      expect(text(option)).toContain(`特${item.selection}`)
      expect(text(option)).toContain('1.98')
      option.props.onClick!()
    }
    expect(choices().filter(node => node.props.disabled)).toHaveLength(20)
    for (const option of choices('special_combo')) {
      expect(option.props.disabled).toBe(true)
      expect(text(option)).toContain('待配置')
    }
    expect(text(render())).toContain('已选 2 组 · 4 注')
    find(node => node.props.className === 'full-bet-confirm')!.props.onClick!()
    await settle()
    expect(props.onConfirm).toHaveBeenCalledExactlyOnceWith(ordinaryOptions.map(item => ({
      play_code: item.play_code, play_name: item.play_name, position: 7, selection: item.selection, amount: 20,
    })))
  })

  it('shows exactly 15 head and tail choices together without zodiac or submarket tabs', async () => {
    props.oddsInfo = { ...oddsInfo, items: [quote('marksix_special_head_0', 4.5), quote('marksix_special_tail_0', 9.8)] }
    button('头尾数').props.onClick!()
    expectNoMarketTabs()
    expect(choices()).toHaveLength(15)
    expect(choices('special_head')).toHaveLength(5)
    expect(choices('special_tail')).toHaveLength(10)
    expect(choices('special_zodiac')).toHaveLength(0)
    expect(choice('0头', 'special_head').props.disabled).toBe(false)
    expect(choice('0尾', 'special_tail').props.disabled).toBe(false)
    expect(choice('1头', 'special_head').props.disabled).toBe(true)
    expect(choice('1尾', 'special_tail').props.disabled).toBe(true)

    choice('0头', 'special_head').props.onClick!()
    choice('0尾', 'special_tail').props.onClick!()
    expect(text(render())).toContain('已选 2 组 · 2 注')
    find(node => node.props.className === 'full-bet-confirm')!.props.onClick!()
    await settle()
    expect(props.onConfirm).toHaveBeenCalledWith([
      { play_code: 'marksix_special_head_0', play_name: '特码0头', position: 7, selection: '0头', amount: 20 },
      { play_code: 'marksix_special_tail_0', play_name: '特码0尾', position: 7, selection: '0尾', amount: 20 },
    ])
  })

  it('shows 49 regular numbers plus four shared total choices and keeps repeated entrances as one ticket', async () => {
    button('正码').props.onClick!()
    expectNoMarketTabs()
    expect(choices('regular_number')).toHaveLength(49)
    expect(choices('total_big_small')).toHaveLength(2)
    expect(choices('total_odd_even')).toHaveLength(2)
    expect(choices()).toHaveLength(53)
    choice('总和大', 'total_big_small').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 1 注')

    button('两面').props.onClick!()
    expect(choice('总和大', 'total_big_small').props['aria-pressed']).toBe(true)
    expect(text(render())).toContain('已选 1 组 · 1 注')
    choice('总和大', 'total_big_small').props.onClick!()
    expect(text(render())).toContain('已选 0 组 · 0 注')
    choice('总和大', 'total_big_small').props.onClick!()

    button('正码').props.onClick!()
    expect(choice('总和大', 'total_big_small').props['aria-pressed']).toBe(true)
    choice('1', 'regular_number').props.onClick!()
    expect(text(render())).toContain('已选 2 组 · 2 注')
    find(node => node.props.className === 'full-bet-confirm')!.props.onClick!()
    await settle()
    const submitted = vi.mocked(props.onConfirm).mock.calls[0][0]
    expect(submitted).toHaveLength(2)
    expect(submitted.filter(item => item.play_code === 'marksix_total_big_small')).toEqual([
      { play_code: 'marksix_total_big_small', play_name: '总和大小', position: 0, selection: '总和大', amount: 20 },
    ])
    expect(submitted).toContainEqual({ play_code: 'marksix_regular_number', play_name: '正码', position: 0, selection: '1', amount: 20 })
  })

  it('keeps regular-number union shortcuts separate from shared total wagers', () => {
    button('正码').props.onClick!()
    choice('总和双', 'total_odd_even').props.onClick!()
    button('红波').props.onClick!()
    button('大').props.onClick!()
    expectShortcutSelection(['红波', '大'])
    expect(text(render())).toContain('已选 2 组 · 35 注')
    expect(choice('总和双', 'total_odd_even').props['aria-pressed']).toBe(true)

    button('大').props.onClick!()
    expect(text(render())).toContain('已选 2 组 · 18 注')
    expect(choice('29', 'regular_number').props['aria-pressed']).toBe(true)
    expect(choice('25', 'regular_number').props['aria-pressed']).toBe(false)
    button('两面').props.onClick!()
    expect(choice('总和双', 'total_odd_even').props['aria-pressed']).toBe(true)
    button('正码').props.onClick!()
    expectShortcutSelection('红波')
    expect(props.onConfirm).not.toHaveBeenCalled()
  })

  it('shows 13 regular-position attributes together with six independent positions and no number market', () => {
    props.oddsInfo = { ...oddsInfo, items: [quote('marksix_regular_position_odd_even', 1.96), quote('marksix_regular_color_red', 2.7)] }
    button('正码1–6').props.onClick!()
    expectNoMarketTabs()
    expect(choices()).toHaveLength(13)
    expect(choices('regular_position_number')).toHaveLength(0)
    expect(choices('regular_position_wave')).toHaveLength(3)
    for (let position = 1; position <= 6; position += 1) expect(find(node => node.props['aria-label'] === `编辑正${position}`)).toBeDefined()
    expect(choice('大', 'regular_position_big_small').props.disabled).toBe(true)
    expect(choice('单', 'regular_position_odd_even').props.disabled).toBe(false)
    expect(choice('红波', 'regular_position_wave').props.disabled).toBe(false)
    expect(choice('蓝波', 'regular_position_wave').props.disabled).toBe(true)
    expect(choice('绿波', 'regular_position_wave').props.disabled).toBe(true)

    choice('单', 'regular_position_odd_even').props.onClick!()
    editPosition(2)
    expect(choice('单', 'regular_position_odd_even').props['aria-pressed']).toBe(false)
    choice('单', 'regular_position_odd_even').props.onClick!()
    expect(text(render())).toContain('已选 2 组 · 2 注')
    editPosition(1)
    expect(choice('单', 'regular_position_odd_even').props['aria-pressed']).toBe(true)
    expect(props.onConfirm).not.toHaveBeenCalled()
  })

  it('keeps regular-special numbers, two-sided and wave boards separate while shortcuts stay independent by position', async () => {
    props.onConfirm = vi.fn(async () => ({ ...result, bet_count: 51, total: 1020, balance: 0 }))
    button('正码特').props.onClick!()
    expect(find(node => /\bmark-six-market-tabs\b/.test(node.props.className ?? ''))).toBeDefined()
    expect(choices()).toHaveLength(49)
    expect(choices('regular_special_number')).toHaveLength(49)
    expect(choices('regular_special_sides')).toHaveLength(0)
    expect(choices('regular_special_wave')).toHaveLength(0)
    const content = find(node => node.props.className === 'full-bet-content')!
    expect(text(content)).toContain('正码特两面')
    expect(text(content)).toContain('正码特色波')
    for (let position = 1; position <= 6; position += 1) expect(find(node => node.props['aria-label'] === `编辑正${position}`)).toBeDefined()

    button('红波').props.onClick!()
    button('大').props.onClick!()
    expectShortcutSelection(['红波', '大'])
    expect(text(render())).toContain('已选 1 组 · 34 注')
    editPosition(2)
    expectShortcutSelection(null)
    expect(choice('29', 'regular_special_number').props['aria-pressed']).toBe(false)
    button('红波').props.onClick!()
    button('大').props.onClick!()
    expect(text(render())).toContain('已选 2 组 · 68 注')
    button('大').props.onClick!()
    expectShortcutSelection('红波')
    expect(text(render())).toContain('已选 2 组 · 51 注')
    expect(choice('29', 'regular_special_number').props['aria-pressed']).toBe(true)
    expect(choice('25', 'regular_special_number').props['aria-pressed']).toBe(false)

    editPosition(1)
    expectShortcutSelection(['红波', '大'])
    expect(choice('25', 'regular_special_number').props['aria-pressed']).toBe(true)
    find(node => node.props.className === 'full-bet-confirm')!.props.onClick!()
    await settle()
    const submitted = vi.mocked(props.onConfirm).mock.calls[0][0]
    expect(submitted).toHaveLength(51)
    expect(submitted.every(item => item.play_code === 'marksix_regular_special_number')).toBe(true)
    expect(submitted.filter(item => item.position === 1)).toHaveLength(34)
    expect(submitted.filter(item => item.position === 2)).toHaveLength(17)
    expect(submitted.filter(item => item.selection === '29')).toHaveLength(2)
    expect(new Set(submitted.map(item => `${item.play_code}:${item.position}:${item.selection}`)).size).toBe(51)
  })

  it('enables each quoted 一肖 and 一尾 atom independently', async () => {
    button('一肖尾数').props.onClick!()
    expect(numberBallLabels(choice('马', 'one_zodiac'))).toEqual(['01', '13', '25', '37', '49'])
    expect(choice('马', 'one_zodiac').props.disabled).toBe(true)
    expect(choices('one_zodiac').every(node => node.props.disabled)).toBe(true)
    props.oddsInfo = { ...oddsInfo, items: [...oddsInfo.items, quote('marksix_one_zodiac_horse', 2.15)] }
    expect(choice('马', 'one_zodiac').props.disabled).toBe(false)
    expect(choice('羊', 'one_zodiac').props.disabled).toBe(true)
    choice('马', 'one_zodiac').props.onClick!()
    expect(choice('马', 'one_zodiac').props['aria-pressed']).toBe(true)
    expect(choice('羊', 'one_zodiac').props['aria-pressed']).toBe(false)
    find(node => node.props.className === 'full-bet-confirm')!.props.onClick!()
    await settle()
    expect(props.onConfirm).toHaveBeenCalledWith([{ play_code: 'marksix_one_zodiac_horse', play_name: '一肖马', position: 0, selection: '马', amount: 20 }])
    props.oddsInfo = { ...oddsInfo, items: [...oddsInfo.items, quote('marksix_one_tail_0', 2.09)] }
    find(node => node.type === 'button' && node.props['aria-label'] === '尾数')!.props.onClick!()
    expect(numberBallLabels(choice('0尾', 'one_tail'))).toEqual(['10', '20', '30', '40'])
    expect(numberBallLabels(choice('2尾', 'one_tail'))).toEqual(['02', '12', '22', '32', '42'])
    expect(choice('0尾', 'one_tail').props.disabled).toBe(false)
    expect(choice('1尾', 'one_tail').props.disabled).toBe(true)
    choice('0尾', 'one_tail').props.onClick!()
    expect(choice('0尾', 'one_tail').props['aria-pressed']).toBe(true)
    expect(props.onConfirm).toHaveBeenCalledTimes(1)
  })

  it('prices 总肖 and 七色波 options independently and submits only selected atoms', async () => {
    props.oddsInfo = { ...oddsInfo, items: [
      quote('marksix_total_zodiac_5', 4.6),
      quote('marksix_seven_color_red', 2.8),
      quote('marksix_seven_color_draw', 9.5),
    ] }
    button('其他').props.onClick!()
    button('总肖').props.onClick!()
    expect(choice('5肖', 'total_zodiac').props.disabled).toBe(false)
    expect(choice('6肖', 'total_zodiac').props.disabled).toBe(true)
    choice('5肖', 'total_zodiac').props.onClick!()
    expect(choice('5肖', 'total_zodiac').props['aria-pressed']).toBe(true)
    button('七色波').props.onClick!()
    expect(choice('红波', 'seven_color_wave').props.disabled).toBe(false)
    expect(choice('蓝波', 'seven_color_wave').props.disabled).toBe(true)
    expect(choice('和局', 'seven_color_wave').props.disabled).toBe(false)
    choice('红波', 'seven_color_wave').props.onClick!()
    choice('和局', 'seven_color_wave').props.onClick!()
    expect(choice('红波', 'seven_color_wave').props['aria-pressed']).toBe(true)
    expect(choice('和局', 'seven_color_wave').props['aria-pressed']).toBe(true)
    expect(choice('蓝波', 'seven_color_wave').props['aria-pressed']).toBe(false)
    find(node => node.props.className === 'full-bet-confirm')!.props.onClick!()
    await settle()
    expect(props.onConfirm).toHaveBeenCalledWith([
      { play_code: 'marksix_total_zodiac_5', play_name: '总肖5肖', position: 0, selection: '5肖', amount: 20 },
      { play_code: 'marksix_seven_color_red', play_name: '七色波红波', position: 0, selection: '红波', amount: 20 },
      { play_code: 'marksix_seven_color_draw', play_name: '七色波和局', position: 0, selection: '和局', amount: 20 },
    ])
  })

  it('opens 2–11合肖 and 5–11自选不中 through count variants', async () => {
    props.oddsInfo = { ...oddsInfo, items: [
      ...oddsInfo.items,
      quote('marksix_combined_zodiac_2', 5.76),
      quote('marksix_not_in_6', 2.5),
    ] }
    button('其他').props.onClick!()
    button('合肖').props.onClick!()
    expect(choices('combined_zodiac_2')).toHaveLength(12)
    choice('鼠', 'combined_zodiac_2').props.onClick!()
    choice('马', 'combined_zodiac_2').props.onClick!()
    find(node => node.type === 'button' && text(node).startsWith('加入') && text(node).includes('清单'))!.props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 1 注')

    button('自选不中').props.onClick!()
    expect(choices('not_in_5')).toHaveLength(49)
    for (const value of ['1', '2', '3', '4', '5']) choice(value, 'not_in_5').props.onClick!()
    expect(choice('6', 'not_in_5').props.disabled).toBe(true)
    find(node => node.type === 'button' && text(node).startsWith('加入') && text(node).includes('清单'))!.props.onClick!()
    button('6不中').props.onClick!()
    for (const value of ['1', '2', '3', '4', '5', '6']) choice(value, 'not_in_6').props.onClick!()
    find(node => node.type === 'button' && text(node).startsWith('加入') && text(node).includes('清单'))!.props.onClick!()
    expect(text(render())).toContain('已选 3 组 · 3 注')
    find(node => node.props.className === 'full-bet-confirm')!.props.onClick!()
    await settle()
    expect(props.onConfirm).toHaveBeenCalledWith([
      { play_code: 'marksix_combined_zodiac_2', play_name: '2合肖', position: 7, selection: '鼠,马', amount: 20 },
      { play_code: 'marksix_not_in', play_name: '五不中', position: 0, selection: '1,2,3,4,5', amount: 20 },
      { play_code: 'marksix_not_in_6', play_name: '6不中', position: 0, selection: '1,2,3,4,5,6', amount: 20 },
    ])
  })

  it('submits one linked-zodiac ticket at the lowest selected configured price', async () => {
    props.oddsInfo = { ...oddsInfo, items: [
      ...oddsInfo.items,
      quote('marksix_link_zodiac_2_rat', 4.2),
      quote('marksix_link_zodiac_2_horse', 3.55),
    ] }
    button('连肖').props.onClick!()
    expect(choice('鼠', 'link_zodiac_2').props.disabled).toBe(false)
    expect(choice('马', 'link_zodiac_2').props.disabled).toBe(false)
    expect(choice('牛', 'link_zodiac_2').props.disabled).toBe(true)
    choice('鼠', 'link_zodiac_2').props.onClick!()
    choice('马', 'link_zodiac_2').props.onClick!()
    expect(text(render())).toContain('组合赔率：3.55')
    find(node => node.type === 'button' && text(node) === '加入2连肖清单')!.props.onClick!()
    find(node => node.props.className === 'full-bet-confirm')!.props.onClick!()
    await settle()
    expect(props.onConfirm).toHaveBeenCalledWith([
      { play_code: 'marksix_link_zodiac_2', play_name: '2连肖', position: 0, selection: '鼠,马', amount: 20 },
    ])
  })

  it('shows both tier prices but submits 三中二 as one ticket and one stake', async () => {
    props.oddsInfo = { ...oddsInfo, items: [
      ...oddsInfo.items,
      quote('marksix_combo_3_2_exact2', 20.1),
      quote('marksix_combo_3_2_exact3', 125),
    ] }
    button('连码').props.onClick!()
    expect(text(render())).toContain('中二 20.10 / 中三 125.00')
    for (const value of ['1', '2', '3']) choice(value, 'combo_3_2').props.onClick!()
    find(node => node.type === 'button' && text(node) === '加入三中二清单')!.props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 1 注')
    find(node => node.props.className === 'full-bet-confirm')!.props.onClick!()
    await settle()
    expect(props.onConfirm).toHaveBeenCalledWith([
      { play_code: 'marksix_combo_3_2', play_name: '三中二', position: 0, selection: '1,2,3', amount: 20 },
    ])
  })

  it('clears an unsubmitted cart when the server-confirmed betting issue changes', () => {
    button('红波').props.onClick!()
    choice('29').props.onClick!()
    choice('3').props.onClick!()
    button('特码B').props.onClick!()
    button('蓝波').props.onClick!()
    choice('25').props.onClick!()
    choice('1').props.onClick!()
    expect(text(render())).toContain('已选 2 组 · 33 注')
    expectShortcutSelection('蓝波')
    props.game = { ...game, period: '115049457' }
    render()
    expect(text(render())).toContain('第 115049457 期')
    expect(text(render())).toContain('已选 0 组 · 0 注')
    expectShortcutSelection(null)
    button('红波').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 17 注')
    expect(choice('29').props['aria-pressed']).toBe(true)
    expect(choice('3').props['aria-pressed']).toBe(false)
    button('红波').props.onClick!()
    expect(text(render())).toContain('已选 0 组 · 0 注')
    button('特码B').props.onClick!()
    expectShortcutSelection(null)
    button('蓝波').props.onClick!()
    expect(text(render())).toContain('已选 1 组 · 16 注')
    expect(choice('25').props['aria-pressed']).toBe(true)
    expect(choice('1').props['aria-pressed']).toBe(false)
    button('蓝波').props.onClick!()
    expect(text(render())).toContain('已选 0 组 · 0 注')
    expect(props.onConfirm).not.toHaveBeenCalled()
  })
})
