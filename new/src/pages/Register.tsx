import { useEffect, useState } from 'react'
import { Icon } from '../components/Icon'
import { memberApi } from '../api/member'
import { setToken } from '../api/client'
import { BRAND_NAME } from '../data/brand'
import type { Theme } from '../types'

type Props = {
  onContinue: (account: string, nickname: string) => void
  onBack: () => void
  theme?: Theme
}

/** 会员注册：邀请码可选；房间号由上级为代理配置，注册时不填写。 */
export function Register({ onContinue, onBack, theme = 'day' }: Props) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [nickname, setNickname] = useState('')
  const [inviteCode, setInviteCode] = useState('')
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const invite = params.get('invite') ?? params.get('ref')
    if (invite) setInviteCode(invite)
  }, [])

  const submit = async () => {
    const value = username.trim()
    if (value.length < 3) return setError('帐号至少 3 位')
    if (password.length < 6) return setError('密码至少 6 位')
    const invite = inviteCode.trim().replace(/^u/i, '')
    if (invite && invite.length < 4) return setError('邀请码至少 4 位')
    setLoading(true)
    setError('')
    setSuccess('')
    try {
      const result = await memberApi.register({
        username: value,
        password,
        nickname: nickname.trim() || value,
        invite_code: inviteCode.trim(),
      })
      setToken(result.token)
      if (result.message) setSuccess(result.message)
      onContinue(result.user.username, result.user.nickname || result.user.username)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '注册失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <main className={`login-page theme-${theme}`}>
      <section className="login-card">
        <header className="login-brand">
          <img alt={BRAND_NAME} src="/images/king-racing-mark.jpg" />
          <div><b>{BRAND_NAME}</b><small>CREATE ACCOUNT</small></div>
        </header>
        <div className="login-copy">
          <small>新用户注册</small>
          <h1>创建帐号</h1>
          <p>填写帐号与密码；如有代理邀请码可一并填写。登录后再输入房间号进入。</p>
        </div>
        <label className="login-field"><span>帐号</span><div><i>◉</i><input autoComplete="username" value={username} onChange={(e) => { setUsername(e.target.value.replace(/\s/g, '')); setError('') }} placeholder="至少 3 位" /></div></label>
        <label className="login-field"><span>密码</span><div><i>●</i><input type="password" autoComplete="new-password" value={password} onChange={(e) => { setPassword(e.target.value); setError('') }} placeholder="至少 6 位" /></div></label>
        <label className="login-field"><span>昵称</span><div><i>◎</i><input value={nickname} onChange={(e) => setNickname(e.target.value)} placeholder="可选" /></div></label>
        <label className="login-field"><span>邀请码</span><div><i>礼</i><input value={inviteCode} onChange={(e) => setInviteCode(e.target.value)} placeholder="例如 U1001（至少 4 位，可选）" /></div></label>
        {success && <p className="login-error login-success" role="status">{success}</p>}
        {error && <p className="login-error" role="alert">{error}</p>}
        <button className="login-primary" disabled={loading} onClick={() => void submit()}>{loading ? '注册中…' : '注册并登录'} <Icon name="arrow" /></button>
        <button className="room-entry-back register-back" onClick={onBack}>已有帐号？返回登录</button>
      </section>
    </main>
  )
}
