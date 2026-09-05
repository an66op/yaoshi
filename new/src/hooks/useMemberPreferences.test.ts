import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { HookHarness } from '../test/hookHarness'
import { normalizeBetModePreference, useMemberPreferences } from './useMemberPreferences'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))

describe('member betting surface preference migration', () => {
  it.each(['quick', 'dual', 'numbers', '', null, undefined, 'unexpected'])('migrates the legacy or invalid value %j to chat', value => {
    expect(normalizeBetModePreference(value)).toBe('chat')
  })

  it('preserves the two real room surfaces', () => {
    expect(normalizeBetModePreference('chat')).toBe('chat')
    expect(normalizeBetModePreference('detail')).toBe('detail')
  })
})

describe('member preference persistence', () => {
  const values = new Map<string, string>()

  beforeEach(() => {
    runtime.hooks = new HookHarness()
    values.clear()
    const events = new EventTarget()
    vi.stubGlobal('CustomEvent', class<T> extends Event {
      detail: T
      constructor(type: string, init: { detail: T }) { super(type); this.detail = init.detail }
    })
    vi.stubGlobal('window', Object.assign(events, {
      localStorage: {
        getItem: (key: string) => values.get(key) ?? null,
        setItem: (key: string, value: string) => values.set(key, value),
      },
    }))
  })

  afterEach(() => {
    runtime.hooks?.unmount()
    vi.unstubAllGlobals()
  })

  const render = () => {
    const value = runtime.hooks!.render(useMemberPreferences)
    runtime.hooks!.flushEffects()
    return value
  }

  it('rewrites a legacy keyboard-tab value as the chat surface', () => {
    values.set('seven-star-bet-mode', JSON.stringify('numbers'))
    expect(render().defaultBetMode).toBe('chat')
    render()
    expect(JSON.parse(values.get('seven-star-bet-mode')!)).toBe('chat')
  })

  it('persists the selected detail surface', () => {
    const preferences = render()
    preferences.setDefaultBetMode('detail')
    expect(render().defaultBetMode).toBe('detail')
    expect(JSON.parse(values.get('seven-star-bet-mode')!)).toBe('detail')
  })
})
