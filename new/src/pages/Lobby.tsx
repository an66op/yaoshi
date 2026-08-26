import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import type { CSSProperties } from 'react'
import { BRAND_NAME } from '../data/brand'
import { ballTone } from '../data/games'
import { Icon } from '../components/Icon'
import { AnnouncementDialog } from '../components/Dialogs'
import type { Game, Theme } from '../types'
import { portalApi } from '../api/portal'
import type { AnnouncementItem } from '../api/portal'

type Filter = '168' | 'bingo' | 'pc' | 'mark-six' | '捕鱼' | '体育' | '真人' | '电子' | '电竞'
type RoomHistoryItem = {
  code: string
  name: string
  lastUsedAt?: number
}

type Props = {
  room: string
  roomHistory: RoomHistoryItem[]
  games: Game[]
  theme: Theme
  gamesLive?: boolean
  gamesError?: string
  onOpenGame: (id: string) => void
  onToggleTheme: () => void
  onSwitchRoom: (roomCode: string) => Promise<void>
}

const filters: Array<{ id: Filter; label: string }> = [
  { id: '168', label: '彩票' },
  { id: 'bingo', label: '宾果' },
  { id: 'pc', label: 'PC' },
  { id: 'mark-six', label: '六合彩' },
  { id: '捕鱼', label: '捕鱼' },
  { id: '体育', label: '体育' },
  { id: '真人', label: '真人' },
  { id: '电子', label: '电子' },
  { id: '电竞', label: '电竞' },
]

const markSixIDs = new Set(['hong-kong-mark-six', 'happy8-mark-six', 'new-macau-mark-six', 'old-macau-mark-six'])
const entertainmentFilters = new Set<Filter>(['捕鱼', '体育', '真人', '电子', '电竞'])
const upcomingEntertainment: Record<'捕鱼' | '体育' | '真人' | '电子' | '电竞', string[]> = {
  捕鱼: ['捕鱼大厅', '捕鱼王3D'],
  体育: ['FB体育', 'IM体育'],
  真人: ['百家乐', '龙虎', '炸金花', '德州扑克'],
  电子: ['赏金船长', '麻将胡了'],
  电竞: ['王者荣耀', '炉石传说', '英雄联盟'],
}

function gameGroup(game: Game): Exclude<Filter, '捕鱼' | '体育' | '真人' | '电子' | '电竞'> {
  if (game.id.startsWith('bingo-')) return 'bingo'
  if (game.id.startsWith('pc-') || game.id.startsWith('canada-') || game.category === 'PC') return 'pc'
  if (markSixIDs.has(game.id)) return 'mark-six'
  return '168'
}

