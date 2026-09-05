import { useEffect, useRef, useState } from 'react'
import { Icon } from '../components/Icon'
import { SessionCheckNotice } from '../components/SessionStartup'
import { memberApi } from '../api/member'
import {
  MEMBER_LOGIN_USERNAME_MAX_LENGTH,
  PASSWORD_MAX_BYTES,
  USERNAME_MIN_LENGTH,
  passwordByteLength,
  truncateUnicode,
  unicodeLength,
} from '../authLimits'
import { BRAND_NAME, DEMO_ACCOUNT, DEMO_PASSWORD } from '../data/brand'
import type { Theme } from '../types'
import { loadTestLogin } from '../utils/testLogin'

type Props = { onContinue: (account: string, nickname: string) => void; onRegister?: () => void; theme?: Theme; verificationPending?: boolean }
type LoginCaptcha = { id: string; image: string; expiresAt: number }
const CAPTCHA_LENGTH = 4

/** 会员登录：调用后端 /api/member/login */
export function Login({ onContinue, onRegister, theme = 'day', verificationPending = false }: Props) {
  const [account, setAccount] = useState(() => import.meta.env.DEV ? DEMO_ACCOUNT : '')
  const [password, setPassword] = useState(() => import.meta.env.DEV ? DEMO_PASSWORD : '')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [testPrefilled, setTestPrefilled] = useState(false)
  const [captcha, setCaptcha] = useState<LoginCaptcha | null>(null)
  const [captchaCode, setCaptchaCode] = useState('')
  const [captchaLoading, setCaptchaLoading] = useState(true)
  const [captchaReady, setCaptchaReady] = useState(false)
  const [captchaError, setCaptchaError] = useState('')
  const edited = useRef(false)
  const mounted = useRef(false)
  const submitting = useRef(false)
  const captchaRequest = useRef<AbortController | null>(null)
  const captchaGeneration = useRef(0)
  const activeCaptcha = useRef<LoginCaptcha | null>(null)
  const captchaImageReady = useRef(false)
  const captchaTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const captchaRequestTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const refreshCaptchaCurrent = useRef<() => void>(() => undefined)
  const captchaStatusMessage = captchaError || (captchaLoading || !captchaReady ? '正在加载验证码…' : '')

  const clearCaptchaTimer = () => {
    if (captchaTimer.current !== null) clearTimeout(captchaTimer.current)
    captchaTimer.current = null
  }

  const clearCaptchaRequestTimer = () => {
    if (captchaRequestTimer.current !== null) clearTimeout(captchaRequestTimer.current)
    captchaRequestTimer.current = null
  }

  const expireCaptcha = () => {
    activeCaptcha.current = null
    captchaImageReady.current = false
    clearCaptchaTimer()
    setCaptcha(null)
    setCaptchaReady(false)
    setCaptchaCode('')
    refreshCaptchaCurrent.current()
  }

  const refreshCaptcha = async () => {
    if (!mounted.current || submitting.current) return
    captchaRequest.current?.abort()
    clearCaptchaRequestTimer()
    clearCaptchaTimer()
    const request = new AbortController()
    captchaRequest.current = request
    const generation = ++captchaGeneration.current
    const startedAt = Date.now()
    activeCaptcha.current = null
    captchaImageReady.current = false
    setCaptcha(null)
    setCaptchaCode('')
    setCaptchaReady(false)
    setCaptchaError('')
    setCaptchaLoading(true)
    // Supplying our own AbortSignal opts out of the API client's timeout.
    // Finish the visible loading state here even if a stalled transport never
    // settles after cancellation; a late response remains invalidated below.
    captchaRequestTimer.current = setTimeout(() => {
      if (!mounted.current || request.signal.aborted || generation !== captchaGeneration.current) return
      captchaRequestTimer.current = null
      request.abort()
      setCaptchaLoading(false)
      setCaptchaError('验证码加载超时，请点击重试')
    }, 15_000)
    try {
      const result = await memberApi.loginCaptcha(request.signal)
      if (!mounted.current || request.signal.aborted || generation !== captchaGeneration.current) return
      if (!result || typeof result.id !== 'string' || !result.id.trim() ||
        typeof result.image !== 'string' || !/^data:image\/png;base64,[a-zA-Z0-9+/]+=*$/.test(result.image) ||
        !Number.isFinite(result.expires_in) || result.expires_in <= 0) {
        throw new Error('验证码加载失败，请点击重试')
      }
      // Start the local lifetime with the request, so network latency cannot
      // make a challenge look fresh after its server-side expiry.
      const next = { id: result.id, image: result.image, expiresAt: startedAt + Math.min(result.expires_in, 120) * 1000 }
      setCaptcha(next)
      if (next.expiresAt <= Date.now()) {
        expireCaptcha()
        return
      }
      activeCaptcha.current = next
      captchaTimer.current = setTimeout(() => {
        if (mounted.current && generation === captchaGeneration.current) expireCaptcha()
      }, next.expiresAt - Date.now())
    } catch (reason) {
      if (!mounted.current || request.signal.aborted || generation !== captchaGeneration.current) return
      setCaptchaError(reason instanceof Error ? reason.message : '验证码加载失败，请点击重试')
    } finally {
      if (generation === captchaGeneration.current) clearCaptchaRequestTimer()
      if (mounted.current && !request.signal.aborted && generation === captchaGeneration.current) setCaptchaLoading(false)
    }
  }

  refreshCaptchaCurrent.current = () => { void refreshCaptcha() }

  useEffect(() => {
    mounted.current = true
    void refreshCaptcha()
    return () => {
      mounted.current = false
      captchaRequest.current?.abort()
      clearCaptchaTimer()
      clearCaptchaRequestTimer()
    }
  }, [])

  useEffect(() => {
    if (import.meta.env.DEV) return
    const request = new AbortController()
    void loadTestLogin(request.signal).then(preset => {
      if (!preset || request.signal.aborted || edited.current) return
      setAccount(preset.username)
      setPassword(preset.password)
      setTestPrefilled(true)
    })
    return () => request.abort()
  }, [])

  const markEdited = () => {
    edited.current = true
    setTestPrefilled(false)
    setError('')
  }

  const submit = async () => {
    // The form may paint immediately, but a refresh must finish its existing
    // cookie check before starting another login (including keyboard submit).
    if (verificationPending || submitting.current || captchaLoading) return
    const value = account.trim()
    if (unicodeLength(value) < USERNAME_MIN_LENGTH) return setError(`请输入至少 ${USERNAME_MIN_LENGTH} 位帐号`)
    if (!password) return setError('请输入登录密码')
    if (passwordByteLength(password) > PASSWORD_MAX_BYTES) return setError(`登录密码最多 ${PASSWORD_MAX_BYTES} 字节`)
    const challenge = activeCaptcha.current
    if (!challenge || !captchaImageReady.current) return setError('请先加载有效的图片验证码')
    if (challenge.expiresAt <= Date.now()) {
      expireCaptcha()
      return
    }
    if (!new RegExp(`^\\d{${CAPTCHA_LENGTH}}$`).test(captchaCode)) return setError(`请输入图片中的 ${CAPTCHA_LENGTH} 位数字验证码`)
    // React state updates are batched; the ref also locks a second keyboard
    // or button event in the same frame before disabled can repaint.
    submitting.current = true
    setLoading(true)
    setError('')
    try {
      const result = await memberApi.login(value, password, { captcha_id: challenge.id, captcha_code: captchaCode })
      if (mounted.current) onContinue(result.user.username, result.user.nickname || result.user.username)
    } catch (reason) {
      if (!mounted.current) return
      setError(reason instanceof Error ? reason.message : '登录失败')
      // Every attempt consumes its challenge. Fetch once after failure and
      // leave a failed refresh retryable, without an automatic request loop.
      submitting.current = false
      void refreshCaptcha()
    } finally {
      submitting.current = false
      if (mounted.current) setLoading(false)
    }
  }

  return (
    <main className={`login-page theme-${theme}`}>
      <section className="login-card">
        <div className="login-glow glow-one" />
        <div className="login-glow glow-two" />
        <header className="login-brand">
          <img alt={BRAND_NAME} src="/images/king-racing-mark.jpg" />
          <div><b>{BRAND_NAME}</b><small>王者 · GAME SPACE</small></div>
          <em>安全入口</em>
        </header>
        <div className="login-copy">
          <h1>欢迎回来</h1>
        </div>
        <label className="login-field">
          <span>帐号</span>
          <div>
            <i>◉</i>
            <input
              autoComplete="username"
              autoFocus
              onChange={(event) => { markEdited(); setAccount(truncateUnicode(event.target.value, MEMBER_LOGIN_USERNAME_MAX_LENGTH)) }}
              onKeyDown={(event) => event.key === 'Enter' && void submit()}
              placeholder="输入帐号"
              value={account}
            />
          </div>
        </label>
        <label className="login-field">
          <span>密码</span>
          <div>
            <i>●</i>
            <input
              autoComplete="current-password"
              maxLength={PASSWORD_MAX_BYTES}
              onChange={(event) => { markEdited(); setPassword(event.target.value) }}
              onKeyDown={(event) => event.key === 'Enter' && void submit()}
              placeholder="输入登录密码"
              type="password"
              value={password}
            />
          </div>
        </label>
        <div className="login-field login-captcha-field">
          <span id="login-captcha-label">验证码</span>
          <div className="login-captcha-row">
            <input
              aria-labelledby="login-captcha-label"
              aria-describedby={captchaStatusMessage ? 'login-captcha-status' : undefined}
              autoComplete="off"
              inputMode="numeric"
              maxLength={CAPTCHA_LENGTH}
              pattern={`[0-9]{${CAPTCHA_LENGTH}}`}
              onChange={(event) => { setCaptchaCode(event.target.value.replace(/\D/g, '').slice(0, CAPTCHA_LENGTH)); setError('') }}
              onKeyDown={(event) => event.key === 'Enter' && void submit()}
              placeholder={`输入 ${CAPTCHA_LENGTH} 位数字`}
              value={captchaCode}
            />
            <button
              aria-label={captchaError ? '重新加载验证码' : '换一张验证码'}
              aria-busy={captchaLoading}
              className="login-captcha-image"
              disabled={loading}
              onClick={() => void refreshCaptcha()}
              type="button"
            >
              {captcha ? <img
                alt="登录图片验证码"
                src={captcha.image}
                onLoad={() => {
                  if (!mounted.current || activeCaptcha.current?.id !== captcha.id) return
                  if (captcha.expiresAt <= Date.now()) return expireCaptcha()
                  captchaImageReady.current = true
                  setCaptchaReady(true)
                }}
                onError={() => {
                  if (!mounted.current || activeCaptcha.current?.id !== captcha.id) return
                  activeCaptcha.current = null
                  captchaImageReady.current = false
                  clearCaptchaTimer()
                  setCaptcha(null)
                  setCaptchaReady(false)
                  setCaptchaError('验证码图片加载失败，请点击重试')
                }}
              /> : <span>{captchaLoading ? '加载中…' : '点击重试'}</span>}
            </button>
          </div>
          {captchaStatusMessage && <small id="login-captcha-status" className={captchaError ? 'login-captcha-status is-error' : 'login-captcha-status'} role="status">{captchaStatusMessage}</small>}
        </div>
        {testPrefilled && <p className="login-test-notice" role="status">测试环境 · 已填充体验账号<small>体验账号公开，仅用于测试，请勿存入真实资金或个人资料。</small></p>}
        {error && <p className="login-error" role="alert">{error}</p>}
        <button className="login-primary" disabled={loading || verificationPending || captchaLoading || !captchaReady} onClick={() => void submit()}>
          {verificationPending ? '连接中…' : loading ? '登录中…' : '验证并继续'} <Icon name="arrow" />
        </button>
        {verificationPending && <SessionCheckNotice className="login-status login-session-status" />}
        <footer className="login-foot">
          {onRegister ? <button className="room-entry-back" onClick={onRegister}>没有帐号？注册</button> : <p>安全登录 · 账户信息已加密</p>}
        </footer>
      </section>
    </main>
  )
}
