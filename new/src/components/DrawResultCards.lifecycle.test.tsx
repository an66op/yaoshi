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
vi.mock('../utils/drawResultCardCanvas', async (importOriginal) => ({
  ...await importOriginal<typeof import('../utils/drawResultCardCanvas')>(),
  paintCurrentDrawCard: runtime.current,
  paintRecentDrawCard: runtime.recent,
}))

type Props = ComponentProps<typeof DrawResultCards>
const draw: DrawResult = { id: 11, game_id: 'speed-racing', issue: '34136854', numbers: [6, 1, 2, 8, 9, 10, 4, 7, 5, 3], draw_at: '2026-08-30T06:46:00Z' }

describe('draw card paint effects', () => {
  const attachCanvasRefs = (node: ReactNode) => {
    if (Array.isArray(node)) { node.forEach(attachCanvasRefs); return }
    if (!isValidElement<{ ref?: { current: unknown }; children?: ReactNode }>(node)) return
    if (node.type === 'canvas' && node.props.ref && !node.props.ref.current) {
      const canvas = { width: 0, height: 0, toDataURL: vi.fn(() => {
        expect(canvas.width).toBeGreaterThan(0)
        expect(canvas.height).toBeGreaterThan(0)
        return 'data:image/png;base64,test'
      }) }
      node.props.ref.current = canvas
    }
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
    runtime.current.mockReset().mockImplementation((canvas: HTMLCanvasElement) => { canvas.width = 1440; canvas.height = 900 })
    runtime.recent.mockReset().mockImplementation((canvas: HTMLCanvasElement) => { canvas.width = 1440; canvas.height = 310 })
    vi.stubGlobal('window', { devicePixelRatio: 2 })
    vi.stubGlobal('Image', class {
      decoding = ''
      onload?: () => void
      set src(_value: string) { queueMicrotask(() => this.onload?.()) }
    })
  })
  afterEach(() => { runtime.hooks?.unmount(); vi.unstubAllGlobals() })

  const installObserver = () => {
    const observations: { callback: IntersectionObserverCallback; canvas?: HTMLCanvasElement; disconnect: () => void }[] = []
    vi.stubGlobal('IntersectionObserver', class {
      observation: typeof observations[number]
      constructor(callback: IntersectionObserverCallback, options?: IntersectionObserverInit) {
        expect(options).toEqual({ rootMargin: '240px 0px', threshold: 0 })
        this.observation = { callback, disconnect: vi.fn() }
        observations.push(this.observation)
      }
      observe(canvas: HTMLCanvasElement) { this.observation.canvas = canvas }
      disconnect() { this.observation.disconnect() }
    })
    const enter = (index: number, isIntersecting: boolean) => {
      const observation = observations[index]
      observation.callback([{ target: observation.canvas, isIntersecting } as unknown as IntersectionObserverEntry], {} as IntersectionObserver)
    }
    return { observations, enter }
  }

  const buttons = (node: ReactNode): { onClick: () => void; disabled: boolean }[] => {
    if (Array.isArray(node)) return node.flatMap(buttons)
    if (!isValidElement<{ className?: string; onClick: () => void; disabled: boolean; children?: ReactNode }>(node)) return []
    if (node.props.className === 'draw-image-trigger') return [node.props]
    return buttons(node.props.children)
  }

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

  it('paints each image only near the viewport, releases distant bitmaps and repaints the latest props on re-entry', async () => {
    const { observations, enter } = installObserver()
    const props: Props = { title: '极速赛车', draw, draws: [draw] }
    render(props)
    await Promise.resolve(); await Promise.resolve()
    render(props)
    expect(observations).toHaveLength(2)
    expect(runtime.current).not.toHaveBeenCalled()
    expect(runtime.recent).not.toHaveBeenCalled()

    enter(0, true)
    render(props)
    expect(runtime.current).toHaveBeenCalledOnce()
    expect(runtime.recent).not.toHaveBeenCalled()
    expect(observations[0].canvas?.width).toBe(1440)
    enter(1, true)
    render(props)
    expect(runtime.recent).toHaveBeenCalledOnce()

    enter(0, false); enter(1, false)
    render(props)
    for (const observation of observations) {
      expect(observation.canvas?.width).toBe(0)
      expect(observation.canvas?.height).toBe(0)
    }
    runtime.current.mockClear(); runtime.recent.mockClear()
    const next = { ...draw, id: 12, issue: '34136855' }
    const updated = { title: '极速赛车', draw: next, draws: [next, draw] }
    render(updated)
    expect(runtime.current).not.toHaveBeenCalled()
    expect(runtime.recent).not.toHaveBeenCalled()
    enter(0, true); enter(1, true)
    render(updated)
    expect(runtime.current.mock.lastCall?.[2]).toBe(next)
    expect(runtime.recent.mock.lastCall?.[2]).toBe(updated.draws)

    runtime.hooks!.unmount()
    for (const observation of observations) {
      expect(observation.disconnect).toHaveBeenCalledOnce()
      expect(observation.canvas?.width).toBe(0)
      expect(observation.canvas?.height).toBe(0)
    }
    runtime.hooks = null
  })

  it('synchronously paints the correct issue before exporting even if the observer has not reported it visible', async () => {
    const { observations } = installObserver()
    const props: Props = { title: '极速赛车', draw, draws: [draw] }
    render(props)
    await Promise.resolve(); await Promise.resolve()
    const next = { ...draw, id: 12, issue: '34136855', numbers: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10] }
    const updated = { title: '极速赛车', draw: next, draws: [next, draw] }
    const tree = render(updated)
    const triggers = buttons(tree)
    expect(triggers).toHaveLength(2)
    expect(triggers[0].disabled).toBe(false)
    expect(runtime.current).not.toHaveBeenCalled()
    triggers[0].onClick()
    expect(runtime.current.mock.lastCall?.[2]).toBe(next)
    expect(observations[0].canvas?.toDataURL).toHaveBeenCalledExactlyOnceWith('image/png')
    expect(observations[0].canvas?.width).toBe(0)
    triggers[1].onClick()
    expect(runtime.recent.mock.lastCall?.[2]).toBe(updated.draws)
    expect(observations[1].canvas?.toDataURL).toHaveBeenCalledExactlyOnceWith('image/png')
    expect(observations[1].canvas?.width).toBe(0)
  })

  it('shares a single artwork load across retained issue cards', async () => {
    const ImageConstructor = vi.fn(function (this: { onload?: () => void; decoding: string; src: string }) {
      Object.defineProperty(this, 'src', { set: () => queueMicrotask(() => this.onload?.()) })
    })
    vi.stubGlobal('Image', ImageConstructor)
    const mounted: HookHarness[] = []
    for (let index = 0; index < 8; index++) {
      runtime.hooks = new HookHarness()
      mounted.push(runtime.hooks)
      render({ title: '极速赛车', draw: { ...draw, id: index }, draws: [draw] })
    }
    await Promise.resolve(); await Promise.resolve()
    expect(ImageConstructor.mock.calls.length).toBeLessThanOrEqual(1)
    mounted.forEach(hooks => hooks.unmount())
    runtime.hooks = null
  })
})
