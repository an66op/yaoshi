import {
  Alert, Avatar, Box, Button, Card, Chip, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle,
  Divider, FormControlLabel, IconButton, InputAdornment, MenuItem, Paper, Stack, Switch, TextField, Typography,
} from '@mui/material'
import SmartToyRounded from '@mui/icons-material/SmartToyRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import EditRounded from '@mui/icons-material/EditRounded'
import AccountBalanceWalletRounded from '@mui/icons-material/AccountBalanceWalletRounded'
import PlayArrowRounded from '@mui/icons-material/PlayArrowRounded'
import CasinoRounded from '@mui/icons-material/CasinoRounded'
import CheckRounded from '@mui/icons-material/CheckRounded'
import AutoAwesomeRounded from '@mui/icons-material/AutoAwesomeRounded'
import RestartAltRounded from '@mui/icons-material/RestartAltRounded'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { adminApi, type AdminGame, type AdminUser, type RobotResetInput, type RobotSetting, type RobotWorkspaceOption, type RoomActivityStatus } from '../api'
import { useFeedback } from '../components/feedback'
import { RobotResetDialog } from '../components/RobotResetDialog'
import { gameLogo } from '../gameLogos'

const money = (value: number) => new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value || 0)
const timeText = (value?: string | null) => value ? new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(new Date(value)) : '尚未运行'
const initialStatus: RoomActivityStatus = { running: false, enabled: false, interval_secs: 30, bots_per_room: 0, bets_per_cycle: 1, chat_chance_percent: 0, target_rooms: 0, enabled_games: 0, bot_accounts: 0, cycles: 0, bets_placed: 0, chats_posted: 0 }

const robotAvatars = [
  ...Array.from({ length: 16 }, (_, index) => `/images/avatars/avatar-anime-${String(index).padStart(2, '0')}.png`),
  ...Array.from({ length: 16 }, (_, index) => `/images/avatars/avatar-${String(index).padStart(2, '0')}.png`),
  ...Array.from({ length: 20 }, (_, index) => `/images/avatars/avatar-${index + 1}.jpg`),
]
const nameHeads = ['星河', '月影', '青柠', '云端', '银翼', '微光', '橙色', '蓝鲸', '晴空', '晚风', '鹿鸣', '星轨']
const nameTails = ['漫游者', '收藏家', '小队', '旅人', '信号', '捕手', '玩家', '来信', '汽水', '探索者', '闪电', '日记']
const nameMarks = ['✦', '·', '「', '」', '⁺', '°', '☆', 'ᵕ̈']
const randomNickname = () => {
  const head = nameHeads[Math.floor(Math.random() * nameHeads.length)]
  const tail = nameTails[Math.floor(Math.random() * nameTails.length)]
  const mark = nameMarks[Math.floor(Math.random() * nameMarks.length)]
  return Math.random() > .5 ? `${mark}${head}${tail}` : `${head}${mark}${tail}`
}

