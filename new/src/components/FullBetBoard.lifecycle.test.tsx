import { isValidElement, type ComponentProps, type ReactElement, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Game } from '../types'
import { HookHarness } from '../test/hookHarness'
import { resolveLotteryTiming } from '../utils/lotteryTiming'
import { parseBetInput } from '../utils/betParser'
import { FullBetBoard } from './FullBetBoard'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null }))
vi.mock('react', async importOriginal => ({ ...await importOriginal<typeof import('react')>(), useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial) }))
type Props = ComponentProps<typeof FullBetBoard>
type NodeProps = { children?: ReactNode; className?: string; 'aria-label'?: string; 'aria-pressed'?: boolean | 'mixed'; 'data-choice'?: string; disabled?: boolean; onClick?: () => void; onChange?: (event: { target: { value: string } }) => void }
const timing = resolveLotteryTiming({ issue_status: 'accepting', source_healthy: true, next_draw_at: '2026-08-30T06:46:00Z', seal_seconds: 30 }, Date.parse('2026-08-30T06:45:00Z'))
const game: Game = { id: 'speed-racing', title: '极速赛车', tag: '', category: 'racing', lobbyCategory: 'lottery', online: '', period: '34137265', latestIssue: '34137264', due: timing.due, timing, balls: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10], color: '', issueStatus: 'accepting', sourceKind: '', sourceName: '', sourceHealthy: true, syncStatus: '', sourceError: '' }
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

