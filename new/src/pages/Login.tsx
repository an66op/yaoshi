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

/** 会员登录：调用后端 /api/member/login */
export function Login({ onContinue, onRegister, theme = 'day', verificationPending = false }: Props) {
  const [account, setAccount] = useState(() => import.meta.env.DEV ? DEMO_ACCOUNT : '')
  const [password, setPassword] = useState(() => import.meta.env.DEV ? DEMO_PASSWORD : '')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [testPrefilled, setTestPrefilled] = useState(false)
  const edited = useRef(false)

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
    if (verificationPending || loading) return
    const value = account.trim()
    if (unicodeLength(value) < USERNAME_MIN_LENGTH) return setError(`请输入至少 ${USERNAME_MIN_LENGTH} 位帐号`)
    if (!password) return setError('请输入登录密码')
    if (passwordByteLength(password) > PASSWORD_MAX_BYTES) return setError(`登录密码最多 ${PASSWORD_MAX_BYTES} 字节`)
    setLoading(true)
    setError('')
    try {
      const result = await memberApi.login(value, password)
      onContinue(result.user.username, result.user.nickname || result.user.username)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '登录失败')
    } finally {
      setLoading(false)
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
          <small>帐号登录</small>
          <h1>欢迎回来</h1>
          <p>验证帐号后，继续进入你的专属彩种空间。</p>
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
        {testPrefilled && <p className="login-test-notice" role="status">测试环境 · 已填充体验账号<small>体验账号公开，仅用于测试，请勿存入真实资金或个人资料。</small></p>}
        {error && <p className="login-error" role="alert">{error}</p>}
        <button className="login-primary" disabled={loading || verificationPending} onClick={() => void submit()}>
          {verificationPending ? '连接中…' : loading ? '登录中…' : '验证并继续'} <Icon name="arrow" />
        </button>
        {verificationPending ? <SessionCheckNotice className="login-status login-session-status" /> : <div className="login-status">
          <span>✓</span>
          <p>登录后输入房间号 · 房间由上级配置并发放给代理</p>
        </div>}
        <footer className="login-foot">
          <span>{BRAND_NAME}娱乐</span>
          {onRegister ? <button className="room-entry-back" onClick={onRegister}>没有帐号？注册</button> : <p>安全登录 · 账户信息已加密</p>}
        </footer>
      </section>
    </main>
  )
}
