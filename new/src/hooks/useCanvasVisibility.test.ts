import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { HookHarness } from '../test/hookHarness'
import { useCanvasVisibility } from './useCanvasVisibility'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null }))
vi.mock('react', async (importOriginal) => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))

describe('canvas visibility observer', () => {
  const ref = { current: {} as HTMLCanvasElement }
  const render = () => {
    const visible = runtime.hooks!.render(() => useCanvasVisibility(ref))
    runtime.hooks!.flushEffects()
    return visible
  }
  beforeEach(() => { runtime.hooks = new HookHarness() })
  afterEach(() => { runtime.hooks?.unmount(); vi.unstubAllGlobals() })

  it('renders normally where IntersectionObserver is unavailable', () => {
    vi.stubGlobal('IntersectionObserver', undefined)
    expect(render()).toBe(true)
  })

  it('falls back to painting if an embedded browser cannot construct its observer', () => {
    vi.stubGlobal('IntersectionObserver', class { constructor() { throw new Error('unsupported observer') } })
    expect(render()).toBe(false)
    expect(render()).toBe(true)
  })

  it('ignores unrelated entries and stale callbacks after cleanup', () => {
    let callback: IntersectionObserverCallback
    const disconnect = vi.fn()
    vi.stubGlobal('IntersectionObserver', class {
      constructor(handler: IntersectionObserverCallback) { callback = handler }
      observe = vi.fn()
      disconnect = disconnect
    })
    expect(render()).toBe(false)
    callback!([{ target: {}, isIntersecting: true } as IntersectionObserverEntry], {} as IntersectionObserver)
    expect(render()).toBe(false)
    callback!([{ target: ref.current, isIntersecting: true } as unknown as IntersectionObserverEntry], {} as IntersectionObserver)
    expect(render()).toBe(true)
    runtime.hooks!.unmount()
    expect(disconnect).toHaveBeenCalledOnce()
    callback!([{ target: ref.current, isIntersecting: false } as unknown as IntersectionObserverEntry], {} as IntersectionObserver)
    expect(render()).toBe(true)
  })
})
