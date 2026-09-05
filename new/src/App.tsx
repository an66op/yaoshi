import { lazy, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import './App.css'
import './overrides.css'
import './game-polish.css'
import './themes.css'
import './lobby-polish.css'
import './packet-polish.css'
import './check-in.css'
import './profile.css'
import './day-fixes.css'
import './chat.css'
import './game-room.css'
import './sound-settings.css'
import './sound-settings-v2.css'
import './login.css'
import './room-entry.css'
import './avatar.css'
import './wallet.css'
import './night-polish.css'
import './appearance.css'
import './typography.css'
import './startup.css'
import './control-surface.css'
import './game-guide.css'
// Shared result-ball styling must load after the generic `.lottery-ball` rules.
import './components/mark-six-ball.css'
import { BottomNav } from './components/BottomNav'
import { SessionStartup } from './components/SessionStartup'
import { RouteChunkBoundary } from './components/RouteChunkBoundary'
import { Login } from './pages/Login'
import { Register } from './pages/Register'
import { RoomEntry } from './pages/RoomEntry'
import { stopNotificationSounds } from './utils/notificationAudio'
import type { Tab, Theme } from './types'
import { pathForChat, pathForGame, pathForGameGuide, pathForLobby, pathForLogin, pathForPlanGame, pathForRegister, pathForResults, pathForRoom, pathForTab, pathForWallet, useAppRouter } from './router'
import { useLotteryGames } from './hooks/useLotteryGames'
import { usePersistentState } from './hooks/usePersistentState'
import { useMemberPreferences } from './hooks/useMemberPreferences'
import { useFontScale } from './hooks/useFontScale'
import { useWebSocket, useWebSocketConnected } from './hooks/useWebSocket'
import { AuthError } from './api/client'
import { memberApi } from './api/member'
import { portalApi } from './api/portal'
import { broadcastMemberLogout, clearLoginAnnouncementMarkers, clearMemberBusinessStorage, MEMBER_AUTH_EVENT_KEY, MEMBER_SESSION_KEY } from './utils/businessStorage'
import {
  activePromotionTitles,
  configuredHiddenMessageRows,
  serverCountedUnreadNotificationCount,
  visibleUnreadNotificationCount,
} from './utils/notificationVisibility'

// Authentication and room entry stay in the initial bundle so a cold start can
// paint immediately. The heavier authenticated routes are fetched only when
// navigation reaches them; named exports are adapted to React.lazy's default
// component contract without adding forwarding wrapper components.
const GameRoom = lazy(() => import('./pages/GameRoom').then(module => ({ default: module.GameRoom })))
const DrawResults = lazy(() => import('./pages/DrawResults').then(module => ({ default: module.DrawResults })))
const Lobby = lazy(() => import('./pages/Lobby').then(module => ({ default: module.Lobby })))
const Wallet = lazy(() => import('./pages/Wallet').then(module => ({ default: module.Wallet })))
const Chats = lazy(() => import('./pages/Chats').then(module => ({ default: module.Chats })))
const Profile = lazy(() => import('./pages/Profile').then(module => ({ default: module.Profile })))
const GameGuidePage = lazy(() => import('./pages/GameGuidePage').then(module => ({ default: module.GameGuidePage })))

type DemoState = {
  theme: Theme
  checkedIn: boolean
  chatUnread: number
}

type Session = {
  account: string
  nickname: string
  publicId?: number
  avatar?: string
  publicTitle?: string
  badge?: string
  room: string
  roomName: string
  roomLogo?: string
  balance: number
}

type RoomHistoryEntry = {
  code: string
  name: string
  logo?: string
  status: 'current' | 'available' | 'pending' | 'disabled'
  lastUsedAt: number
}

const initialDemoState: DemoState = { theme: 'day', checkedIn: false, chatUnread: 0 }

async function loadNotificationSnapshot(expectedUnread: number) {
  let page = await portalApi.notifications(50)
  const items = [...page.items]
  let countedUnread = serverCountedUnreadNotificationCount(items)

  // The unread count endpoint has no visibility/category context. Page until
  // every item included in that count has been observed, including unread rows
  // that may sit behind a long run of already-read history.
  while (page.has_more && countedUnread < expectedUnread) {
    const beforeID = page.next_before_id ?? page.items.at(-1)?.id
    if (!beforeID) break
    page = await portalApi.notifications(50, { before_id: beforeID })
    if (!page.items.length) break
    items.push(...page.items)
    countedUnread += serverCountedUnreadNotificationCount(page.items)
  }
  return items
}

function App() {
  const [session, setSession] = usePersistentState<Session | null>('seven-star-session', null)
  const [roomHistory, setRoomHistory] = useState<RoomHistoryEntry[]>([])
  const [pendingAccount, setPendingAccount] = useState<{ account: string; nickname: string } | null>(null)
  const [demo, setDemo] = usePersistentState<DemoState>('seven-star-demo-state', initialDemoState)
  // Unread state belongs to the authenticated inbox, not to display
  // preferences. Keeping it out of localStorage prevents a failed refresh from
  // resurrecting yesterday's badge after login or a room switch.
  const [chatUnread, setChatUnread] = useState(0)
  const [authenticated, setAuthenticated] = useState(false)
  const [booting, setBooting] = useState(true)
  const [bootError, setBootError] = useState('')
  const [logoutError, setLogoutError] = useState('')
  const [loggingOut, setLoggingOut] = useState(false)
  const [lastLobbyFilter, setLastLobbyFilter] = useState('彩票')
  const websocketConnected = useWebSocketConnected()
  const { route, pathname, navigate, replace } = useAppRouter()
  const { fontScale, displayStyle } = useMemberPreferences()
  const routedLobbyFilter = route.kind === 'tab'
    ? (route.tab === 'lobby' ? route.lobbyFilter : route.returnLobbyFilter)
    : route.kind === 'game' || route.kind === 'chat' || route.kind === 'results'
      ? route.returnLobbyFilter
      : undefined
  useEffect(() => {
    if (routedLobbyFilter?.trim()) setLastLobbyFilter(routedLobbyFilter.trim())
  }, [routedLobbyFilter])
  useLayoutEffect(() => {
    document.documentElement.dataset.memberDisplay = displayStyle
    return () => { delete document.documentElement.dataset.memberDisplay }
  }, [displayStyle])
  // Every route creates a different page root (game room vs. regular app).
  // Rebind the text scaler as soon as that root is mounted.
  // A direct refresh first renders the loading shell and then replaces it
  // with the actual game room without changing the URL. Include boot state so
  // the scaler rebinds to the newly mounted game root as well.
  useFontScale(fontScale, `${pathname}:${booting ? 'booting' : 'ready'}`)
  const activeSession = session && session.account && session.room ? session : null
  const { games: liveGames, live: gamesLive, error: gamesError, loading: gamesLoading } = useLotteryGames(Boolean(authenticated && activeSession), activeSession?.room ?? '', route.kind === 'game' ? route.gameId : null)
  const appContentRef = useRef<HTMLDivElement>(null)
  const unreadRefreshIDRef = useRef(0)

  const displayName = activeSession?.nickname || activeSession?.account || ''
  const historyAccount = pendingAccount?.account ?? activeSession?.account ?? ''
  const historyRoom = activeSession?.room ?? ''

  useEffect(() => {
    if (!authenticated || !historyAccount) {
      setRoomHistory([])
      return
    }
    let cancelled = false
    void memberApi.roomHistory().then((items) => {
      if (cancelled) return
      setRoomHistory(items.map((item) => ({
        code: item.room_code,
        name: item.room_name || item.room_code,
        logo: item.room_logo,
        status: item.status,
        lastUsedAt: Date.parse(item.last_entered_at) || 0,
      })))
    }).catch(() => {
      if (!cancelled) setRoomHistory([])
    })
    return () => { cancelled = true }
  }, [authenticated, historyAccount, historyRoom])

  useEffect(() => {
    let cancelled = false
    void memberApi.me().then((profile) => {
      if (cancelled) return
	  setAuthenticated(true)
      setBootError('')
      if (profile.room_code) {
        setSession({
          account: profile.username,
          nickname: profile.nickname || profile.username,
          publicId: profile.public_id,
          avatar: profile.avatar,
          publicTitle: profile.public_title,
          badge: profile.badge,
          room: profile.room_code,
          roomName: profile.room_name || profile.room_code,
          roomLogo: profile.room_logo,
          balance: profile.balance,
        })
      } else if (session?.room) {
        setSession({
          ...session,
          account: profile.username,
          nickname: profile.nickname || profile.username,
          publicId: profile.public_id,
          avatar: profile.avatar,
          publicTitle: profile.public_title,
          badge: profile.badge,
          roomLogo: profile.room_logo || session.roomLogo,
          balance: profile.balance,
        })
      }
      setPendingAccount({ account: profile.username, nickname: profile.nickname || profile.username })
	  void memberApi.refreshSession().catch(() => undefined)
    }).catch((reason) => {
      // request() already emits the auth-expired event for a genuine 401.
      // A timeout or temporary 5xx must not destroy an otherwise valid local
      // session and force the member through login and room selection again.
      if (reason instanceof AuthError) {
		setAuthenticated(false)
        setChatUnread(0)
        clearMemberBusinessStorage()
        setSession(null)
        setRoomHistory([])
        setDemo((current) => ({ ...current, checkedIn: false, chatUnread: 0 }))
      } else if (!cancelled) {
        // The persisted session is only a navigation aid. Never render its
        // cached balance/room as current business data when verification fails.
        setBootError(reason instanceof Error ? reason.message : '账号信息暂时无法读取')
      }
    }).finally(() => {
      if (!cancelled) setBooting(false)
    })
    return () => { cancelled = true }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    const onExpired = () => {
	  setAuthenticated(false)
      setChatUnread(0)
      clearMemberBusinessStorage()
      setSession(null)
      setRoomHistory([])
      setPendingAccount(null)
      setDemo((current) => ({ ...current, checkedIn: false, chatUnread: 0 }))
      // Login and registration are public routes. An expected 401 from the
      // boot-time /me probe must not break a direct registration/invite link.
      if (route.kind !== 'login' && route.kind !== 'register') navigate(pathForLogin())
    }
    window.addEventListener('yaotu-member-auth-expired', onExpired)
    return () => window.removeEventListener('yaotu-member-auth-expired', onExpired)
  }, [navigate, route.kind, setDemo, setSession])

  useEffect(() => {
    const onStorage = (event: StorageEvent) => {
	  // The HttpOnly cookie cannot be read from JavaScript. The non-sensitive
	  // room-session key is only a cross-tab logout signal, never authority.
      const legacySessionRemoved = event.key === MEMBER_SESSION_KEY && (event.newValue === null || event.newValue === 'null')
      if (event.key !== MEMBER_AUTH_EVENT_KEY && !legacySessionRemoved) return
	  setAuthenticated(false)
      setChatUnread(0)
      clearMemberBusinessStorage()
      setSession(null)
      setRoomHistory([])
      setPendingAccount(null)
      setDemo((current) => ({ ...current, checkedIn: false, chatUnread: 0 }))
      navigate(pathForLogin())
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [navigate, setDemo, setSession])

  // All route changes represent a fresh page in this single-page app. Reset
  // the actual scrolling container, not only window, before the new view paints.
  useLayoutEffect(() => {
    stopNotificationSounds()
    appContentRef.current?.scrollTo({ top: 0, left: 0, behavior: 'auto' })
    window.scrollTo({ top: 0, left: 0, behavior: 'auto' })
  }, [pathname])

  const refreshUnread = useCallback(async () => {
	if (!authenticated) {
      setChatUnread(0)
      return
    }
    const refreshID = ++unreadRefreshIDRef.current
    try {
      const [{ unread }, settingsResult, activitiesResult] = await Promise.all([
        portalApi.unreadCount(),
        portalApi.roomSettings().then((value) => ({ ok: true as const, value })).catch(() => ({ ok: false as const })),
        portalApi.activities().then((value) => ({ ok: true as const, value })).catch(() => ({ ok: false as const })),
      ])
      if (refreshID !== unreadRefreshIDRef.current) return
      if (unread <= 0) {
        setChatUnread(0)
        return
      }

      const notifications = await loadNotificationSnapshot(unread)
      if (refreshID !== unreadRefreshIDRef.current) return
      const hiddenRows = configuredHiddenMessageRows(settingsResult.ok ? settingsResult.value.game : undefined)
      const promotionTitles = activePromotionTitles(activitiesResult.ok ? activitiesResult.value : [])
      setChatUnread(visibleUnreadNotificationCount(notifications, hiddenRows, promotionTitles))
    } catch {
      // Keep the last verified in-memory value during a transient failure. It
      // is reset separately whenever the account or room context changes.
    }
	}, [authenticated])

  useEffect(() => {
    unreadRefreshIDRef.current += 1
    setChatUnread(0)
  }, [activeSession?.account, activeSession?.room])

  useEffect(() => {
	if (!authenticated) return
    void refreshUnread()
    if (websocketConnected) return
    const timer = window.setInterval(() => void refreshUnread(), 30_000)
    return () => window.clearInterval(timer)
	}, [activeSession?.account, authenticated, refreshUnread, websocketConnected])

  const activeGameId = route.kind === 'game' ? route.gameId : null
  const activeGame = useMemo(() => liveGames.find((game) => game.id === activeGameId), [activeGameId, liveGames])
  const gameReturnLobbyFilter = route.kind === 'game' ? route.returnLobbyFilter : undefined

  // Bookmarks and browser history may still point at a game disabled by the
  // administrator. Once the live list is known, return to the lobby instead
  // of leaving a stale game URL rendering an unrelated page.
  useEffect(() => {
    if (route.kind === 'game' && !gamesLoading && gamesLive && !activeGame) {
      navigate(pathForLobby(gameReturnLobbyFilter))
    }
  }, [activeGame, gameReturnLobbyFilter, gamesLive, gamesLoading, navigate, route.kind])
  const toggleTheme = () => setDemo((current) => ({ ...current, theme: current.theme === 'day' ? 'night' : 'day' }))
  const resetDemo = () => {
    setDemo(initialDemoState)
    setChatUnread(0)
  }

  const refreshBalance = async () => {
    try {
      const profile = await memberApi.me()
      setSession((current) => current ? {
        ...current,
        account: profile.username,
        nickname: profile.nickname || profile.username,
        publicId: profile.public_id,
        avatar: profile.avatar,
        publicTitle: profile.public_title,
        badge: profile.badge,
        room: profile.room_code || current.room,
        roomName: profile.room_name || current.roomName,
        roomLogo: profile.room_logo || current.roomLogo,
        balance: profile.balance,
      } : null)
    } catch {
      /* ignore */
    }
  }

  const switchRoom = async (roomCode: string) => {
    const result = await memberApi.joinRoom(roomCode)
    if (result.status === 'pending') {
      setRoomHistory((current) => [
        { code: result.room_code, name: result.room_name || result.room_code, logo: result.room_logo, status: 'pending' as const, lastUsedAt: Date.now() },
        ...current.filter((item) => item.code !== result.room_code),
      ].slice(0, 8))
      throw new Error(`入房申请已提交（编号 ${result.application_id ?? '—'}），请等待审核`)
    }
    const roomName = result.room_name || result.room_code
    setSession((current) => current ? { ...current, room: result.room_code, roomName, roomLogo: result.room_logo } : current)
    setRoomHistory((current) => [
      { code: result.room_code, name: roomName, logo: result.room_logo, status: 'current' as const, lastUsedAt: Date.now() },
      ...current.filter((item) => item.code !== result.room_code).map((item) => item.status === 'current' ? { ...item, status: 'available' as const } : item),
    ].slice(0, 8))
    void refreshBalance()
  }

  useWebSocket((event) => {
    if (event.type === 'notification') void refreshUnread()
    if (event.type === 'balance') void refreshBalance()
	}, Boolean(activeSession && authenticated))

  // A manual login always asks for a room code. Existing room bindings are
  // retained as history, but must not silently decide which room to enter.
  const continueLogin = async (account: string, nickname: string) => {
	setAuthenticated(true)
    clearLoginAnnouncementMarkers()
    setPendingAccount({ account, nickname })
    navigate(pathForRoom())
  }

  const logout = async () => {
    if (loggingOut) return
    setLoggingOut(true)
    setLogoutError('')
    try {
      await memberApi.logout()
    } catch (reason) {
      // A 401 means the cookie is no longer accepted. Network/5xx failures
      // keep the current UI authenticated because JavaScript cannot remove an
      // HttpOnly cookie and must not claim that logout succeeded.
      if (!(reason instanceof AuthError)) {
        setLogoutError('退出未完成，当前登录仍然有效，请检查网络后重试')
        setLoggingOut(false)
        return
      }
    }
	setAuthenticated(false)
    setChatUnread(0)
    broadcastMemberLogout()
    setSession(null)
    setRoomHistory([])
    setPendingAccount(null)
    setDemo((current) => ({ ...current, checkedIn: false, chatUnread: 0 }))
    navigate(pathForLogin())
    setLoggingOut(false)
  }

  if (booting) {
    // Public sign-in can paint immediately. Keep submission gated until /me
    // finishes; private routes show no persisted balance, room or messages.
    if (route.kind === 'login') return <Login theme={demo.theme} verificationPending onContinue={continueLogin} onRegister={() => navigate(pathForRegister())} />
    return <SessionStartup theme={demo.theme} />
  }

	if (bootError && session) {
    return <SessionStartup theme={demo.theme} error={logoutError || bootError} onRetry={() => window.location.reload()} onLogout={() => void logout()} loggingOut={loggingOut} />
  }

  if (route.kind === 'login') return <Login theme={demo.theme} onContinue={(account, nickname) => void continueLogin(account, nickname)} onRegister={() => navigate(pathForRegister())} />

  if (route.kind === 'register') return (
    <Register
      theme={demo.theme}
      onBack={() => navigate(pathForLogin())}
      onContinue={(account, nickname) => void continueLogin(account, nickname)}
    />
  )

  if (route.kind === 'room') {
	if (!authenticated) return <Login theme={demo.theme} onContinue={continueLogin} />
    const account = pendingAccount?.account ?? activeSession?.account
    if (!account) return <Login theme={demo.theme} onContinue={continueLogin} />
    return (
      <RoomEntry
        theme={demo.theme}
        fromLobby={Boolean(activeSession)}
        roomHistory={roomHistory}
        onBack={() => {
          if (activeSession) {
            navigate(pathForTab('lobby'))
            return
          }
		  void logout()
        }}
        onEnter={(room, roomName) => {
          void memberApi.me().then((profile) => {
            setSession({
              account: profile.username,
              nickname: profile.nickname || profile.username,
              publicId: profile.public_id,
              avatar: profile.avatar,
              publicTitle: profile.public_title,
              badge: profile.badge,
              room,
              roomName,
              roomLogo: profile.room_logo,
              balance: profile.balance,
            })
            setPendingAccount(null)
            navigate(pathForTab('lobby'))
          })
        }}
      />
    )
  }

  if (!activeSession || !authenticated) return <Login theme={demo.theme} onContinue={continueLogin} />

  if (route.kind === 'game-guide') {
    return <main className={`mobile-app theme-${demo.theme} font-scale-${fontScale}`}><div ref={appContentRef} className="app-content"><RouteChunkBoundary resetKey={pathname} theme={demo.theme}><GameGuidePage games={liveGames} initialTab={route.tab} onBack={() => replace(pathForTab('profile'))} /></RouteChunkBoundary></div></main>
  }

  if (route.kind === 'results') {
    const returnPath = route.returnGameId ? pathForGame(route.returnGameId, false, route.returnLobbyFilter) : pathForLobby(route.returnLobbyFilter)
    return <main className={`mobile-app theme-${demo.theme} font-scale-${fontScale}`}><div ref={appContentRef} className="app-content"><RouteChunkBoundary resetKey={pathname} theme={demo.theme}><DrawResults games={liveGames} initialGameId={route.gameId} onBack={() => navigate(returnPath)} onSelectGame={(gameId) => replace(pathForResults(gameId, route.returnGameId, route.returnLobbyFilter))} /></RouteChunkBoundary></div></main>
  }

  if (activeGame) {
    return (
      <RouteChunkBoundary resetKey={pathname} theme={demo.theme}>
        <GameRoom
          // 彩种本身就是独立会话。切换时强制重建会话组件，避免上一个
          // 彩种的本地注单回执、输入草稿或加载中的动态残留到新彩种。
          key={activeGame.id}
          game={activeGame}
          games={liveGames}
          theme={demo.theme}
          nickname={displayName}
          balance={activeSession.balance}
          onBack={() => replace(pathForLobby(gameReturnLobbyFilter))}
          onOpenGame={(gameId) => navigate(pathForGame(gameId, false, gameReturnLobbyFilter))}
          onOpenService={() => navigate(pathForChat('service', activeGame.id, gameReturnLobbyFilter))}
          onOpenWallet={(action) => navigate(pathForWallet(action, activeGame.id, gameReturnLobbyFilter))}
          onOpenResults={() => navigate(pathForResults(activeGame.id, activeGame.id, gameReturnLobbyFilter))}
          startWithQuickMenu={route.kind === 'game' && Boolean(route.quickMenu)}
          onRefreshBalance={refreshBalance}
        />
      </RouteChunkBoundary>
    )
  }

  const activeTab: Tab = route.kind === 'game' ? 'lobby' : route.tab
  const walletAction = route.kind === 'tab' && route.tab === 'shop' ? route.walletAction : undefined
  const walletReturnGameId = route.kind === 'tab' && route.tab === 'shop' ? route.returnGameId : undefined
  const showBottomNav = (route.kind !== 'chat' || route.view === 'list') && !walletAction
  const content = route.kind === 'chat'
    ? <Chats key={`${activeSession.room}:${route.view}:${route.planGameId ?? ''}`} view={route.view} unreadCount={chatUnread} onNavigate={(view) => navigate(pathForChat(view))} onServiceBack={route.returnGameId ? () => navigate(pathForGame(route.returnGameId!, false, route.returnLobbyFilter)) : undefined} onRefreshUnread={refreshUnread} games={liveGames} planGameId={route.planGameId} onOpenPlanGame={(gameId) => navigate(pathForPlanGame(gameId))} />
    : activeTab === 'lobby'
      ? <Lobby room={activeSession.room} roomName={activeSession.roomName} roomLogo={activeSession.roomLogo} roomHistory={roomHistory} games={liveGames} theme={demo.theme} gamesLive={gamesLive} gamesError={gamesError} initialFilter={route.kind === 'tab' ? route.lobbyFilter ?? lastLobbyFilter : lastLobbyFilter} onFilterChange={(filter) => { setLastLobbyFilter(filter); replace(pathForLobby(filter)) }} onToggleTheme={toggleTheme} onOpenGame={(gameId, sourceFilter) => { setLastLobbyFilter(sourceFilter); navigate(pathForGame(gameId, false, sourceFilter)) }} onSwitchRoom={switchRoom} />
      : activeTab === 'shop'
        ? <Wallet balance={activeSession.balance} walletAction={walletAction} returnGameId={walletReturnGameId} onBackToGame={walletReturnGameId ? () => navigate(pathForGame(walletReturnGameId, true, route.kind === 'tab' ? route.returnLobbyFilter : undefined)) : undefined} onRefresh={() => void refreshBalance()} onNavigate={navigate} />
        : <Profile account={displayName} publicId={activeSession.publicId} balance={activeSession.balance} avatarUrl={activeSession.avatar} publicTitle={activeSession.publicTitle} badge={activeSession.badge} theme={demo.theme} onLogout={logout} logoutError={logoutError} loggingOut={loggingOut} onResetDemo={resetDemo} onToggleTheme={toggleTheme} onOpenGuide={(tab) => navigate(pathForGameGuide(tab))} onChangeAvatar={async (avatar) => {
          const profile = await memberApi.updateAvatar(avatar)
          setSession((current) => current ? { ...current, avatar: profile.avatar || avatar, publicTitle: profile.public_title, badge: profile.badge, publicId: profile.public_id, balance: profile.balance } : current)
        }} onChangeNickname={async (nickname) => {
          const profile = await memberApi.updateNickname(nickname)
          setSession((current) => current ? { ...current, nickname: profile.nickname || nickname, avatar: profile.avatar || current.avatar, publicTitle: profile.public_title, badge: profile.badge, publicId: profile.public_id, balance: profile.balance } : current)
        }} />

  return (
    <main className={`mobile-app theme-${demo.theme} font-scale-${fontScale} ${showBottomNav ? 'has-bottom-nav' : ''}`}>
      <div ref={appContentRef} className="app-content"><RouteChunkBoundary resetKey={pathname} theme={demo.theme}>{content}</RouteChunkBoundary></div>
      {showBottomNav && <BottomNav activeTab={activeTab} theme={demo.theme} unreadCount={chatUnread} onSelect={(tab) => navigate(tab === 'shop' ? pathForWallet() : tab === 'lobby' ? pathForLobby(lastLobbyFilter) : pathForTab(tab))} />}
    </main>
  )
}

export default App
