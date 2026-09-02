import { isValidElement, type ComponentProps, type ReactElement, type ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { DrawResult } from '../api/lottery'
import type { Game } from '../types'
import { HookHarness } from '../test/hookHarness'
import { DrawResults } from './DrawResults'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null, draws: [] as DrawResult[], loading: false }))
vi.mock('react', async original => ({
  ...await original<typeof import('react')>(),
  useState: <T,>(value: T | (() => T)) => runtime.hooks!.useState(value),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))
vi.mock('../hooks/useGameDraws', () => ({ useGameDraws: () => ({ draws: runtime.draws, loading: runtime.loading }) }))
type Props = ComponentProps<typeof DrawResults>
type NodeProps = { children?: ReactNode; className?: string; 'aria-label'?: string; onClick?: () => void; onChange?: (event: { target: { value: string } }) => void }
type Node = ReactElement<NodeProps>
function elements(node: ReactNode): Node[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  return isValidElement<NodeProps>(node) ? [node, ...elements(node.props.children)] : []
}
function text(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(text).join('')
  return isValidElement<NodeProps>(node) ? text(node.props.children) : ''
}
const makeGame = (id: string, balls: number[]) => ({ id, title: id, tag: '', color: '', balls }) as Game
const makeDraw = (gameId: string, numbers: number[], issue = '20260831-00000123456789'): DrawResult => ({ id: 1, game_id: gameId, issue, numbers, draw_at: '2026-08-31T08:00:01Z' })
const words = (html: string) => [...html.matchAll(/<b class="result-(?:blue|orange|neutral)">([^<]+)<\/b>/g)].map(match => match[1])

describe('draw history uses explicit per-game rule profiles', () => {
  let props: Props
  const render = () => {
    runtime.hooks!.render(() => DrawResults(props))
    runtime.hooks!.flushEffects()
    return runtime.hooks!.render(() => DrawResults(props))
  }
  const markup = () => renderToStaticMarkup(render())
  const clickMode = (label: string) => elements(render()).find(node => node.type === 'button' && text(node) === label)!.props.onClick!()
  const modeLabels = () => {
    const nav = elements(render()).find(node => node.props['aria-label'] === '开奖显示方式')!
    return elements(nav).filter(node => node.type === 'button').map(text)
  }

  beforeEach(() => {
    runtime.hooks = new HookHarness()
    runtime.loading = false
    const game = makeGame('speed-ssc', [9, 8, 1, 2, 3])
    props = { games: [game], initialGameId: game.id, onBack: vi.fn(), onSelectGame: vi.fn() }
    runtime.draws = [makeDraw(game.id, game.balls)]
  })

  it('uses all five SSC balls for totals and compares only the two actual mirror pairs', () => {
    expect(modeLabels()).toEqual(['号码', '大小', '单双', '总和/龙虎'])
    clickMode('总和/龙虎')
    const html = markup()
    expect(html).toContain('<strong>23</strong>')
    expect(words(html)).toEqual(['大', '单', '龙', '龙'])
    expect(html).toContain('总和：23大单，1–2 龙虎：龙龙')
    expect(html).not.toContain('虎虎虎虎虎')
  })

  it('uses one first-versus-fifth result only for exact digits5-v3 products', () => {
    props.games = [{ ...props.games[0], ruleVersion: 'digits5-v3' }]
    clickMode('总和/龙虎')
    expect(words(markup())).toEqual(['大', '单', '龙'])
    expect(markup()).toContain('第一球 vs 第五球 龙虎和：龙')

    const bingo = { ...makeGame('bingo-ssc-1', [9, 8, 1, 2, 3]), ruleVersion: 'digits5-v3' }
    runtime.hooks = new HookHarness()
    props.games = [bingo]
    props.initialGameId = bingo.id
    runtime.draws = [makeDraw(bingo.id, bingo.balls)]
    clickMode('总和/龙虎')
    expect(words(markup())).toEqual(['大', '单', '龙'])

    const sg = { ...makeGame('sg-ssc', [9, 8, 1, 2, 3]), ruleVersion: 'digits5-v3' }
    runtime.hooks = new HookHarness()
    props.games = [sg]
    props.initialGameId = sg.id
    runtime.draws = [makeDraw(sg.id, sg.balls)]
    clickMode('总和/龙虎')
    expect(words(markup())).toEqual(['大', '单', '龙', '龙'])
  })

  it('uses SSC thresholds despite mutable titles or a live snapshot containing a ten', () => {
    props.games = [{ ...props.games[0], title: '赛车', category: 'racing', balls: [10, 9, 8, 7, 6] }]
    runtime.draws = [makeDraw('speed-ssc', [0, 4, 5, 8, 9])]
    clickMode('大小')
    expect(words(markup())).toEqual(['小', '小', '大', '大', '大'])
    clickMode('单双')
    expect(words(markup())).toEqual(['双', '双', '单', '双', '单'])
  })

  it.each(['official-fc3d', 'official-pl3'])('uses three-ball totals and represents ties for %s', id => {
    const game = makeGame(id, [7, 0, 7])
    props.games = [game]
    props.initialGameId = id
    runtime.draws = [makeDraw(id, game.balls)]
    clickMode('总和/龙虎')
    const html = markup()
    expect(html).toContain('<strong>14</strong>')
    expect(words(html)).toEqual(['大', '双', '和'])
    expect(html).toContain('result-neutral')
    expect(html).toContain('1–1 龙虎：和')
  })

  it.each(['pc-canada', 'canada-28', 'canada-20'])('shows the three PC28 balls, their sum and first-versus-third result for %s', id => {
    const game = makeGame(id, [9, 1, 9])
    props.games = [game]
    props.initialGameId = id
    runtime.draws = [makeDraw(id, game.balls)]
    expect(modeLabels()).toEqual(['号码', '大小', '单双', '和值/龙虎'])
    clickMode('和值/龙虎')
    const html = markup()
    expect(html).toContain('<strong>19</strong>')
    expect(words(html)).toEqual(['大', '单', '和'])
    expect(html).toContain('和值：19大单，第一球 vs 第三球 龙虎和：和')
  })

  it('uses racing first-two totals and the racing single-number threshold from identity', () => {
    const game = makeGame('speed-racing', [0, 1, 2])
    props.games = [game]
    props.initialGameId = game.id
    runtime.draws = [makeDraw(game.id, [5, 6, 1, 2, 3, 4, 7, 8, 9, 10])]
    expect(modeLabels()).toEqual(['号码', '大小', '单双', '冠亚和/龙虎'])
    clickMode('大小')
    expect(words(markup()).slice(0, 2)).toEqual(['小', '大'])
    clickMode('冠亚和/龙虎')
    expect(markup()).toContain('<strong>11</strong>')
    expect(words(markup())).toEqual(['小', '单', '虎', '虎', '虎', '虎', '虎'])
  })

  it('renders Bingo Mark Six with fixed wave colours, a separated seventh ball and special attributes', () => {
    const markSix = makeGame('bingo-mark-six', [5, 9, 40, 47, 29, 2, 49])
    props.games = [markSix]
    props.initialGameId = markSix.id
    runtime.draws = [makeDraw(markSix.id, markSix.balls)]
    expect(modeLabels()).toEqual(['号码', '大小', '单双', '特码属性'])
    const numberMarkup = markup()
    expect(numberMarkup).toContain('mark-six-ball wave-green')
    expect(numberMarkup).toContain('mark-six-ball wave-blue')
    expect(numberMarkup).toContain('mark-six-ball wave-green mark-six-special-ball')
    expect(numberMarkup).toContain('>05</b><small>虎</small>')
    expect(numberMarkup).toContain('>49</b><small>马</small>')
    clickMode('大小')
    expect(words(markup())).toEqual(['小', '小', '大', '大', '大', '小', '和'])
    clickMode('单双')
    expect(words(markup())).toEqual(['单', '单', '双', '单', '单', '双', '和'])
    clickMode('特码属性')
    expect(markup()).toContain('特码：49 绿波 和局')
  })

  it.each(['hk-marksix', 'official-qxc', 'official-tw-bingo'])('offers only original numbers for an unconfigured %s game', id => {
    const game = makeGame(id, [1, 10, 3])
    props.games = [game]
    props.initialGameId = id
    runtime.draws = [makeDraw(id, game.balls)]
    expect(modeLabels()).toEqual(['号码'])
    const html = markup()
    expect(html).toContain('result-mode-numbers')
    expect(html).not.toContain('racing-results-table')
    expect(html).not.toContain('trend-cells')
    expect(words(html)).toEqual([])
  })

  it('preserves all twenty numbers and the complete long issue with ten-column wrapping', () => {
    const game = makeGame('official-tw-bingo', Array.from({ length: 20 }, (_, index) => index + 1))
    props.games = [game]
    props.initialGameId = game.id
    runtime.draws = [makeDraw(game.id, game.balls, '20260831-00000123456789')]
    const html = markup()
    expect(html).toContain('<b>20260831-00000123456789</b>')
    expect(html).toContain('style="--result-count:10"')
    const cells = html.match(/class="draw-result-cells number-cells"[^>]*>(.*?)<\/div>/)![1]
    expect([...cells.matchAll(/<b\b[^>]*>(\d+)<\/b>/g)].map(match => Number(match[1]))).toEqual(game.balls)
  })

  it('shows raw numbers rather than invented metrics for a malformed known-game draw', () => {
    runtime.draws = [makeDraw('speed-ssc', [1, 2, 3, 4, 10])]
    clickMode('总和/龙虎')
    const html = markup()
    expect(html).toContain('number-cells')
    expect(html).not.toContain('trend-cells')
    expect(words(html)).toEqual([])
  })

  it('does not borrow latest game numbers for empty historical records or retain another game’s draw', () => {
    runtime.draws = [makeDraw('speed-ssc', []), makeDraw('speed-racing', [1, 2, 3], 'OTHER-GAME-ISSUE')]
    const html = markup()
    expect(html).toContain('暂无号码')
    expect(html).not.toContain('number-cells')
    expect(html).not.toContain('OTHER-GAME-ISSUE')
  })

  it('resets unavailable display modes when switching from a known game to an unknown game', () => {
    const unknown = makeGame('hk-marksix', [1, 2, 3])
    props.games = [...props.games, unknown]
    clickMode('总和/龙虎')
    elements(render()).find(node => node.props.className === 'draw-game-picker-trigger')!.props.onClick!()
    elements(render()).find(node => node.type === 'button' && text(node) === 'hk-marksix')!.props.onClick!()
    runtime.draws = [makeDraw(unknown.id, unknown.balls)]
    expect(props.onSelectGame).toHaveBeenCalledWith(unknown.id)
    expect(modeLabels()).toEqual(['号码'])
    expect(markup()).toContain('result-mode-numbers')
  })

  it('retains a manual selection across countdown refreshes and clears it only when removed', () => {
    const second = makeGame('official-fc3d', [2, 3, 4])
    props.games = [...props.games, second]
    elements(render()).find(node => node.props.className === 'draw-game-picker-trigger')!.props.onClick!()
    elements(render()).find(node => node.type === 'button' && text(node) === second.id)!.props.onClick!()
    runtime.draws = [makeDraw(second.id, second.balls)]
    props.games = props.games.map(game => ({ ...game, balls: [...game.balls] }))
    expect(markup()).toContain('official-fc3d · 历史开奖记录')
    props.games = [props.games[0]]
    expect(markup()).toContain('speed-ssc · 历史开奖记录')
  })
})
