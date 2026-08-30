import { isValidElement, type ComponentProps, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { DrawResult } from '../api/lottery'
import { HookHarness } from '../test/hookHarness'
import { DrawResultCards } from './DrawResultCards'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null, current: vi.fn(), recent: vi.fn() }))
vi.mock('react', async (importOriginal) => ({
  ...await importOriginal<typeof import('react')>(),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))
vi.mock('../utils/drawResultCardCanvas', () => ({
  drawCardIssueLabel: (issue: string) => issue,
  paintCurrentDrawCard: runtime.current,
  paintRecentDrawCard: runtime.recent,
}))

type Props = ComponentProps<typeof DrawResultCards>
const draw: DrawResult = { id: 11, game_id: 'speed-racing', issue: '34136854', numbers: [6, 1, 2, 8, 9, 10, 4, 7, 5, 3], draw_at: '2026-08-30T06:46:00Z' }

describe('draw card paint effects', () => {
  const attachCanvasRefs = (node: ReactNode) => {
    if (Array.isArray(node)) { node.forEach(attachCanvasRefs); return }
    if (!isValidElement<{ ref?: { current: unknown }; children?: ReactNode }>(node)) return
    if (node.type === 'canvas' && node.props.ref) node.props.ref.current = { toDataURL: vi.fn(() => 'data:image/png;base64,test') }
    attachCanvasRefs(node.props.children)
  }
  const render = (props: Props) => {
    // Execute this component's hook lifecycle without a DOM implementation.
    // React's normal memo wrapper remains intact (no custom comparator).
    const component = DrawResultCards as unknown as { type: (props: Props) => ReactNode }
    const result = runtime.hooks!.render(() => component.type(props))
    attachCanvasRefs(result)
    runtime.hooks!.flushEffects()
    return result
  }

  beforeEach(() => {
    runtime.hooks = new HookHarness()
    runtime.current.mockClear()
    runtime.recent.mockClear()
    vi.stubGlobal('window', { devicePixelRatio: 2 })
    vi.stubGlobal('Image', class {
      decoding = ''
      onload?: () => void
      set src(_value: string) { queueMicrotask(() => this.onload?.()) }
    })
  })
  afterEach(() => { runtime.hooks?.unmount(); vi.unstubAllGlobals() })

  it('does not repaint on parent clock ticks; title, latest draw, history and artwork updates remain independent', async () => {
    const props: Props = { title: '极速赛车', draw, draws: [draw] }
    render(props)
    await Promise.resolve(); await Promise.resolve()
    render(props)
    expect(runtime.current.mock.lastCall?.[3]).toBeTruthy()
    expect(runtime.recent.mock.lastCall?.[3]).toBeTruthy()
    runtime.current.mockClear(); runtime.recent.mockClear()

    for (let tick = 0; tick < 30; tick++) render({ ...props })
    expect(runtime.current).not.toHaveBeenCalled()
    expect(runtime.recent).not.toHaveBeenCalled()

    const older = { ...draw, id: 10, issue: '34136853' }
    const withHistory = { ...props, draws: [draw, older] }
    render(withHistory)
    expect(runtime.current).not.toHaveBeenCalled()
    expect(runtime.recent).toHaveBeenCalledOnce()
    runtime.recent.mockClear()

    const corrected = { ...draw, numbers: [1, 6, 2, 8, 9, 10, 4, 7, 5, 3] }
    const withCorrection = { ...withHistory, draw: corrected }
    render(withCorrection)
    expect(runtime.current).toHaveBeenCalledExactlyOnceWith(expect.anything(), { title: '极速赛车' }, corrected, expect.anything(), 2)
    expect(runtime.recent).not.toHaveBeenCalled()
    runtime.current.mockClear()

    render({ ...withCorrection, title: '新的彩种名称' })
    expect(runtime.current).toHaveBeenCalledOnce()
    expect(runtime.recent).toHaveBeenCalledOnce()
    expect(runtime.current.mock.lastCall?.[1]).toEqual({ title: '新的彩种名称' })
    expect(runtime.recent.mock.lastCall?.[1]).toEqual({ title: '新的彩种名称' })
  })
})
