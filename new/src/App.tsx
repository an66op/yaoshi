import { useEffect, useMemo, useRef, useState } from 'react'
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
import './login.css'
import './room-entry.css'
import './avatar.css'
import './wallet.css'
import './night-polish.css'
import { BottomNav } from './components/BottomNav'
import { GameRoom } from './pages/GameRoom'
import { Lobby } from './pages/Lobby'
import { Wallet } from './pages/Wallet'
import { Chats } from './pages/Chats'
import { Profile } from './pages/Profile'
import { Login } from './pages/Login'
import { Register } from './pages/Register'
import { RoomEntry } from './pages/RoomEntry'
import type { Tab, Theme } from './types'
import { pathForChat, pathForGame, pathForLogin, pathForRegister, pathForRoom, pathForTab, useAppRouter } from './router'
import { useLotteryGames } from './hooks/useLotteryGames'
import { usePersistentState } from './hooks/usePersistentState'
import { useWebSocket } from './hooks/useWebSocket'
import { clearToken, getToken } from './api/client'
import { memberApi } from './api/member'
import { portalApi } from './api/portal'
import { playNotificationSound } from './utils/notificationAudio'

type DemoState = {
  theme: Theme
  checkedIn: boolean
  chatUnread: number
}

type Session = {
  account: string
  nickname: string
  room: string
  roomName: string
  balance: number
}

const initialDemoState: DemoState = { theme: 'day', checkedIn: false, chatUnread: 0 }