export function RobotsPage() {
  const [items, setItems] = useState<AdminUser[]>([])
  const [workspaces, setWorkspaces] = useState<RobotWorkspaceOption[]>([])
  const [workspaceID, setWorkspaceID] = useState(0)
  const [games, setGames] = useState<AdminGame[]>([])
  const [runtime, setRuntime] = useState<RobotSetting | null>(null)
  const [status, setStatus] = useState<RoomActivityStatus>(initialStatus)
  const [query, setQuery] = useState('')
  const [stateFilter, setStateFilter] = useState('all')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState<AdminUser | null>(null)
  const [nickname, setNickname] = useState('')
  const [avatar, setAvatar] = useState('')
  const [enabled, setEnabled] = useState(true)
  const [gameIDs, setGameIDs] = useState<string[]>([])
  const [activeStart, setActiveStart] = useState('')
  const [activeEnd, setActiveEnd] = useState('')
  const [minBet, setMinBet] = useState('')
  const [maxBet, setMaxBet] = useState('')
  const [balanceUser, setBalanceUser] = useState<AdminUser | null>(null)
  const [balanceAmount, setBalanceAmount] = useState('')
  const [balanceRemark, setBalanceRemark] = useState('机器人积分调整')
  const [resetOpen, setResetOpen] = useState(false)
  const loadSequence = useRef(0)
  const { showMessage } = useFeedback()

  const load = useCallback(async () => {
    if (!workspaceID) return
    const sequence = ++loadSequence.current
    setLoading(true); setError('')
    try {
      const [users, gameRows, robotSetting, activity] = await Promise.all([
        adminApi.users({ kind: 'robot', workspaceId: workspaceID, page: 1, pageSize: 100 }), adminApi.robotWorkspaceGames(workspaceID), adminApi.robotSetting(workspaceID), adminApi.roomActivityStatus(),
      ])
      if (sequence !== loadSequence.current) return
      setItems(users.items); setGames(gameRows.filter(item => item.enabled)); setRuntime(robotSetting); setStatus(activity)
    } catch (reason) {
      if (sequence === loadSequence.current) setError(reason instanceof Error ? reason.message : '读取机器人配置失败')
    } finally {
      if (sequence === loadSequence.current) setLoading(false)
    }
  }, [workspaceID])
  useEffect(() => {
    let cancelled = false
    void adminApi.robotWorkspaces().then(rows => {
      if (cancelled) return
      setWorkspaces(rows)
      setWorkspaceID(current => current && rows.some(row => row.workspace_id === current) ? current : (rows[0]?.workspace_id ?? 0))
      if (!rows.length) setLoading(false)
    }).catch(reason => {
      if (cancelled) return
      setError(reason instanceof Error ? reason.message : '读取机器人工作区失败'); setLoading(false)
    })
    return () => { cancelled = true }
  }, [])
  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0)
    return () => window.clearTimeout(timer)
  }, [load])

  const filtered = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    return items.filter(item => {
      if (stateFilter === 'active' && item.status !== 1) return false
      if (stateFilter === 'disabled' && item.status !== 0) return false
      return !keyword || [item.nickname, item.username, String(item.public_id)].some(value => String(value || '').toLowerCase().includes(keyword))
    })
  }, [items, query, stateFilter])
  const gameGroups = useMemo(() => {
    const groups = new Map<string, AdminGame[]>()
    games.forEach(game => { const category = game.lobby_category?.trim() || game.category?.trim() || '其他'; groups.set(category, [...(groups.get(category) ?? []), game]) })
    return Array.from(groups.entries())
  }, [games])
  const selectedWorkspace = useMemo(() => workspaces.find(item => item.workspace_id === workspaceID), [workspaceID, workspaces])
  const selectedWorkspaceLabel = selectedWorkspace?.type === 'platform' ? '平台大厅' : selectedWorkspace?.room_code ? `房间 ${selectedWorkspace.room_code}（${selectedWorkspace.name}）` : selectedWorkspace?.name || '所选工作区'

  const openEdit = (item: AdminUser) => {
    setEditing(item); setNickname(item.nickname || item.username); setAvatar(item.robot_avatar || robotAvatars[item.id % robotAvatars.length]); setEnabled(item.status === 1)
    setGameIDs(item.robot_game_ids ?? []); setActiveStart(item.robot_active_start ?? ''); setActiveEnd(item.robot_active_end ?? '')
    setMinBet(item.robot_min_bet ? String(item.robot_min_bet) : ''); setMaxBet(item.robot_max_bet ? String(item.robot_max_bet) : '')
  }
  const saveRobot = async () => {
    if (!editing || !nickname.trim()) return
    setSaving(true); setError('')
    try {
      const updated = await adminApi.updateRobot(editing.id, { nickname: nickname.trim(), avatar, status: enabled ? 1 : 0, game_ids: gameIDs, active_start: activeStart, active_end: activeEnd, min_bet: Number(minBet) || 0, max_bet: Number(maxBet) || 0 })
      setItems(current => current.map(item => item.id === updated.id ? updated : item)); setEditing(null); showMessage('机器人配置已保存')
    } catch (reason) { setError(reason instanceof Error ? reason.message : '保存机器人配置失败') }
    finally { setSaving(false) }
  }
  const updateRuntime = async (payload: Partial<Pick<RobotSetting, 'enabled' | 'interval_secs' | 'bets_per_cycle' | 'daily_bet_limit' | 'max_pending_bets'>>) => {
    if (!runtime) return
    const previous = runtime; setRuntime({ ...runtime, ...payload }); setSaving(true)
    try {
      const next = await adminApi.updateRobotSetting(workspaceID, payload); setRuntime(next); setStatus(await adminApi.roomActivityStatus())
      if (payload.enabled !== undefined) showMessage(payload.enabled ? '自动下注已启动' : '自动下注已停止')
    } catch (reason) { setRuntime(previous); setError(reason instanceof Error ? reason.message : '保存自动下注配置失败') }
    finally { setSaving(false) }
  }
  const adjustBalance = async () => {
    const amount = Number(balanceAmount)
    if (!balanceUser || !Number.isFinite(amount) || amount === 0 || !balanceRemark.trim()) return
    setSaving(true)
    try { const updated = await adminApi.adjustUserBalance(balanceUser.id, amount, balanceRemark.trim()); setItems(current => current.map(item => item.id === updated.id ? updated : item)); setBalanceUser(null); setBalanceAmount(''); showMessage('机器人积分已调整') }
    catch (reason) { setError(reason instanceof Error ? reason.message : '调整积分失败') }
    finally { setSaving(false) }
  }
  const resetRobots = async (payload: RobotResetInput) => {
    setSaving(true); setError('')
    try {
      const result = await adminApi.resetRobots({ ...payload, workspace_id: workspaceID })
      setItems(result.items); setResetOpen(false)
      showMessage(result.duplicate ? '该次重置已处理，无需重复执行' : `已重置当前工作区 ${result.count} 个机器人`)
    } catch (reason) {
      const message = reason instanceof Error ? reason.message : '批量重置机器人失败'
      setError(message); showMessage(message, 'error')
    }
    finally { setSaving(false) }
  }

  if (loading && !runtime) return <Box minHeight="60vh" display="grid" sx={{ placeItems: 'center' }}><CircularProgress size={28} /></Box>
  const statCards = [
    { label: '已启用机器人', value: items.filter(item => item.status === 1).length, Icon: SmartToyRounded },
    { label: '可参与彩种', value: games.length, Icon: CasinoRounded }, { label: '全局累计执行', value: status.cycles, Icon: PlayArrowRounded },
    { label: '全局累计下注', value: status.bets_placed, Icon: AccountBalanceWalletRounded },
  ]

  return <Box p={{ xs: 2, lg: 2.5 }}>
    {error && <Alert severity="error" onClose={() => setError('')} sx={{ mb: 1.5 }}>{error}</Alert>}
    <Stack direction={{ xs: 'column', md: 'row' }} gap={1.4} mb={1.5}>{statCards.map(({ label, value, Icon }) => <Paper key={label} sx={{ flex: 1, p: 1.5, minWidth: 0 }}><Stack direction="row" alignItems="center" gap={1.2}><Avatar sx={{ bgcolor: 'primary.main', width: 38, height: 38 }}><Icon /></Avatar><Box><Typography fontSize={11} color="text.secondary">{label}</Typography><Typography fontSize={20} fontWeight={900}>{String(value)}</Typography></Box></Stack></Paper>)}</Stack>

    <Card sx={{ p: 1.6, mb: 1.5 }}><Stack direction={{ xs: 'column', lg: 'row' }} alignItems={{ lg: 'center' }} gap={1.2}>
      <Box minWidth={220}><Typography fontSize={15} fontWeight={900}>{selectedWorkspaceLabel} · 自动下注</Typography><Typography fontSize={10.5} color="text.secondary">今日 {runtime?.today_bets ?? 0}/{runtime?.daily_bet_limit ?? 200} · 待结 {runtime?.pending_bets ?? 0}/{runtime?.max_pending_bets ?? 50} · 上次 {timeText(runtime?.last_run_at)}</Typography></Box>
      <FormControlLabel control={<Switch disabled={saving} checked={Boolean(runtime?.enabled)} onChange={event => void updateRuntime({ enabled: event.target.checked })} />} label={runtime?.enabled ? '运行中' : '已停止'} />
      <TextField size="small" type="number" label="执行间隔（秒）" value={runtime?.interval_secs ?? 30} onChange={event => setRuntime(current => current ? { ...current, interval_secs: Math.max(5, Number(event.target.value) || 5) } : current)} onBlur={() => runtime && void updateRuntime({ interval_secs: runtime.interval_secs })} sx={{ width: 155 }} />
      <TextField size="small" type="number" label="每轮注数" value={runtime?.bets_per_cycle ?? 1} onChange={event => setRuntime(current => current ? { ...current, bets_per_cycle: Math.max(1, Number(event.target.value) || 1) } : current)} onBlur={() => runtime && void updateRuntime({ bets_per_cycle: runtime.bets_per_cycle })} sx={{ width: 130 }} />
      <TextField size="small" type="number" label="每日上限" value={runtime?.daily_bet_limit ?? 200} onChange={event => setRuntime(current => current ? { ...current, daily_bet_limit: Math.max(1, Number(event.target.value) || 1) } : current)} onBlur={() => runtime && void updateRuntime({ daily_bet_limit: runtime.daily_bet_limit })} sx={{ width: 130 }} />
      <TextField size="small" type="number" label="待结保护" value={runtime?.max_pending_bets ?? 50} onChange={event => setRuntime(current => current ? { ...current, max_pending_bets: Math.max(1, Number(event.target.value) || 1) } : current)} onBlur={() => runtime && void updateRuntime({ max_pending_bets: runtime.max_pending_bets })} sx={{ width: 130 }} />
      <Button color="warning" size="small" variant="outlined" startIcon={<RestartAltRounded />} disabled={saving || !workspaceID || !items.length} onClick={() => setResetOpen(true)} sx={{ whiteSpace: 'nowrap' }}>一键重置昵称与余额</Button>
    </Stack>{(runtime?.pause_reason || runtime?.last_error || status.paused_reason || status.last_error) && <Alert severity={runtime?.pause_reason || status.paused_reason ? 'info' : 'warning'} sx={{ mt: 1.2 }}>{runtime?.pause_reason || runtime?.last_error || status.paused_reason || status.last_error}</Alert>}</Card>

    <Card sx={{ overflow: 'hidden' }}>
      <Stack direction={{ xs: 'column', md: 'row' }} gap={1} p={1.4} alignItems={{ md: 'center' }}>
        <TextField size="small" select label="目标工作区" value={workspaceID || ''} disabled={saving} onChange={event => { loadSequence.current += 1; setWorkspaceID(Number(event.target.value)); setItems([]); setRuntime(null) }} sx={{ minWidth: 210 }}>{workspaces.map(workspace => <MenuItem key={workspace.workspace_id} value={workspace.workspace_id}>{workspace.type === 'platform' ? '平台大厅' : workspace.room_code ? `${workspace.room_code} · ${workspace.name}` : workspace.name}（{workspace.robot_count}）</MenuItem>)}</TextField>
        <TextField size="small" value={query} onChange={event => setQuery(event.target.value)} placeholder="搜索编号、账号或昵称" sx={{ width: { xs: '100%', md: 260 } }} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> } }} />
        <TextField size="small" select label="状态" value={stateFilter} onChange={event => setStateFilter(event.target.value)} sx={{ minWidth: 130 }}><MenuItem value="all">全部状态</MenuItem><MenuItem value="active">已启用</MenuItem><MenuItem value="disabled">已停用</MenuItem></TextField>
        <Typography ml={{ md: 'auto' }} fontSize={11} color="text.secondary">每个机器人都是独立账号 · 未指定彩种时参与全部开放彩票</Typography>
      </Stack><Divider />
      <Box sx={{ overflowX: 'auto' }}><Box sx={{ minWidth: 790 }}>
        <Box sx={{ display: 'grid', gridTemplateColumns: '64px 140px 1.35fr 130px 90px 1.45fr 150px', px: 1.6, py: 1, bgcolor: 'action.hover' }}>{['头像', '会员编号', '账号 / 昵称', '积分', '状态', '参与彩种', '操作'].map(label => <Typography key={label} fontSize={11} fontWeight={850} color="text.secondary">{label}</Typography>)}</Box>
        {filtered.map(item => <Box key={item.id} sx={{ display: 'grid', gridTemplateColumns: '64px 140px 1.35fr 130px 90px 1.45fr 150px', alignItems: 'center', px: 1.6, py: 1.05, borderTop: 1, borderColor: 'divider', '&:hover': { bgcolor: 'action.hover' } }}>
          <Avatar src={item.robot_avatar} sx={{ width: 38, height: 38, bgcolor: item.status === 1 ? 'primary.main' : 'grey.500' }}>{(item.nickname || item.username).slice(0, 1)}</Avatar>
          <Typography fontSize={12} fontWeight={800}>ID {item.public_id}</Typography>
          <Box minWidth={0}><Typography fontSize={12.5} fontWeight={850} noWrap>{item.nickname || item.username}</Typography><Typography fontSize={10} color="text.secondary" noWrap>{item.username}</Typography></Box>
          <Typography fontSize={12} fontWeight={800}>{money(item.balance)}</Typography>
          <Chip size="small" color={item.status === 1 ? 'success' : 'default'} label={item.status === 1 ? '已启用' : '已停用'} sx={{ width: 'fit-content', height: 24, fontSize: 10 }} />
          <Box minWidth={0}><Typography fontSize={11} color="text.secondary" noWrap>{item.robot_game_ids?.length ? `${item.robot_game_ids.length} 个指定彩种` : '全部开放彩种'}</Typography><Typography fontSize={9.5} color="text.disabled" noWrap>{item.robot_active_start && item.robot_active_end ? `${item.robot_active_start}–${item.robot_active_end}` : '全天运行'} · {item.robot_min_bet && item.robot_max_bet ? `${item.robot_min_bet}–${item.robot_max_bet}/注` : '默认金额'}</Typography></Box>
          <Stack direction="row" gap={.5}><Button size="small" startIcon={<EditRounded />} onClick={() => openEdit(item)}>配置</Button><Button size="small" color="secondary" startIcon={<AccountBalanceWalletRounded />} onClick={() => { setBalanceUser(item); setBalanceAmount(''); setBalanceRemark('机器人积分调整') }}>积分</Button></Stack>
        </Box>)}
        {!filtered.length && <Box py={8} textAlign="center"><SmartToyRounded sx={{ fontSize: 42, color: 'text.disabled' }} /><Typography color="text.secondary" fontSize={12}>暂无机器人账号</Typography></Box>}
      </Box></Box>
    </Card>

    <Dialog open={Boolean(editing)} onClose={() => !saving && setEditing(null)} fullWidth maxWidth="md">
      <DialogTitle><Stack direction="row" alignItems="center" gap={1}><Avatar src={avatar} sx={{ bgcolor: 'primary.main' }}><SmartToyRounded /></Avatar><Box><Typography fontSize={17} fontWeight={900}>机器人配置</Typography><Typography fontSize={10.5} color="text.secondary">{editing?.username}</Typography></Box></Stack></DialogTitle>
      <DialogContent dividers>
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.2} alignItems={{ sm: 'center' }} mb={1.4}><TextField fullWidth size="small" label="显示昵称" value={nickname} onChange={event => setNickname(event.target.value)} inputProps={{ maxLength: 50 }} /><Button variant="outlined" startIcon={<AutoAwesomeRounded />} sx={{ whiteSpace: 'nowrap' }} onClick={() => setNickname(randomNickname())}>随机昵称</Button><FormControlLabel sx={{ minWidth: 130 }} control={<Switch checked={enabled} onChange={event => setEnabled(event.target.checked)} />} label={enabled ? '已启用' : '已停用'} /></Stack>
        <Box mb={1.6}><Typography fontSize={12} fontWeight={850} mb={.8}>选择头像</Typography><Stack direction="row" gap={.75} flexWrap="wrap" useFlexGap>{robotAvatars.map(path => <IconButton key={path} onClick={() => setAvatar(path)} sx={{ p: .25, border: 2, borderColor: avatar === path ? 'primary.main' : 'transparent' }}><Avatar src={path} sx={{ width: 36, height: 36 }} /></IconButton>)}</Stack></Box>
        <Paper variant="outlined" sx={{ p: 1.25, mb: 1.6 }}><Stack direction={{ xs: 'column', sm: 'row' }} gap={1.1}><TextField fullWidth size="small" type="time" label="运行开始" value={activeStart} onChange={event => setActiveStart(event.target.value)} slotProps={{ inputLabel: { shrink: true } }} helperText="留空表示全天" /><TextField fullWidth size="small" type="time" label="运行结束" value={activeEnd} onChange={event => setActiveEnd(event.target.value)} slotProps={{ inputLabel: { shrink: true } }} helperText="支持跨午夜" /><TextField fullWidth size="small" type="number" label="最小单注" value={minBet} onChange={event => setMinBet(event.target.value)} inputProps={{ min: 0, step: .01 }} helperText="0 使用默认" /><TextField fullWidth size="small" type="number" label="最大单注" value={maxBet} onChange={event => setMaxBet(event.target.value)} inputProps={{ min: 0, step: .01 }} helperText="0 使用默认" /></Stack></Paper>
        <Stack direction="row" justifyContent="space-between" alignItems="center" mb={1}><Box><Typography fontSize={13} fontWeight={850}>参与彩种</Typography><Typography fontSize={10} color="text.secondary">勾选后只参与指定彩种；不选则跟随全部</Typography></Box><Button size="small" onClick={() => setGameIDs([])}>跟随全部</Button></Stack>
        <Stack gap={1.25}>{gameGroups.map(([category, rows]) => <Paper key={category} variant="outlined" sx={{ p: 1.25 }}><Typography fontSize={11} fontWeight={900} mb={.8}>{category}</Typography><Stack direction="row" gap={.7} flexWrap="wrap" useFlexGap>{rows.map(game => { const selected = gameIDs.includes(game.id); return <Chip key={game.id} clickable color={selected ? 'primary' : 'default'} variant={selected ? 'filled' : 'outlined'} icon={selected ? <CheckRounded /> : undefined} avatar={<Avatar src={gameLogo(game.id)}>{game.name.slice(0, 1)}</Avatar>} label={game.name} onClick={() => setGameIDs(current => current.includes(game.id) ? current.filter(id => id !== game.id) : [...current, game.id])} /> })}</Stack></Paper>)}</Stack>
      </DialogContent>
      <DialogActions><Button onClick={() => setEditing(null)}>取消</Button><Button variant="contained" disabled={saving || !nickname.trim()} onClick={() => void saveRobot()}>保存配置</Button></DialogActions>
    </Dialog>

    <Dialog open={Boolean(balanceUser)} onClose={() => !saving && setBalanceUser(null)} fullWidth maxWidth="xs"><DialogTitle>调整机器人积分</DialogTitle><DialogContent><Typography fontSize={11} color="text.secondary" mb={1.5}>{balanceUser?.nickname} · 当前 {money(balanceUser?.balance ?? 0)}</Typography><Stack gap={1.4}><TextField autoFocus fullWidth type="number" label="调整金额" value={balanceAmount} onChange={event => setBalanceAmount(event.target.value)} helperText="正数增加，负数扣减" /><TextField fullWidth label="备注" value={balanceRemark} onChange={event => setBalanceRemark(event.target.value)} inputProps={{ maxLength: 300 }} /></Stack></DialogContent><DialogActions><Button onClick={() => setBalanceUser(null)}>取消</Button><Button variant="contained" disabled={saving || !Number(balanceAmount) || !balanceRemark.trim()} onClick={() => void adjustBalance()}>确认调整</Button></DialogActions></Dialog>
    {resetOpen && <RobotResetDialog open robotCount={items.length} scopeLabel={selectedWorkspaceLabel} submitting={saving} onClose={() => setResetOpen(false)} onSubmit={resetRobots} />}
  </Box>
}
