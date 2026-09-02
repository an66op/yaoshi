import { isValidElement, type DependencyList, type ReactElement, type ReactNode, type SetStateAction } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { LoginPage } from './LoginPage'

// Small component state/effect driver for Node-only tests. MUI child components
// remain opaque React elements; these tests do not simulate a browser renderer.
class FormHarness {
  private slots: Array<{ value?: unknown; deps?: DependencyList; effect?: () => void | (() => void); cleanup?: void | (() => void) }> = []
  private cursor = 0
  private effects: number[] = []
  render<T>(factory: () => T): T { this.cursor = 0; return factory() }
  useState<T>(initial: T | (() => T)): [T, (value: SetStateAction<T>) => void] {
    const slot = this.slots[this.cursor++] ??= { value: typeof initial === 'function' ? (initial as () => T)() : initial }
    return [slot.value as T, value => { slot.value = typeof value === 'function' ? (value as (previous: T) => T)(slot.value as T) : value }]
  }
  useRef<T>(initial: T) {
    const slot = this.slots[this.cursor++] ??= { value: { current: initial } }
    return slot.value as { current: T }
  }
  useCallback<T>(callback: T, deps: DependencyList): T {
    const index = this.cursor++
    const previous = this.slots[index]
    if (!previous || !this.sameDeps(previous.deps, deps)) this.slots[index] = { value: callback, deps }
    return this.slots[index].value as T
  }
  private sameDeps(left?: DependencyList, right?: DependencyList) {
    return Boolean(left && right && left.length === right.length && left.every((value, index) => Object.is(value, right[index])))
  }
  useEffect(effect: () => void | (() => void), deps?: DependencyList) {
    const index = this.cursor++
    const previous = this.slots[index]
    if (!previous || !this.sameDeps(previous.deps, deps)) {
      previous?.cleanup?.()
      this.slots[index] = { effect, deps }; this.effects.push(index)
    }
  }
  flushEffects() { for (const index of this.effects.splice(0)) this.slots[index].cleanup = this.slots[index].effect?.() }
  unmount() { for (const slot of this.slots) slot.cleanup?.() }
}

const runtime = vi.hoisted(() => ({ hooks: null as FormHarness | null, loadTestLogin: vi.fn(), login: vi.fn(), loginCaptcha: vi.fn() }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useCallback: <T,>(callback: T, deps: DependencyList) => runtime.hooks!.useCallback(callback, deps),
  useEffect: (effect: () => void | (() => void), deps?: DependencyList) => runtime.hooks!.useEffect(effect, deps),
}))
vi.mock('../utils/testLogin', () => ({ loadTestLogin: runtime.loadTestLogin }))
vi.mock('../api', () => ({ adminApi: { login: runtime.login, loginCaptcha: runtime.loginCaptcha } }))
vi.mock('../auth', () => ({ clearLegacyAdminSession: vi.fn() }))

