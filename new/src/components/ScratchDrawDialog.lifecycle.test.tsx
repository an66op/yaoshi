import { isValidElement, type ReactElement, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Game } from '../types'
import { HookHarness } from '../test/hookHarness'
import { ScratchDrawDialog } from './ScratchDrawDialog'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null, createMask: vi.fn(), reveal: null as null | (() => void) }))
vi.mock('react', async (original) => ({
  ...await original<typeof import('react')>(),
  useRef: <T,>(value: T) => runtime.hooks!.useRef(value),
  useState: <T,>(value: T | (() => T)) => runtime.hooks!.useState(value),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))
vi.mock('../utils/scratchDraw', async (original) => ({ ...await original<typeof import('../utils/scratchDraw')>(), createScratchMask: runtime.createMask }))

type NodeProps = { children?: ReactNode; className?: string; ref?: { current: unknown }; onClick?: () => void; [key: string]: unknown }
type Node = ReactElement<NodeProps>
const elements = (node: ReactNode): Node[] => {
  if (Array.isArray(node)) return node.flatMap(elements)
  return isValidElement<NodeProps>(node) ? [node, ...elements(node.props.children)] : []
}
const game = { id: 'official-fc3d', title: '福彩3D', period: '20260830002', latestIssue: '20260830001', balls: [3, 7, 8] } as Game

describe('scratch surface pointer lifecycle', () => {
  let outer: HookHarness
  let surface: HookHarness
  let key: string | null
  let captured: number | null
  let resizeCallback: () => void
  let scrollSurface: { scrollLeft: number }
  let canvas: {
    getBoundingClientRect: ReturnType<typeof vi.fn>
    setPointerCapture: ReturnType<typeof vi.fn>
    hasPointerCapture: ReturnType<typeof vi.fn>
    releasePointerCapture: ReturnType<typeof vi.fn>
  }
  const mask = { resize: vi.fn(), scratch: vi.fn(), reveal: vi.fn(), reset: vi.fn() }
  const observer = { observe: vi.fn(), disconnect: vi.fn() }
  const render = (current = game): Node => {
    runtime.hooks = outer
    const dialog = outer.render(() => ScratchDrawDialog({ game: current, onClose: () => {} }))
    const wrapper = (dialog.props as NodeProps).children as Node
    const child = wrapper.props.children as ReactElement<{ numbers: number[]; gameId: string }>
    if (child.key !== key) { surface.unmount(); surface = new HookHarness(); key = child.key }
    runtime.hooks = surface
    const tree = surface.render(() => (child.type as (props: { numbers: number[]; gameId: string }) => Node)(child.props))
    elements(tree).find((node) => node.props.className?.startsWith('scratch-draw-surface'))!.props.ref!.current = scrollSurface
    elements(tree).find((node) => node.type === 'canvas')!.props.ref!.current = canvas
    surface.flushEffects()
    return tree
  }
  const event = (pointerId = 1, clientX = 80, clientY = 100, button = 0) => ({
    currentTarget: canvas, pointerId, clientX, clientY, button, isPrimary: true, preventDefault: vi.fn(),
  })
  const pointer = (tree: Node, handler: string, value = event()) => {
    const node = elements(tree).find((item) => item.type === 'canvas')!
    ;(node.props[handler] as (event: unknown) => void)(value)
    return value
  }

  beforeEach(() => {
    vi.useFakeTimers()
    outer = new HookHarness(); surface = new HookHarness(); key = null; captured = null
    scrollSurface = { scrollLeft: 0 }
    canvas = {
      getBoundingClientRect: vi.fn(() => ({ left: 20, top: 40, width: 240, height: 120 })),
      setPointerCapture: vi.fn((id) => { captured = id }),
      hasPointerCapture: vi.fn((id) => captured === id),
      releasePointerCapture: vi.fn(() => { captured = null }),
    }
    vi.stubGlobal('window', { devicePixelRatio: 2, addEventListener: vi.fn(), removeEventListener: vi.fn() })
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: () => void) { resizeCallback = callback }
      observe = observer.observe
      disconnect = observer.disconnect
    })
    runtime.createMask.mockImplementation((_canvas, onReveal: () => void) => { runtime.reveal = onReveal; return mask })
    mask.reveal.mockImplementation(() => runtime.reveal?.())
  })
  afterEach(() => { outer.unmount(); surface.unmount(); vi.clearAllMocks(); vi.unstubAllGlobals(); vi.useRealTimers() })

  it('never auto-reveals; clock renders preserve the mask and a new real result remounts it', () => {
    render()
    vi.advanceTimersByTime(60_000)
    for (let tick = 0; tick < 30; tick++) render({ ...game, due: `00:${tick}`, balls: [...game.balls] })
    expect(runtime.createMask).toHaveBeenCalledOnce()
    expect(mask.reveal).not.toHaveBeenCalled()
    expect(mask.reset).not.toHaveBeenCalled()
    expect(mask.resize).toHaveBeenCalledOnce()
    resizeCallback()
    expect(mask.resize).toHaveBeenCalledTimes(2)
    render({ ...game, latestIssue: '20260830002' })
    expect(runtime.createMask).toHaveBeenCalledTimes(2)
    expect(observer.disconnect).toHaveBeenCalledOnce()
  })

  it('captures primary pointers, erases continuous normalized strokes and cancels/release cleanly', () => {
    render(); const tree = render()
    pointer(tree, 'onPointerDown', event(1, 80, 100, 2))
    expect(mask.scratch).not.toHaveBeenCalled()
    pointer(tree, 'onPointerDown')
    expect(canvas.setPointerCapture).toHaveBeenCalledWith(1)
    expect(mask.scratch).toHaveBeenLastCalledWith({ x: .25, y: .5 }, { x: .25, y: .5 })
    pointer(tree, 'onPointerMove', event(2, 260, 160))
    expect(mask.scratch).toHaveBeenCalledOnce()
    pointer(tree, 'onPointerMove', event(1, 300, 200))
    expect(mask.scratch).toHaveBeenLastCalledWith({ x: .25, y: .5 }, { x: 1, y: 1 })
    pointer(tree, 'onPointerCancel')
    expect(canvas.releasePointerCapture).toHaveBeenCalledWith(1)
    pointer(tree, 'onPointerMove')
    expect(mask.scratch).toHaveBeenCalledTimes(2)
    pointer(tree, 'onPointerDown')
    surface.unmount()
    expect(captured).toBeNull()
  })

  it('reveals through the keyboard button, releases an active stroke and restores the mask on replay', () => {
    render(); let tree = render()
    expect(elements(tree).some((node) => node.props.className === 'scratch-draw-total')).toBe(false)
    pointer(tree, 'onPointerDown')
    elements(tree).find((node) => node.type === 'button' && node.props.children === '全部揭晓')!.props.onClick!()
    tree = render()
    expect(captured).toBeNull()
    expect(elements(tree).some((node) => node.props.className?.includes('is-revealed'))).toBe(true)
    expect(elements(tree).some((node) => node.props.className === 'scratch-draw-total')).toBe(true)
    elements(tree).find((node) => node.type === 'button' && node.props.children === '再刮一次')!.props.onClick!()
    tree = render()
    expect(mask.reset).toHaveBeenCalledOnce()
    expect(elements(tree).some((node) => node.props.className?.includes('is-revealed'))).toBe(false)
    expect(elements(tree).some((node) => node.props.className === 'scratch-draw-total')).toBe(false)
  })

  it('returns a long revealed row to its start before covering it again', () => {
    const longResult = { ...game, balls: Array.from({ length: 20 }, (_, index) => index + 1) }
    render(longResult); let tree = render(longResult)
    elements(tree).find((node) => node.type === 'button' && node.props.children === '全部揭晓')!.props.onClick!()
    tree = render(longResult)
    scrollSurface.scrollLeft = 150
    elements(tree).find((node) => node.type === 'button' && node.props.children === '再刮一次')!.props.onClick!()
    expect(scrollSurface.scrollLeft).toBe(0)
    expect(mask.reset).toHaveBeenCalledOnce()
    tree = render(longResult)
    expect(elements(tree).some((node) => node.props.className?.includes('is-revealed'))).toBe(false)
  })

  it.each(['hk-marksix', 'official-tw-bingo'])('does not invent a sum for an unknown %s game with three drawn numbers', id => {
    const unknown = { ...game, id }
    render(unknown)
    let tree = render(unknown)
    elements(tree).find(node => node.type === 'button' && node.props.children === '全部揭晓')!.props.onClick!()
    tree = render(unknown)
    expect(elements(tree).some(node => node.props.className?.includes('is-revealed'))).toBe(true)
    expect(elements(tree).some(node => node.props.className === 'scratch-draw-total')).toBe(false)
    expect(elements(tree).filter(node => node.props.className === 'scratch-draw-cell')).toHaveLength(3)
  })

  it.each([
    { id: 'speed-ssc', balls: [9, 8, 1, 2, 3], label: '总和', total: 23 },
    { id: 'speed-racing', balls: [5, 6, 1, 2, 3, 4, 7, 8, 9, 10], label: '冠亚和', total: 11 },
    { id: 'pc-canada', balls: [9, 1, 9], label: '和值', total: 19 },
    { id: 'canada-28', balls: [3, 7, 8], label: '和值', total: 18 },
    { id: 'canada-20', balls: [0, 0, 0], label: '和值', total: 0 },
  ])('uses the shared $label definition for $id', ({ id, balls, label, total }) => {
    const known = { ...game, id, balls }
    render(known)
    let tree = render(known)
    elements(tree).find(node => node.type === 'button' && node.props.children === '全部揭晓')!.props.onClick!()
    tree = render(known)
    const result = elements(tree).find(node => node.props.className === 'scratch-draw-total')!
    expect((result.props.children as ReactNode[])[0]).toBe(label)
    expect(elements(result).find(node => node.type === 'b')!.props.children).toBe(total)
  })
})
