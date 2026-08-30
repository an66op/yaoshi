import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { DrawResult } from '../api/lottery'
import { HookHarness } from '../test/hookHarness'
import { useGameTimelineWindow } from './useGameTimelineWindow'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null }))
vi.mock('react', async original => ({ ...await original<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useEffect: (effect: () => void | (() => void), deps?: readonly unknown[]) => runtime.hooks!.useEffect(effect, deps),
}))
const draw = (issue: number, gameId = 'speed-racing'): DrawResult => ({ id: issue, game_id: gameId, issue: String(issue), numbers: [7, 5, 10, 3, 9, 8, 4, 1, 2, 6], draw_at: new Date(Date.UTC(2026, 7, 30, 12, issue)).toISOString() })
const render = (rows: DrawResult[], loading = false, gameId = 'speed-racing') => {
  runtime.hooks!.render(() => useGameTimelineWindow(gameId, rows, loading))
  runtime.hooks!.flushEffects()
  return runtime.hooks!.render(() => useGameTimelineWindow(gameId, rows, loading))
}

describe('one-draw entry boundary and append-only visit', () => {
  beforeEach(() => { runtime.hooks = new HookHarness() })

  it('waits for the first draw read, then starts from only the latest confirmed issue', () => {
    expect(render([], true).ready).toBe(false)
    const state = render([draw(32), draw(34), draw(33)])
    expect(state).toMatchObject({ ready: true, anchorIssue: '34', startAt: Date.parse(draw(34).draw_at) })
    expect(state.draws.map(row => row.issue)).toEqual(['34'])
  })

  it('keeps the entry boundary and all seen draws while API history rolls over for a long stay', () => {
    const initial = render([draw(34), draw(33)])
    for (let issue = 35; issue <= 50; issue++) {
      const state = render([draw(issue), draw(issue - 1)])
      expect(state.startAt).toBe(initial.startAt)
      expect(state.draws.map(row => row.issue)).toEqual(Array.from({ length: issue - 33 }, (_, i) => String(i + 34)))
    }
    expect(render([], true).draws).toHaveLength(17)
  })

  it('re-entry discards the prior visit and starts at the then-latest draw', () => {
    render([draw(34), draw(33)])
    render([draw(35), draw(34)])
    runtime.hooks!.unmount()
    runtime.hooks = new HookHarness()
    expect(render([draw(35), draw(34), draw(33)]).draws.map(row => row.issue)).toEqual(['35'])
  })

  it('never exposes a different game snapshot and resets when switching games', () => {
    render([draw(34)])
    expect(render([draw(34)], false, 'speed-fly')).toMatchObject({ ready: false, draws: [] })
    const next = render([draw(40, 'speed-fly'), draw(39, 'speed-fly')], false, 'speed-fly')
    expect(next.draws.map(row => row.issue)).toEqual(['40'])
    expect(next.startAt).toBe(Date.parse(draw(40).draw_at))
  })

  it('updates a corrected result in place without moving the visit boundary', () => {
    const initial = render([draw(34)])
    const corrected = { ...draw(34), numbers: [5, 7, 10, 3, 9, 8, 4, 1, 2, 6] }
    const state = render([draw(35), corrected, draw(33)])
    expect(state.draws).toEqual([corrected, draw(35)])
    expect(state.startAt).toBe(initial.startAt)
  })
})
