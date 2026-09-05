import { isValidElement, type ComponentProps, type ReactElement, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { HookHarness } from '../test/hookHarness'
import { Login } from './Login'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null, login: vi.fn(), loginCaptcha: vi.fn(), loadTestLogin: vi.fn() }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))
vi.mock('../api/member', () => ({ memberApi: { login: runtime.login, loginCaptcha: runtime.loginCaptcha } }))
vi.mock('../utils/testLogin', () => ({ loadTestLogin: runtime.loadTestLogin }))

type FormElement = ReactElement<{
  children?: ReactNode
  className?: string
  autoComplete?: string
  value?: string
  disabled?: boolean
  id?: string
  alt?: string
  src?: string
  inputMode?: string
  maxLength?: number
  'aria-labelledby'?: string
  'aria-label'?: string
  onChange?: (event: { target: { value: string } }) => void
  onKeyDown?: (event: { key: string }) => void
  onClick?: () => void
  onLoad?: () => void
  onError?: () => void
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

function text(node: ReactNode): string {
  if (typeof node === 'string') return node
  if (Array.isArray(node)) return node.map(text).join('')
  return isValidElement<{ children?: ReactNode }>(node) ? text(node.props.children) : ''
}

const captchaFixture = (id = 'challenge-1') => ({ id, image: `data:image/png;base64,${Buffer.from(id).toString('base64')}`, expires_in: 120 })
const captchaInput = (tree: ReactNode) => find(tree, node => node.props['aria-labelledby'] === 'login-captcha-label')!
const captchaImage = (tree: ReactNode) => find(tree, node => node.props.alt === '登录图片验证码')!
const refreshButton = (tree: ReactNode) => find(tree, node => node.props.className?.startsWith('login-captcha-image') === true)!
const submitButton = (tree: ReactNode) => find(tree, node => node.props.className === 'login-primary')!
const settle = async () => { await Promise.resolve(); await Promise.resolve() }

describe('sign-in during startup verification', () => {
  const render = (props: ComponentProps<typeof Login>) => {
    const result = runtime.hooks!.render(() => Login(props))
    runtime.hooks!.flushEffects()
    return result
  }
  const fillCredentials = (tree: ReactNode) => {
    find(tree, node => node.props.autoComplete === 'username')!.props.onChange!({ target: { value: 'testmember' } })
    find(tree, node => node.props.autoComplete === 'current-password')!.props.onChange!({ target: { value: 'valid-password' } })
  }
  const completeCaptcha = async (props: ComponentProps<typeof Login>) => {
    await settle()
    const imageLoaded = render(props)
    captchaImage(imageLoaded).props.onLoad!()
    captchaInput(imageLoaded).props.onChange!({ target: { value: '1234' } })
    return render(props)
  }
  beforeEach(() => {
    vi.useFakeTimers()
    runtime.hooks = new HookHarness()
    runtime.login.mockReset().mockResolvedValue({ user: { username: 'testmember', nickname: '会员' } })
    runtime.loginCaptcha.mockReset().mockResolvedValue(captchaFixture())
    runtime.loadTestLogin.mockReset().mockResolvedValue(undefined)
    vi.stubEnv('DEV', false)
  })
  afterEach(() => { runtime.hooks?.unmount(); vi.unstubAllEnvs(); vi.useRealTimers() })

  it('fills enabled runtime credentials without logging in automatically', async () => {
    runtime.loadTestLogin.mockResolvedValue({ username: 'runtime-member', password: 'runtime-password' })
    const props = { onContinue: vi.fn() }
    render(props)
    await Promise.resolve()
    const filled = render(props)
    expect(find(filled, node => node.props.autoComplete === 'username')!.props.value).toBe('runtime-member')
    expect(find(filled, node => node.props.autoComplete === 'current-password')!.props.value).toBe('runtime-password')
    expect(find(filled, node => node.props.className === 'login-test-notice')).toBeDefined()
    expect(captchaInput(filled).props.value).toBe('')
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
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(1)
    expect(captchaInput(result).props.value).toBe('')
  })

  it('paints the requested theme and permits typing without starting a competing login', async () => {
    const onContinue = vi.fn()
    const props = { onContinue, theme: 'day' as const, verificationPending: true }
    const initial = render(props)
    expect(initial.props.className).toBe('login-page theme-day')
    const username = find(initial, node => node.props.autoComplete === 'username')!
    username.props.onChange!({ target: { value: 'testmember' } })
    find(initial, node => node.props.autoComplete === 'current-password')!.props.onChange!({ target: { value: 'valid-password' } })
    const checking = await completeCaptcha(props)
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
    const ready = await completeCaptcha({ ...props, verificationPending: false })
    expect(ready.props.className).toBe('login-page theme-night')
    expect(find(ready, node => node.props.autoComplete === 'username')!.props.value).toBe('testmember')
    const button = find(ready, node => node.props.className === 'login-primary')!
    expect(button.props.disabled).toBe(false)
    button.props.onClick!()
    await Promise.resolve()
    expect(runtime.login).toHaveBeenCalledWith('testmember', 'valid-password', { captcha_id: 'challenge-1', captcha_code: '1234' })
    expect(onContinue).toHaveBeenCalledWith('testmember', '会员')
  })

  it('blocks button and keyboard submissions until both the challenge and image have loaded', async () => {
    let resolve!: (value: ReturnType<typeof captchaFixture>) => void
    runtime.loginCaptcha.mockImplementation(() => new Promise(done => { resolve = done }))
    const props = { onContinue: vi.fn() }
    fillCredentials(render(props))
    captchaInput(render(props)).props.onChange!({ target: { value: '1234' } })
    const pending = render(props)
    expect(submitButton(pending).props.disabled).toBe(true)
    submitButton(pending).props.onClick!()
    captchaInput(pending).props.onKeyDown!({ key: 'Enter' })
    expect(runtime.login).not.toHaveBeenCalled()
    resolve(captchaFixture())
    await settle()
    const undecoded = render(props)
    expect(submitButton(undecoded).props.disabled).toBe(true)
    submitButton(undecoded).props.onClick!()
    expect(runtime.login).not.toHaveBeenCalled()
    captchaImage(undecoded).props.onLoad!()
    expect(submitButton(render(props)).props.disabled).toBe(false)
  })

  it('accepts only four digits and never exposes an answer in the image alt text', async () => {
    const props = { onContinue: vi.fn() }
    fillCredentials(render(props))
    await completeCaptcha(props)
    captchaInput(render(props)).props.onChange!({ target: { value: '12a 345678' } })
    const ready = render(props)
    expect(captchaInput(ready).props).toMatchObject({ value: '1234', inputMode: 'numeric', maxLength: 4 })
    expect(captchaImage(ready).props.alt).toBe('登录图片验证码')
    captchaInput(ready).props.onChange!({ target: { value: '123' } })
    submitButton(render(props)).props.onClick!()
    expect(runtime.login).not.toHaveBeenCalled()
    expect(text(render(props))).toContain('请输入图片中的 4 位数字验证码')
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(1)
  })

  it('locks same-frame duplicate attempts and ignores refresh while login is in flight', async () => {
    let resolve!: (value: unknown) => void
    runtime.login.mockImplementation(() => new Promise(done => { resolve = done }))
    const props = { onContinue: vi.fn() }
    fillCredentials(render(props))
    const ready = await completeCaptcha(props)
    submitButton(ready).props.onClick!()
    submitButton(ready).props.onClick!()
    captchaInput(ready).props.onKeyDown!({ key: 'Enter' })
    refreshButton(ready).props.onClick!()
    expect(runtime.login).toHaveBeenCalledTimes(1)
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(1)
    resolve({ user: { username: 'testmember', nickname: '会员' } })
    await settle()
    expect(props.onContinue).toHaveBeenCalledTimes(1)
  })

  it('aborts superseded refreshes and ignores their out-of-order responses', async () => {
    let resolveFirst!: (value: ReturnType<typeof captchaFixture>) => void
    let resolveSecond!: (value: ReturnType<typeof captchaFixture>) => void
    runtime.loginCaptcha
      .mockImplementationOnce(() => new Promise(done => { resolveFirst = done }))
      .mockImplementationOnce(() => new Promise(done => { resolveSecond = done }))
    const props = { onContinue: vi.fn() }
    const first = render(props)
    const firstSignal = runtime.loginCaptcha.mock.calls[0][0] as AbortSignal
    refreshButton(first).props.onClick!()
    expect(firstSignal.aborted).toBe(true)
    resolveSecond(captchaFixture('newer'))
    await settle()
    captchaImage(render(props)).props.onLoad!()
    resolveFirst(captchaFixture('older'))
    await settle()
    expect(captchaImage(render(props)).props.src).toBe(captchaFixture('newer').image)
    expect(submitButton(render(props)).props.disabled).toBe(false)
  })

  it('clears the answer on refresh and rejects load events from the previous image', async () => {
    const props = { onContinue: vi.fn() }
    render(props)
    const old = await completeCaptcha(props)
    runtime.loginCaptcha.mockResolvedValueOnce(captchaFixture('next'))
    refreshButton(old).props.onClick!()
    captchaImage(old).props.onLoad!()
    expect(captchaInput(render(props)).props.value).toBe('')
    expect(submitButton(render(props)).props.disabled).toBe(true)
    await settle()
    captchaImage(old).props.onError!()
    expect(captchaImage(render(props)).props.src).toBe(captchaFixture('next').image)
    expect(submitButton(render(props)).props.disabled).toBe(true)
    captchaImage(render(props)).props.onLoad!()
    expect(submitButton(render(props)).props.disabled).toBe(false)
  })

  it('offers manual retry after a failed fetch without starting an automatic retry loop', async () => {
    runtime.loginCaptcha.mockRejectedValueOnce(new Error('网络不可用'))
    const props = { onContinue: vi.fn() }
    render(props)
    await settle()
    expect(text(render(props))).toContain('网络不可用')
    expect(refreshButton(render(props)).props['aria-label']).toBe('重新加载验证码')
    expect(submitButton(render(props)).props.disabled).toBe(true)
    vi.advanceTimersByTime(300_000)
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(1)
    refreshButton(render(props)).props.onClick!()
    await completeCaptcha(props)
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(2)
    expect(submitButton(render(props)).props.disabled).toBe(false)
  })

  it('offers manual retry when the PNG cannot load', async () => {
    const props = { onContinue: vi.fn() }
    render(props)
    await settle()
    captchaImage(render(props)).props.onError!()
    expect(text(render(props))).toContain('验证码图片加载失败，请点击重试')
    expect(submitButton(render(props)).props.disabled).toBe(true)
    expect(vi.getTimerCount()).toBe(0)
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(1)
    refreshButton(render(props)).props.onClick!()
    await completeCaptcha(props)
    expect(submitButton(render(props)).props.disabled).toBe(false)
  })

  it.each([
    null,
    { ...captchaFixture(), id: 123 },
    { ...captchaFixture(), id: '  ' },
    { ...captchaFixture(), image: 123 },
    { ...captchaFixture(), image: 'https://example.invalid/captcha.png' },
    { ...captchaFixture(), image: 'data:image/svg+xml;base64,PHN2Zz4=' },
    { ...captchaFixture(), image: 'data:image/png;base64,' },
    { ...captchaFixture(), image: 'data:image/png;base64,not a png' },
    { ...captchaFixture(), expires_in: 0 },
    { ...captchaFixture(), expires_in: -1 },
    { ...captchaFixture(), expires_in: Number.POSITIVE_INFINITY },
    { ...captchaFixture(), expires_in: '120' },
  ])('rejects an invalid challenge response: %j', async value => {
    runtime.loginCaptcha.mockResolvedValueOnce(value)
    const props = { onContinue: vi.fn() }
    render(props)
    await settle()
    expect(text(render(props))).toContain('验证码加载失败，请点击重试')
    expect(captchaImage(render(props))).toBeUndefined()
    expect(submitButton(render(props)).props.disabled).toBe(true)
    expect(vi.getTimerCount()).toBe(0)
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(1)
  })

  it('caps a server-provided validity window at 120 seconds', async () => {
    runtime.loginCaptcha.mockResolvedValueOnce({ ...captchaFixture(), expires_in: 3600 })
    const props = { onContinue: vi.fn() }
    render(props)
    await completeCaptcha(props)
    await vi.advanceTimersByTimeAsync(120_000)
    expect(text(render(props))).not.toContain('验证码已过期')
    expect(submitButton(render(props)).props.disabled).toBe(true)
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(2)
  })

  it('times out a stalled captcha request after 15 seconds and allows manual retry', async () => {
    let resolve!: (value: ReturnType<typeof captchaFixture>) => void
    runtime.loginCaptcha.mockImplementationOnce(() => new Promise(done => { resolve = done }))
    const props = { onContinue: vi.fn() }
    render(props)
    const signal = runtime.loginCaptcha.mock.calls[0][0] as AbortSignal
    vi.advanceTimersByTime(14_999)
    expect(signal.aborted).toBe(false)
    vi.advanceTimersByTime(1)
    expect(signal.aborted).toBe(true)
    const timedOut = render(props)
    expect(text(timedOut)).toContain('验证码加载超时，请点击重试')
    expect(text(refreshButton(timedOut))).toContain('点击重试')
    expect(submitButton(timedOut).props.disabled).toBe(true)
    expect(vi.getTimerCount()).toBe(0)
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(1)
    refreshButton(timedOut).props.onClick!()
    await completeCaptcha(props)
    resolve(captchaFixture('late'))
    await settle()
    expect(captchaImage(render(props)).props.src).toBe(captchaFixture().image)
    expect(submitButton(render(props)).props.disabled).toBe(false)
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(2)
  })

  it('clears the timeout when a request completes and does not time out the next image', async () => {
    const props = { onContinue: vi.fn() }
    render(props)
    await completeCaptcha(props)
    expect(vi.getTimerCount()).toBe(1)
    vi.advanceTimersByTime(15_000)
    expect(text(render(props))).not.toContain('加载超时')
    expect(submitButton(render(props)).props.disabled).toBe(false)
  })

  it('automatically replaces the captcha after 120 seconds and clears the old answer', async () => {
    const props = { onContinue: vi.fn() }
    fillCredentials(render(props))
    await completeCaptcha(props)
    await vi.advanceTimersByTimeAsync(119_999)
    expect(submitButton(render(props)).props.disabled).toBe(false)
    await vi.advanceTimersByTimeAsync(1)
    const expired = render(props)
    expect(submitButton(expired).props.disabled).toBe(true)
    expect(captchaInput(expired).props.value).toBe('')
    expect(text(expired)).not.toContain('验证码已过期')
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(2)
    expect(vi.getTimerCount()).toBe(1)
    submitButton(expired).props.onClick!()
    expect(runtime.login).not.toHaveBeenCalled()
  })

  it('checks wall-clock expiry even when a background tab delays its timer', async () => {
    const props = { onContinue: vi.fn() }
    fillCredentials(render(props))
    const ready = await completeCaptcha(props)
    vi.setSystemTime(Date.now() + 120_001)
    submitButton(ready).props.onClick!()
    expect(runtime.login).not.toHaveBeenCalled()
    expect(text(render(props))).not.toContain('验证码已过期')
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(2)
  })

  it('refreshes once after failed login while preserving credentials and clearing only the answer', async () => {
    runtime.login.mockRejectedValueOnce(new Error('帐号或密码不正确'))
    const props = { onContinue: vi.fn() }
    fillCredentials(render(props))
    const ready = await completeCaptcha(props)
    runtime.loginCaptcha.mockRejectedValueOnce(new Error('换图失败'))
    submitButton(ready).props.onClick!()
    await settle()
    const failed = render(props)
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(2)
    expect(text(failed)).toContain('帐号或密码不正确')
    expect(text(failed)).toContain('换图失败')
    expect(captchaInput(failed).props.value).toBe('')
    expect(find(failed, node => node.props.autoComplete === 'username')!.props.value).toBe('testmember')
    expect(find(failed, node => node.props.autoComplete === 'current-password')!.props.value).toBe('valid-password')
    expect(submitButton(failed).props.disabled).toBe(true)
    vi.advanceTimersByTime(300_000)
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(2)
    expect(props.onContinue).not.toHaveBeenCalled()
  })

  it('aborts captcha loading on unmount and ignores its late response', async () => {
    let resolve!: (value: ReturnType<typeof captchaFixture>) => void
    runtime.loginCaptcha.mockImplementationOnce(() => new Promise(done => { resolve = done }))
    const props = { onContinue: vi.fn() }
    render(props)
    const signal = runtime.loginCaptcha.mock.calls[0][0] as AbortSignal
    runtime.hooks!.unmount()
    expect(signal.aborted).toBe(true)
    resolve(captchaFixture())
    await settle()
    expect(captchaImage(render(props))).toBeUndefined()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('clears expiry timers and ignores late login completion after leaving the page', async () => {
    let resolve!: (value: unknown) => void
    runtime.login.mockImplementationOnce(() => new Promise(done => { resolve = done }))
    const props = { onContinue: vi.fn() }
    fillCredentials(render(props))
    submitButton(await completeCaptcha(props)).props.onClick!()
    expect(vi.getTimerCount()).toBe(1)
    runtime.hooks!.unmount()
    expect(vi.getTimerCount()).toBe(0)
    resolve({ user: { username: 'testmember', nickname: '会员' } })
    await settle()
    expect(props.onContinue).not.toHaveBeenCalled()
    expect(runtime.loginCaptcha).toHaveBeenCalledTimes(1)
  })
})