type FormElement = ReactElement<{
  children?: ReactNode
  autoComplete?: string
  'aria-label'?: string
  value?: string
  label?: string
  type?: string
  disabled?: boolean
  src?: string
  alt?: string
  draggable?: boolean
  id?: string
  slotProps?: { htmlInput?: { inputMode?: string; maxLength?: number; pattern?: string; 'aria-describedby'?: string } }
  severity?: string
  component?: string
  onClick?: () => void
  onLoad?: () => void
  onError?: () => void
  onSubmit?: (event: { preventDefault: () => void }) => void
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
const captchaField = (node: ReactNode) => find(node, element => element.props.label === '验证码')!
const captchaImage = (node: ReactNode) => find(node, element => element.props.component === 'img')
const refreshButton = (node: ReactNode) => find(node, element => element.props['aria-label'] === '更换登录验证码')!
const submitButton = (node: ReactNode) => find(node, element => element.props.type === 'submit')!
const submitForm = (node: ReactNode) => find(node, element => element.props.component === 'form')!.props.onSubmit!({ preventDefault: vi.fn() })
const challenge = (id = 'captcha-id') => ({ id, image: `data:image/png;base64,aW1hZ2U=`, expires_in: 120 })
const settle = async () => { for (let index = 0; index < 6; index++) await Promise.resolve(); return render() }
const loadImage = async () => { const ready = await settle(); captchaImage(ready)!.props.onLoad!(); return render() }
const answer = (node: ReactNode, code = '123456') => { captchaField(node).props.onChange!({ target: { value: code } }); return render() }
const credentials = (node: ReactNode) => {
  field(node, 'username').props.onChange!({ target: { value: 'test-account' } })
  field(node, 'current-password').props.onChange!({ target: { value: 'test-password' } })
  return render()
}
function contents(node: ReactNode): string {
  if (Array.isArray(node)) return node.map(contents).join('')
  if (isValidElement<FormElement['props']>(node)) return contents(node.props.children)
  return typeof node === 'string' || typeof node === 'number' ? String(node) : ''
}

beforeEach(() => {
  runtime.hooks = new FormHarness()
  runtime.loadTestLogin.mockReset().mockResolvedValue(undefined)
  runtime.login.mockReset()
  runtime.loginCaptcha.mockReset().mockResolvedValue(challenge())
  props.onSuccess.mockClear()
  vi.stubEnv('DEV', false)
  vi.useFakeTimers()
})
afterEach(() => { runtime.hooks!.unmount(); vi.clearAllTimers(); vi.useRealTimers(); vi.unstubAllEnvs() })

describe('management runtime test login lifecycle', () => {
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
      expect(field(ready, 'organization')).toBeUndefined()
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
    expect(field(ready, 'organization')).toBeUndefined()
  })
  it.each(['username', 'current-password'])('preserves manual edits to %s during a pending response', async name => {
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

  it.each(['platform', 'tenant', 'agent'] as const)('authenticates %s with credentials and a manual captcha, and trusts the server role', async identity => {
    runtime.loadTestLogin.mockResolvedValue(presets)
    const user = { id: 22, role: 'agent', username: 'real-account' }
    runtime.login.mockResolvedValue({ user })
    render(); await Promise.resolve()
    choose(render(), identity)
    const ready = answer(await loadImage())
    submitForm(ready)
    await Promise.resolve(); await Promise.resolve()
    expect(runtime.login).toHaveBeenCalledWith(presets[identity].username, presets[identity].password, { captcha_id: 'captcha-id', captcha_code: '123456' })
    expect(runtime.login.mock.calls[0]).toHaveLength(3)
    expect(props.onSuccess).toHaveBeenCalledWith(user)
    expect(field(ready, 'organization')).toBeUndefined()
  })
})

describe('management login captcha lifecycle', () => {
  it.each([true, false])('requires a manually entered captcha in DEV=%s, even with filled test credentials', async dev => {
    vi.stubEnv('DEV', dev)
    runtime.loadTestLogin.mockResolvedValue(presets)
    const initial = render()
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(1)
    expect(captchaField(initial).props.value).toBe('')
    expect(captchaField(initial).props.disabled).toBe(true)
    const ready = await loadImage()
    expect(captchaField(ready).props.value).toBe('')
    expect(submitButton(ready).props.disabled).toBe(true)
    submitForm(ready)
    expect(runtime.login).not.toHaveBeenCalled()
    expect(contents(render())).toContain('请输入图中6位数字验证码')
  })

  it('does not enable captcha submission until its PNG image actually loads', async () => {
    credentials(render())
    const pendingImage = await settle()
    expect(captchaField(pendingImage).props.disabled).toBe(true)
    expect(submitButton(pendingImage).props.disabled).toBe(true)
    submitForm(pendingImage)
    expect(runtime.login).not.toHaveBeenCalled()
    expect(captchaImage(pendingImage)?.props).toMatchObject({ src: challenge().image, alt: '登录验证码', draggable: false })
    captchaImage(pendingImage)!.props.onLoad!()
    const ready = render()
    expect(captchaField(ready).props.disabled).toBe(false)
    expect(captchaField(ready).props).toMatchObject({ autoComplete: 'off', slotProps: { htmlInput: { inputMode: 'numeric', maxLength: 6, pattern: '[0-9]{6}' } } })
    expect(submitButton(answer(ready)).props.disabled).toBe(false)
  })

  it('keeps only six digits and refuses incomplete answers without consuming the challenge', async () => {
    credentials(render())
    let ready = answer(await loadImage(), 'a12 b34')
    expect(captchaField(ready).props.value).toBe('1234')
    submitForm(ready)
    expect(runtime.login).not.toHaveBeenCalled()
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(1)
    ready = answer(render(), 'ab123456789')
    expect(captchaField(ready).props.value).toBe('123456')
    expect(submitButton(ready).props.disabled).toBe(false)
  })

  it.each(['账号或密码错误', '请求超时，请稍后重试'])('refreshes a consumed challenge after %s while preserving credentials', async failure => {
    runtime.login.mockRejectedValueOnce(new Error(failure))
    runtime.loginCaptcha.mockResolvedValueOnce(challenge('first')).mockResolvedValueOnce(challenge('second'))
    credentials(render())
    const ready = answer(await loadImage())
    submitForm(ready)
    const rejected = await settle()
    expect(runtime.login).toHaveBeenCalledWith('test-account', 'test-password', { captcha_id: 'first', captcha_code: '123456' })
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(2)
    expect(field(rejected, 'username').props.value).toBe('test-account')
    expect(field(rejected, 'current-password').props.value).toBe('test-password')
    expect(captchaField(rejected).props.value).toBe('')
    expect(submitButton(rejected).props.disabled).toBe(true)
    expect(contents(rejected)).toContain(failure)
    expect(props.onSuccess).not.toHaveBeenCalled()
    runtime.login.mockResolvedValueOnce({ user: { id: 7, role: 'tenant', username: 'real-account' } })
    captchaImage(rejected)!.props.onLoad!()
    submitForm(answer(render(), '654321'))
    await settle()
    expect(runtime.login).toHaveBeenLastCalledWith('test-account', 'test-password', { captcha_id: 'second', captcha_code: '654321' })
    expect(props.onSuccess).toHaveBeenCalledTimes(1)
  })

  it('releases the login lock when a post-failure captcha refresh stalls', async () => {
    runtime.login.mockRejectedValueOnce(new Error('账号或密码错误'))
    runtime.loginCaptcha.mockResolvedValueOnce(challenge('first'))
      .mockImplementationOnce(() => new Promise(() => {}))
      .mockResolvedValueOnce(challenge('retry'))
    credentials(render())
    submitForm(answer(await loadImage()))
    await settle()
    await vi.advanceTimersByTimeAsync(15_000)
    const failed = render()
    expect(contents(failed)).toContain('验证码加载超时')
    expect(refreshButton(failed).props.disabled).toBe(false)
    expect(field(failed, 'username').props.value).toBe('test-account')
    refreshButton(failed).props.onClick!()
    expect(submitButton(answer(await loadImage())).props.disabled).toBe(false)
  })

  it('shows a recoverable loading error without automatically retrying in a loop', async () => {
    runtime.loginCaptcha.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(challenge('retried'))
    credentials(render())
    const failed = await settle()
    expect(contents(failed)).toContain('验证码加载失败')
    expect(captchaImage(failed)).toBeUndefined()
    expect(submitButton(failed).props.disabled).toBe(true)
    await vi.advanceTimersByTimeAsync(300_000)
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(1)
    refreshButton(render()).props.onClick!()
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(2)
    expect(submitButton(answer(await loadImage())).props.disabled).toBe(false)
  })

  it('keeps image decoding failures blocked and offers an explicit retry', async () => {
    credentials(render())
    const failed = await settle()
    captchaImage(failed)!.props.onError!()
    const ready = render()
    expect(contents(ready)).toContain('验证码图片无法显示')
    expect(submitButton(ready).props.disabled).toBe(true)
    expect(captchaField(ready).props.value).toBe('')
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(1)
  })

  it('times out a stalled request after 15 seconds and ignores its late response after manual retry', async () => {
    let resolve!: (value: unknown) => void
    runtime.loginCaptcha.mockImplementationOnce(() => new Promise(done => { resolve = done })).mockResolvedValueOnce(challenge('retry-after-timeout'))
    credentials(render())
    const timedOutSignal = runtime.loginCaptcha.mock.calls[0][0] as AbortSignal
    await vi.advanceTimersByTimeAsync(15_000)
    const timedOut = render()
    expect(timedOutSignal.aborted).toBe(true)
    expect(contents(timedOut)).toContain('验证码加载超时，请点击换一张重试')
    expect(captchaImage(timedOut)).toBeUndefined()
    expect(submitButton(timedOut).props.disabled).toBe(true)
    expect(refreshButton(timedOut).props.disabled).toBe(false)
    await vi.advanceTimersByTimeAsync(60_000)
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(1)
    refreshButton(render()).props.onClick!()
    await loadImage()
    resolve(challenge('late-timed-out-response'))
    await settle()
    runtime.login.mockResolvedValueOnce({ user: { id: 9, role: 'admin', username: 'test-account' } })
    submitForm(answer(render()))
    await settle()
    expect(runtime.login).toHaveBeenCalledWith('test-account', 'test-password', { captcha_id: 'retry-after-timeout', captcha_code: '123456' })
  })

  it('expires after two minutes, clears the answer, and requires a new image', async () => {
    credentials(render())
    answer(await loadImage())
    await vi.advanceTimersByTimeAsync(120_000)
    const expired = render()
    expect(contents(expired)).toContain('验证码已过期，请换一张')
    expect(captchaField(expired).props.value).toBe('')
    expect(submitButton(expired).props.disabled).toBe(true)
    submitForm(expired)
    expect(runtime.login).not.toHaveBeenCalled()
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(1)
    refreshButton(render()).props.onClick!()
    const refreshed = answer(await loadImage())
    expect(submitButton(refreshed).props.disabled).toBe(false)
  })

  it('also rejects an expired challenge when background timers have not fired', async () => {
    credentials(render())
    const ready = answer(await loadImage())
    vi.setSystemTime(Date.now() + 120_001)
    submitForm(ready)
    expect(runtime.login).not.toHaveBeenCalled()
    expect(contents(render())).toContain('验证码已过期，请换一张')
  })

  it('does not extend validity by the time spent waiting for a slow challenge response', async () => {
    let resolve!: (value: unknown) => void
    runtime.loginCaptcha.mockImplementationOnce(() => new Promise(done => { resolve = done }))
    credentials(render())
    vi.setSystemTime(Date.now() + 121_000)
    resolve(challenge())
    const expired = await settle()
    expect(contents(expired)).toContain('验证码已过期，请换一张')
    captchaImage(expired)!.props.onLoad!()
    expect(submitButton(render()).props.disabled).toBe(true)
  })

  it('aborts superseded refreshes and ignores both old network replies and old image events', async () => {
    let firstResolve!: (value: unknown) => void
    let secondResolve!: (value: unknown) => void
    runtime.loginCaptcha.mockImplementationOnce(() => new Promise(done => { firstResolve = done }))
      .mockImplementationOnce(() => new Promise(done => { secondResolve = done }))
    const initial = credentials(render())
    const firstSignal = runtime.loginCaptcha.mock.calls[0][0] as AbortSignal
    refreshButton(initial).props.onClick!()
    expect(firstSignal.aborted).toBe(true)
    secondResolve(challenge('second'))
    const second = await loadImage()
    const oldImage = captchaImage(second)!
    firstResolve(challenge('obsolete'))
    await settle()
    const user = { username: 'server-account', role: 'agent' }
    runtime.login.mockResolvedValue({ user })
    runtime.loginCaptcha.mockResolvedValueOnce(challenge('third'))
    refreshButton(render()).props.onClick!()
    await loadImage()
    oldImage.props.onError!()
    const latest = answer(render())
    expect(submitButton(latest).props.disabled).toBe(false)
    submitForm(latest)
    await settle()
    expect(runtime.login).toHaveBeenCalledWith('test-account', 'test-password', { captcha_id: 'third', captcha_code: '123456' })
  })

  it('guards same-frame duplicate submission and blocks manual refresh and identity changes while submitting', async () => {
    let resolve!: (value: unknown) => void
    runtime.login.mockImplementationOnce(() => new Promise(done => { resolve = done }))
    credentials(render())
    const ready = answer(await loadImage())
    submitForm(ready)
    submitForm(ready)
    refreshButton(ready).props.onClick!()
    choose(ready, 'agent')
    expect(runtime.login).toHaveBeenCalledTimes(1)
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(1)
    expect(captchaField(render()).props.value).toBe('')
    expect(field(render(), 'username').props.value).toBe('test-account')
    expect(refreshButton(render()).props.disabled).toBe(true)
    resolve({ user: { id: 3, username: 'test-account', role: 'admin' } })
    await settle()
    expect(props.onSuccess).toHaveBeenCalledTimes(1)
  })

  it('aborts on unmount and ignores a late challenge without scheduling expiry work', async () => {
    let resolve!: (value: unknown) => void
    runtime.loginCaptcha.mockImplementationOnce(() => new Promise(done => { resolve = done }))
    render()
    const signal = runtime.loginCaptcha.mock.calls[0][0] as AbortSignal
    runtime.hooks!.unmount()
    expect(signal.aborted).toBe(true)
    resolve(challenge('late'))
    const discarded = await settle()
    expect(captchaImage(discarded)).toBeUndefined()
    expect(vi.getTimerCount()).toBe(0)
    expect(props.onSuccess).not.toHaveBeenCalled()
  })

  it('does not apply a login reply to an unmounted form', async () => {
    let resolve!: (value: unknown) => void
    runtime.login.mockImplementationOnce(() => new Promise(done => { resolve = done }))
    credentials(render())
    submitForm(answer(await loadImage()))
    runtime.hooks!.unmount()
    resolve({ user: { username: 'late-account', role: 'admin' } })
    await settle()
    expect(props.onSuccess).not.toHaveBeenCalled()
    expect(vi.getTimerCount()).toBe(0)
  })

  it.each([
    { ...challenge(), id: '' },
    { ...challenge(), image: 'https://untrusted.example.test/image.png' },
    { ...challenge(), expires_in: 0 },
  ])('rejects unusable challenge payloads instead of enabling login', async invalid => {
    runtime.loginCaptcha.mockResolvedValueOnce(invalid)
    credentials(render())
    const rejected = await settle()
    expect(contents(rejected)).toContain('验证码加载失败')
    expect(captchaImage(rejected)).toBeUndefined()
    expect(submitButton(rejected).props.disabled).toBe(true)
    expect(runtime.login).not.toHaveBeenCalled()
  })
})
