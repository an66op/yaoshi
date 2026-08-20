import { useState } from 'react'
import { Icon } from '../components/Icon'

type Props = { onContinue: (account: string) => void }

/** 第一页：帐号与密码校验，仅模拟前端登录流程。 */
export function Login({ onContinue }: Props) {
  const [account, setAccount] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const submit = () => {
    const value = account.trim()
    if (value.length < 3) return setError('请输入至少 3 位帐号')
    if (password.length < 6) return setError('请输入至少 6 位密码')
    onContinue(value)
  }
  return <main className="login-page"><section className="login-card"><div className="login-glow glow-one" /><div className="login-glow glow-two" /><header className="login-brand"><img alt="曜图" src="/images/yaotu-logo-concept.png" /><div><b>曜图</b><small>YAO TU · GAME SPACE</small></div><em>安全入口</em></header><div className="login-copy"><small>帐号登录</small><h1>欢迎回来</h1><p>验证帐号后，继续进入你的专属彩种空间。</p></div><label className="login-field"><span>帐号</span><div><i>◉</i><input autoComplete="username" autoFocus maxLength={20} onChange={(event) => { setAccount(event.target.value.replace(/\s/g, '')); setError('') }} onKeyDown={(event) => event.key === 'Enter' && submit()} placeholder="输入帐号，例如 YT8021" value={account} /></div></label><label className="login-field"><span>密码</span><div><i>●</i><input autoComplete="current-password" maxLength={32} onChange={(event) => { setPassword(event.target.value); setError('') }} onKeyDown={(event) => event.key === 'Enter' && submit()} placeholder="输入至少 6 位密码" type="password" value={password} /></div></label>{error && <p className="login-error" role="alert">{error}</p>}<button className="login-primary" onClick={submit}>验证并继续 <Icon name="arrow" /></button><div className="login-status"><span>✓</span><p>本次为前端演示登录，不会传输帐号或密码</p></div><footer className="login-foot"><span>yaotu.app</span><p>登录后还需输入房间号 · 请向管理员获取</p></footer></section></main>
}
