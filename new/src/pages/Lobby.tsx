import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import type { CSSProperties } from 'react'
import { BRAND_NAME } from '../data/brand'
import { ballTone } from '../data/games'
import { Icon } from '../components/Icon'
import { AnnouncementDialog } from '../components/Dialogs'
import type { Game, Theme } from '../types'
import { portalApi } from '../api/portal'
import { memberApi, type EntertainmentPlatform } from '../api/member'

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
  const [announcementTitle, setAnnouncementTitle] = useState('平台公告')
  const [announcementBody, setAnnouncementBody] = useState('')
  const [entertainment, setEntertainment] = useState<EntertainmentPlatform[]>([])
  const [entertainmentMessage, setEntertainmentMessage] = useState('')
  const [launching, setLaunching] = useState('')
  const showingEntertainment = entertainmentFilters.has(filter)
  const visibleGames = showingEntertainment ? [] : games.filter((game) => gameGroup(game) === filter)
  const visibleEntertainment = showingEntertainment ? entertainment.filter((item) => item.category === filter) : []
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

  const launchEntertainment = async (item: EntertainmentPlatform) => {
    setLaunching(item.code)
    setEntertainmentMessage('')
    try {
      const result = await memberApi.launchEntertainment(item.code)
      if (result.ready && result.launch_url) {
        window.location.assign(result.launch_url)
        return
      }
      setEntertainmentMessage(result.message || `${item.name}正在接入中`)
    } catch (reason) {
      setEntertainmentMessage(reason instanceof Error ? reason.message : `${item.name}暂时不可用`)
    } finally {
      setLaunching('')
    }
  }

  useEffect(() => {
    void portalApi.roomSettings().then((settings) => {
      const notice = settings.room_notice || `欢迎来到${BRAND_NAME}`
      setRoomNotice(`【公告】${notice}`)
      setAnnouncementBody(notice)
    }).catch(() => setRoomNotice(`【公告】欢迎来到${BRAND_NAME}`))
    void portalApi.activities('banner').then((items) => {
      const active = items.find((item) => item.status === 'active')
      if (active) {
        setAnnouncementTitle(active.title)
        setAnnouncementBody(active.subtitle || active.title)
      }
    }).catch(() => undefined)
    void memberApi.entertainment().then(setEntertainment).catch(() => setEntertainment([]))
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
      <button className="announcement" onClick={() => setAnnouncementOpen(true)}><span>●</span><p>{roomNotice}</p><Icon name="arrow" /></button>
    </header>
    <section className="lobby-body">
      <div className="lobby-category-shell"><div className="lobby-toolbar" aria-label="大厅分类" ref={categoryRailRef}>{filters.map((item) => <button aria-pressed={filter === item.id} className={filter === item.id ? 'toolbar-active' : ''} key={item.id} onClick={() => { setFilter(item.id); setEntertainmentMessage('') }}>{item.label}</button>)}</div><button className="lobby-category-next" aria-label="向右查看更多分类" onClick={() => categoryRailRef.current?.scrollBy({ left: 150, behavior: 'smooth' })}><Icon name="arrow" /></button></div>
      {!showingEntertainment && <div className="game-list">{visibleGames.map((game) => <button className="game-card" key={game.id} onClick={() => onOpenGame(game.id)}><div className="game-top" style={{ '--game': game.color } as CSSProperties}><div className="game-lead"><span className="game-logo">{game.tag.split(' ')[0].slice(0, 2)}</span><div><strong>{game.title}</strong><small>在线 {game.online} 人 · 实时开奖</small></div></div><div className="game-clock"><small>第 {game.period} 期</small><b>{game.due.split('').map((number, index) => <i key={index}>{number}</i>)}</b></div></div><footer><span>上期 {game.period.slice(-8)}</span><div>{game.balls.map((number, index) => <b className={ballTone(number)} key={index}>{number}</b>)}</div><Icon name="arrow" /></footer></button>)}</div>}
      {showingEntertainment && <div className="entertainment-list">{visibleEntertainment.map((item, index) => <button disabled={launching === item.code} key={item.code} onClick={() => void launchEntertainment(item)}><span style={{ '--entertainment-hue': `${(index * 47 + filter.length * 19) % 360}deg` } as CSSProperties}>{item.name.slice(0, 2)}</span><div><b>{item.name}</b><small>{item.status === 'enabled' ? '已接入 · 点击进入' : item.remark || '第三方线路接入中'}</small></div><em className={item.status}>{launching === item.code ? '连接中' : item.status === 'enabled' ? '进入' : '接入中'}</em><Icon name="arrow" /></button>)}{visibleEntertainment.length === 0 && <div className="entertainment-empty"><b>{filter}专区</b><small>项目正在配置，稍后会从后台自动显示。</small></div>}</div>}
      {entertainmentMessage && <p className="entertainment-message">{entertainmentMessage}</p>}
      {!showingEntertainment && <p className="lobby-tip">{gamesLive ? '彩种与倒计时来自后端' : gamesError ? `后端离线，使用演示数据（${gamesError}）` : '加载彩种中…'}</p>}
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
    {announcementOpen && <AnnouncementDialog title={announcementTitle} body={announcementBody} onClose={() => setAnnouncementOpen(false)} />}
  </>
}
