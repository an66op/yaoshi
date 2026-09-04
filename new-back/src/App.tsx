import { Box, CircularProgress, CssBaseline, ThemeProvider } from '@mui/material'
import type { PaletteMode } from '@mui/material'
import { lazy, Suspense, useEffect, useMemo, useRef, useState } from 'react'
import { adminApi, AuthError } from './api'
import { ADMIN_AUTH_EVENT_KEY, broadcastAdminLogout, clearLegacyAdminSession, setCurrentUser, type AuthUser } from './auth'
import { AdminShell } from './components/AdminShell'
import { FeedbackProvider } from './components/FeedbackProvider'
import { createAdminTheme } from './theme'
import { LoginPage } from './pages/LoginPage'
import { useManagementWebSocket } from './hooks/useManagementWebSocket'
import { isRetiredAccountPath, resolveAdminPath } from './adminRoutes'

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
const PlanManagementPage = lazy(() => import('./pages/PlanManagementPage').then(module => ({ default: module.PlanManagementPage })))
const MenuManagementPage = lazy(() => import('./pages/MenuManagementPage').then(module => ({ default: module.MenuManagementPage })))
const AgentWorkspacePage = lazy(() => import('./pages/AgentWorkspacePage').then(module => ({ default: module.AgentWorkspacePage })))
const ManagementPage = lazy(() => import('./pages/ManagementPages').then(module => ({ default: module.ManagementPage })))
const RobotsPage = lazy(() => import('./pages/RobotsPage').then(module => ({ default: module.RobotsPage })))
const WorkspaceRobotsPage = lazy(() => import('./pages/WorkspaceRobotsPage').then(module => ({ default: module.WorkspaceRobotsPage })))
const RoomSettingsPage = lazy(() => import('./pages/RoomSettingsPage').then(module => ({ default: module.RoomSettingsPage })))
const WorkspaceGamesPage = lazy(() => import('./pages/WorkspaceGamesPage').then(module => ({ default: module.WorkspaceGamesPage })))
const DataMaintenancePage = lazy(() => import('./pages/DataMaintenancePage').then(module => ({ default: module.DataMaintenancePage })))
const FlyOrderPage = lazy(() => import('./pages/FlyOrderPage').then(module => ({ default: module.FlyOrderPage })))
const GameDocumentationPage = lazy(() => import('./pages/GameDocumentationPage').then(module => ({ default: module.GameDocumentationPage })))
const SystemLogsPage = lazy(() => import('./pages/SystemLogsPage').then(module => ({ default: module.SystemLogsPage })))

const currentPath = () => resolveAdminPath(window.location.pathname)
const allowPageNavigation = () => window.dispatchEvent(new Event('yaotu-before-navigate', { cancelable: true }))
const storedPaletteMode = (): PaletteMode => {
  try { return window.localStorage.getItem('yaotu-back-theme') === 'dark' ? 'dark' : 'light' } catch { return 'light' }
}