function App() {
  const [session, setSession] = usePersistentState<Session | null>('seven-star-session', null)
  const [pendingAccount, setPendingAccount] = useState<{ account: string; nickname: string } | null>(null)
  const [demo, setDemo] = usePersistentState<DemoState>('seven-star-demo-state', initialDemoState)
  const [booting, setBooting] = useState(Boolean(getToken() && session))
  const { route, navigate } = useAppRouter()
  const { games: liveGames, live: gamesLive, error: gamesError } = useLotteryGames()

  const activeSession = session && session.account && session.room ? session : null

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
          room: profile.room_code,
          roomName: profile.room_name || profile.room_code,
          balance: profile.balance,
        })
      } else if (session?.room) {
        setSession({
          ...session,
          account: profile.username,
          nickname: profile.nickname || profile.username,
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

  const prevUnreadRef = useRef(demo.chatUnread)

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
    if (demo.chatUnread <= prevUnreadRef.current) {
      prevUnreadRef.current = demo.chatUnread
      return
    }
    void portalApi.notifications(1).then((rows) => {
      const latest = rows[0]
      const title = latest?.title ?? ''
      if (/中奖|开奖|未中奖/.test(title)) playNotificationSound('lottery')
      else if (/奖励|红包|签到|邀请/.test(title)) playNotificationSound('reward')
      else if (latest?.category === 'system') playNotificationSound('announcement')
      else playNotificationSound('message')
    }).catch(() => playNotificationSound('message'))
    prevUnreadRef.current = demo.chatUnread
  }, [demo.chatUnread])

  useEffect(() => {
    if (!getToken()) return
    void refreshUnread()
    const timer = window.setInterval(() => void refreshUnread(), 30_000)
    return () => window.clearInterval(timer)
  }, [activeSession?.account])

  const activeGameId = route.kind === 'game' ? route.gameId : null
  const activeGame = useMemo(() => liveGames.find((game) => game.id === activeGameId), [activeGameId, liveGames])
  const toggleTheme = () => setDemo((current) => ({ ...current, theme: current.theme === 'day' ? 'night' : 'day' }))
  const resetDemo = () => setDemo(initialDemoState)

  const refreshBalance = async () => {
    try {
      const profile = await memberApi.me()
      setSession((current) => current ? { ...current, balance: profile.balance } : null)
    } catch {
      /* ignore */
    }
  }

  useWebSocket((event) => {
    if (event.type === 'notification') void refreshUnread()
    if (event.type === 'balance') void refreshBalance()
  }, Boolean(activeSession && getToken()))

  const continueLogin = async (account: string, nickname: string) => {
    setPendingAccount({ account, nickname })
    try {
      const profile = await memberApi.me()
      if (profile.room_code) {
        setSession({
          account: profile.username,
          nickname: profile.nickname || profile.username,
          room: profile.room_code,
          roomName: profile.room_name || profile.room_code,
          balance: profile.balance,
        })
        navigate(pathForTab('lobby'))
        return
      }
    } catch {
      /* fall through to room entry */
    }
    navigate(pathForRoom())
  }

  const logout = () => {
    clearToken()
    setSession(null)
    setPendingAccount(null)
    navigate(pathForLogin())
  }

  if (booting) {
    return <main className="mobile-app"><div className="app-content"><p style={{ padding: 24 }}>加载中…</p></div></main>
  }

  if (route.kind === 'login') return <Login onContinue={(account, nickname) => void continueLogin(account, nickname)} onRegister={() => navigate(pathForRegister())} />

  if (route.kind === 'register') return (
    <Register
      onBack={() => navigate(pathForLogin())}
      onContinue={(account, nickname) => void continueLogin(account, nickname)}
    />
  )

  if (route.kind === 'room') {
    if (!getToken()) return <Login onContinue={continueLogin} />
    const account = pendingAccount?.account ?? activeSession?.account
    if (!account) return <Login onContinue={continueLogin} />
    return (
      <RoomEntry
        onBack={() => { clearToken(); setPendingAccount(null); navigate(pathForLogin()) }}
        onEnter={(room, roomName) => {
          void memberApi.me().then((profile) => {
            setSession({
              account: profile.username,
              nickname: profile.nickname || profile.username,
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

  if (!activeSession || !getToken()) return <Login onContinue={continueLogin} />

  if (activeGame) {
    return (
      <GameRoom
        game={activeGame}
        games={liveGames}
        theme={demo.theme}
        nickname={activeSession.nickname}
        balance={activeSession.balance}
        onBack={() => navigate(pathForTab('lobby'))}
        onOpenGame={(gameId) => navigate(pathForGame(gameId))}
        onOpenService={() => navigate(pathForChat('service'))}
        onRefreshBalance={refreshBalance}
      />
    )
  }

  const activeTab: Tab = route.kind === 'game' ? 'lobby' : route.tab
  const showBottomNav = route.kind !== 'chat' || route.view === 'list'
  const content = route.kind === 'chat'
    ? <Chats view={route.view} unreadCount={demo.chatUnread} onMarkAllRead={() => { void portalApi.markAllRead().then(() => refreshUnread()) }} onNavigate={(view) => navigate(pathForChat(view))} onRefreshUnread={() => void refreshUnread()} />
    : activeTab === 'lobby'
      ? <Lobby account={activeSession.nickname} room={activeSession.room} games={liveGames} theme={demo.theme} gamesLive={gamesLive} gamesError={gamesError} onToggleTheme={toggleTheme} onOpenGame={(gameId) => navigate(pathForGame(gameId))} />
      : activeTab === 'shop'
        ? <Wallet balance={activeSession.balance} onRefresh={() => void refreshBalance()} />
        : <Profile account={activeSession.nickname} balance={activeSession.balance} theme={demo.theme} onLogout={logout} onResetDemo={resetDemo} onToggleTheme={toggleTheme} />

  return (
    <main className={`mobile-app theme-${demo.theme} ${showBottomNav ? 'has-bottom-nav' : ''}`}>
      <div className="app-content">{content}</div>
      {showBottomNav && <BottomNav activeTab={activeTab} theme={demo.theme} unreadCount={demo.chatUnread} onSelect={(tab) => navigate(pathForTab(tab))} />}
    </main>
  )
}

export default App
