import { useMemo, useState } from 'react'
import './App.css'
import './overrides.css'
import './game-polish.css'
import './themes.css'
import './lobby-polish.css'
import './profile.css'
import './day-fixes.css'
import './chat.css'
import './game-room.css'
import './sound-settings.css'
import './login.css'
import './room-entry.css'
import './avatar.css'
import './wallet.css'
import { BottomNav } from './components/BottomNav'
import { GameRoom } from './pages/GameRoom'
import { Lobby } from './pages/Lobby'
import { Wallet } from './pages/Wallet'
import { Chats } from './pages/Chats'
import { Profile } from './pages/Profile'
import { Login } from './pages/Login'
import { RoomEntry } from './pages/RoomEntry'
import type { Tab, Theme } from './types'
import { pathForChat, pathForGame, pathForLogin, pathForRoom, pathForTab, useAppRouter } from './router'
import { useLiveGames } from './hooks/useLiveGames'
import { usePersistentState } from './hooks/usePersistentState'

type DemoState = {
  theme: Theme
  points: number
  checkedIn: boolean
  chatUnread: number
}
type Session = { account: string; room: string }

const initialDemoState: DemoState = { theme: 'day', points: 2468, checkedIn: false, chatUnread: 46 }

function App() {
  const [session, setSession] = usePersistentState<Session | null>('seven-star-session', null)
  const [pendingAccount, setPendingAccount] = useState<string | null>(null)
  const [demo, setDemo] = usePersistentState<DemoState>('seven-star-demo-state', initialDemoState)
  const { route, navigate } = useAppRouter()
  const liveGames = useLiveGames()
  const activeSession = session && typeof session.account === 'string' && session.account.trim() && typeof session.room === 'string' && session.room.trim() ? session : null
  const activeGameId = route.kind === 'game' ? route.gameId : null
  const activeGame = useMemo(() => liveGames.find((game) => game.id === activeGameId), [activeGameId, liveGames])
  const toggleTheme = () => setDemo((current) => ({ ...current, theme: current.theme === 'day' ? 'night' : 'day' }))
  const resetDemo = () => setDemo(initialDemoState)

  const continueLogin = (account: string) => {
    setPendingAccount(account)
    navigate(pathForRoom())
  }

  if (route.kind === 'login') return <Login onContinue={continueLogin} />

  if (route.kind === 'room') {
    const account = pendingAccount ?? activeSession?.account
    if (!account) return <Login onContinue={continueLogin} />
    return <RoomEntry account={account} onBack={() => { setPendingAccount(null); navigate(pathForLogin()) }} onEnter={(room) => { setSession({ account, room }); setPendingAccount(null); navigate(pathForTab('lobby')) }} />
  }

  if (!activeSession) return <Login onContinue={continueLogin} />

  if (activeGame) return <GameRoom game={activeGame} games={liveGames} theme={demo.theme} onBack={() => navigate(pathForTab('lobby'))} onOpenGame={(gameId) => navigate(pathForGame(gameId))} onOpenService={() => navigate(pathForChat('service'))} />

  const activeTab: Tab = route.kind === 'game' ? 'lobby' : route.tab
  const showBottomNav = route.kind !== 'chat' || route.view === 'list'
  const content = route.kind === 'chat'
    ? <Chats view={route.view} unreadCount={demo.chatUnread} onMarkAllRead={() => setDemo((current) => ({ ...current, chatUnread: 0 }))} onNavigate={(view) => navigate(pathForChat(view))} />
    : activeTab === 'lobby'
      ? <Lobby account={activeSession.account} room={activeSession.room} games={liveGames} theme={demo.theme} onToggleTheme={toggleTheme} onOpenGame={(gameId) => navigate(pathForGame(gameId))} />
      : activeTab === 'shop'
      ? <Wallet points={demo.points} />
        : <Profile account={activeSession.account} theme={demo.theme} onLogout={() => { setSession(null); setPendingAccount(null); navigate(pathForLogin()) }} onResetDemo={resetDemo} onToggleTheme={toggleTheme} />

  return <main className={`mobile-app theme-${demo.theme} ${showBottomNav ? 'has-bottom-nav' : ''}`}><div className="app-content">{content}</div>{showBottomNav && <BottomNav activeTab={activeTab} theme={demo.theme} unreadCount={demo.chatUnread} onSelect={(tab) => navigate(pathForTab(tab))} />}</main>
}

export default App
