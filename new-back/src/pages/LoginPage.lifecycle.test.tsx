import { isValidElement, type ReactElement, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { LoginPage } from './LoginPage'

// Small component state/effect driver for Node-only tests. MUI child components
// remain opaque React elements; these tests do not simulate a browser renderer.
class FormHarness {
  private slots: Array<{ value?: unknown; effect?: () => void | (() => void); cleanup?: void | (() => void) }> = []
  private cursor = 0
  private effects: number[] = []
  render<T>(factory: () => T): T { this.cursor = 0; return factory() }
  useState<T>(initial: T | (() => T)): [T, (value: T) => void] {
    const slot = this.slots[this.cursor++] ??= { value: typeof initial === 'function' ? (initial as () => T)() : initial }
    return [slot.value as T, value => { slot.value = value }]
  }
  useRef<T>(initial: T) {
    const slot = this.slots[this.cursor++] ??= { value: { current: initial } }
    return slot.value as { current: T }
  }
  useEffect(effect: () => void | (() => void)) {
    const index = this.cursor++
    if (!this.slots[index]) { this.slots[index] = { effect }; this.effects.push(index) }
  }
  flushEffects() { for (const index of this.effects.splice(0)) this.slots[index].cleanup = this.slots[index].effect?.() }
  unmount() { for (const slot of this.slots) slot.cleanup?.() }
}

const runtime = vi.hoisted(() => ({ hooks: null as FormHarness | null, loadTestLogin: vi.fn(), login: vi.fn() }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useEffect: (effect: () => void | (() => void)) => runtime.hooks!.useEffect(effect),
}))
vi.mock('../utils/testLogin', () => ({ loadTestLogin: runtime.loadTestLogin }))
vi.mock('../api', () => ({ adminApi: { login: runtime.login } }))

type FormElement = ReactElement<{
  children?: ReactNode
  autoComplete?: string
  'aria-label'?: string
  value?: string
  severity?: string
  onChange?: (event: { target: { value: string } }, next?: string) => void
}>

function find(node: ReactNode, match: (node: FormElement) => boolean): FormElement | undefined {
  if (Array.isArray(node)) {
    for (const child of node) { const result = find(child, match); if (result) return result }
  }
  if (!isValidElement<FormElement['props']>(node)) return
  return match(node) ? node : find(node.props.children, match)
}
const presets = {
  platform: { username: 'test-admin', password: 'admin-password', workspace: '平台' },
  tenant: { username: 'test-tenant', password: 'tenant-password', workspace: '平台' },
  agent: { username: 'test-agent', password: 'agent-password', workspace: 'test-tenant' },
}
const props = { onSuccess: vi.fn() }
const render = () => {
  const result = runtime.hooks!.render(() => LoginPage(props))
  runtime.hooks!.flushEffects()
  return result
}
const field = (node: ReactNode, name: string) => find(node, element => element.props.autoComplete === name)!
const choose = (node: ReactNode, identity: string) => find(node, element => element.props['aria-label'] === '选择登录身份')!.props.onChange!({ target: { value: '' } }, identity)

describe('management runtime test login lifecycle', () => {
  beforeEach(() => {
    runtime.hooks = new FormHarness()
    runtime.loadTestLogin.mockReset().mockResolvedValue(undefined)
    runtime.login.mockReset()
    vi.stubEnv('DEV', false)
  })
  afterEach(() => { runtime.hooks!.unmount(); vi.unstubAllEnvs() })

  it('keeps ordinary production sign-in empty when test configuration is unavailable', async () => {
    render()
    await Promise.resolve()
    const ready = render()
    expect(field(ready, 'username').props.value).toBe('')
    expect(field(ready, 'current-password').props.value).toBe('')
    expect(find(ready, node => node.props.severity === 'info')).toBeUndefined()
  })
  it('fills all roles independently and never automatically logs in', async () => {
    runtime.loadTestLogin.mockResolvedValue(presets)
    render()
    await Promise.resolve()
    let ready = render()
    for (const identity of ['platform', 'tenant', 'agent'] as const) {
      choose(ready, identity)
      ready = render()
      expect(field(ready, 'username').props.value).toBe(presets[identity].username)
      expect(field(ready, 'current-password').props.value).toBe(presets[identity].password)
      expect(field(ready, 'organization').props.value).toBe(presets[identity].workspace)
      expect(find(ready, node => node.props.severity === 'info')).toBeDefined()
    }
    expect(runtime.login).not.toHaveBeenCalled()
  })
  it('applies a late response to the selected role, not the initial administrator', async () => {
    let resolve!: (value: unknown) => void
    runtime.loadTestLogin.mockImplementation(() => new Promise(done => { resolve = done }))
    choose(render(), 'agent')
    resolve(presets)
    await Promise.resolve()
    const ready = render()
    expect(field(ready, 'username').props.value).toBe('test-agent')
    expect(field(ready, 'organization').props.value).toBe('test-tenant')
  })
  it.each(['username', 'current-password', 'organization'])('preserves manual edits to %s during a pending response', async name => {
    let resolve!: (value: unknown) => void
    runtime.loadTestLogin.mockImplementation(() => new Promise(done => { resolve = done }))
    const initial = render()
    field(initial, name).props.onChange!({ target: { value: 'my-manual-value' } })
    resolve(presets)
    await Promise.resolve()
    let ready = render()
    expect(field(ready, name).props.value).toBe('my-manual-value')
    expect(find(ready, node => node.props.severity === 'info')).toBeUndefined()
    // An explicit subsequent role switch is allowed to select its test profile.
    choose(ready, 'tenant')
    ready = render()
    expect(field(ready, 'username').props.value).toBe('test-tenant')
  })
  it('clears credentials when changing to a role without an enabled test profile', async () => {
    runtime.loadTestLogin.mockResolvedValue({ platform: presets.platform })
    render()
    await Promise.resolve()
    choose(render(), 'agent')
    const ready = render()
    expect(field(ready, 'username').props.value).toBe('')
    expect(field(ready, 'current-password').props.value).toBe('')
    expect(find(ready, node => node.props.severity === 'info')).toBeUndefined()
  })
  it('aborts on unmount and ignores late credentials', async () => {
    let resolve!: (value: unknown) => void
    runtime.loadTestLogin.mockImplementation(() => new Promise(done => { resolve = done }))
    render()
    const signal = runtime.loadTestLogin.mock.calls[0][0] as AbortSignal
    runtime.hooks!.unmount()
    resolve(presets)
    await Promise.resolve()
    expect(signal.aborted).toBe(true)
    expect(field(render(), 'username').props.value).toBe('')
  })
  it('keeps local development accounts independent of runtime files', () => {
    vi.stubEnv('DEV', true)
    const initial = render()
    expect(field(initial, 'username').props.value).toBe('admin')
    expect(runtime.loadTestLogin).not.toHaveBeenCalled()
  })
})
