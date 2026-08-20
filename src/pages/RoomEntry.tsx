import { useState } from 'react'
import { Icon } from '../components/Icon'

type Props = { account: string; onBack: () => void; onEnter: (room: string) => void }

/** 第二页：独立的房间入口，视觉语言与游戏大厅保持一致。 */
export function RoomEntry({ onBack, onEnter }: Props) {
  const [room, setRoom] = useState('')
  const [error, setError] = useState('')
  const submit = () => {
    const value = room.trim()
    if (value.length < 4) return setError('请输入至少 4 位房间号')
    onEnter(value)
  }
  return <main className="room-entry-page"><header className="room-entry-hero"><div className="room-entry-top"><img alt="曜图" src="/images/yaotu-logo-concept.png" /><div><b>曜图</b><small>YAO TU · PRIVATE ROOM</small></div></div></header><section className="room-entry-content"><button className="room-entry-back" onClick={onBack}><Icon name="back" />返回登录</button><article className="room-entry-card"><div className="room-entry-icon"><Icon name="game" /></div><div className="room-entry-copy"><small>PRIVATE ROOM</small><h1>输入房间号</h1><p>房间号由管理员或邀请人提供，用于连接对应的游戏与聊天会话。</p></div><label className="room-entry-field"><span>房间号</span><div><input autoComplete="off" autoFocus inputMode="numeric" maxLength={12} onChange={(event) => { setRoom(event.target.value.replace(/\D/g, '')); setError('') }} onKeyDown={(event) => event.key === 'Enter' && submit()} placeholder="例如 1024 8801" value={room} /><i>ROOM ID</i></div></label>{error && <p className="room-entry-error" role="alert">{error}</p>}<button className="room-entry-submit" onClick={submit}><span>进入房间</span><Icon name="arrow" /></button></article><div className="room-entry-note"><span>前端演示</span><p>房间信息仅保存在当前设备，不会发送至服务器。</p></div></section></main>
}
