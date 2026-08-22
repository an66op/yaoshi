import { AppBar, Avatar, Badge, Box, Button, Chip, Dialog, DialogContent, DialogTitle, Divider, Drawer, IconButton, InputAdornment, List, ListItemButton, ListItemIcon, ListItemText, Menu, MenuItem, Stack, TextField, Toolbar, Tooltip, Typography, useMediaQuery } from '@mui/material'
import { useTheme } from '@mui/material/styles'
import DashboardRounded from '@mui/icons-material/DashboardRounded'
import PeopleAltRounded from '@mui/icons-material/PeopleAltRounded'
import SupportAgentRounded from '@mui/icons-material/SupportAgentRounded'
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
import CheckCircleRounded from '@mui/icons-material/CheckCircleRounded'
import type { ReactNode } from 'react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { adminApi, type AdminNotification } from '../api'
import { useServerClock } from '../hooks/useServerClock'
import { useFeedback } from './feedback'
import type { AuthUser } from '../auth'

const drawerWidth = 244
const groups = [
  { label: '总览', items: [{ path: '/', label: '运营首页', icon: <DashboardRounded /> }] },
  { label: '业务管理', items: [{ path: '/users', label: '用户管理', icon: <PeopleAltRounded /> }, { path: '/agents', label: '代理管理', icon: <SupportAgentRounded /> }, { path: '/applications', label: '申请管理', icon: <FactCheckRounded /> }, { path: '/reports', label: '数据报表', icon: <AssessmentRounded /> }, { path: '/wallet', label: '钱包配置', icon: <AccountBalanceWalletRounded /> }] },
  { label: '游戏运营', items: [{ path: '/activities', label: '活动管理', icon: <CampaignRounded /> }, { path: '/monitor', label: '现场监控', icon: <MonitorHeartRounded /> }, { path: '/bets', label: '注单管理', icon: <ListAltRounded /> }, { path: '/results', label: '开奖结果查询', icon: <ReceiptLongRounded /> }, { path: '/limits', label: '赔率限额', icon: <TuneRounded /> }, { path: '/board-report', label: '打盘报表', icon: <FlightTakeoffRounded /> }] },
  { label: '扩展服务', items: [{ path: '/lottery-network', label: '开奖线路', icon: <PublicRounded /> }, { path: '/entertainment', label: '彩票娱乐', icon: <SportsEsportsRounded /> }, { path: '/special-numbers', label: '房间靓号', icon: <StarsRounded /> }] },
  { label: '系统', items: [{ path: '/system', label: '系统设置', icon: <SettingsRounded /> }] },
]

const routeNames = Object.fromEntries(groups.flatMap(group => group.items.map(item => [item.path, item.label])))

