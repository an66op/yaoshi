import { useState } from 'react'
import { Icon } from '../components/Icon'
import { memberApi } from '../api/member'

import { BRAND_NAME, DEMO_ROOM } from '../data/brand'
import type { Theme } from '../types'

type Props = {
  onBack: () => void
  onEnter: (room: string, roomName: string) => void
  theme?: Theme
  fromLobby?: boolean
}

/** 房间入口：校验房间号并绑定 parent_agent_id */
export function RoomEntry({ onBack, onEnter, theme = 'day', fromLobby = false }: Props) {
  const [room, setRoom] = useState(DEMO_ROOM)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [loading, setLoading] = useState(false)

  const submit = async () => {
    const value = room.trim()
    if (value.length < 4) return setError('请输入至少 4 位房间号')
    setLoading(true)
    setError('')
    setNotice('')
    try {
      const result = await memberApi.joinRoom(value)
      if (result.status === 'pending') {
        setNotice(`入房申请已提交（编号 ${result.application_id ?? '—'}），审核通过后即可进入 ${result.room_code}`)
        return
      }
      onEnter(result.room_code, result.room_name)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '无法验证房间号')
    } finally {
      setLoading(false)
    }
  }

  return (
    <main className={`room-entry-page theme-${theme}`}>
      <header className="room-entry-hero">
        <div className="room-entry-top">
          <img alt={BRAND_NAME} src="/images/king-racing-mark.jpg" />
          <div><b>{BRAND_NAME}</b><small>王者 · PRIVATE ROOM</small></div>
        </div>
      </header>
      <section className="room-entry-content">
        <button className="room-entry-back" onClick={onBack}><Icon name="back" />{fromLobby ? '返回大厅' : '返回登录'}</button>
        <article className="room-entry-card">
          <div className="room-entry-copy">
            <small>PRIVATE ROOM</small>
            <h1>输入房间号</h1>
            <p>房间号由上级配置并发放给代理；输入后进入对应代理房间。</p>
          </div>
          <label className="room-entry-field">
            <span>房间号</span>
            <div>
              <input
                autoComplete="off"
                autoFocus
                inputMode="numeric"
                maxLength={12}
                onChange={(event) => { setRoom(event.target.value.replace(/\D/g, '')); setError(''); setNotice('') }}
                onKeyDown={(event) => event.key === 'Enter' && void submit()}
                placeholder="例如 1024 8801"
                value={room}
              />
              <i>ROOM ID</i>
            </div>
          </label>
          {error && <p className="room-entry-error" role="alert">{error}</p>}
          {notice && <p className="room-entry-success" role="status">{notice}</p>}
          <button className="room-entry-submit" disabled={loading} onClick={() => void submit()}>
            <span>{loading ? '验证中…' : '进入房间'}</span>
            <Icon name="arrow" />
          </button>
        </article>
        <div className="room-entry-note">
          <span>代理房间</span>
          <p>系统会校验房间号并绑定到你的会员账号。</p>
        </div>
      </section>
    </main>
  )
}
