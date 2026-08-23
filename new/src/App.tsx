import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
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
import { BottomNav } from './components/BottomNav'
import { GameRoom } from './pages/GameRoom'
import { DrawResults } from './pages/DrawResults'
import { Lobby } from './pages/Lobby'
import { Wallet } from './pages/Wallet'
import { Chats } from './pages/Chats'
import { Profile } from './pages/Profile'
import { Login } from './pages/Login'
import { Register } from './pages/Register'
import { RoomEntry } from './pages/RoomEntry'
import type { Tab, Theme } from './types'
import { pathForChat, pathForGame, pathForLogin, pathForRegister, pathForResults, pathForRoom, pathForTab, pathForWallet, useAppRouter } from './router'
import { useLotteryGames } from './hooks/useLotteryGames'
import { usePersistentState } from './hooks/usePersistentState'
import { useMemberPreferences } from './hooks/useMemberPreferences'
import { useFontScale } from './hooks/useFontScale'
import { useWebSocket } from './hooks/useWebSocket'
import { clearToken, getToken } from './api/client'
import { memberApi } from './api/member'
import { portalApi } from './api/portal'

type DemoState = {
  theme: Theme
  checkedIn: boolean
  chatUnread: number
}

type Session = {
  account: string
  nickname: string
  publicId?: number
  displayName?: string
  room: string
  roomName: string
  balance: number
}

type RoomHistoryEntry = {
  code: string
  name: string
  lastUsedAt: number
}

type RoomHistoryByAccount = Record<string, RoomHistoryEntry[]>

const initialDemoState: DemoState = { theme: 'day', checkedIn: false, chatUnread: 0 }

