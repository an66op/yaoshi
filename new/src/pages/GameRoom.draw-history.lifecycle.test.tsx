import { isValidElement, type ComponentProps, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { DrawResult } from '../api/lottery'
import { DrawResultCards } from '../components/DrawResultCards'
import { HookHarness } from '../test/hookHarness'
import { DrawAssistantMessage } from './GameRoom'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useMemo: <T,>(factory: () => T, dependencies: readonly unknown[]) => runtime.hooks!.useMemo(factory, dependencies),
}))
vi.mock('../api/client', () => ({ apiBase: 'http://localhost:8080/api', request: vi.fn(), publicRequest: vi.fn() }))

type Props = ComponentProps<typeof DrawAssistantMessage>
type CardProps = ComponentProps<typeof DrawResultCards>
const draw = (issue: number): DrawResult => ({ id: issue, game_id: 'speed-racing', issue: String(issue), numbers: [6, 1, 2, 8, 9, 10, 4, 7, 5, 3], draw_at: `2026-08-30T06:${issue}:00Z` })

function findCards(node: ReactNode): CardProps | undefined {
  if (Array.isArray(node)) return node.map(findCards).find(Boolean)
  if (!isValidElement<CardProps & { children?: ReactNode }>(node)) return
  if (node.type === DrawResultCards) return node.props
  return findCards(node.props.children)
}

describe('immutable historical draw announcements', () => {
  const previous = draw(10)
  const current = draw(11)
  const props: Props = { gameTitle: '极速赛车', draw: current, draws: [current, previous] }
  const render = (updates: Partial<Props> = {}) => findCards(runtime.hooks!.render(() => DrawAssistantMessage({ ...props, ...updates })))!

  beforeEach(() => { runtime.hooks = new HookHarness() })

  it('retains its original range when new results append or older API rows roll out', () => {
    const initial = render()
    expect(initial.draws.map(row => row.issue)).toEqual(['11', '10'])
    const next = draw(12)
    const updated = render({ draws: [next, current] })
    expect(updated.draws).toBe(initial.draws)
    expect(updated.draws.map(row => row.issue)).toEqual(['11', '10'])
    expect(updated.draw).toBe(current)
  })

  it('does not repaint the historical range on unchanged parent renders', () => {
    const initial = render()
    for (let tick = 0; tick < 30; tick += 1) expect(render().draws).toBe(initial.draws)
  })

  it('allows a verified correction of this issue without adding future results', () => {
    render()
    const corrected = { ...current, numbers: [1, 6, 2, 8, 9, 10, 4, 7, 5, 3] }
    const updated = render({ draw: corrected, draws: [draw(12), corrected, previous] })
    expect(updated.draws).toEqual([corrected, previous])
    expect(updated.draw).toBe(corrected)
  })
})
