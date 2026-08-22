import { useState } from 'react'
import { Icon } from '../components/Icon'
import { memberApi } from '../api/member'

type Props = {
  onBack: () => void
  onEnter: (room: string, roomName: string) => void
}

/** 房间入口：校验房间号并绑定 parent_agent_id */
export function RoomEntry({ onBack, onEnter }: Props) {
  const [room, setRoom] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const submit = async () => {
    const value = room.trim()
    if (value.length < 4) return setError('请输入至少 4 位房间号')
    setLoading(true)
    setError('')
    try {
      const result = await memberApi.joinRoom(value)
      onEnter(result.room_code, result.room_name)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '无法验证房间号')
    } finally {
      setLoading(false)
    }
  }

  return (
    <main className="room-entry-page">
      <header className="room-entry-hero">
        <div className="room-entry-top">
          <img alt="曜图" src="/images/yaotu-logo-concept.png" />
          <div><b>曜图</b><small>YAO TU · PRIVATE ROOM</small></div>
        </div>
      </header>
      <section className="room-entry-content">
        <button className="room-entry-back" onClick={onBack}><Icon name="back" />返回登录</button>
        <article className="room-entry-card">
          <div className="room-entry-icon"><Icon name="game" /></div>
          <div className="room-entry-copy">
            <small>PRIVATE ROOM</small>
            <h1>输入房间号</h1>
            <p>房间号即代理靓号，由管理员发放给代理；输入后进入对应代理房间。</p>
          </div>
          <label className="room-entry-field">
            <span>房间号</span>
            <div>
              <input
                autoComplete="off"
                autoFocus
                inputMode="numeric"
                maxLength={12}
                onChange={(event) => { setRoom(event.target.value.replace(/\D/g, '')); setError('') }}
                onKeyDown={(event) => event.key === 'Enter' && void submit()}
                placeholder="例如 1024 8801"
                value={room}
              />
              <i>ROOM ID</i>
            </div>
          </label>
          {error && <p className="room-entry-error" role="alert">{error}</p>}
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