function App() {
  const [session, setSession] = usePersistentState<Session | null>('seven-star-session', null)
  const [roomHistoryByAccount, setRoomHistoryByAccount] = usePersistentState<RoomHistoryByAccount>('seven-star-room-history', {})
  const [pendingAccount, setPendingAccount] = useState<{ account: string; nickname: string } | null>(null)
  const [demo, setDemo] = usePersistentState<DemoState>('seven-star-demo-state', initialDemoState)
  const [booting, setBooting] = useState(Boolean(getToken() && session))
  const { route, pathname, navigate } = useAppRouter()
  const { fontScale } = useMemberPreferences()
  // Every route creates a different page root (game room vs. regular app).
  // Rebind the text scaler as soon as that root is mounted.
  // A direct refresh first renders the loading shell and then replaces it
  // with the actual game room without changing the URL. Include boot state so
  // the scaler rebinds to the newly mounted game root as well.
  useFontScale(fontScale, `${pathname}:${booting ? 'booting' : 'ready'}`)
  const { games: liveGames, live: gamesLive, error: gamesError, loading: gamesLoading } = useLotteryGames()
  const appContentRef = useRef<HTMLDivElement>(null)

  const activeSession = session && session.account && session.room ? session : null
  const displayName = activeSession?.displayName || activeSession?.nickname || ''
  const rememberRoom = useCallback((account: string, code: string, name: string) => {
    if (!account || !code) return
    setRoomHistoryByAccount((current) => {
      const previous = current[account] ?? []
      const next = [
        { code, name: name || code, lastUsedAt: Date.now() },
        ...previous.filter((item) => item.code !== code),
      ].slice(0, 6)
      return { ...current, [account]: next }
    })
  }, [setRoomHistoryByAccount])

  useEffect(() => {
    if (!activeSession) return
    rememberRoom(activeSession.account, activeSession.room, activeSession.roomName)
  }, [activeSession, rememberRoom])

  useEffect(() => {
    if (!getToken()) {
      setBooting(false)
      return
    }
    let cancelled = false
    void memberApi.me().then((profile) => {
      if (cancelled) return
      if (profile.room_code) {
        setSession({
          account: profile.username,
          nickname: profile.nickname || profile.username,
          publicId: profile.public_id,
          displayName: session?.displayName,
          room: profile.room_code,
          roomName: profile.room_name || profile.room_code,
          balance: profile.balance,
        })
      } else if (session?.room) {
        setSession({
          ...session,
          account: profile.username,
          nickname: profile.nickname || profile.username,
          publicId: profile.public_id,
          balance: profile.balance,
        })
      }
      setPendingAccount({ account: profile.username, nickname: profile.nickname || profile.username })
    }).catch(() => {
      clearToken()
      setSession(null)
    }).finally(() => {
      if (!cancelled) setBooting(false)
    })
    return () => { cancelled = true }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    const onExpired = () => {
      setSession(null)
      setPendingAccount(null)
      navigate(pathForLogin())
    }
    window.addEventListener('yaotu-member-auth-expired', onExpired)
    return () => window.removeEventListener('yaotu-member-auth-expired', onExpired)
  }, [navigate])

  // All route changes represent a fresh page in this single-page app. Reset
  // the actual scrolling container, not only window, before the new view paints.
  useLayoutEffect(() => {
    appContentRef.current?.scrollTo({ top: 0, left: 0, behavior: 'auto' })
    window.scrollTo({ top: 0, left: 0, behavior: 'auto' })
  }, [pathname])

  const refreshUnread = async () => {
    if (!getToken()) return
    try {
      const { unread } = await portalApi.unreadCount()
      setDemo((current) => ({ ...current, chatUnread: unread }))
    } catch {
      /* ignore */
    }
  }

  useEffect(() => {
    if (!getToken()) return
    void refreshUnread()
    const timer = window.setInterval(() => void refreshUnread(), 30_000)
    return () => window.clearInterval(timer)
  }, [activeSession?.account])

  const activeGameId = route.kind === 'game' ? route.gameId : null
  const activeGame = useMemo(() => liveGames.find((game) => game.id === activeGameId), [activeGameId, liveGames])

  // Bookmarks and browser history may still point at a game disabled by the
  // administrator. Once the live list is known, return to the lobby instead
  // of leaving a stale game URL rendering an unrelated page.
  useEffect(() => {
    if (route.kind === 'game' && !gamesLoading && gamesLive && !activeGame) {
      navigate(pathForTab('lobby'))
    }
  }, [activeGame, gamesLive, gamesLoading, navigate, route.kind])
  const toggleTheme = () => setDemo((current) => ({ ...current, theme: current.theme === 'day' ? 'night' : 'day' }))
  const resetDemo = () => setDemo(initialDemoState)

  const refreshBalance = async () => {
    try {
      const profile = await memberApi.me()
      setSession((current) => current ? { ...current, publicId: profile.public_id, balance: profile.balance } : null)
    } catch {
      /* ignore */
    }
  }

  const switchRoom = async (roomCode: string) => {
    const result = await memberApi.joinRoom(roomCode)
    const roomName = result.room_name || result.room_code
    setSession((current) => current ? { ...current, room: result.room_code, roomName } : current)
    if (activeSession) rememberRoom(activeSession.account, result.room_code, roomName)
    void refreshBalance()
  }

  useWebSocket((event) => {
    if (event.type === 'notification') void refreshUnread()
    if (event.type === 'balance') void refreshBalance()
  }, Boolean(activeSession && getToken()))

  // A manual login always asks for a room code. Existing room bindings are
  // retained as history, but must not silently decide which room to enter.
  const continueLogin = async (account: string, nickname: string) => {
    setPendingAccount({ account, nickname })
    navigate(pathForRoom())
  }

  const logout = () => {
    clearToken()
    setSession(null)
    setPendingAccount(null)
    navigate(pathForLogin())
  }

  if (booting) {
    return <main className={`mobile-app font-scale-${fontScale}`}><div className="app-content"><p className="app-loading">加载中…</p></div></main>
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
    if (!getToken()) return <Login theme={demo.theme} onContinue={continueLogin} />
    const account = pendingAccount?.account ?? activeSession?.account
    if (!account) return <Login theme={demo.theme} onContinue={continueLogin} />
    return (
      <RoomEntry
        theme={demo.theme}
        fromLobby={Boolean(activeSession)}
        onBack={() => {
          if (activeSession) {
            navigate(pathForTab('lobby'))
            return
          }
          clearToken()
          setPendingAccount(null)
          navigate(pathForLogin())
        }}
        onEnter={(room, roomName) => {
          void memberApi.me().then((profile) => {
            setSession({
              account: profile.username,
              nickname: profile.nickname || profile.username,
              publicId: profile.public_id,
              room,
              roomName,
              balance: profile.balance,
            })
            setPendingAccount(null)
            navigate(pathForTab('lobby'))
          })
        }}
      />
    )
  }

  if (!activeSession || !getToken()) return <Login theme={demo.theme} onContinue={continueLogin} />

  if (route.kind === 'results') {
    const returnPath = route.returnGameId ? pathForGame(route.returnGameId) : pathForTab('lobby')
    return <main className={`mobile-app theme-${demo.theme} font-scale-${fontScale}`}><div ref={appContentRef} className="app-content"><DrawResults games={liveGames} initialGameId={route.gameId} onBack={() => navigate(returnPath)} /></div></main>
  }

  if (activeGame) {
    return (
      <GameRoom
        // 彩种本身就是独立会话。切换时强制重建会话组件，避免上一个
        // 彩种的本地注单回执、输入草稿或加载中的动态残留到新彩种。
        key={activeGame.id}
        game={activeGame}
        games={liveGames}
        theme={demo.theme}
        nickname={displayName}
        balance={activeSession.balance}
        onBack={() => navigate(pathForTab('lobby'))}
        onOpenGame={(gameId) => navigate(pathForGame(gameId))}
        onOpenService={() => navigate(pathForChat('service', activeGame.id))}
        onOpenWallet={(action) => navigate(pathForWallet(action, activeGame.id))}
        onOpenResults={() => navigate(pathForResults(activeGame.id, activeGame.id))}
        startWithQuickMenu={route.kind === 'game' && Boolean(route.quickMenu)}
        onRefreshBalance={refreshBalance}
      />
    )
  }

  const activeTab: Tab = route.kind === 'game' ? 'lobby' : route.tab
  const walletAction = route.kind === 'tab' && route.tab === 'shop' ? route.walletAction : undefined
  const walletReturnGameId = route.kind === 'tab' && route.tab === 'shop' ? route.returnGameId : undefined
  const showBottomNav = (route.kind !== 'chat' || route.view === 'list') && !walletAction
  const content = route.kind === 'chat'
    ? <Chats key={`${activeSession.room}:${route.view}`} view={route.view} unreadCount={demo.chatUnread} onMarkAllRead={async () => { await portalApi.markAllRead(); await refreshUnread() }} onNavigate={(view) => navigate(pathForChat(view))} onServiceBack={route.returnGameId ? () => navigate(pathForGame(route.returnGameId!)) : undefined} onRefreshUnread={() => void refreshUnread()} />
    : activeTab === 'lobby'
      ? <Lobby room={activeSession.room} roomHistory={roomHistoryByAccount[activeSession.account] ?? []} games={liveGames} theme={demo.theme} gamesLive={gamesLive} gamesError={gamesError} onToggleTheme={toggleTheme} onOpenGame={(gameId) => navigate(pathForGame(gameId))} onSwitchRoom={switchRoom} />
      : activeTab === 'shop'
        ? <Wallet balance={activeSession.balance} walletAction={walletAction} returnGameId={walletReturnGameId} onBackToGame={walletReturnGameId ? () => navigate(pathForGame(walletReturnGameId, true)) : undefined} onRefresh={() => void refreshBalance()} onNavigate={navigate} />
        : <Profile account={displayName} publicId={activeSession.publicId} balance={activeSession.balance} theme={demo.theme} onLogout={logout} onResetDemo={resetDemo} onToggleTheme={toggleTheme} onChangeNickname={async (nickname) => {
          const profile = await memberApi.updateNickname(nickname)
          setSession((current) => current ? { ...current, nickname: profile.nickname || nickname, displayName: profile.nickname || nickname, publicId: profile.public_id, balance: profile.balance } : current)
        }} />

  return (
    <main className={`mobile-app theme-${demo.theme} font-scale-${fontScale} ${showBottomNav ? 'has-bottom-nav' : ''}`}>
      <div ref={appContentRef} className="app-content">{content}</div>
      {showBottomNav && <BottomNav activeTab={activeTab} theme={demo.theme} unreadCount={demo.chatUnread} onSelect={(tab) => navigate(tab === 'shop' ? pathForWallet() : pathForTab(tab))} />}
    </main>
  )
}

export default App