export function AdminShell({ path, onNavigate, mode, onToggleMode, user, onLogout, children }: { path: string; onNavigate: (path: string) => void; mode: 'light' | 'dark'; onToggleMode: () => void; user: AuthUser; onLogout: () => void; children: ReactNode }) {
  const theme = useTheme()
  const desktop = useMediaQuery(theme.breakpoints.up('md'))
  const [mobileOpen, setMobileOpen] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [backendOnline, setBackendOnline] = useState<boolean | null>(null)
  const [notificationAnchor, setNotificationAnchor] = useState<HTMLElement | null>(null)
  const [profileAnchor, setProfileAnchor] = useState<HTMLElement | null>(null)
  const [roomCode, setRoomCode] = useState('1231')
  const [notifications, setNotifications] = useState<AdminNotification[]>([])
  const { now: serverNow, synced: clockSynced, latency } = useServerClock()
  const { showMessage } = useFeedback()
  const navigate = (next: string) => { onNavigate(next); setMobileOpen(false) }
  const pages = useMemo(() => groups.flatMap(group => group.items).filter(item => item.label.includes(query.trim())), [query])
  const unreadCount = useMemo(() => notifications.filter(item => !item.read).length, [notifications])

  const loadShellMeta = useCallback(() => {
    void Promise.all([
      adminApi.settings().then(settings => setRoomCode(settings.room_code || settings.room_name || '1231')).catch(() => undefined),
      adminApi.notifications(12).then(setNotifications).catch(() => setNotifications([])),
    ])
  }, [])

  useEffect(() => {
    const check = () => adminApi.health().then(() => setBackendOnline(true)).catch(() => setBackendOnline(false))
    check()
    loadShellMeta()
    const timer = window.setInterval(check, 30000)
    const metaTimer = window.setInterval(loadShellMeta, 60000)
    return () => { window.clearInterval(timer); window.clearInterval(metaTimer) }
  }, [loadShellMeta])
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') { event.preventDefault(); setSearchOpen(true) }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
  const drawer = <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column', bgcolor: '#071f38', color: '#b9d1db' }}>
    <Toolbar sx={{ minHeight: '72px !important', px: 2.2, gap: 1.3 }}><Box sx={{ width: 40, height: 40, borderRadius: 2.5, display: 'grid', placeItems: 'center', color: 'white', fontWeight: 900, fontSize: 19, background: 'linear-gradient(145deg,#1684ad,#29bdb0)', boxShadow: '0 8px 18px rgba(25,159,168,.35)' }}>曜</Box><Box><Typography fontWeight={850} color="white" letterSpacing={2}>曜图</Typography><Typography fontSize={10} color="#6f94a5" letterSpacing={1.4}>管理中心</Typography></Box></Toolbar>
    <Box sx={{ flex: 1, overflowY: 'auto', px: 1.2, pb: 1 }}>{groups.map(group => <Box key={group.label} mb={1}><Typography variant="overline" sx={{ display: 'block', px: 1.5, py: .6, color: '#64879a', fontSize: 9, fontWeight: 800, letterSpacing: 1.4 }}>{group.label}</Typography><List dense disablePadding>{group.items.map(item => <ListItemButton key={item.path} selected={path === item.path} onClick={() => navigate(item.path)} sx={{ minHeight: 42, mb: .4, px: 1.4, borderRadius: 2.2, color: '#a9c3ce', '& .MuiListItemIcon-root': { color: 'inherit' }, '&.Mui-selected': { color: 'white', background: 'linear-gradient(105deg,#1682aa,#24afa9)', boxShadow: '0 6px 16px rgba(5,55,75,.45)' }, '&.Mui-selected:hover': { background: 'linear-gradient(105deg,#1682aa,#24afa9)' }, '&:hover': { color: 'white', bgcolor: 'rgba(255,255,255,.06)' } }}><ListItemIcon sx={{ minWidth: 34 }}>{item.icon}</ListItemIcon><ListItemText primary={item.label} primaryTypographyProps={{ fontSize: 12, fontWeight: 650 }} /></ListItemButton>)}</List></Box>)}</Box>
    <Box sx={{ m: 1.5, p: 1.3, borderRadius: 2.5, border: '1px solid rgba(255,255,255,.08)', bgcolor: 'rgba(255,255,255,.035)' }}><Stack direction="row" alignItems="center" gap={1}><Box sx={{ width: 7, height: 7, borderRadius: '50%', bgcolor: backendOnline === false ? '#ee7272' : backendOnline === null ? '#dca94c' : '#42d996', boxShadow: `0 0 0 4px ${backendOnline === false ? 'rgba(238,114,114,.12)' : 'rgba(66,217,150,.12)'}` }} /><Typography fontSize={10}>{backendOnline === null ? 'backend 检测中' : backendOnline ? 'backend 已连接' : 'backend 未连接'}</Typography></Stack><Typography fontSize={9} color="#668899" mt={.7}>曜图后台 · Vite + MUI</Typography></Box>
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
            <Typography fontSize={10} color="text.secondary">曜图管理中心 / 房间 {roomCode}</Typography>
            <Stack direction="row" alignItems="center" gap={1}>
              <Typography fontWeight={800} noWrap>{routeNames[path] ?? '运营首页'}</Typography>
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
            <IconButton aria-label="通知" onClick={event => { setNotificationAnchor(event.currentTarget); loadShellMeta() }}>
              <Badge badgeContent={unreadCount || undefined} color="error" variant={unreadCount ? 'standard' : 'dot'}>
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
            <Typography fontWeight={800}>系统通知</Typography>
            <Stack direction="row" gap={1} alignItems="center">
              {unreadCount > 0 ? <Chip size="small" label={`${unreadCount} 条未读`} color="error" /> : null}
              <Button size="small" onClick={() => void adminApi.markAllNotificationsRead().then(loadShellMeta).then(() => showMessage('已全部标记已读'))}>全部已读</Button>
            </Stack>
          </Stack>
          <Typography variant="caption" color="text.secondary">及时关注运行状态和风险提醒</Typography>
        </Box>
        <Divider />
        {notifications.length === 0 ? (
          <MenuItem disabled><ListItemText primary="暂无通知" /></MenuItem>
        ) : notifications.map(item => (
          <MenuItem
            key={item.id}
            sx={{ alignItems: 'flex-start', py: 1.5, whiteSpace: 'normal', opacity: item.read ? .7 : 1 }}
            onClick={() => {
              void adminApi.markNotificationRead(item.id).then(loadShellMeta)
              setNotificationAnchor(null)
              if (item.link) navigate(item.link)
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
                <ListItemText primary={item.label} secondary={groups.find(group => group.items.some(entry => entry.path === item.path))?.label} />
              </ListItemButton>
            ))}
            {pages.length === 0 && <Typography color="text.secondary" textAlign="center" py={4}>没有找到对应页面</Typography>}
          </List>
        </DialogContent>
      </Dialog>
    </Box>
  )
}
