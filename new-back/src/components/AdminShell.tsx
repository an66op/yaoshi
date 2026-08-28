import { Alert, AppBar, Avatar, Badge, Box, Button, Chip, Dialog, DialogContent, DialogTitle, Divider, Drawer, IconButton, InputAdornment, List, ListItemButton, ListItemIcon, ListItemText, Menu, MenuItem, Snackbar, Stack, TextField, Toolbar, Tooltip, Typography, useMediaQuery } from '@mui/material'
import { useTheme } from '@mui/material/styles'
import DashboardRounded from '@mui/icons-material/DashboardRounded'
import PeopleAltRounded from '@mui/icons-material/PeopleAltRounded'
import SupportAgentRounded from '@mui/icons-material/SupportAgentRounded'
import ForumRounded from '@mui/icons-material/ForumRounded'
import FactCheckRounded from '@mui/icons-material/FactCheckRounded'
import AssessmentRounded from '@mui/icons-material/AssessmentRounded'
import AccountBalanceWalletRounded from '@mui/icons-material/AccountBalanceWalletRounded'
import CampaignRounded from '@mui/icons-material/CampaignRounded'
import MonitorHeartRounded from '@mui/icons-material/MonitorHeartRounded'
import ReceiptLongRounded from '@mui/icons-material/ReceiptLongRounded'
import ListAltRounded from '@mui/icons-material/ListAltRounded'
import TuneRounded from '@mui/icons-material/TuneRounded'
import FlightTakeoffRounded from '@mui/icons-material/FlightTakeoffRounded'
import PublicRounded from '@mui/icons-material/PublicRounded'
import SportsEsportsRounded from '@mui/icons-material/SportsEsportsRounded'
import StarsRounded from '@mui/icons-material/StarsRounded'
import SettingsRounded from '@mui/icons-material/SettingsRounded'
import MenuRounded from '@mui/icons-material/MenuRounded'
import NotificationsNoneRounded from '@mui/icons-material/NotificationsNoneRounded'
import DarkModeRounded from '@mui/icons-material/DarkModeRounded'
import LightModeRounded from '@mui/icons-material/LightModeRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import KeyboardCommandKeyRounded from '@mui/icons-material/KeyboardCommandKeyRounded'
import LogoutRounded from '@mui/icons-material/LogoutRounded'
import PersonRounded from '@mui/icons-material/PersonRounded'
import ApartmentRounded from '@mui/icons-material/ApartmentRounded'
import CheckCircleRounded from '@mui/icons-material/CheckCircleRounded'
import ViewListRounded from '@mui/icons-material/ViewListRounded'
import SmartToyRounded from '@mui/icons-material/SmartToyRounded'
import StorageRounded from '@mui/icons-material/StorageRounded'
import CloseRounded from '@mui/icons-material/CloseRounded'
import VolumeUpRounded from '@mui/icons-material/VolumeUpRounded'
import VolumeOffRounded from '@mui/icons-material/VolumeOffRounded'
import ArrowForwardRounded from '@mui/icons-material/ArrowForwardRounded'
import type { ReactNode } from 'react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { adminApi, agentApi, tenantApi, type AdminChatConversation, type AdminChatUnreadSummary, type AdminNotification, type ApplicationStats, type ManagementWsEvent } from '../api'
import { DEFAULT_ADMIN_MENU, DEFAULT_AGENT_MENU, DEFAULT_TENANT_MENU, normalizeAdminMenu, normalizeRoleMenu, type AdminMenuItemConfig } from '../adminMenu'
import { MANAGEMENT_WS_EVENT, MANAGEMENT_WS_STATUS_EVENT, type ManagementWsStatus } from '../hooks/useManagementWebSocket'
import { useServerClock } from '../hooks/useServerClock'
import { useFeedback } from './feedback'
import type { AuthUser } from '../auth'
import {
  CHAT_ACTIVE_CONVERSATION_EVENT, CHAT_UNREAD_CHANGED_EVENT, getActiveChatConversation,
  requestOpenChatConversation, sameChatTarget, shouldSuppressChatAlert, type ChatConversationTarget,
} from '../utils/chatNotifications'
import {
  managementAlertFromEvent, mergeManagementAlertQueue, shouldPlayManagementAlertSound,
  type ManagementAlert,
} from '../utils/managementAlerts'