function App() {
  const [path, setPath] = useState(currentPath)
  const displayedPath = useRef(path)
  const [mode, setMode] = useState<PaletteMode>(storedPaletteMode)
	const [user, setUser] = useState<AuthUser | null>(null)
	const [authChecking, setAuthChecking] = useState(true)
  const theme = useMemo(() => createAdminTheme(mode), [mode])
  useManagementWebSocket(user?.role, Boolean(user))

  useEffect(() => {
    const listener = () => {
      const next = currentPath()
      if (next !== displayedPath.current && !allowPageNavigation()) {
        window.history.pushState({}, '', displayedPath.current)
        return
      }
      displayedPath.current = next
      if (isRetiredAccountPath(window.location.pathname)) window.history.replaceState({}, '', next)
      setPath(next)
    }
    listener()
    window.addEventListener('popstate', listener)
    return () => window.removeEventListener('popstate', listener)
  }, [])

  useEffect(() => {
    const onExpired = () => {
      setCurrentUser(null)
      setUser(null)
    }
    window.addEventListener('yaotu-auth-expired', onExpired)
    return () => window.removeEventListener('yaotu-auth-expired', onExpired)
  }, [])

  useEffect(() => {
    const onStorage = (event: StorageEvent) => {
	  if (event.key !== ADMIN_AUTH_EVENT_KEY) return
      setCurrentUser(null)
      setUser(null)
      window.history.replaceState({}, '', '/')
      setPath('/')
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  useEffect(() => {
	clearLegacyAdminSession()
    let cancelled = false
    void adminApi.me()
      .then(profile => {
        if (cancelled) return
        setCurrentUser(profile)
        setUser(profile)
		void adminApi.refreshSession().catch(() => undefined)
      })
      .catch(() => {
        if (cancelled) return
        setCurrentUser(null)
        setUser(null)
      })
      .finally(() => {
        if (!cancelled) setAuthChecking(false)
      })
    return () => { cancelled = true }
  }, [])

  const navigate = (next: string) => {
    const destination = resolveAdminPath(next)
    if (destination === path) return
    if (!allowPageNavigation()) return
    window.history.pushState({}, '', destination)
    displayedPath.current = destination
    setPath(destination)
  }
  const toggleMode = () => setMode(current => {
    const next = current === 'light' ? 'dark' : 'light'
    try { window.localStorage.setItem('yaotu-back-theme', next) } catch { /* Theme still changes for this tab. */ }
    return next
  })
  const logout = async () => {
	if (!allowPageNavigation()) return
	try {
	  await adminApi.logout()
	} catch (reason) {
	  // An explicit 401 means the server already considers the cookie invalid.
	  // Transport and 5xx failures keep the current session visible so the UI
	  // never claims an HttpOnly session was removed when it was not.
	  if (!(reason instanceof AuthError)) throw reason
	}
	broadcastAdminLogout()
    setCurrentUser(null)
    setUser(null)
    window.history.pushState({}, '', '/')
    setPath('/')
  }

  if (authChecking) {
    return <ThemeProvider theme={theme}><CssBaseline /><Box minHeight="100vh" display="grid" sx={{ placeItems: 'center' }}><CircularProgress size={30} /></Box></ThemeProvider>
  }

  if (!user) {
    return <ThemeProvider theme={theme}><CssBaseline /><LoginPage onSuccess={profile => { setCurrentUser(profile); setUser(profile) }} /></ThemeProvider>
  }

  const agentPage = path === '/members' ? <AgentWorkspacePage key="members" section="users" />
		: path === '/fly-orders' ? <FlyOrderPage role="agent" />
    : path === '/room-reviews' ? <AgentWorkspacePage key="room-reviews" section="room-reviews" />
    : path === '/applications' ? <ApplicationsPage />
    : path === '/entertainment' ? <WorkspaceGamesPage />
    : path === '/bets' ? <AgentWorkspacePage key="bets" section="bets" />
    : path === '/lottery-chat' ? <AgentWorkspacePage key="lottery-chat" section="lottery-chat" />
    : path === '/chat' ? <AgentWorkspacePage key="chat" section="chat" />
    : path === '/reports' ? <ReportsPage />
    : path === '/robots' ? <WorkspaceRobotsPage />
    : path === '/announcements' ? <RoomSettingsPage section="content" />
    : path === '/limits' ? <RoomSettingsPage section="limits" />
    : path === '/wallet' ? <RoomSettingsPage section="wallet" />
    : path === '/system' ? <RoomSettingsPage section="room" />
    : <AgentWorkspacePage key="dashboard" section="dashboard" />

  const tenantPage = path === '/agents'
    ? <TenantWorkspacePage key="agents" section="agents" />
    : path === '/members' ? <AgentWorkspacePage key="tenant-members" section="users" tenantDirect />
		: path === '/fly-orders' ? <FlyOrderPage role="tenant" />
    : path === '/applications' ? <ApplicationsPage />
    : path === '/entertainment' ? <WorkspaceGamesPage />
    : path === '/bets' ? <AgentWorkspacePage key="tenant-bets" section="bets" tenantDirect />
    : path === '/lottery-chat' ? <AgentWorkspacePage key="tenant-lottery-chat" section="lottery-chat" tenantDirect />
    : path === '/chat' ? <AgentWorkspacePage key="tenant-chat" section="chat" tenantDirect />
    : path === '/reports' ? <ReportsPage />
    : path === '/robots' ? <WorkspaceRobotsPage />
    : path === '/announcements' ? <RoomSettingsPage section="content" />
    : path === '/limits' ? <RoomSettingsPage section="limits" />
    : path === '/wallet' ? <RoomSettingsPage section="wallet" />
    : path === '/system' ? <RoomSettingsPage section="room" />
    : <TenantWorkspacePage key="dashboard" section="dashboard" />

  const page = user.role === 'agent' ? agentPage
    : user.role === 'tenant' ? tenantPage
    : path === '/' ? <DashboardPage />
    : path === '/results' ? <ResultsPage />
    : path === '/members' ? <UsersPage />
    : path === '/robots' ? <RobotsPage />
		: path === '/fly-orders' ? <FlyOrderPage role="admin" />
    : path === '/agents' ? <AgentsPage />
    : path === '/tenants' ? <TenantsPage />
    : path === '/lottery-chat' ? <ChatPage key="lottery" view="lottery" />
    : path === '/chat' ? <ChatPage key="support" />
    : path === '/announcements' ? <AnnouncementPage />
    : path === '/plans' ? <PlanManagementPage />
    : path === '/applications' ? <ApplicationsPage />
    : path === '/room-reviews' ? <ApplicationsPage initialCategory="wallet" />
    : path === '/reports' ? <ReportsPage />
    : path === '/wallet' ? <WalletPage />
    : path === '/limits' ? <LimitsPage />
    : path === '/menu-management' ? <MenuManagementPage />
    : path === '/logs' ? <SystemLogsPage />
    : path === '/data-maintenance' ? <DataMaintenancePage />
    : path === '/game-guide' ? <GameDocumentationPage />
    : path === '/system' ? <SystemPage />
    : path === '/interface-test' || path === '/lottery-network' ? <NetworkPage />
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
