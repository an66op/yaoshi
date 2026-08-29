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
  const roomDescription = [error && 'room-entry-error', notice && 'room-entry-notice', 'room-entry-hint'].filter(Boolean).join(' ')

  const updateRoom = (value: string) => {
    setRoom(value)
    setError('')
    setNotice('')
  }

  const submit = async () => {
    const value = room.trim()
    if (!/^\d{5,12}$/.test(value)) return setError('请输入 5–12 位数字房间号')
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
      <section className="room-entry-shell">
        <header className="room-entry-top">
          <button aria-label={fromLobby ? '返回大厅' : '返回登录'} className="room-entry-back" onClick={onBack} type="button"><Icon name="back" /></button>
          <div className="room-entry-brand">
            <img alt="" src="/images/king-racing-mark.jpg" />
            <span><b>{BRAND_NAME}</b><small>PRIVATE ROOM</small></span>
          </div>
          <span aria-hidden="true" className="room-entry-top-spacer" />
        </header>
        <form className="room-entry-card" onSubmit={(event) => { event.preventDefault(); void submit() }}>
          <div className="room-entry-copy">
            <h1>输入房间号</h1>
          </div>
          <label className="room-entry-field" htmlFor="room-entry-code">
            <span>房间号</span>
            <div className="room-entry-input-shell">
              <Icon name="room" />
              <input
                aria-describedby={roomDescription}
                aria-invalid={Boolean(error) || undefined}
                autoComplete="off"
                autoFocus
                disabled={loading}
                id="room-entry-code"
                inputMode="numeric"
                maxLength={12}
                onChange={(event) => updateRoom(event.currentTarget.value.replace(/\D/g, '').slice(0, 12))}
                pattern="[0-9]*"
                placeholder="请输入数字房间号"
                value={room}
              />
            </div>
          </label>
          {error && <p className="room-entry-error" id="room-entry-error" role="alert">{error}</p>}
          {notice && <p className="room-entry-success" id="room-entry-notice" role="status">{notice}</p>}
          <button aria-busy={loading} aria-label={loading ? '正在验证房间' : '进入房间'} className="room-entry-submit" disabled={loading} type="submit">
            <b>{loading ? '验证中…' : '进入房间'}</b>
            <span><Icon name="arrow" /></span>
          </button>
          <p className="room-entry-hint" id="room-entry-hint">仅支持 5–12 位数字，房间号由上级提供</p>
        </form>
      </section>
    </main>
  )
}
