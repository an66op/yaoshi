import { isValidElement, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { HookHarness } from '../test/hookHarness'
import { SessionCheckNotice } from './SessionStartup'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))

function visibleText(node: ReactNode): string {
  if (typeof node === 'string') return node
  if (Array.isArray(node)) return node.map(visibleText).join('')
  return isValidElement<{ children?: ReactNode }>(node) ? visibleText(node.props.children) : ''
}

describe('slow session verification feedback', () => {
  const render = () => {
    const result = runtime.hooks!.render(() => SessionCheckNotice({}))
    runtime.hooks!.flushEffects()
    return visibleText(result)
  }
  beforeEach(() => {
    vi.useFakeTimers()
    runtime.hooks = new HookHarness()
    vi.stubGlobal('window', { setTimeout, clearTimeout })
  })
  afterEach(() => {
    runtime.hooks?.unmount()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('explains a slower connection without polling or faking successful login', () => {
    expect(render()).toBe('正在确认登录状态…')
    vi.advanceTimersByTime(3999)
    expect(render()).toBe('正在确认登录状态…')
    vi.advanceTimersByTime(1)
    expect(render()).toBe('连接较慢，请稍候…')
    expect(vi.getTimerCount()).toBe(0)
  })

  it('removes the delayed update after the check finishes', () => {
    render()
    expect(vi.getTimerCount()).toBe(1)
    runtime.hooks!.unmount()
    expect(vi.getTimerCount()).toBe(0)
  })
})