describe('bet board click feedback and generated commands', () => {
  let props: Props
  const render = () => runtime.hooks!.render(() => FullBetBoard(props))
  const label = (value: string) => find(render(), node => node.props['aria-label'] === value)!
  const choice = (value: string) => find(render(), node => node.props['data-choice'] === value)!
  const confirm = () => find(render(), node => node.props.className === 'full-bet-confirm')!
  const clickChoice = (value: string) => { expect(choice(value).props.disabled).toBe(false); choice(value).props.onClick!() }
  const tab = (value: string) => find(render(), node => node.type === 'button' && text(node) === value)!.props.onClick!()

  beforeEach(() => {
    runtime.hooks = new HookHarness()
    props = { game, mode: 'quick', odds: { ball_1_5: 9.9, two_sided: 1.993, dragon_tiger: 1.993, sum: 1.993 }, oddsHidden: false, oddsResponseReady: true, onModeChange: value => { props.mode = value }, onConfirm: vi.fn(), onClose: vi.fn() }
  })

  it('highlights selected numbers, supports different ranks, and submits exactly those choices', () => {
    props.mode = 'numbers'
    clickChoice('5')
    expect(choice('5').props['aria-pressed']).toBe(true)
    expect(text(render())).toContain('已选 1 组 · 1 注')
    label('编辑亚军').props.onClick!()
    expect(choice('5').props['aria-pressed']).toBe(false)
    clickChoice('10')
    expect(text(confirm())).toBe('立即投注 ¥ 40')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenCalledWith('1/5/20#2/0/20')
    expect(text(label('已选投注'))).toContain('冠军 5亚军 10')
  })

  it('matches screenshot mixed rank selections and has all ten quick numbers', () => {
    for (let n = 1; n <= 10; n++) expect(choice(String(n))).toBeDefined()
    label('编辑亚军').props.onClick!()
    for (const value of ['大', '小', '单', '双', '1', '2']) clickChoice(value)
    expect(text(render())).toContain('已选 2 组 · 12 注')
    expect(text(confirm())).toBe('立即投注 ¥ 240')
    confirm().props.onClick!()
    const command = vi.mocked(props.onConfirm).mock.calls[0][0]
    expect(parseBetInput(command).payloads).toHaveLength(12)
    expect(parseBetInput(command).total).toBe(240)
    tab('车号 1–10')
    expect(choice('1').props['aria-pressed']).toBe(true)
    tab('两面盘')
    expect(choice('大').props['aria-pressed']).toBe(true)
    clickChoice('大')
    expect(text(render())).toContain('已选 2 组 · 11 注')
    label('编辑亚军').props.onClick!()
    expect(choice('大').props['aria-pressed']).toBe(true)
  })

  it('keeps partial choices visually neutral and adds only the missing rank', () => {
    clickChoice('2')
    label('编辑亚军').props.onClick!()
    expect(choice('2').props['aria-pressed']).toBe('mixed')
    expect(choice('2').props.className).toBe('board-choice')
    clickChoice('2')
    expect(choice('2').props['aria-pressed']).toBe(true)
    expect(choice('2').props.className).toBe('board-choice selected')
    expect(text(render())).toContain('已选 2 组 · 2 注')
    expect(text(confirm())).toBe('立即投注 ¥ 40')
  })

  it('leaves rank-three numbers in the cart without ghost marks when rank four is selected', () => {
    props.mode = 'numbers'
    label('编辑第三名').props.onClick!()
    clickChoice('2')
    clickChoice('3')
    label('编辑第四名').props.onClick!()
    for (const number of ['2', '3']) expect(choice(number).props.className).toBe('board-choice')
    expect(text(render())).toContain('已选 1 组 · 2 注')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenLastCalledWith('3/23/20')

    // Editing only rank four does not inherit or delete rank three's choices.
    for (const number of ['2', '3']) {
      expect(choice(number).props['aria-pressed']).toBe(false)
      expect(choice(number).props.className).toBe('board-choice')
    }
    clickChoice('5')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenLastCalledWith('3/23/20#4/5/20')
    label('编辑第三名').props.onClick!()
    for (const number of ['2', '3']) expect(choice(number).props.className).toBe('board-choice selected')
    expect(choice('5').props.className).toBe('board-choice')
  })

  it.each(['numbers', 'dual'] as const)('does not bet rank eight after only visiting it, then editing rank nine in %s', mode => {
    props.mode = mode
    const value = mode === 'numbers' ? '4' : '大'
    label('编辑第八名').props.onClick!()
    expect(confirm().props.disabled).toBe(true)
    label('编辑第九名').props.onClick!()
    expect(label('编辑第八名').props['aria-pressed']).toBe(false)
    expect(label('编辑第九名').props['aria-pressed']).toBe(true)
    expect(label('切换名次（独立编辑）')).toBeDefined()
    clickChoice(value)
    expect(text(render())).toContain('已选 1 组 · 1 注')
    expect(text(confirm())).toContain('¥ 20')
    expect(text(confirm())).not.toContain('.00')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenLastCalledWith(`9/${value}/20`)
    label('编辑第八名').props.onClick!()
    expect(choice(value).props['aria-pressed']).toBe(false)
    expect(text(label('编辑第八名'))).toBe('第八名')
    label('编辑第九名').props.onClick!()
    label('编辑第九名').props.onClick!()
    expect(choice(value).props['aria-pressed']).toBe(true)
    clickChoice(value)
    expect(text(render())).toContain('已选 0 组 · 0 注')
  })

  it('keeps quick batch targets separate from independent number and side editing', () => {
    label('编辑亚军').props.onClick!()
    expect(label('编辑冠军').props['aria-pressed']).toBe(true)
    expect(label('编辑亚军').props['aria-pressed']).toBe(true)
    tab('车号 1–10')
    label('编辑第八名').props.onClick!()
    label('编辑第九名').props.onClick!()
    clickChoice('4')
    tab('两面盘')
    expect(label('编辑第九名').props['aria-pressed']).toBe(true)
    clickChoice('大')
    tab('快捷')
    expect(label('批量选择名次（可多选）')).toBeDefined()
    expect(label('编辑冠军').props['aria-pressed']).toBe(true)
    expect(label('编辑亚军').props['aria-pressed']).toBe(true)
    expect(label('编辑第九名').props['aria-pressed']).toBe(false)
    clickChoice('2')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenLastCalledWith('1/2/20#2/2/20#9/4大/20')
    expect(text(render())).toContain('已选 3 组 · 4 注')
    tab('车号 1–10')
    expect(label('编辑第九名').props['aria-pressed']).toBe(true)
    expect(choice('2').props['aria-pressed']).toBe(false)
    expect(choice('4').props['aria-pressed']).toBe(true)
    clickChoice('4')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenLastCalledWith('1/2/20#2/2/20#9/大/20')
  })

  it('matches the reported seventh/eighth/ninth sequence without changing earlier bets', () => {
    props.mode = 'numbers'
    label('编辑第七名').props.onClick!()
    clickChoice('2')
    label('编辑第八名').props.onClick!()
    label('编辑第九名').props.onClick!()
    clickChoice('4')
    expect(text(render())).toContain('已选 2 组 · 2 注')
    expect(text(confirm())).toBe('立即投注 ¥ 40')
    expect(text(label('已选投注'))).toBe('第七名 2第九名 4')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenLastCalledWith('7/2/20#9/4/20')
    label('编辑第七名').props.onClick!()
    expect(choice('2').props['aria-pressed']).toBe(true)
    expect(choice('4').props['aria-pressed']).toBe(false)
  })

  it('preserves one independent number per rank through repeated tab switches', () => {
    props.mode = 'numbers'
    const ranks = ['冠军', '亚军', '第三名', '第四名', '第五名', '第六名', '第七名', '第八名', '第九名', '第十名']
    for (let index = 0; index < ranks.length; index++) {
      label(`编辑${ranks[index]}`).props.onClick!()
      clickChoice(String(index + 1))
    }
    expect(text(render())).toContain('已选 10 组 · 10 注')
    expect(text(confirm())).toBe('立即投注 ¥ 200')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenLastCalledWith('1/1/20#2/2/20#3/3/20#4/4/20#5/5/20#6/6/20#7/7/20#8/8/20#9/9/20#0/0/20')
    for (let index = 0; index < ranks.length; index++) {
      label(`编辑${ranks[index]}`).props.onClick!()
      for (let number = 1; number <= 10; number++) expect(choice(String(number)).props['aria-pressed']).toBe(number === index + 1)
    }
  })

  it('validates custom amounts and gates submission on server odds, timing and in-flight state', () => {
    clickChoice('1')
    label('自定义单注金额').props.onChange!({ target: { value: '0.001' } })
    expect(confirm().props.disabled).toBe(true)
    confirm().props.onClick!()
    expect(props.onConfirm).not.toHaveBeenCalled()
    label('自定义单注金额').props.onChange!({ target: { value: '1.25' } })
    expect(text(confirm())).toContain('1.25')
    props.odds = {}
    expect(confirm().props.disabled).toBe(true)
    props.oddsHidden = true
    expect(confirm().props.disabled).toBe(false)
    props.oddsResponseReady = false
    expect(confirm().props.disabled).toBe(true)
    props.oddsResponseReady = true
    props.submitting = true
    expect(choice('1').props.disabled).toBe(true)
    expect(label('编辑亚军').props.disabled).toBe(true)
    expect(label('自定义单注金额').props.disabled).toBe(true)
    props.submitting = false
    props.game = { ...game, timing: { ...timing, accepting: false, statusLabel: '封盘中' } }
    expect(confirm().props.disabled).toBe(true)
    expect(text(confirm())).toContain('封盘中')
  })

  it('keeps sum selections separate from rank six and blocks invalid dragon/tiger choices', () => {
    tab('两面盘')
    label('编辑第六名').props.onClick!()
    expect(choice('龙').props.disabled).toBe(true)
    clickChoice('大')
    tab('冠亚和')
    clickChoice('大')
    clickChoice('10')
    confirm().props.onClick!()
    expect(props.onConfirm).toHaveBeenCalledWith('6/大/20#冠亚/大/20#冠亚/10/20')
    find(render(), node => node.props.className === 'full-bet-selection-toggle')!.props.onClick!()
    label('移除冠亚和10').props.onClick!()
    expect(text(confirm())).toBe('立即投注 ¥ 40')
    tab('快捷')
    expect(choice('大').props['aria-pressed']).toBe(false)
    label('编辑冠军').props.onClick!()
    expect(choice('1').props.disabled).toBe(true)
    tab('两面盘')
    expect(label('编辑第六名').props['aria-pressed']).toBe(true)
    expect(choice('大').props['aria-pressed']).toBe(true)
  })
})