const drawerWidth = 208
type ShellMenuItem = { path: string; label: string; icon: ReactNode }
type ShellMenuGroup = { label: string; items: ShellMenuItem[] }
const adminIcons: Record<string, ReactNode> = {
  '/': <DashboardRounded />, '/tenants': <ApartmentRounded />, '/users': <PeopleAltRounded />, '/members': <PeopleAltRounded />, '/agents': <SupportAgentRounded />,
	'/robots': <SmartToyRounded />,
  '/applications': <FactCheckRounded />, '/room-reviews': <CheckCircleRounded />, '/chat': <SupportAgentRounded />, '/lottery-chat': <ForumRounded />, '/reports': <AssessmentRounded />, '/wallet': <AccountBalanceWalletRounded />,
  '/announcements': <CampaignRounded />, '/activities': <CampaignRounded />, '/monitor': <MonitorHeartRounded />, '/bets': <ListAltRounded />, '/results': <ReceiptLongRounded />,
  '/limits': <TuneRounded />, '/board-report': <FlightTakeoffRounded />, '/lottery-network': <PublicRounded />, '/interface-test': <PublicRounded />, '/entertainment': <SportsEsportsRounded />,
  '/special-numbers': <StarsRounded />, '/menu-management': <ViewListRounded />, '/system': <SettingsRounded />,
	'/audit': <ReceiptLongRounded />,
  '/data-maintenance': <StorageRounded />,
}

const createAdminGroups = (menu: AdminMenuItemConfig[]) => {
  const result: ShellMenuGroup[] = []
  const indexes = new Map<string, number>()
  menu.filter(item => item.visible).sort((a, b) => a.order - b.order).forEach(item => {
    let index = indexes.get(item.group)
    if (index === undefined) {
      index = result.length
      indexes.set(item.group, index)
      result.push({ label: item.group, items: [] })
    }
    result[index].items.push({ path: item.path, label: item.label, icon: adminIcons[item.path] ?? <ViewListRounded /> })
  })
  return result
}

const pendingApplicationCount = (role: string, stats: ApplicationStats) => {
  const safeCount = (value: unknown) => Math.max(0, Number(value) || 0)
  if (role !== 'admin') return safeCount(stats.pending)
  const categories = stats.pending_by_category
  if (!categories || typeof categories !== 'object') return safeCount(stats.pending)
  return safeCount(categories.wallet) + safeCount(categories.entertainment)
}

