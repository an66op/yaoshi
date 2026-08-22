import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { ballTone } from '../data/games'
import { Icon } from '../components/Icon'
import { AnnouncementDialog } from '../components/Dialogs'
import type { Game, Theme } from '../types'
import { portalApi } from '../api/portal'

type Filter = 'all' | 'favorite' | 'latest'
type Props = {
  account: string
  room: string
  games: Game[]
  theme: Theme
  gamesLive?: boolean
  gamesError?: string
  onOpenGame: (id: string) => void
  onToggleTheme: () => void
}

const filters: Array<{ id: Filter; label: string }> = [{ id: 'all', label: '全部彩种' }, { id: 'favorite', label: '常玩' }, { id: 'latest', label: '最新开奖' }]

export function Lobby({ account, room, games, theme, gamesLive, gamesError, onOpenGame, onToggleTheme }: Props) {
  const [announcementOpen, setAnnouncementOpen] = useState(false)
  const [filter, setFilter] = useState<Filter>('all')
  const [roomNotice, setRoomNotice] = useState('【公告】加载中…')
  const [announcementTitle, setAnnouncementTitle] = useState('平台公告')
  const [announcementBody, setAnnouncementBody] = useState('')
  const visibleGames = filter === 'favorite' ? games.slice(0, 2) : filter === 'latest' ? [...games].reverse() : games

  useEffect(() => {
    void portalApi.roomSettings().then((settings) => {
      const notice = settings.room_notice || '欢迎来到曜图'
      setRoomNotice(`【公告】${notice}`)
      setAnnouncementBody(notice)
    }).catch(() => setRoomNotice('【公告】欢迎来到曜图'))
    void portalApi.activities('banner').then((items) => {
      const active = items.find((item) => item.status === 'active')
      if (active) {
        setAnnouncementTitle(active.title)
        setAnnouncementBody(active.subtitle || active.title)
      }
    }).catch(() => undefined)
  }, [])

  return <>
    <header className="lobby-hero">
      <div className="hero-top"><span className="brand-word">曜图</span><div className="room-badge"><b>{room}</b></div><div className="hero-tools"><button className="theme-switch" onClick={onToggleTheme} aria-label="切换昼夜模式">{theme === 'day' ? '☾' : '☀'}</button><button className="avatar-user" aria-label="用户资料">{account.slice(0, 1).toUpperCase()}</button></div></div>
      <button className="announcement" onClick={() => setAnnouncementOpen(true)}><span>●</span><p>{roomNotice}</p><Icon name="arrow" /></button>
    </header>
    <section className="lobby-body">
      <div className="lobby-toolbar">{filters.map((item) => <button className={filter === item.id ? 'toolbar-active' : ''} key={item.id} onClick={() => setFilter(item.id)}>{item.label}</button>)}<span>共 {visibleGames.length} 个彩种</span></div>
      <div className="game-list">{visibleGames.map((game) => <button className="game-card" key={game.id} onClick={() => onOpenGame(game.id)}><div className="game-top" style={{ '--game': game.color } as CSSProperties}><div className="game-lead"><span className="game-logo">{game.tag.split(' ')[0].slice(0, 2)}</span><div><strong>{game.title}</strong><small>在线 {game.online} 人 · 实时开奖</small></div></div><div className="game-clock"><small>第 {game.period} 期</small><b>{game.due.split('').map((number, index) => <i key={index}>{number}</i>)}</b></div></div><footer><span>上期 {game.period.slice(-8)}</span><div>{game.balls.map((number, index) => <b className={ballTone(number)} key={index}>{number}</b>)}</div><Icon name="arrow" /></footer></button>)}</div>
      <p className="lobby-tip">{gamesLive ? '彩种与倒计时来自后端' : gamesError ? `后端离线，使用演示数据（${gamesError}）` : '加载彩种中…'}</p>
    </section>
    {announcementOpen && <AnnouncementDialog title={announcementTitle} body={announcementBody} onClose={() => setAnnouncementOpen(false)} />}
  </>
}
