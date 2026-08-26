import { Box, CircularProgress, CssBaseline, ThemeProvider } from '@mui/material'
import type { PaletteMode } from '@mui/material'
import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import { adminApi } from './api'
import { clearSession, getStoredUser, getToken, type AuthUser } from './auth'
import { AdminShell } from './components/AdminShell'
import { FeedbackProvider } from './components/FeedbackProvider'
import { createAdminTheme } from './theme'
import { LoginPage } from './pages/LoginPage'
import { useManagementWebSocket } from './hooks/useManagementWebSocket'

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
const TenantsPage = lazy(() => import('./pages/TenantsPage').then(module => ({ default: module.TenantsPage })))
const TenantWorkspacePage = lazy(() => import('./pages/TenantWorkspacePage').then(module => ({ default: module.TenantWorkspacePage })))
const ChatPage = lazy(() => import('./pages/ChatPage').then(module => ({ default: module.ChatPage })))
const AnnouncementPage = lazy(() => import('./pages/AnnouncementPage').then(module => ({ default: module.AnnouncementPage })))
const MenuManagementPage = lazy(() => import('./pages/MenuManagementPage').then(module => ({ default: module.MenuManagementPage })))
const AgentWorkspacePage = lazy(() => import('./pages/AgentWorkspacePage').then(module => ({ default: module.AgentWorkspacePage })))
const ManagementPage = lazy(() => import('./pages/ManagementPages').then(module => ({ default: module.ManagementPage })))

const routes = new Set(['/', '/users', '/members', '/tenants', '/agents', '/applications', '/room-reviews', '/chat', '/lottery-chat', '/announcements', '/reports', '/wallet', '/activities', '/monitor', '/bets', '/results', '/limits', '/board-report', '/lottery-network', '/entertainment', '/special-numbers', '/menu-management', '/system'])
const currentPath = () => routes.has(window.location.pathname) ? window.location.pathname : '/'

function App() {
  const [path, setPath] = useState(currentPath)
  const [mode, setMode] = useState<PaletteMode>(() => window.localStorage.getItem('yaotu-back-theme') === 'dark' ? 'dark' : 'light')
  const [user, setUser] = useState<AuthUser | null>(() => getToken() ? getStoredUser() : null)
  const [authChecking, setAuthChecking] = useState(() => Boolean(getToken()))
  const theme = useMemo(() => createAdminTheme(mode), [mode])
  useManagementWebSocket(user?.role, Boolean(user))

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
    if (!getToken()) return
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

  const agentPage = path === '/members' ? <AgentWorkspacePage key="members" section="users" />
    : path === '/room-reviews' ? <AgentWorkspacePage key="room-reviews" section="room-reviews" />
    : path === '/applications' ? <AgentWorkspacePage key="applications" section="applications" />
    : path === '/bets' ? <AgentWorkspacePage key="bets" section="bets" />
    : path === '/lottery-chat' ? <AgentWorkspacePage key="lottery-chat" section="lottery-chat" />
    : path === '/chat' ? <AgentWorkspacePage key="chat" section="chat" />
    : path === '/reports' ? <AgentWorkspacePage key="reports" section="reports" />
    : <AgentWorkspacePage key="dashboard" section="dashboard" />

  const tenantPage = path === '/agents' ? <TenantWorkspacePage key="agents" section="agents" />
    : path === '/members' ? <TenantWorkspacePage key="members" section="users" />
    : path === '/room-reviews' ? <TenantWorkspacePage key="room-reviews" section="room-reviews" />
    : path === '/applications' ? <TenantWorkspacePage key="applications" section="applications" />
    : path === '/bets' ? <TenantWorkspacePage key="bets" section="bets" />
    : path === '/lottery-chat' ? <TenantWorkspacePage key="lottery-chat" section="lottery-chat" />
    : path === '/chat' ? <TenantWorkspacePage key="chat" section="chat" />
    : path === '/reports' ? <TenantWorkspacePage key="reports" section="reports" />
    : <TenantWorkspacePage key="dashboard" section="dashboard" />

  const page = user.role === 'agent' ? agentPage
	: user.role === 'tenant' ? tenantPage
    : path === '/' ? <DashboardPage />
    : path === '/results' ? <ResultsPage />
    : path === '/users' ? <UsersPage view="accounts" />
    : path === '/members' ? <UsersPage view="members" />
    : path === '/agents' ? <AgentsPage />
    : path === '/tenants' ? <TenantsPage />
    : path === '/lottery-chat' ? <ChatPage key="lottery" view="lottery" />
    : path === '/chat' ? <ChatPage key="support" />
    : path === '/announcements' ? <AnnouncementPage />
    : path === '/applications' ? <ApplicationsPage />
    : path === '/room-reviews' ? <ApplicationsPage initialCategory="join" />
    : path === '/reports' ? <ReportsPage />
    : path === '/wallet' ? <WalletPage />
    : path === '/limits' ? <LimitsPage />
    : path === '/menu-management' ? <MenuManagementPage />
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