export function AdminShell({ path, onNavigate, mode, onToggleMode, user, onLogout, children }: { path: string; onNavigate: (path: string) => void; mode: 'light' | 'dark'; onToggleMode: () => void; user: AuthUser; onLogout: () => void; children: ReactNode }) {
  const theme = useTheme()
  const desktop = useMediaQuery(theme.breakpoints.up('md'))
  const [mobileOpen, setMobileOpen] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [backendOnline, setBackendOnline] = useState<boolean | null>(null)
  const [notificationAnchor, setNotificationAnchor] = useState<HTMLElement | null>(null)
  const [profileAnchor, setProfileAnchor] = useState<HTMLElement | null>(null)
  const [roomCode, setRoomCode] = useState('')
  const [notifications, setNotifications] = useState<AdminNotification[]>([])
  const [chatUnread, setChatUnread] = useState<AdminChatUnreadSummary>({ items: [], total_unread: 0 })
  const [liveAlerts, setLiveAlerts] = useState<ManagementAlert[]>([])
  const [soundEnabled, setSoundEnabled] = useState(false)
  const [pendingApplications, setPendingApplications] = useState(0)
  const [adminMenu, setAdminMenu] = useState<AdminMenuItemConfig[]>(() => normalizeAdminMenu(DEFAULT_ADMIN_MENU))
	const [roleMenu, setRoleMenu] = useState<AdminMenuItemConfig[]>(() => user.role === 'tenant' ? DEFAULT_TENANT_MENU : DEFAULT_AGENT_MENU)
  const { now: serverNow, synced: clockSynced, latency } = useServerClock()
  const { showMessage } = useFeedback()
  const visibleGroups = useMemo<ShellMenuGroup[]>(() => createAdminGroups(user.role === 'admin' ? adminMenu : roleMenu), [adminMenu, roleMenu, user.role])
  const navigate = (next: string) => { onNavigate(next); setMobileOpen(false) }
  const pages = useMemo(() => visibleGroups.flatMap(group => group.items).filter(item => item.label.includes(query.trim())), [query, visibleGroups])
  const visibleNotifications = notifications
  const standaloneNotificationCount = useMemo(() => visibleNotifications.filter(item => !item.read).length, [visibleNotifications])
  const attentionCount = standaloneNotificationCount + pendingApplications + chatUnread.total_unread
  const chatUnreadRequest = useRef(0)
  const websocketWasConnected = useRef(false)
  const seenLiveAlertKeys = useRef(new Map<string, number>())
  const userInteracted = useRef(Boolean(navigator.userActivation?.hasBeenActive))
  const audioContext = useRef<AudioContext | null>(null)
  const liveAlert = liveAlerts[0]

  const chatUnreadApi = useMemo(() => user.role === 'agent'
    ? agentApi.chatUnread
    : user.role === 'tenant'
      ? tenantApi.chatUnread
      : adminApi.chatUnread, [user.role])

  const loadChatUnread = useCallback(async () => {
    const requestID = ++chatUnreadRequest.current
    try {
      const result = await chatUnreadApi(30)
      if (requestID === chatUnreadRequest.current) {
        const items = Array.isArray(result?.items) ? result.items : []
        setChatUnread({ items, total_unread: Number(result?.total_unread) || 0 })
        // The unread request may enrich an already-visible live alert, but it
        // never creates one. That keeps page load/reconnect history silent.
        setLiveAlerts(current => {
          let changed = false
          const next = current.map(alert => {
            if (alert.kind !== 'service' || !alert.target) return alert
            const conversation = items.find(item => sameChatTarget(item, alert.target))
            if (!conversation) return alert
            const title = `客服新消息 · ${conversation.title || '会员'}`
            const content = conversation.latest_text || alert.content
            if (title === alert.title && content === alert.content) return alert
            changed = true
            return { ...alert, title, content }
          })
          return changed ? next : current
        })
      }
    } catch {
      // Preserve the last authoritative unread snapshot through transient
      // network failures; reconnect, focus or the next event will retry.
    }
  }, [chatUnreadApi])

  const openChatUnread = (conversation: AdminChatConversation) => {
    requestOpenChatConversation(conversation)
    setLiveAlerts(current => current.filter(alert => !alert.target || !sameChatTarget(alert.target, conversation)))
    navigate('/chat')
  }

  const playLiveAlertSound = useCallback((alert: ManagementAlert) => {
    if (!shouldPlayManagementAlertSound(alert, soundEnabled, userInteracted.current)) return
    type AudioWindow = Window & typeof globalThis & { webkitAudioContext?: typeof AudioContext }
    const AudioContextConstructor = window.AudioContext || (window as AudioWindow).webkitAudioContext
    if (!AudioContextConstructor) return
    try {
      const context = audioContext.current?.state === 'closed' ? null : audioContext.current
      const current = context ?? new AudioContextConstructor()
      audioContext.current = current
      const play = () => {
        const oscillator = current.createOscillator()
        const gain = current.createGain()
        const now = current.currentTime
        oscillator.type = 'sine'
        oscillator.frequency.setValueAtTime(alert.kind === 'application' ? 620 : 760, now)
        gain.gain.setValueAtTime(0.0001, now)
        gain.gain.exponentialRampToValueAtTime(0.035, now + 0.012)
        gain.gain.exponentialRampToValueAtTime(0.0001, now + 0.16)
        oscillator.connect(gain)
        gain.connect(current.destination)
        oscillator.start(now)
        oscillator.stop(now + 0.17)
      }
      if (current.state === 'suspended') void current.resume().then(play).catch(() => undefined)
      else play()
    } catch {
      // Browsers can reject audio even after interaction; alerts remain visual.
    }
  }, [soundEnabled])

  const loadShellMeta = useCallback(() => {
    if (user.role === 'agent') {
      void agentApi.dashboard().then(data => setRoomCode(data.room_code)).catch(() => undefined)
      void agentApi.applicationStats().then(data => setPendingApplications(pendingApplicationCount(user.role, data))).catch(() => undefined)
		void agentApi.settings().then(settings => setSoundEnabled(Boolean(settings.sound_enabled))).catch(() => undefined)
		void agentApi.menuTemplate().then(value => setRoleMenu(normalizeRoleMenu('agent', value))).catch(() => undefined)
      return
    }
	if (user.role === 'tenant') { void tenantApi.roomDashboard().then(data => setRoomCode(data.room_code)).catch(() => undefined); void tenantApi.applicationStats().then(data => setPendingApplications(pendingApplicationCount(user.role, data))).catch(() => undefined); void tenantApi.settings().then(settings => setSoundEnabled(Boolean(settings.sound_enabled))).catch(() => undefined); void tenantApi.menuTemplate().then(value => setRoleMenu(normalizeRoleMenu('tenant', value))).catch(() => undefined); return }
    void Promise.all([
      adminApi.settings().then(settings => {
        setAdminMenu(normalizeAdminMenu(settings.game?.admin_menu))
        setSoundEnabled(Boolean(settings.sound_enabled))
      }).catch(() => undefined),
	      // Keep the last confirmed notices through a transient health/network
	      // failure; an empty replacement incorrectly looks like they were read.
	      adminApi.notifications(12).then(setNotifications).catch(() => undefined),
      adminApi.applicationStats().then(data => setPendingApplications(pendingApplicationCount(user.role, data))).catch(() => undefined),
    ])
  }, [user.role])

  useEffect(() => {
    const check = () => adminApi.health().then(() => setBackendOnline(true)).catch(() => setBackendOnline(false))
    const initial = window.setTimeout(() => { void check(); loadShellMeta(); void loadChatUnread() }, 0)
    const timer = window.setInterval(check, 30000)
    const metaTimer = window.setInterval(loadShellMeta, 60000)
    return () => { window.clearTimeout(initial); window.clearInterval(timer); window.clearInterval(metaTimer) }
  }, [loadChatUnread, loadShellMeta])
  useEffect(() => {
    const refreshRealtimeState = (event: Event) => {
      const detail = (event as CustomEvent<ManagementWsEvent>).detail
      if (detail?.type === 'application') loadShellMeta()
      if (detail?.type === 'chat_message' && detail.data?.room_type === 'service') void loadChatUnread()

      const alert = managementAlertFromEvent(detail, {
        role: user.role,
        path,
        visibility: document.visibilityState,
        focused: document.hasFocus(),
        activeChat: getActiveChatConversation(),
      })
      if (!alert) return
      const now = Date.now()
      if (seenLiveAlertKeys.current.has(alert.key)) return
      seenLiveAlertKeys.current.set(alert.key, now)
      if (seenLiveAlertKeys.current.size > 256) {
        for (const [key, seenAt] of seenLiveAlertKeys.current) {
          if (now - seenAt > 10 * 60_000 || seenLiveAlertKeys.current.size > 256) seenLiveAlertKeys.current.delete(key)
        }
      }
      setLiveAlerts(current => mergeManagementAlertQueue(current, alert))
      playLiveAlertSound(alert)
    }
    window.addEventListener(MANAGEMENT_WS_EVENT, refreshRealtimeState)
    return () => window.removeEventListener(MANAGEMENT_WS_EVENT, refreshRealtimeState)
  }, [loadChatUnread, loadShellMeta, path, playLiveAlertSound, user.role])
  useEffect(() => {
    const markInteracted = () => {
      userInteracted.current = true
      window.removeEventListener('pointerdown', markInteracted)
      window.removeEventListener('keydown', markInteracted)
    }
    window.addEventListener('pointerdown', markInteracted)
    window.addEventListener('keydown', markInteracted)
    return () => {
      window.removeEventListener('pointerdown', markInteracted)
      window.removeEventListener('keydown', markInteracted)
      const context = audioContext.current
      audioContext.current = null
      if (context && context.state !== 'closed') void context.close().catch(() => undefined)
    }
  }, [])
  useEffect(() => {
    const refresh = () => void loadChatUnread()
    const onVisibility = () => { if (document.visibilityState === 'visible') refresh() }
    const onActiveChat = (event: Event) => {
      const target = (event as CustomEvent<ChatConversationTarget | null>).detail ?? null
      if (!target || !shouldSuppressChatAlert(path, document.visibilityState, document.hasFocus(), target, target)) return
      setLiveAlerts(current => {
        const next = current.filter(alert => !alert.target || !sameChatTarget(alert.target, target))
        return next.length === current.length ? current : next
      })
    }
    const onSocketStatus = (event: Event) => {
      const connected = Boolean((event as CustomEvent<ManagementWsStatus>).detail?.connected)
      if (connected && !websocketWasConnected.current) refresh()
      websocketWasConnected.current = connected
    }
    window.addEventListener(CHAT_UNREAD_CHANGED_EVENT, refresh)
    window.addEventListener(CHAT_ACTIVE_CONVERSATION_EVENT, onActiveChat)
    window.addEventListener('online', refresh)
    window.addEventListener('focus', refresh)
    window.addEventListener(MANAGEMENT_WS_STATUS_EVENT, onSocketStatus)
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      window.removeEventListener(CHAT_UNREAD_CHANGED_EVENT, refresh)
      window.removeEventListener(CHAT_ACTIVE_CONVERSATION_EVENT, onActiveChat)
      window.removeEventListener('online', refresh)
      window.removeEventListener('focus', refresh)
      window.removeEventListener(MANAGEMENT_WS_STATUS_EVENT, onSocketStatus)
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [loadChatUnread, path])
  useEffect(() => {
    const refreshMenu = () => loadShellMeta()
    window.addEventListener('yaotu-admin-menu-updated', refreshMenu)
    return () => window.removeEventListener('yaotu-admin-menu-updated', refreshMenu)
  }, [loadShellMeta])
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') { event.preventDefault(); setSearchOpen(true) }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
  const dismissLiveAlert = () => setLiveAlerts(current => current.slice(1))
  const openLiveAlert = () => {
    if (!liveAlert) return
    setLiveAlerts(current => current.slice(1))
    if (liveAlert.target) requestOpenChatConversation(liveAlert.target)
    navigate(liveAlert.path)
  }
  const drawer = <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column', bgcolor: '#071f38', color: '#b9d1db' }}>
    <Toolbar sx={{ minHeight: '72px !important', px: 2.2, gap: 1.3 }}><Box sx={{ width: 40, height: 40, borderRadius: 2.5, display: 'grid', placeItems: 'center', color: 'white', fontWeight: 900, fontSize: 19, background: 'linear-gradient(145deg,#1684ad,#29bdb0)', boxShadow: '0 8px 18px rgba(25,159,168,.35)' }}>王</Box><Box><Typography fontWeight={850} color="white" letterSpacing={2}>王者</Typography><Typography fontSize={10} color="#6f94a5" letterSpacing={1.4}>管理中心</Typography></Box></Toolbar>
    <Box sx={{ flex: 1, overflowY: 'auto', px: 1.2, pb: 1 }}>{visibleGroups.map(group => <Box key={group.label} mb={1}><Typography variant="overline" sx={{ display: 'block', px: 1.5, py: .6, color: '#64879a', fontSize: 9, fontWeight: 800, letterSpacing: 1.4 }}>{group.label}</Typography><List dense disablePadding>{group.items.map(item => <ListItemButton key={item.path} selected={path === item.path} onClick={() => navigate(item.path)} sx={{ minHeight: 42, mb: .4, px: 1.4, borderRadius: 2.2, color: '#a9c3ce', '& .MuiListItemIcon-root': { color: 'inherit' }, '&.Mui-selected': { color: 'white', background: 'linear-gradient(105deg,#1682aa,#24afa9)', boxShadow: '0 6px 16px rgba(5,55,75,.45)' }, '&.Mui-selected:hover': { background: 'linear-gradient(105deg,#1682aa,#24afa9)' }, '&:hover': { color: 'white', bgcolor: 'rgba(255,255,255,.06)' } }}><ListItemIcon sx={{ minWidth: 34 }}>{item.icon}</ListItemIcon><ListItemText primary={item.label} primaryTypographyProps={{ fontSize: 12, fontWeight: 650 }} />{item.path === '/applications' && pendingApplications > 0 ? <Chip size="small" color="error" label={pendingApplications > 99 ? '99+' : pendingApplications} sx={{ height: 19, minWidth: 27, '& .MuiChip-label': { px: .7, fontSize: 9 } }} /> : null}{item.path === '/chat' && chatUnread.total_unread > 0 ? <Chip size="small" color="error" label={chatUnread.total_unread > 99 ? '99+' : chatUnread.total_unread} sx={{ height: 19, minWidth: 27, '& .MuiChip-label': { px: .7, fontSize: 9 } }} /> : null}</ListItemButton>)}</List></Box>)}</Box>
  </Box>

  return (
    <Box sx={{ minHeight: '100vh', display: 'flex', bgcolor: 'background.default' }}>
      <AppBar
        position="fixed"
        elevation={0}
        color="inherit"
        sx={{
          zIndex: theme.zIndex.drawer - 1,
          height: 72,
          left: { xs: 0, md: drawerWidth },
          width: { xs: '100%', md: `calc(100% - ${drawerWidth}px)` },
          borderBottom: 1,
          borderColor: 'divider',
          bgcolor: mode === 'light' ? 'rgba(255,255,255,.92)' : 'rgba(9,34,55,.94)',
          backdropFilter: 'blur(16px)',
        }}
      >
        <Toolbar sx={{ minHeight: '72px !important', px: { xs: 1.5, md: 2.5 }, gap: 1 }}>
          <IconButton aria-label="打开菜单" onClick={() => setMobileOpen(true)} sx={{ display: { md: 'none' } }}>
            <MenuRounded />
          </IconButton>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Typography fontSize={10} color="text.secondary">王者管理中心 / {user.role === 'agent' || user.role === 'tenant' ? `房间 ${roomCode || '—'}` : '平台'}</Typography>
            <Stack direction="row" alignItems="center" gap={1}>
              <Typography fontWeight={800} noWrap>{visibleGroups.flatMap(group => group.items).find(item => item.path === path)?.label ?? '运营首页'}</Typography>
              <Chip size="small" color={backendOnline === false ? 'error' : 'success'} label={backendOnline === false ? '服务离线' : '运营正常'} sx={{ height: 20, fontSize: 9, display: { xs: 'none', sm: 'inline-flex' } }} />
            </Stack>
          </Box>
          <Tooltip title={clockSynced ? `已与服务器校准 · 网络 ${latency}ms` : '正在校准服务器时间'}>
            <Box sx={{ display: { xs: 'none', xl: 'block' }, textAlign: 'right', px: 1 }}>
              <Typography fontFamily="ui-monospace, SFMono-Regular, Menlo, monospace" fontSize={14} fontWeight={800} letterSpacing={.8}>
                {new Date(serverNow).toLocaleTimeString('zh-CN', { hour12: false })}
              </Typography>
              <Typography fontSize={9} color={clockSynced ? 'success.main' : 'text.secondary'}>北京时间 · {clockSynced ? '已校准' : '校准中'}</Typography>
            </Box>
          </Tooltip>
          <Button color="inherit" onClick={() => setSearchOpen(true)} startIcon={<SearchRounded />} endIcon={<KeyboardCommandKeyRounded />} sx={{ display: { xs: 'none', lg: 'inline-flex' }, color: 'text.secondary', bgcolor: 'action.hover', minWidth: 168, justifyContent: 'space-between', fontSize: 11 }}>
            搜索页面
          </Button>
          <Tooltip title="通知">
            <IconButton aria-label="通知" onClick={event => { setNotificationAnchor(event.currentTarget); loadShellMeta(); void loadChatUnread() }}>
              <Badge badgeContent={attentionCount} max={99} invisible={attentionCount === 0} color="error">
                <NotificationsNoneRounded />
              </Badge>
            </IconButton>
          </Tooltip>
          <Tooltip title={mode === 'light' ? '切换夜间模式' : '切换日间模式'}>
            <IconButton aria-label="切换主题" onClick={onToggleMode}>{mode === 'light' ? <DarkModeRounded /> : <LightModeRounded />}</IconButton>
          </Tooltip>
          <Stack
            component="button"
            type="button"
            onClick={event => setProfileAnchor(event.currentTarget)}
            direction="row"
            alignItems="center"
            gap={1}
            sx={{ p: .3, border: 0, borderRadius: 2, bgcolor: 'transparent', color: 'inherit', cursor: 'pointer', '&:hover': { bgcolor: 'action.hover' } }}
          >
            <Avatar sx={{ width: 38, height: 38, bgcolor: 'primary.main', fontSize: 14 }}>{(user.nickname || user.username || '曜').slice(0, 1)}</Avatar>
            <Box sx={{ display: { xs: 'none', sm: 'block' }, textAlign: 'left' }}>
              <Typography fontSize={11} fontWeight={700}>{user.nickname || user.username}</Typography>
              <Typography fontSize={9} color="success.main">在线</Typography>
            </Box>
          </Stack>
        </Toolbar>
      </AppBar>
      <Drawer variant={desktop ? 'permanent' : 'temporary'} open={desktop || mobileOpen} onClose={() => setMobileOpen(false)} ModalProps={{ keepMounted: true }} sx={{ width: { md: drawerWidth }, flexShrink: 0, '& .MuiDrawer-paper': { width: drawerWidth, border: 0 } }}>{drawer}</Drawer>
      <Box component="main" sx={{ flex: 1, minWidth: 0, pt: '72px', minHeight: '100vh' }}>{children}</Box>
      <Menu anchorEl={notificationAnchor} open={Boolean(notificationAnchor)} onClose={() => setNotificationAnchor(null)} slotProps={{ paper: { sx: { width: 330, maxWidth: 'calc(100vw - 24px)', mt: 1 } } }}>
        <Box px={2} py={1}>
          <Stack direction="row" justifyContent="space-between" alignItems="center">
            <Typography fontWeight={800}>待办与消息</Typography>
            <Stack direction="row" gap={1} alignItems="center">
              <Button
                size="small"
                color={soundEnabled ? 'primary' : 'inherit'}
                startIcon={soundEnabled ? <VolumeUpRounded /> : <VolumeOffRounded />}
                onClick={() => { setNotificationAnchor(null); navigate('/system') }}
              >提示音{soundEnabled ? '已开启' : '已关闭'}</Button>
              {user.role === 'admin' && standaloneNotificationCount > 0 ? <Button size="small" onClick={() => void adminApi.markAllNotificationsRead().then(loadShellMeta).then(() => showMessage('已全部标记已读'))}>全部已读</Button> : null}
            </Stack>
          </Stack>
        </Box>
        <Divider />
        {pendingApplications > 0 ? <MenuItem onClick={() => { setNotificationAnchor(null); navigate('/applications') }} sx={{ py: 1.35 }}>
          <ListItemIcon><FactCheckRounded color="warning" /></ListItemIcon>
          <ListItemText primary={`${pendingApplications} 条申请待处理`} secondary="点击进入申请管理" primaryTypographyProps={{ fontSize: 12, fontWeight: 800 }} secondaryTypographyProps={{ fontSize: 10 }} />
          <ArrowForwardRounded fontSize="small" color="action" />
        </MenuItem> : null}
        {chatUnread.items.slice(0, 5).map(item => <MenuItem key={`${item.scope}:${item.room_scope}:${item.game_id}`} onClick={() => { setNotificationAnchor(null); openChatUnread(item) }} sx={{ alignItems: 'flex-start', py: 1.25, whiteSpace: 'normal' }}>
          <ListItemIcon><SupportAgentRounded color="primary" /></ListItemIcon>
          <ListItemText
            primary={`${item.title || '会员'} · ${item.unread_count || 1} 条未读`}
            secondary={item.latest_text || '点击查看客服会话'}
            primaryTypographyProps={{ fontSize: 12, fontWeight: 800 }}
            secondaryTypographyProps={{ fontSize: 10, noWrap: true }}
          />
          <ArrowForwardRounded fontSize="small" color="action" />
        </MenuItem>)}
        {(pendingApplications > 0 || chatUnread.items.length > 0) && visibleNotifications.length > 0 ? <Divider /> : null}
        {visibleNotifications.length === 0 && pendingApplications === 0 && chatUnread.items.length === 0 ? (
          <MenuItem disabled><ListItemText primary="暂无待办或未读消息" /></MenuItem>
        ) : visibleNotifications.map(item => (
          <MenuItem
            key={item.id}
            sx={{ alignItems: 'flex-start', py: 1.5, whiteSpace: 'normal', opacity: item.read ? .7 : 1 }}
            onClick={() => {
              void adminApi.markNotificationRead(item.id).then(loadShellMeta)
              setNotificationAnchor(null)
              if (item.link && user.role !== 'agent') navigate(item.link)
            }}
          >
            <ListItemIcon>
              <CheckCircleRounded color={item.level === 'warning' ? 'warning' : item.level === 'error' ? 'error' : 'success'} />
            </ListItemIcon>
            <ListItemText
              primary={item.title}
              secondary={`${item.content} · ${new Date(item.created_at).toLocaleString('zh-CN', { hour12: false })}`}
              primaryTypographyProps={{ fontSize: 12, fontWeight: item.read ? 600 : 750 }}
              secondaryTypographyProps={{ fontSize: 10 }}
            />
          </MenuItem>
        ))}
      </Menu>
      <Menu anchorEl={profileAnchor} open={Boolean(profileAnchor)} onClose={() => setProfileAnchor(null)} slotProps={{ paper: { sx: { minWidth: 180, mt: 1 } } }}>
        <MenuItem onClick={() => { setProfileAnchor(null); navigate('/system') }}><ListItemIcon><PersonRounded fontSize="small" /></ListItemIcon>账户与设置</MenuItem>
        <Divider />
        <MenuItem onClick={() => { setProfileAnchor(null); onLogout(); showMessage('已退出登录', 'info') }}><ListItemIcon><LogoutRounded fontSize="small" /></ListItemIcon>退出登录</MenuItem>
      </Menu>
      <Dialog open={searchOpen} onClose={() => setSearchOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>快速前往</DialogTitle>
        <DialogContent>
          <TextField autoFocus fullWidth placeholder="输入页面名称，例如：用户、开奖、设置" value={query} onChange={event => setQuery(event.target.value)} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded /></InputAdornment> } }} sx={{ mt: .5, mb: 1.5 }} />
          <List disablePadding>
            {pages.map(item => (
              <ListItemButton key={item.path} selected={item.path === path} onClick={() => { navigate(item.path); setSearchOpen(false); setQuery('') }} sx={{ borderRadius: 2, mb: .5 }}>
                <ListItemIcon>{item.icon}</ListItemIcon>
                <ListItemText primary={item.label} secondary={visibleGroups.find(group => group.items.some(entry => entry.path === item.path))?.label} />
              </ListItemButton>
            ))}
            {pages.length === 0 && <Typography color="text.secondary" textAlign="center" py={4}>没有找到对应页面</Typography>}
          </List>
        </DialogContent>
      </Dialog>
      <Snackbar
        key={liveAlert ? `${liveAlert.key}:${liveAlert.count}` : 'empty'}
        open={Boolean(liveAlert)}
        autoHideDuration={7000}
        onClose={(_, reason) => { if (reason !== 'clickaway') dismissLiveAlert() }}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        sx={{ maxWidth: { xs: 'calc(100vw - 24px)', sm: 410 }, '& .MuiAlert-root': { width: '100%' } }}
      >
        <Alert
          severity={liveAlert?.kind === 'application' ? 'warning' : 'info'}
          variant="filled"
          icon={liveAlert?.kind === 'application' ? <FactCheckRounded /> : liveAlert?.kind === 'service' ? <SupportAgentRounded /> : <ForumRounded />}
          action={<Stack direction="row" alignItems="center">
            <Button size="small" color="inherit" onClick={openLiveAlert} sx={{ fontWeight: 850 }}>查看</Button>
            <IconButton size="small" color="inherit" aria-label="关闭实时提醒" onClick={dismissLiveAlert}><CloseRounded fontSize="small" /></IconButton>
          </Stack>}
          sx={{ alignItems: 'center', boxShadow: 8 }}
        >
          <Typography fontSize={12.5} fontWeight={850}>{liveAlert?.title}</Typography>
          <Typography fontSize={11} sx={{ opacity: .92 }} noWrap>{liveAlert && liveAlert.count > 1 ? `${liveAlert.count} 条新消息 · ` : ''}{liveAlert?.content}</Typography>
        </Alert>
      </Snackbar>
    </Box>
  )
}
