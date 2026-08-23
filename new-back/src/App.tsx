import { Box, CircularProgress, CssBaseline, ThemeProvider } from '@mui/material'
import type { PaletteMode } from '@mui/material'
import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import { adminApi } from './api'
import { clearSession, getStoredUser, getToken, type AuthUser } from './auth'
import { AdminShell } from './components/AdminShell'
import { FeedbackProvider } from './components/FeedbackProvider'
import { createAdminTheme } from './theme'
import { LoginPage } from './pages/LoginPage'

const DashboardPage = lazy(() => import('./pages/DashboardPage').then(module => ({ default: module.DashboardPage })))
const ResultsPage = lazy(() => import('./pages/ResultsPage').then(module => ({ default: module.ResultsPage })))
const UsersPage = lazy(() => import('./pages/UsersPage').then(module => ({ default: module.UsersPage })))
const ApplicationsPage = lazy(() => import('./pages/ApplicationsPage').then(module => ({ default: module.ApplicationsPage })))
const ReportsPage = lazy(() => import('./pages/ReportsPage').then(module => ({ default: module.ReportsPage })))
const WalletPage = lazy(() => import('./pages/WalletPage').then(module => ({ default: module.WalletPage })))
const LimitsPage = lazy(() => import('./pages/LimitsPage').then(module => ({ default: module.LimitsPage })))
const SystemPage = lazy(() => import('./pages/SystemPage').then(module => ({ default: module.SystemPage })))
const NetworkPage = lazy(() => import('./pages/NetworkPage').then(module => ({ default: module.NetworkPage })))
const MonitorPage = lazy(() => import('./pages/MonitorPage').then(module => ({ default: module.MonitorPage })))
const BoardReportPage = lazy(() => import('./pages/BoardReportPage').then(module => ({ default: module.BoardReportPage })))
const BetsPage = lazy(() => import('./pages/BetsPage').then(module => ({ default: module.BetsPage })))
const AgentsPage = lazy(() => import('./pages/AgentsPage').then(module => ({ default: module.AgentsPage })))
const ChatPage = lazy(() => import('./pages/ChatPage').then(module => ({ default: module.ChatPage })))
const ManagementPage = lazy(() => import('./pages/ManagementPages').then(module => ({ default: module.ManagementPage })))

const routes = new Set(['/', '/users', '/agents', '/applications', '/chat', '/reports', '/wallet', '/activities', '/monitor', '/bets', '/results', '/limits', '/board-report', '/lottery-network', '/entertainment', '/special-numbers', '/system'])
const currentPath = () => routes.has(window.location.pathname) ? window.location.pathname : '/'

function App() {
  const [path, setPath] = useState(currentPath)
  const [mode, setMode] = useState<PaletteMode>(() => window.localStorage.getItem('yaotu-back-theme') === 'dark' ? 'dark' : 'light')
  const [user, setUser] = useState<AuthUser | null>(() => getToken() ? getStoredUser() : null)
  const [authChecking, setAuthChecking] = useState(() => Boolean(getToken()))
  const theme = useMemo(() => createAdminTheme(mode), [mode])

  useEffect(() => {
    const listener = () => setPath(currentPath())
    window.addEventListener('popstate', listener)
    return () => window.removeEventListener('popstate', listener)
  }, [])

  useEffect(() => {
    const onExpired = () => {
      clearSession()
      setUser(null)
    }
    window.addEventListener('yaotu-auth-expired', onExpired)
    return () => window.removeEventListener('yaotu-auth-expired', onExpired)
  }, [])

  useEffect(() => {
    if (!getToken()) {
      setAuthChecking(false)
      return
    }
    let cancelled = false
    void adminApi.me()
      .then(profile => {
        if (cancelled) return
        setUser(profile)
        window.localStorage.setItem('yaotu-admin-user', JSON.stringify(profile))
      })
      .catch(() => {
        if (cancelled) return
        clearSession()
        setUser(null)
      })
      .finally(() => {
        if (!cancelled) setAuthChecking(false)
      })
    return () => { cancelled = true }
  }, [])

  const navigate = (next: string) => { if (next === path) return; window.history.pushState({}, '', next); setPath(next) }
  const toggleMode = () => setMode(current => { const next = current === 'light' ? 'dark' : 'light'; window.localStorage.setItem('yaotu-back-theme', next); return next })
  const logout = () => {
    clearSession()
    setUser(null)
    window.history.pushState({}, '', '/')
    setPath('/')
  }

  if (authChecking) {
    return <ThemeProvider theme={theme}><CssBaseline /><Box minHeight="100vh" display="grid" sx={{ placeItems: 'center' }}><CircularProgress size={30} /></Box></ThemeProvider>
  }

  if (!user) {
    return <ThemeProvider theme={theme}><CssBaseline /><LoginPage onSuccess={setUser} /></ThemeProvider>
  }

  const page = path === '/' ? <DashboardPage />
    : path === '/results' ? <ResultsPage />
    : path === '/users' ? <UsersPage />
    : path === '/agents' ? <AgentsPage />
    : path === '/chat' ? <ChatPage />
    : path === '/applications' ? <ApplicationsPage />
    : path === '/reports' ? <ReportsPage />
    : path === '/wallet' ? <WalletPage />
    : path === '/limits' ? <LimitsPage />
    : path === '/system' ? <SystemPage />
    : path === '/lottery-network' ? <NetworkPage />
    : path === '/monitor' ? <MonitorPage />
    : path === '/bets' ? <BetsPage />
    : path === '/board-report' ? <BoardReportPage />
    : <ManagementPage path={path} />

  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <FeedbackProvider>
        <AdminShell path={path} onNavigate={navigate} mode={mode} onToggleMode={toggleMode} user={user} onLogout={logout}>
          <Suspense fallback={<Box minHeight="60vh" display="grid" sx={{ placeItems: 'center' }}><CircularProgress size={30} /></Box>}>
            {page}
          </Suspense>
        </AdminShell>
      </FeedbackProvider>
    </ThemeProvider>
  )
}

export default App