export function Lobby({ room, roomHistory, games, theme, gamesLive, gamesError, onOpenGame, onToggleTheme, onSwitchRoom }: Props) {
  const categoryRailRef = useRef<HTMLDivElement>(null)
  const [announcementOpen, setAnnouncementOpen] = useState(false)
  const [filter, setFilter] = useState<Filter>('168')
  const [roomSwitcherOpen, setRoomSwitcherOpen] = useState(false)
  const [switchingRoom, setSwitchingRoom] = useState('')
  const [roomSwitchError, setRoomSwitchError] = useState('')
  const [roomCodeInput, setRoomCodeInput] = useState('')
  const [roomNotice, setRoomNotice] = useState('【公告】加载中…')
  const [announcements, setAnnouncements] = useState<AnnouncementItem[]>([])
  const [dialogAnnouncements, setDialogAnnouncements] = useState<AnnouncementItem[]>([])
  const [entertainmentMessage, setEntertainmentMessage] = useState('')
  const showingEntertainment = entertainmentFilters.has(filter)
  const visibleGames = showingEntertainment ? [] : games.filter((game) => gameGroup(game) === filter)
  const visibleEntertainment = showingEntertainment ? upcomingEntertainment[filter as keyof typeof upcomingEntertainment] : []
  const recentRooms = [...roomHistory]
    .sort((left, right) => (right.lastUsedAt ?? 0) - (left.lastUsedAt ?? 0))
    .filter((item) => item.code !== room)
    .slice(0, 5)

  const chooseRoom = async (roomCode: string) => {
    if (roomCode === room) {
      setRoomSwitcherOpen(false)
      return
    }
    setSwitchingRoom(roomCode)
    setRoomSwitchError('')
    try {
      await onSwitchRoom(roomCode)
      setRoomSwitcherOpen(false)
    } catch (reason) {
      setRoomSwitchError(reason instanceof Error ? reason.message : '切换房间失败，请稍后再试')
    } finally {
      setSwitchingRoom('')
    }
  }

  const submitRoomCode = () => {
    const roomCode = roomCodeInput.trim()
    if (roomCode.length < 4) {
      setRoomSwitchError('请输入至少 4 位房间号')
      return
    }
    void chooseRoom(roomCode)
  }

  useEffect(() => {
    void portalApi.roomSettings().then((settings) => {
      const source = Array.isArray(settings.announcements) ? settings.announcements : []
      const active = source
        .filter(item => item.enabled)
        .sort((left, right) => left.sort_order - right.sort_order)
      const fallback: AnnouncementItem = { id: 'welcome', title: '欢迎公告', content: settings.room_notice || `欢迎来到${BRAND_NAME}`, enabled: true, popup_on_login: true, sort_order: 10 }
      const next = active.length ? active : source.length === 0 && settings.room_notice ? [fallback] : []
      setAnnouncements(next)
      setRoomNotice(next[0]?.content || '')
      const loginNotices = next.filter(item => item.popup_on_login)
      if (loginNotices.length && sessionStorage.getItem('wangzhe-login-announcements-shown') !== '1') {
        sessionStorage.setItem('wangzhe-login-announcements-shown', '1')
        setDialogAnnouncements(loginNotices)
        setAnnouncementOpen(true)
      }
    }).catch(() => {
      const fallback: AnnouncementItem = { id: 'welcome', title: '欢迎公告', content: `欢迎来到${BRAND_NAME}`, enabled: true, popup_on_login: false, sort_order: 10 }
      setAnnouncements([fallback])
      setRoomNotice(fallback.content)
    })
  }, [])

  useEffect(() => {
    if (!roomSwitcherOpen) return
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setRoomSwitcherOpen(false)
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [roomSwitcherOpen])

  return <>
    <header className="lobby-hero">
      <div className="hero-top"><span className="brand-word brand-logo"><img alt={BRAND_NAME} src="/images/wangzhe-header-logo.png" /></span><button className="room-cluster" aria-expanded={roomSwitcherOpen} aria-label={`切换房间，当前房间 ${room}`} onClick={() => { setRoomSwitchError(''); setRoomSwitcherOpen((open) => !open) }}><Icon name="room" /><b>{room}</b><Icon name="arrow" /></button><div className="hero-tools"><button className="theme-switch" onClick={onToggleTheme} aria-label="切换昼夜模式">{theme === 'day' ? '☾' : '☀'}</button></div></div>
      {announcements.length > 0 && <button className="announcement lobby-announcement" onClick={() => { setDialogAnnouncements(announcements); setAnnouncementOpen(true) }}><span className="announcement-badge">公告</span><p><b>{announcements[0]?.title || '大厅公告'}</b><small>{roomNotice}</small></p>{announcements.length > 1 && <em className="announcement-count">{announcements.length}</em>}<Icon name="arrow" /></button>}
    </header>
    <section className="lobby-body">
      <div className="lobby-category-shell"><div className="lobby-toolbar" aria-label="大厅分类" ref={categoryRailRef}>{filters.map((item) => <button aria-pressed={filter === item.id} className={filter === item.id ? 'toolbar-active' : ''} key={item.id} onClick={() => { setFilter(item.id); setEntertainmentMessage('') }}>{item.label}</button>)}</div><button className="lobby-category-next" aria-label="向右查看更多分类" onClick={() => categoryRailRef.current?.scrollBy({ left: 150, behavior: 'smooth' })}><Icon name="arrow" /></button></div>
      {!showingEntertainment && <div className="game-list">{visibleGames.map((game) => <button className="game-card" key={game.id} onClick={() => onOpenGame(game.id)}><div className="game-top" style={{ '--game': game.color } as CSSProperties}><div className="game-lead"><span className={`game-logo${game.logo ? ' has-image' : ''}${game.id === 'fly-racing' ? ' compact-source-logo' : ''}`}>{game.logo ? <img alt={`${game.title} Logo`} src={game.logo} /> : game.tag.split(' ')[0].slice(0, 2)}</span><div><strong>{game.title}</strong><small>{game.online === '—' ? '实时开奖' : `在线 ${game.online} 人 · 实时开奖`}</small></div></div><div className="game-clock"><small>第 {game.period} 期</small><b>{game.due.split('').map((number, index) => <i key={index}>{number}</i>)}</b></div></div><footer><span>上期 {game.period.slice(-8)}</span><div>{game.balls.map((number, index) => <b className={ballTone(number)} key={index}>{number}</b>)}</div><Icon name="arrow" /></footer></button>)}</div>}
      {showingEntertainment && <div className="entertainment-list">{visibleEntertainment.map((name, index) => <button key={name} onClick={() => setEntertainmentMessage(`${name}暂未开放`)}><span style={{ '--entertainment-hue': `${(index * 47 + filter.length * 19) % 360}deg` } as CSSProperties}>{name.slice(0, 2)}</span><div><b>{name}</b><small>功能准备中</small></div><em className="maintenance">未开放</em><Icon name="arrow" /></button>)}</div>}
      {entertainmentMessage && <p className="entertainment-message">{entertainmentMessage}</p>}
      {!showingEntertainment && !gamesLive && <p className="lobby-tip">{gamesError ? '连接失败，正在自动重试' : '正在加载彩种…'}</p>}
    </section>
    {roomSwitcherOpen && createPortal(
      <div className="room-switcher-layer" role="presentation" onClick={() => setRoomSwitcherOpen(false)}>
        <section className="room-switcher" role="dialog" aria-modal="true" aria-label="切换房间" onClick={(event) => event.stopPropagation()}>
          <header>
            <div><small>ROOM CENTER</small><b>切换房间</b></div>
            <button aria-label="关闭房间列表" onClick={() => setRoomSwitcherOpen(false)}>×</button>
          </header>
          <form className="room-switcher-entry room-switcher-entry-first" onSubmit={(event) => { event.preventDefault(); submitRoomCode() }}>
            <label htmlFor="room-code-input">输入房间号加入</label>
            <div><input autoComplete="off" disabled={Boolean(switchingRoom)} id="room-code-input" inputMode="numeric" maxLength={12} onChange={(event) => { setRoomCodeInput(event.target.value.replace(/\D/g, '')); setRoomSwitchError('') }} placeholder="例如 8801" value={roomCodeInput} /><button aria-label="进入房间" disabled={Boolean(switchingRoom)} type="submit">{switchingRoom === roomCodeInput ? '…' : <Icon name="arrow" />}</button></div>
          </form>
          {roomSwitchError && <small className="room-switcher-error">{roomSwitchError}</small>}
          <div className="room-switcher-recent-title"><span><b>最近进入的房间</b><small>按最近使用排序</small></span><em>{recentRooms.length} 个记录</em></div>
          {recentRooms.length > 0 ? <div className="room-switcher-list">{recentRooms.map((item, index) => <button disabled={Boolean(switchingRoom)} key={item.code} onClick={() => void chooseRoom(item.code)}><span className="room-switcher-list-icon">{index + 1}</span><span><b>{item.name}</b><small>ROOM · {item.code}</small></span>{switchingRoom === item.code ? <em>切换中…</em> : index === 0 ? <em>最新</em> : <Icon name="arrow" />}</button>)}</div> : <div className="room-switcher-empty"><span>⌁</span><p>暂无历史房间</p><small>输入房间号后，会自动保存在这里。</small></div>}
        </section>
      </div>,
      document.body,
    )}
    {announcementOpen && <AnnouncementDialog items={dialogAnnouncements.length ? dialogAnnouncements : announcements} onClose={() => setAnnouncementOpen(false)} />}
  </>
}
