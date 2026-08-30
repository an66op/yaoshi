import { isValidElement, type ComponentProps, type ReactElement, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { HookHarness } from '../test/hookHarness'
import { Login } from './Login'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null, login: vi.fn(), loadTestLogin: vi.fn() }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))
vi.mock('../api/member', () => ({ memberApi: { login: runtime.login } }))
vi.mock('../utils/testLogin', () => ({ loadTestLogin: runtime.loadTestLogin }))

type FormElement = ReactElement<{
  children?: ReactNode
  className?: string
  autoComplete?: string
  value?: string
  disabled?: boolean
  onChange?: (event: { target: { value: string } }) => void
  onKeyDown?: (event: { key: string }) => void
  onClick?: () => void
}>

function find(node: ReactNode, match: (node: FormElement) => boolean): FormElement | undefined {
  if (Array.isArray(node)) {
    for (const child of node) {
      const result = find(child, match)
      if (result) return result
    }
  }
  if (!isValidElement<FormElement['props']>(node)) return
  return match(node) ? node : find(node.props.children, match)
}

describe('sign-in during startup verification', () => {
  const render = (props: ComponentProps<typeof Login>) => {
    const result = runtime.hooks!.render(() => Login(props))
    runtime.hooks!.flushEffects()
    return result
  }
  beforeEach(() => {
    runtime.hooks = new HookHarness()
    runtime.login.mockReset().mockResolvedValue({ user: { username: 'testmember', nickname: '会员' } })
    runtime.loadTestLogin.mockReset().mockResolvedValue(undefined)
    vi.stubEnv('DEV', false)
  })
  afterEach(() => { runtime.hooks?.unmount(); vi.unstubAllEnvs() })

  it('fills enabled runtime credentials without logging in automatically', async () => {
    runtime.loadTestLogin.mockResolvedValue({ username: 'runtime-member', password: 'runtime-password' })
    const props = { onContinue: vi.fn() }
    render(props)
    await Promise.resolve()
    const filled = render(props)
    expect(find(filled, node => node.props.autoComplete === 'username')!.props.value).toBe('runtime-member')
    expect(find(filled, node => node.props.autoComplete === 'current-password')!.props.value).toBe('runtime-password')
    expect(find(filled, node => node.props.className === 'login-test-notice')).toBeDefined()
    expect(runtime.login).not.toHaveBeenCalled()
  })

  it('does not overwrite a form edited before a delayed runtime response', async () => {
    let resolve!: (value: unknown) => void
    runtime.loadTestLogin.mockImplementation(() => new Promise(done => { resolve = done }))
    const props = { onContinue: vi.fn() }
    const initial = render(props)
    find(initial, node => node.props.autoComplete === 'username')!.props.onChange!({ target: { value: 'my-account' } })
    resolve({ username: 'runtime-member', password: 'runtime-password' })
    await Promise.resolve()
    const next = render(props)
    expect(find(next, node => node.props.autoComplete === 'username')!.props.value).toBe('my-account')
    expect(find(next, node => node.props.autoComplete === 'current-password')!.props.value).toBe('')
    expect(find(next, node => node.props.className === 'login-test-notice')).toBeUndefined()
  })

  it('cancels runtime loading when leaving and ignores a late result', async () => {
    let resolve!: (value: unknown) => void
    runtime.loadTestLogin.mockImplementation(() => new Promise(done => { resolve = done }))
    const props = { onContinue: vi.fn() }
    render(props)
    const signal = runtime.loadTestLogin.mock.calls[0][0] as AbortSignal
    runtime.hooks!.unmount()
    expect(signal.aborted).toBe(true)
    resolve({ username: 'runtime-member', password: 'runtime-password' })
    await Promise.resolve()
    expect(find(render(props), node => node.props.autoComplete === 'username')!.props.value).toBe('')
  })

  it('retains development presets without requesting public test configuration', () => {
    vi.stubEnv('DEV', true)
    const result = render({ onContinue: vi.fn() })
    expect(find(result, node => node.props.autoComplete === 'username')!.props.value).toBe('wangzhe88')
    expect(runtime.loadTestLogin).not.toHaveBeenCalled()
  })

  it('paints the requested theme and permits typing without starting a competing login', () => {
    const onContinue = vi.fn()
    const props = { onContinue, theme: 'day' as const, verificationPending: true }
    const initial = render(props)
    expect(initial.props.className).toBe('login-page theme-day')
    const username = find(initial, node => node.props.autoComplete === 'username')!
    username.props.onChange!({ target: { value: 'testmember' } })
    find(initial, node => node.props.autoComplete === 'current-password')!.props.onChange!({ target: { value: 'valid-password' } })
    const checking = render(props)
    const button = find(checking, node => node.props.className === 'login-primary')!
    expect(button.props.disabled).toBe(true)
    button.props.onClick!()
    find(checking, node => node.props.autoComplete === 'username')!.props.onKeyDown!({ key: 'Enter' })
    expect(runtime.login).not.toHaveBeenCalled()
    expect(onContinue).not.toHaveBeenCalled()
  })

  it('retains typed input after verification and enables the normal authenticated login request', async () => {
    const onContinue = vi.fn()
    const props = { onContinue, theme: 'night' as const, verificationPending: true }
    const initial = render(props)
    find(initial, node => node.props.autoComplete === 'username')!.props.onChange!({ target: { value: 'testmember' } })
    find(initial, node => node.props.autoComplete === 'current-password')!.props.onChange!({ target: { value: 'valid-password' } })
    const ready = render({ ...props, verificationPending: false })
    expect(ready.props.className).toBe('login-page theme-night')
    expect(find(ready, node => node.props.autoComplete === 'username')!.props.value).toBe('testmember')
    const button = find(ready, node => node.props.className === 'login-primary')!
    expect(button.props.disabled).toBe(false)
    button.props.onClick!()
    await Promise.resolve()
    expect(runtime.login).toHaveBeenCalledWith('testmember', 'valid-password')
    expect(onContinue).toHaveBeenCalledWith('testmember', '会员')
  })
})
