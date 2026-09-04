import { useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import type { CSSProperties } from 'react'
import { BRAND_NAME } from '../data/brand'
import { ballTone } from '../data/games'
import { Icon } from '../components/Icon'
import { LotteryCountdown } from '../components/LotteryCountdown'
import { AnnouncementDialog } from '../components/Dialogs'
import type { Game, Theme } from '../types'
import { portalApi } from '../api/portal'
import type { AnnouncementItem } from '../api/portal'
import { buildRoomEntries, DEFAULT_ROOM_LOGO } from '../utils/roomHistory'
import type { RoomHistoryItem } from '../utils/roomHistory'
import { controlSurfaceProps } from '../utils/controlSurface'
import { markSixDrawBallClass, usesMarkSixDrawPresentation } from '../utils/lotteryRules'
import { gameAvailability } from '../utils/gameAvailability'

type Filter = string

type Props = {
  room: string
  roomName: string
  roomLogo?: string
  roomHistory: RoomHistoryItem[]
  games: Game[]
  theme: Theme
  gamesLive?: boolean
  gamesError?: string
  initialFilter?: string
  onFilterChange?: (filter: string) => void
  onOpenGame: (id: string, sourceFilter: string) => void
  onToggleTheme: () => void
  onSwitchRoom: (roomCode: string) => Promise<void>
}

const entertainmentFilterItems: Array<{ id: Filter; label: string }> = [
  { id: '捕鱼', label: '捕鱼' },
  { id: '体育', label: '体育' },
  { id: '真人', label: '真人' },
  { id: '电子', label: '电子' },
  { id: '电竞', label: '电竞' },
]

const entertainmentFilters = new Set<Filter>(['捕鱼', '体育', '真人', '电子', '电竞'])
const upcomingEntertainment: Record<'捕鱼' | '体育' | '真人' | '电子' | '电竞', string[]> = {
  捕鱼: ['捕鱼大厅', '捕鱼王3D'],
  体育: ['FB体育', 'IM体育'],
  真人: ['百家乐', '龙虎', '炸金花', '德州扑克'],
  电子: ['赏金船长', '麻将胡了'],
  电竞: ['王者荣耀', '炉石传说', '英雄联盟'],
}

function gameCardSubtitle(game: Game) {
  const availability = gameAvailability(game)
  if (availability) return availability.cardText
  return game.online === '—' ? '实时开奖' : `在线 ${game.online} 人 · 实时开奖`
}

export function Lobby({ room, roomName, roomLogo, roomHistory, games, theme, gamesLive, gamesError, initialFilter = '', onFilterChange, onOpenGame, onToggleTheme, onSwitchRoom }: Props) {
  const categoryRailRef = useRef<HTMLDivElement>(null)
  const roomSwitcherDialogRef = useRef<HTMLElement>(null)
  const roomSwitcherTriggerRef = useRef<HTMLButtonElement>(null)
  const roomCodeInputRef = useRef<HTMLInputElement>(null)
  const [announcementOpen, setAnnouncementOpen] = useState(false)
  const [filter, setFilter] = useState<Filter>(() => initialFilter.trim() || '彩票')
  const [roomSwitcherOpen, setRoomSwitcherOpen] = useState(false)
  const [switchingRoom, setSwitchingRoom] = useState('')
  const [roomSwitchError, setRoomSwitchError] = useState('')
  const [roomCodeInput, setRoomCodeInput] = useState('')
  const [roomNotice, setRoomNotice] = useState('【公告】加载中…')
  const [announcements, setAnnouncements] = useState<AnnouncementItem[]>([])
  const [dialogAnnouncements, setDialogAnnouncements] = useState<AnnouncementItem[]>([])
  const [entertainmentMessage, setEntertainmentMessage] = useState('')
  const lotteryFilters = useMemo(() => Array.from(new Set(games.map(game => game.lobbyCategory).filter(Boolean))).map(category => ({ id: category, label: category })), [games])
  const filters = [...lotteryFilters, ...entertainmentFilterItems]
  const showingEntertainment = entertainmentFilters.has(filter)
  const visibleGames = showingEntertainment ? [] : games.filter((game) => game.lobbyCategory === filter)
  const visibleEntertainment = showingEntertainment ? upcomingEntertainment[filter as keyof typeof upcomingEntertainment] : []
  const roomEntries = buildRoomEntries({ code: room, name: roomName || room, logo: roomLogo }, roomHistory)

  const chooseFilter = (next: Filter) => {
    setFilter(next)
    setEntertainmentMessage('')
    onFilterChange?.(next)
  }

  const chooseRoom = async (roomCode: string) => {
    if (roomCode === room) {
      setRoomSwitcherOpen(false)
      return
    }
    setSwitchingRoom(roomCode)
    setRoomSwitchError('')
    try {
      await onSwitchRoom(roomCode)
      setRoomCodeInput('')
      setRoomSwitcherOpen(false)
    } catch (reason) {
      setRoomSwitchError(reason instanceof Error ? reason.message : '切换房间失败，请稍后再试')
    } finally {
      setSwitchingRoom('')
    }
  }

  const submitRoomCode = () => {
    if (switchingRoom) return
    const roomCode = roomCodeInput.trim()
    if (!/^\d{5,12}$/.test(roomCode)) {
      setRoomSwitchError('请输入 5–12 位数字房间号')
      return
    }
    void chooseRoom(roomCode)
  }

  useEffect(() => {
    if (lotteryFilters.length === 0 || showingEntertainment || lotteryFilters.some(item => item.id === filter)) return
    chooseFilter(lotteryFilters[0].id)
  }, [filter, lotteryFilters, showingEntertainment]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    const next = initialFilter.trim() || '彩票'
    setFilter(current => current === next ? current : next)
  }, [initialFilter])

  useEffect(() => {
    let activeRequest = true
    setAnnouncements([])
    setRoomNotice('')
    void portalApi.roomSettings().then((settings) => {
      if (!activeRequest) return
      const source = Array.isArray(settings.announcements) ? settings.announcements : []
      const active = source
        .filter(item => item.enabled)
        .sort((left, right) => left.sort_order - right.sort_order)
      const fallback: AnnouncementItem = { id: 'welcome', title: '欢迎公告', content: settings.room_notice || `欢迎来到${BRAND_NAME}`, enabled: true, popup_on_login: true, sort_order: 10 }
      const next = active.length ? active : source.length === 0 && settings.room_notice ? [fallback] : []
      setAnnouncements(next)
      setRoomNotice(next[0]?.content || '')
      const loginNotices = next.filter(item => item.popup_on_login)
      const shownKey = `wangzhe-login-announcements-shown:${room}`
      if (loginNotices.length && sessionStorage.getItem(shownKey) !== '1') {
        sessionStorage.setItem(shownKey, '1')
        setDialogAnnouncements(loginNotices)
        setAnnouncementOpen(true)
      }
    }).catch(() => {
      if (!activeRequest) return
      // Announcements are business data. Keep this area empty when the API is
      // unavailable instead of presenting a locally fabricated notice.
      setAnnouncements([])
      setRoomNotice('')
    })
    return () => { activeRequest = false }
  }, [room])

  useEffect(() => {
    if (!roomSwitcherOpen) return
    const previousBodyOverflow = document.body.style.overflow
    const roomSwitcherTrigger = roomSwitcherTriggerRef.current
    document.body.style.overflow = 'hidden'
    const focusFrame = requestAnimationFrame(() => {
      const mobileLayout = window.matchMedia('(max-width: 480px), (pointer: coarse)').matches
      if (mobileLayout) {
        roomSwitcherDialogRef.current?.querySelector<HTMLButtonElement>('header > button')?.focus()
        return
      }
      roomCodeInputRef.current?.focus()
    })
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setRoomSwitcherOpen(false)
      if (event.key !== 'Tab') return
      const focusable = Array.from(roomSwitcherDialogRef.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), [tabindex]:not([tabindex="-1"])') || [])
      if (!focusable.length) return
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => {
      cancelAnimationFrame(focusFrame)
      document.body.style.overflow = previousBodyOverflow
      window.removeEventListener('keydown', closeOnEscape)
      roomSwitcherTrigger?.focus()
    }
  }, [roomSwitcherOpen])

  return <>
    <header className="lobby-hero">
      <div className="hero-top"><span className="brand-word brand-logo"><img alt={BRAND_NAME} src="/images/wangzhe-header-logo.png" /></span><button className="room-cluster" aria-expanded={roomSwitcherOpen} aria-haspopup="dialog" aria-label={`切换房间，当前房间 ${room}，${roomName}`} onClick={() => { setRoomSwitchError(''); setRoomSwitcherOpen((open) => !open) }} ref={roomSwitcherTriggerRef}><Icon name="room" /><span className="room-cluster-copy"><b>{room}</b><small>{roomName || '当前房间'}</small></span><Icon name="arrow" /></button><div className="hero-tools"><button className="theme-switch" onClick={onToggleTheme} aria-label="切换昼夜模式">{theme === 'day' ? '☾' : '☀'}</button></div></div>
      {announcements.length > 0 && <button className="announcement lobby-announcement" onClick={() => { setDialogAnnouncements(announcements); setAnnouncementOpen(true) }}><span className="announcement-badge">公告</span><p><b>{announcements[0]?.title || '大厅公告'}</b><small>{roomNotice}</small></p>{announcements.length > 1 && <em className="announcement-count">{announcements.length}</em>}<Icon name="arrow" /></button>}
    </header>
    <section className="lobby-body">
      <div className="lobby-category-shell" {...controlSurfaceProps}><div className="lobby-toolbar" aria-label="大厅分类" ref={categoryRailRef}>{filters.map((item) => <button aria-pressed={filter === item.id} className={filter === item.id ? 'toolbar-active' : ''} key={item.id} onClick={() => chooseFilter(item.id)}>{item.label}</button>)}</div><button className="lobby-category-next" aria-label="向右查看更多分类" onClick={() => categoryRailRef.current?.scrollBy({ left: 150, behavior: 'smooth' })}><Icon name="arrow" /></button></div>
      {!showingEntertainment && <div className="game-list">{visibleGames.map((game) => <button className="game-card" key={game.id} onClick={() => onOpenGame(game.id, filter)}><div className="game-top" style={{ '--game': game.color } as CSSProperties}><div className="game-lead"><span className={`game-logo${game.logo ? ' has-image' : ''}`}>{game.logo ? <img alt={`${game.title} Logo`} src={game.logo} /> : game.tag.split(' ')[0].slice(0, 2)}</span><div><strong>{game.title}</strong><small>{gameCardSubtitle(game)}</small></div></div><div className="game-clock"><small>第 {game.period} 期</small><LotteryCountdown timing={game.timing} /></div></div><footer><span>上期 {game.latestIssue.slice(-8)}</span><div>{game.balls.map((number, index) => <b className={usesMarkSixDrawPresentation(game.id) ? markSixDrawBallClass(number, index, game.balls.length) : ballTone(number)} key={index}>{number}</b>)}</div><Icon name="arrow" /></footer></button>)}</div>}
      {showingEntertainment && <div className="entertainment-list">{visibleEntertainment.map((name, index) => <button key={name} onClick={() => setEntertainmentMessage(`${name}暂未开放`)}><span style={{ '--entertainment-hue': `${(index * 47 + filter.length * 19) % 360}deg` } as CSSProperties}>{name.slice(0, 2)}</span><div><b>{name}</b><small>功能准备中</small></div><em className="maintenance">未开放</em><Icon name="arrow" /></button>)}</div>}
      {entertainmentMessage && <p className="entertainment-message">{entertainmentMessage}</p>}
      {!showingEntertainment && !gamesLive && <p className="lobby-tip">{gamesError ? '连接失败，正在自动重试' : '正在加载彩种…'}</p>}
    </section>
    {roomSwitcherOpen && createPortal(
      <div className={`room-switcher-layer theme-${theme}`} role="presentation" onClick={() => setRoomSwitcherOpen(false)}>
        <section aria-labelledby="room-switcher-title" className="room-switcher" ref={roomSwitcherDialogRef} role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()}>
          <header>
            <div><small>ROOM CENTER</small><b id="room-switcher-title">切换房间</b><em>当前 {room} · {roomName || room}</em></div>
            <button aria-label="关闭房间列表" onClick={() => setRoomSwitcherOpen(false)} type="button">×</button>
          </header>
          <form className="room-switcher-entry" noValidate onSubmit={(event) => { event.preventDefault(); submitRoomCode() }}>
            <label htmlFor="room-code-input">输入其他房间号</label>
            <div className="room-switcher-entry-row">
              <input
                aria-describedby={roomSwitchError ? 'room-code-error' : undefined}
                aria-busy={Boolean(switchingRoom)}
                aria-invalid={Boolean(roomSwitchError) || undefined}
                autoComplete="off"
                enterKeyHint="go"
                id="room-code-input"
                inputMode="numeric"
                maxLength={12}
                minLength={5}
                onChange={(event) => { setRoomCodeInput(event.currentTarget.value.replace(/\D/g, '').slice(0, 12)); setRoomSwitchError('') }}
                pattern="[0-9]{5,12}"
                placeholder="请输入 5–12 位房间号"
                readOnly={Boolean(switchingRoom)}
                ref={roomCodeInputRef}
                spellCheck={false}
                type="text"
                value={roomCodeInput}
              />
              <button aria-busy={Boolean(switchingRoom)} aria-label={switchingRoom ? '正在进入房间' : '进入输入的房间'} className="room-switcher-submit" disabled={Boolean(switchingRoom)} type="submit">
                {switchingRoom && switchingRoom === roomCodeInput ? <span aria-hidden="true" className="room-switcher-submit-loading" /> : <Icon name="arrow" />}
              </button>
            </div>
          </form>
          {roomSwitchError && <small className="room-switcher-error" id="room-code-error" role="alert">{roomSwitchError}</small>}
          <div className="room-switcher-recent-title"><span><b>我的房间</b><small>当前房间置顶 · 最近进入排序</small></span><em>{roomEntries.length} 个房间</em></div>
          <div className="room-switcher-list" role="group" aria-label="我的房间" {...controlSurfaceProps}>{roomEntries.map((item, index) => {
            const disabled = item.status === 'disabled'
            const statusLabel = item.status === 'current' ? '当前' : item.status === 'pending' ? '待审核' : item.status === 'disabled' ? '已停用' : index === 1 ? '最近' : ''
            return <button type="button" aria-label={`${item.name}，房间 ${item.code}${statusLabel ? `，${statusLabel}` : ''}`} aria-current={item.status === 'current' ? 'true' : undefined} className={`room-switcher-list-${item.status}`} disabled={Boolean(switchingRoom) || disabled} key={item.code} onClick={() => void chooseRoom(item.code)} title={`${item.name} · ${item.code}`}>
              <span className="room-switcher-list-icon has-image"><img alt="" src={item.logo} draggable={false} onError={event => { if (event.currentTarget.getAttribute('src') !== DEFAULT_ROOM_LOGO) event.currentTarget.src = DEFAULT_ROOM_LOGO }} /></span>
              <span><b>{item.name}</b><small>ROOM · {item.code}</small></span>
              {switchingRoom === item.code ? <em>切换中…</em> : statusLabel ? <em className={`room-switcher-status-${item.status}`}>{statusLabel}</em> : <Icon name="arrow" />}
            </button>
          })}</div>
        </section>
      </div>,
      document.body,
    )}
    {announcementOpen && <AnnouncementDialog items={dialogAnnouncements.length ? dialogAnnouncements : announcements} onClose={() => setAnnouncementOpen(false)} />}
  </>
}
