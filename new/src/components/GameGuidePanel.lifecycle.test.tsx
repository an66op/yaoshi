import { isValidElement, type ReactElement, type ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { GameOdds } from '../api/portal'
import { HookHarness } from '../test/hookHarness'
import type { Game } from '../types'
import { GameGuidePanel } from './GameGuidePanel'

const runtime = vi.hoisted(() => ({
  hooks: null as HookHarness | null,
  requests: [] as Array<{ gameId: string; resolve: (value: GameOdds) => void; reject: (reason: Error) => void }>,
}))

vi.mock('react', async original => ({
  ...await original<typeof import('react')>(),
  useState: <T,>(value: T | (() => T)) => runtime.hooks!.useState(value),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
  useMemo: <T,>(factory: () => T, dependencies: readonly unknown[]) => runtime.hooks!.useMemo(factory, dependencies),
  useRef: <T,>(value: T) => runtime.hooks!.useRef(value),
}))
vi.mock('../api/portal', () => ({
  portalApi: {
    gameOdds: (gameId: string) => new Promise<GameOdds>((resolve, reject) => runtime.requests.push({ gameId, resolve, reject })),
  },
}))

type NodeProps = {
  children?: ReactNode
  className?: string
  role?: string
  onClick?: () => void
  'aria-expanded'?: boolean
  'aria-haspopup'?: string
  'aria-pressed'?: boolean
  'data-game-manual-id'?: string
}
type Node = ReactElement<NodeProps>
function elements(node: ReactNode): Node[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  return isValidElement<NodeProps>(node) ? [node, ...elements(node.props.children)] : []
}
const game = (id: string, title: string) => ({ id, title }) as Game
const response = (gameId: string, gameName: string, odds: number): GameOdds => ({
  game_id: gameId, game_name: gameName, rules_ready: true, rule_version: gameId === 'speed-racing' ? 'racing-v2' : 'digits5-v2',
  bet_modes: { chat: true, web: true }, show_odds: true,
  items: [{ play_code: 'two_sided', play_name: `${gameName}两面`, odds, min_bet: 1, max_bet: 500, max_user_period: 1000 }],
})

describe('GameGuidePanel odds switching', () => {
  const games = [game('speed-racing', '极速赛车'), game('speed-ssc', '极速时时彩')]
  const render = () => runtime.hooks!.render(() => GameGuidePanel({ games, initialTab: 'odds' }))
  const chooseGame = (id: string) => {
    elements(render()).find(node => node.props.className === 'game-guide-picker-trigger')!.props.onClick!()
    elements(render()).find(node => node.props['data-game-manual-id'] === id)!.props.onClick!()
  }

  beforeEach(() => {
    runtime.hooks = new HookHarness()
    runtime.requests = []
  })

  it('reloads effective odds per game and ignores a late response from the previous game', async () => {
    render(); runtime.hooks!.flushEffects()
    expect(runtime.requests.map(item => item.gameId)).toEqual(['speed-racing'])
    const first = runtime.requests[0]
    chooseGame('speed-ssc')
    const switchingHTML = renderToStaticMarkup(render())
    expect(switchingHTML).toContain('正在读取当前房间赔率')
    expect(switchingHTML).not.toContain('极速赛车两面')
    render(); runtime.hooks!.flushEffects()
    expect(runtime.requests.map(item => item.gameId)).toEqual(['speed-racing', 'speed-ssc'])

    runtime.requests[1].resolve(response('speed-ssc', '极速时时彩', 1.998))
    await Promise.resolve(); await Promise.resolve()
    first.resolve(response('speed-racing', '极速赛车', 9.9))
    await Promise.resolve(); await Promise.resolve()

    const html = renderToStaticMarkup(render())
    expect(html).toContain('极速时时彩两面')
    expect(html).toContain('1.998')
    expect(html).not.toContain('极速赛车两面')
    expect(html).not.toContain('9.9</strong>')
  })

  it('shows reference-only manuals without requesting fabricated odds', () => {
    render(); runtime.hooks!.flushEffects()
    chooseGame('reference-animal-1m')
    render(); runtime.hooks!.flushEffects()
    expect(runtime.requests).toHaveLength(1)
    expect(renderToStaticMarkup(render())).toContain('没有可用赔率')
  })

  it('clears an already displayed old game before rendering the next title', async () => {
    render(); runtime.hooks!.flushEffects()
    runtime.requests[0].resolve(response('speed-racing', '极速赛车', 9.9))
    await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve()
    expect(renderToStaticMarkup(render())).toContain('极速赛车两面')

    chooseGame('speed-ssc')
    const html = renderToStaticMarkup(render())
    expect(html).toContain('正在读取当前房间赔率')
    expect(html).not.toContain('极速赛车两面')
    expect(html).not.toContain('9.9</strong>')
  })

  it('uses an accessible custom picker instead of the browser native select', () => {
    const closed = render()
    const trigger = elements(closed).find(node => node.props.className === 'game-guide-picker-trigger')!
    expect(trigger.props['aria-haspopup']).toBe('dialog')
    expect(trigger.props['aria-expanded']).toBe(false)
    expect(renderToStaticMarkup(closed)).not.toContain('<select')

    trigger.props.onClick!()
    const opened = render()
    expect(elements(opened).some(node => node.props.role === 'dialog')).toBe(true)
    expect(elements(opened).find(node => node.props['data-game-manual-id'] === 'speed-racing')!.props['aria-pressed']).toBe(true)

    elements(opened).find(node => node.props['data-game-manual-id'] === 'speed-ssc')!.props.onClick!()
    const switched = render()
    expect(elements(switched).some(node => node.props.role === 'dialog')).toBe(false)
    expect(renderToStaticMarkup(switched)).toContain('极速时时彩')
  })
})
