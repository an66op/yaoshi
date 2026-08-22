import { useEffect, useState } from 'react'
import { Icon } from '../components/Icon'
import { memberApi } from '../api/member'
import { setToken } from '../api/client'

type Props = {
  onContinue: (account: string, nickname: string) => void
  onBack: () => void
}

/** 会员注册：支持邀请码与房间号 */
export function Register({ onContinue, onBack }: Props) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [nickname, setNickname] = useState('')
  const [inviteCode, setInviteCode] = useState('')
  const [roomCode, setRoomCode] = useState('')
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const invite = params.get('invite') ?? params.get('ref')
    const room = params.get('room')
    if (invite) setInviteCode(invite)
    if (room) setRoomCode(room.replace(/\D/g, ''))
  }, [])

  const submit = async () => {
    const value = username.trim()
    if (value.length < 3) return setError('帐号至少 3 位')
    if (password.length < 6) return setError('密码至少 6 位')
    setLoading(true)
    setError('')
    setSuccess('')
    try {
      const result = await memberApi.register({
        username: value,
        password,
        nickname: nickname.trim() || value,
        invite_code: inviteCode.trim(),
        room_code: roomCode.trim(),
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
    <main className="login-page">
      <section className="login-card">
        <header className="login-brand">
          <img alt="曜图" src="/images/yaotu-logo-concept.png" />
          <div><b>曜图</b><small>CREATE ACCOUNT</small></div>
        </header>
        <div className="login-copy">
          <small>新用户注册</small>
          <h1>创建帐号</h1>
          <p>填写帐号信息，可选填邀请码与房间号。</p>
        </div>
        <label className="login-field"><span>帐号</span><div><i>◉</i><input autoComplete="username" value={username} onChange={(e) => { setUsername(e.target.value.replace(/\s/g, '')); setError('') }} placeholder="至少 3 位" /></div></label>
        <label className="login-field"><span>密码</span><div><i>●</i><input type="password" autoComplete="new-password" value={password} onChange={(e) => { setPassword(e.target.value); setError('') }} placeholder="至少 6 位" /></div></label>
        <label className="login-field"><span>昵称</span><div><i>◎</i><input value={nickname} onChange={(e) => setNickname(e.target.value)} placeholder="可选" /></div></label>
        <label className="login-field"><span>邀请码</span><div><i>礼</i><input value={inviteCode} onChange={(e) => setInviteCode(e.target.value)} placeholder="例如 U1001" /></div></label>
        <label className="login-field"><span>房间号</span><div><i>房</i><input inputMode="numeric" value={roomCode} onChange={(e) => setRoomCode(e.target.value.replace(/\D/g, ''))} placeholder="可选，代理靓号" /></div></label>
        {success && <p className="login-error" role="status" style={{ color: '#2eaf7b' }}>{success}</p>}
        {error && <p className="login-error" role="alert">{error}</p>}
        <button className="login-primary" disabled={loading} onClick={() => void submit()}>{loading ? '注册中…' : '注册并登录'} <Icon name="arrow" /></button>
        <button className="room-entry-back" style={{ marginTop: 12 }} onClick={onBack}>已有帐号？返回登录</button>
      </section>
    </main>
  )
}
