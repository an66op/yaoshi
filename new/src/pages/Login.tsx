import { useState } from 'react'
import { Icon } from '../components/Icon'
import { memberApi } from '../api/member'
import { setToken } from '../api/client'

type Props = { onContinue: (account: string, nickname: string) => void; onRegister?: () => void }

/** 会员登录：调用后端 /api/member/login */
export function Login({ onContinue, onRegister }: Props) {
  const [account, setAccount] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const submit = async () => {
    const value = account.trim()
    if (value.length < 3) return setError('请输入至少 3 位帐号')
    if (password.length < 6) return setError('请输入至少 6 位密码')
    setLoading(true)
    setError('')
    try {
      const result = await memberApi.login(value, password)
      setToken(result.token)
      onContinue(result.user.username, result.user.nickname || result.user.username)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <main className="login-page">
      <section className="login-card">
        <div className="login-glow glow-one" />
        <div className="login-glow glow-two" />
        <header className="login-brand">
          <img alt="曜图" src="/images/yaotu-logo-concept.png" />
          <div><b>曜图</b><small>YAO TU · GAME SPACE</small></div>
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
              maxLength={20}
              onChange={(event) => { setAccount(event.target.value.replace(/\s/g, '')); setError('') }}
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
              maxLength={32}
              onChange={(event) => { setPassword(event.target.value); setError('') }}
              onKeyDown={(event) => event.key === 'Enter' && void submit()}
              placeholder="输入至少 6 位密码"
              type="password"
              value={password}
            />
          </div>
        </label>
        {error && <p className="login-error" role="alert">{error}</p>}
        <button className="login-primary" disabled={loading} onClick={() => void submit()}>
          {loading ? '登录中…' : '验证并继续'} <Icon name="arrow" />
        </button>
        <div className="login-status">
          <span>✓</span>
          <p>登录后还需输入房间号 · 请向代理或管理员获取</p>
        </div>
        <footer className="login-foot">
          <span>yaotu.app</span>
          {onRegister ? <button className="room-entry-back" onClick={onRegister}>没有帐号？注册</button> : <p>会员端已接入后端鉴权</p>}
        </footer>
      </section>
    </main>
  )
}
