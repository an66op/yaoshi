import {
  Alert,
  Avatar,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  Drawer,
  IconButton,
  InputAdornment,
  MenuItem,
  Paper,
  Stack,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material'
import AddRounded from '@mui/icons-material/AddRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import DownloadRounded from '@mui/icons-material/DownloadRounded'
import { createCsv } from '../utils/csv'
import EditRounded from '@mui/icons-material/EditRounded'
import KeyRounded from '@mui/icons-material/KeyRounded'
import AccountBalanceWalletRounded from '@mui/icons-material/AccountBalanceWalletRounded'
import VisibilityRounded from '@mui/icons-material/VisibilityRounded'
import PeopleAltRounded from '@mui/icons-material/PeopleAltRounded'
import PersonAddAltRounded from '@mui/icons-material/PersonAddAltRounded'
import BlockRounded from '@mui/icons-material/BlockRounded'
import AdminPanelSettingsRounded from '@mui/icons-material/AdminPanelSettingsRounded'
import CloseRounded from '@mui/icons-material/CloseRounded'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { adminApi, type AdminGame, type AdminUser, type AgentItem, type BalanceRecord, type UserPayload, type UserStats, type UserTradingConfig } from '../api'
import { GameOddsNavigation, OddsOverrideGrid } from '../components/OddsEditors'
import { PageHeader } from '../components/PageHeader'
import { UserPresenceChip } from '../components/UserPresenceChip'
import { useFeedback } from '../components/feedback'

const roleLabels: Record<AdminUser['role'], string> = { member: '普通会员', agent: '代理', tenant: '租户', admin: '总管理员' }
const riskLabels: Record<AdminUser['risk_level'], string> = { normal: '正常', watch: '关注', restricted: '限制' }
const emptyForm: UserPayload = { username: '', password: '', email: '', nickname: '', phone: '', role: 'member', remark: '', risk_level: 'normal', status: 1 }
const money = (value: number) => new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)
const dateTime = (value?: string | null) => value ? new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(value)) : '从未登录'

export function UsersPage() {
  const [users, setUsers] = useState<AdminUser[]>([])
  const [stats, setStats] = useState<UserStats | null>(null)
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState(20)
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('all')
  const [applied, setApplied] = useState({ query: '', status: 'all' })
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<AdminUser | null>(null)
  const [form, setForm] = useState<UserPayload>(emptyForm)
  const [resetUser, setResetUser] = useState<AdminUser | null>(null)
  const [newPassword, setNewPassword] = useState('')
  const [balanceUser, setBalanceUser] = useState<AdminUser | null>(null)
  const [balanceAmount, setBalanceAmount] = useState('')
  const [balanceRemark, setBalanceRemark] = useState('')
  const [detailUser, setDetailUser] = useState<AdminUser | null>(null)
  const [history, setHistory] = useState<BalanceRecord[]>([])
  const [games, setGames] = useState<AdminGame[]>([])
  const [agents, setAgents] = useState<AgentItem[]>([])
  const [trading, setTrading] = useState<UserTradingConfig | null>(null)
  const [tradingLoading, setTradingLoading] = useState(false)
  const [tradingError, setTradingError] = useState('')
  const [tradingSaving, setTradingSaving] = useState(false)
  const [tradingDirty, setTradingDirty] = useState(false)
  const { showMessage } = useFeedback()
  const loadRequestRef = useRef(0)
  const detailRequestRef = useRef(0)
  const detailUserIdRef = useRef<number | null>(null)
  const tradingRequestRef = useRef(0)
  const tradingRef = useRef<UserTradingConfig | null>(null)
  const tradingLoadingRef = useRef(false)
  const tradingDirtyRef = useRef(false)
  const tradingWriteRef = useRef<number | null>(null)
  const mountedRef = useRef(true)
  const renderedDetailRequest = detailRequestRef.current
  const renderedTradingRequest = tradingRequestRef.current
  const tradingUnavailable = tradingLoading || tradingSaving || !trading || trading.user_id !== detailUser?.id

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      loadRequestRef.current += 1
      detailRequestRef.current += 1
      tradingRequestRef.current += 1
      detailUserIdRef.current = null
      tradingRef.current = null
      tradingLoadingRef.current = false
      tradingDirtyRef.current = false
      tradingWriteRef.current = null
    }
  }, [])

  const load = useCallback(async (notify = false, silent = false) => {
    const requestID = ++loadRequestRef.current
    if (!silent) {
      setLoading(true)
      setError('')
    }
    try {
      const [list, nextStats] = await Promise.all([
        adminApi.users({ ...applied, kind: 'member', role: 'member', page: page + 1, pageSize }),
        adminApi.userStats('member'),
      ])
      if (requestID !== loadRequestRef.current) return
      const nextUsers = Array.isArray(list?.items) ? list.items.filter(user => user.role === 'member') : []
      setUsers(nextUsers)
      setDetailUser(current => {
        if (!current) return current
        const refreshed = nextUsers.find(item => item.id === current.id)
        return refreshed ? { ...current, online: refreshed.online === true } : current
      })
      setTotal(list.total)
      setStats(nextStats)
      if (notify) showMessage('会员数据已刷新')
    } catch (reason) {
      if (requestID === loadRequestRef.current && !silent) setError(reason instanceof Error ? reason.message : '读取会员数据失败')
    } finally {
      if (requestID === loadRequestRef.current) setLoading(false)
    }
  }, [applied, page, pageSize, showMessage])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])
  useEffect(() => {
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') void load(false, true)
    }, 25_000)
    return () => window.clearInterval(timer)
  }, [load])

  const openCreate = async () => {
    setEditing(null)
    setForm({ ...emptyForm, parent_agent_id: 0 })
    setFormOpen(true)
    if (agents.length === 0) {
      try {
        const result = await adminApi.agents({ page: 1, pageSize: 100 })
        const nextAgents = Array.isArray(result?.items) ? result.items : []
        setAgents(nextAgents)
        if (nextAgents.length) setForm(current => ({ ...current, parent_agent_id: nextAgents[0].id }))
      } catch (reason) {
        setError(reason instanceof Error ? reason.message : '读取代理房间失败')
      }
    }
  }

  const openEdit = (user: AdminUser) => {
    setEditing(user)
    setForm({ email: user.email, nickname: user.nickname, phone: user.phone, role: 'member', remark: user.remark, risk_level: user.risk_level, status: user.status })
    setFormOpen(true)
  }

  const submitUser = async () => {
    if (!editing && (!form.username?.trim() || new TextEncoder().encode(form.password ?? '').length < 8)) {
      setError('请填写用户名，并设置 8–72 位密码')
      return
    }
    setSaving(true)
    setError('')
    try {
      if (editing) await adminApi.updateUser(editing.id, { ...form, role: 'member' })
      else await adminApi.createUser({ ...form, role: 'member' })
      setFormOpen(false)
      showMessage(editing ? '会员资料已更新' : '会员创建成功')
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const toggleStatus = async (user: AdminUser) => {
    try {
      const next = await adminApi.setUserStatus(user.id, user.status === 1 ? 0 : 1)
      setUsers(current => current.map(item => item.id === next.id ? next : item))
      setStats(current => current ? { ...current, active: current.active + (next.status === 1 ? 1 : -1), disabled: current.disabled + (next.status === 0 ? 1 : -1) } : current)
      showMessage(`${user.username} 已${next.status === 1 ? '启用' : '停用'}`)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '状态更新失败')
    }
  }

  const submitPassword = async () => {
    if (!resetUser || new TextEncoder().encode(newPassword).length < 8) {
      setError('新密码需要 8–72 个字符')
      return
    }
    setSaving(true)
    try {
      await adminApi.resetUserPassword(resetUser.id, newPassword)
      setResetUser(null)
      setNewPassword('')
      showMessage('密码已重置')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '密码重置失败')
    } finally {
      setSaving(false)
    }
  }

  const submitBalance = async () => {
    const amount = Number(balanceAmount)
    if (!balanceUser || !Number.isFinite(amount) || amount === 0 || !balanceRemark.trim()) {
      setError('请输入非零调整金额并填写原因')
      return
    }
    setSaving(true)
    try {
      const next = await adminApi.adjustUserBalance(balanceUser.id, amount, balanceRemark)
      setUsers(current => current.map(item => item.id === next.id ? next : item))
      setBalanceUser(null)
      setBalanceAmount('')
      setBalanceRemark('')
      showMessage(`余额调整成功，当前余额 ${money(next.balance)}`)
      const nextStats = await adminApi.userStats('member')
      setStats(nextStats)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '余额调整失败')
    } finally {
      setSaving(false)
    }
  }

  const resetTradingContext = () => {
    tradingRequestRef.current += 1
    tradingRef.current = null
    tradingDirtyRef.current = false
    tradingLoadingRef.current = false
    tradingWriteRef.current = null
    setTrading(null)
    setTradingDirty(false)
    setTradingLoading(false)
    setTradingSaving(false)
    setTradingError('')
  }

  const acceptTrading = (next: UserTradingConfig) => {
    tradingRef.current = next
    tradingDirtyRef.current = false
    setTrading(next)
    setTradingDirty(false)
  }

  const loadTrading = async (userId: number, gameId?: string) => {
    if (!mountedRef.current || detailUserIdRef.current !== userId || tradingWriteRef.current !== null) return
    const detailRequest = detailRequestRef.current
    const requestID = ++tradingRequestRef.current
    const isCurrent = () => mountedRef.current && detailRequest === detailRequestRef.current && userId === detailUserIdRef.current && requestID === tradingRequestRef.current
    tradingLoadingRef.current = true
    setTradingLoading(true)
    setTradingError('')
    try {
      const next = await adminApi.userTrading(userId, gameId)
      if (!isCurrent()) return
      if (next.user_id !== userId || (gameId !== undefined && next.game_id !== gameId)) throw new Error('返回的会员或彩种不一致，请重新读取交易配置')
      acceptTrading(next)
    } catch (reason) {
      if (isCurrent()) setTradingError(reason instanceof Error ? reason.message : '读取飞单与赔率失败')
    } finally {
      if (isCurrent()) {
        tradingLoadingRef.current = false
        setTradingLoading(false)
      }
    }
  }

  const openDetail = async (user: AdminUser) => {
    if (!mountedRef.current || tradingWriteRef.current !== null) return
    const requestID = ++detailRequestRef.current
    detailUserIdRef.current = user.id
    resetTradingContext()
    setDetailUser(user)
    setHistory([])
    setGames([])
    if (user.role === 'member') void loadTrading(user.id)
    const [historyResult, dashboardResult] = await Promise.allSettled([
      adminApi.userBalanceHistory(user.id),
      adminApi.dashboard(),
    ])
    if (!mountedRef.current || requestID !== detailRequestRef.current || detailUserIdRef.current !== user.id) return
    if (historyResult.status === 'fulfilled') setHistory(historyResult.value)
    if (dashboardResult.status === 'fulfilled') setGames(dashboardResult.value.games ?? [])
    const failure = [historyResult, dashboardResult].find(result => result.status === 'rejected')
    if (failure?.status === 'rejected') setError(failure.reason instanceof Error ? failure.reason.message : '部分会员详情暂时无法读取')
  }

  const closeDetail = () => {
    if (!mountedRef.current || renderedDetailRequest !== detailRequestRef.current) return
    detailRequestRef.current += 1
    detailUserIdRef.current = null
    resetTradingContext()
    setDetailUser(null)
  }

  const isCurrentTradingEditor = () => mountedRef.current && renderedDetailRequest === detailRequestRef.current && renderedTradingRequest === tradingRequestRef.current && detailUser?.id === detailUserIdRef.current && tradingRef.current?.user_id === detailUser?.id && tradingRef.current?.game_id === trading?.game_id

  const editTrading = (update: (current: UserTradingConfig) => UserTradingConfig) => {
    if (!isCurrentTradingEditor() || !tradingRef.current || tradingLoadingRef.current || tradingWriteRef.current !== null) return
    const next = update(tradingRef.current)
    tradingRef.current = next
    tradingDirtyRef.current = true
    setTrading(next)
    setTradingDirty(true)
  }

  const saveTrading = async () => {
    if (!isCurrentTradingEditor() || !tradingRef.current || !tradingDirtyRef.current || tradingLoadingRef.current || tradingWriteRef.current !== null) return
    const draft = tradingRef.current
    const userId = draft.user_id
    const oddsMultiplier = Number(draft.odds_multiplier ?? 1)
    if (!Number.isFinite(oddsMultiplier) || oddsMultiplier < 0.5 || oddsMultiplier > 1.5) {
      setTradingError('会员赔率倍率必须在 0.50–1.50 之间')
      return
    }
    const detailRequest = detailRequestRef.current
    const requestID = ++tradingRequestRef.current
    tradingWriteRef.current = requestID
    const isCurrent = () => mountedRef.current && detailRequest === detailRequestRef.current && userId === detailUserIdRef.current && requestID === tradingRequestRef.current && tradingWriteRef.current === requestID
    setTradingSaving(true)
    setTradingError('')
    try {
      const next = await adminApi.updateUserTrading(userId, {
        odds_multiplier: oddsMultiplier,
        fly_mode: draft.fly.mode,
        fly_rate: draft.fly.rate,
        rebate_mode: draft.rebate.mode,
        rebate_rate: draft.rebate.rate,
        game_id: draft.game_id,
        odds: draft.odds.map(item => ({ play_code: item.play_code, override: item.has_override ? item.override : null })),
      })
      if (!isCurrent()) return
      if (next.user_id !== userId || next.game_id !== draft.game_id) throw new Error('返回的会员或彩种不一致，请重新读取交易配置')
      acceptTrading(next)
      setDetailUser(current => current?.id === userId ? { ...current, fly_mode: next.fly.mode, fly_rate: next.fly.rate } : current)
      setUsers(current => current.map(item => item.id === userId ? { ...item, fly_mode: next.fly.mode, fly_rate: next.fly.rate } : item))
      showMessage('飞单、返水与单独赔率已保存')
    } catch (reason) {
      if (isCurrent()) setTradingError(reason instanceof Error ? reason.message : '保存失败')
    } finally {
      if (isCurrent()) {
        tradingWriteRef.current = null
        setTradingSaving(false)
      }
    }
  }

  const flyModeLabel = (mode?: string) => ({ inherit: '跟随房间', custom: '单独比例', off: '不飞单' }[mode ?? 'inherit'] ?? mode)

  const applyFilters = () => { setPage(0); setApplied({ query: query.trim(), status }) }
  const resetFilters = () => { setQuery(''); setStatus('all'); setPage(0); setApplied({ query: '', status: 'all' }) }
  const exportCsv = () => {
    const rows = users.map(user => [user.public_id, user.username, user.nickname, roleLabels[user.role], user.email, user.phone, user.balance.toFixed(2), user.online === true ? '在线' : '离线', user.status === 1 ? '启用' : '停用', dateTime(user.created_at)])
    const csv = createCsv([['用户 ID', '用户名', '昵称', '角色', '邮箱', '手机', '余额', '在线状态', '账号状态', '创建时间'], ...rows])
    const link = document.createElement('a')
    link.href = URL.createObjectURL(new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8' }))
    link.download = '会员列表.csv'
    link.click()
    URL.revokeObjectURL(link.href)
  }

  const statCards = useMemo(() => [
    ['会员总数', stats?.total ?? 0, PeopleAltRounded, '#4f7edc'],
    ['正常会员', stats?.active ?? 0, PersonAddAltRounded, '#2eaf7b'],
    ['停用会员', stats?.disabled ?? 0, BlockRounded, '#df746a'],
    ['今日新增', stats?.new_today ?? 0, AdminPanelSettingsRounded, '#8a70df'],
  ] as const, [stats])

  return <Box p={{ xs: 2, lg: 2.5 }}>
    <PageHeader eyebrow="组织与账号 / 会员" title="会员管理" description="" actions={<><Button variant="outlined" startIcon={<DownloadRounded />} disabled={!users.length} onClick={exportCsv}>导出当前页</Button><Button variant="contained" startIcon={<AddRounded />} onClick={() => void openCreate()}>新增会员</Button></>} />
    {error && <Alert severity="error" onClose={() => setError('')} sx={{ mt: 2 }}>{error}</Alert>}
    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2,1fr)', lg: 'repeat(4,1fr)' }, gap: 1.25, mt: 2.5 }}>{statCards.map(([label, value, Icon, color]) => <Card key={label}><CardContent sx={{ p: '15px !important' }}><Stack direction="row" alignItems="center" justifyContent="space-between"><Box><Typography variant="caption" color="text.secondary">{label}</Typography><Typography fontSize={{ xs: 20, sm: 24 }} fontWeight={850} mt={.4}>{value}</Typography></Box><Box sx={{ width: 40, height: 40, borderRadius: 2.5, display: 'grid', placeItems: 'center', color: '#fff', bgcolor: color }}><Icon fontSize="small" /></Box></Stack></CardContent></Card>)}</Box>
    <Paper variant="outlined" sx={{ p: 1.5, mt: 1.5 }}><Stack direction={{ xs: 'column', md: 'row' }} gap={1}><TextField placeholder="搜索会员账号、昵称、代理或联系方式" value={query} onChange={event => setQuery(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') applyFilters() }} sx={{ flex: 1, minWidth: { md: 280 } }} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> } }} /><TextField select label="账号状态" value={status} onChange={event => setStatus(event.target.value)} sx={{ minWidth: 140 }}><MenuItem value="all">全部状态</MenuItem><MenuItem value="active">正常</MenuItem><MenuItem value="disabled">已停用</MenuItem></TextField><Button variant="contained" onClick={applyFilters}>查询</Button><Button variant="text" onClick={resetFilters}>重置</Button></Stack></Paper>
    <Card sx={{ mt: 1.5 }}>{loading && <Box px={2} py={1}><CircularProgress size={18} /></Box>}<TableContainer><Table size="small" sx={{ minWidth: 1260 }}><TableHead><TableRow><TableCell>会员</TableCell><TableCell>登录标识</TableCell><TableCell>角色</TableCell><TableCell align="right">余额</TableCell><TableCell>联系方式</TableCell><TableCell>风控</TableCell><TableCell>在线状态</TableCell><TableCell>账号状态</TableCell><TableCell>最后登录</TableCell><TableCell align="right">操作</TableCell></TableRow></TableHead><TableBody>{users.map(user => <TableRow hover key={user.id}><TableCell><Stack direction="row" alignItems="center" gap={1}><Avatar sx={{ width: 34, height: 34, fontSize: 13, bgcolor: user.role === 'admin' ? 'secondary.main' : 'primary.main' }}>{(user.nickname || user.username).slice(0, 1).toUpperCase()}</Avatar><Box><Typography fontSize={12} fontWeight={800}>{user.nickname || user.username}</Typography><Typography fontSize={10} color="text.secondary">ID {user.public_id}</Typography></Box></Stack></TableCell><TableCell><Typography fontSize={11} fontWeight={750}>{user.login_identity || `平台 / ${user.username}`}</Typography><Typography fontSize={9} color="text.secondary">{[user.tenant_name, user.agent_name].filter(Boolean).join(' · ') || '平台直属'}</Typography></TableCell><TableCell><Chip size="small" variant="outlined" color={user.role === 'admin' ? 'secondary' : user.role === 'agent' ? 'info' : 'default'} label={roleLabels[user.role]} /></TableCell><TableCell align="right"><Typography fontWeight={800}>{money(user.balance)}</Typography></TableCell><TableCell><Typography fontSize={11}>{user.phone || '未填写手机'}</Typography><Typography fontSize={9} color="text.secondary">{user.email || '未填写邮箱'}</Typography></TableCell><TableCell><Chip size="small" color={user.risk_level === 'normal' ? 'success' : user.risk_level === 'watch' ? 'warning' : 'error'} variant="outlined" label={riskLabels[user.risk_level]} /></TableCell><TableCell><UserPresenceChip online={user.online === true} /></TableCell><TableCell><Stack direction="row" alignItems="center" gap={.5}><Switch size="small" checked={user.status === 1} onChange={() => void toggleStatus(user)} /><Typography fontSize={10}>{user.status === 1 ? '正常' : '停用'}</Typography></Stack></TableCell><TableCell><Typography fontSize={10}>{dateTime(user.last_login_at)}</Typography><Typography fontSize={9} color="text.secondary">登录 {user.login_count} 次</Typography></TableCell><TableCell align="right"><Stack direction="row" justifyContent="flex-end"><Tooltip title="查看详情"><IconButton size="small" onClick={() => void openDetail(user)}><VisibilityRounded fontSize="small" /></IconButton></Tooltip><Tooltip title="编辑资料"><IconButton size="small" onClick={() => openEdit(user)}><EditRounded fontSize="small" /></IconButton></Tooltip><Tooltip title="调整余额"><IconButton size="small" onClick={() => { setBalanceUser(user); setBalanceAmount(''); setBalanceRemark('') }}><AccountBalanceWalletRounded fontSize="small" /></IconButton></Tooltip><Tooltip title="重置密码"><IconButton size="small" onClick={() => { setResetUser(user); setNewPassword('') }}><KeyRounded fontSize="small" /></IconButton></Tooltip></Stack></TableCell></TableRow>)}{!loading && !users.length && <TableRow><TableCell colSpan={10} align="center" sx={{ py: 8, color: 'text.secondary' }}>没有找到符合条件的会员</TableCell></TableRow>}</TableBody></Table></TableContainer><TablePagination component="div" count={total} page={page} onPageChange={(_, next) => setPage(next)} rowsPerPage={pageSize} onRowsPerPageChange={event => setPageSize(Number(event.target.value))} rowsPerPageOptions={[10, 20, 50]} labelRowsPerPage="每页" /></Card>

    <Dialog open={formOpen} onClose={() => !saving && setFormOpen(false)} fullWidth maxWidth="md"><DialogTitle>{editing ? `编辑会员 · ${editing.username}` : '新增会员'}</DialogTitle><DialogContent><Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,1fr)' }, gap: 2, pt: 1 }}>{!editing && <><TextField required label="登录帐号" value={form.username ?? ''} onChange={event => setForm(current => ({ ...current, username: event.target.value }))} /><TextField required type="password" label="初始密码" helperText="8–72 个字符" value={form.password ?? ''} onChange={event => setForm(current => ({ ...current, password: event.target.value }))} /><TextField select label="所属代理房间" value={form.parent_agent_id ?? 0} onChange={event => setForm(current => ({ ...current, parent_agent_id: Number(event.target.value) }))}><MenuItem value={0}>平台大厅</MenuItem>{agents.map(agent => <MenuItem key={agent.id} value={agent.id}>{agent.room_code} · {agent.nickname || agent.username}{agent.tenant_name ? ` · ${agent.tenant_name}` : ''}</MenuItem>)}</TextField></>}<TextField label="昵称" value={form.nickname} onChange={event => setForm(current => ({ ...current, nickname: event.target.value }))} /><TextField type="email" label="邮箱" value={form.email} onChange={event => setForm(current => ({ ...current, email: event.target.value }))} /><TextField label="手机号" value={form.phone} onChange={event => setForm(current => ({ ...current, phone: event.target.value }))} /><TextField disabled label="账号角色" value="普通会员" /><TextField select label="风控等级" value={form.risk_level} onChange={event => setForm(current => ({ ...current, risk_level: event.target.value as AdminUser['risk_level'] }))}><MenuItem value="normal">正常</MenuItem><MenuItem value="watch">重点关注</MenuItem><MenuItem value="restricted">限制账号</MenuItem></TextField><TextField select label="账号状态" value={form.status} onChange={event => setForm(current => ({ ...current, status: Number(event.target.value) as 0 | 1 }))}><MenuItem value={1}>正常</MenuItem><MenuItem value={0}>停用</MenuItem></TextField><TextField multiline minRows={3} label="管理备注" value={form.remark} onChange={event => setForm(current => ({ ...current, remark: event.target.value }))} sx={{ gridColumn: { sm: '1/-1' } }} /></Box></DialogContent><DialogActions><Button disabled={saving} onClick={() => setFormOpen(false)}>取消</Button><Button variant="contained" disabled={saving} onClick={() => void submitUser()}>{saving ? '保存中…' : '保存会员'}</Button></DialogActions></Dialog>

    <Dialog open={Boolean(resetUser)} onClose={() => !saving && setResetUser(null)} fullWidth maxWidth="xs"><DialogTitle>重置密码</DialogTitle><DialogContent><Typography variant="body2" color="text.secondary" mb={2}>为 {resetUser?.username} 设置新密码。保存后旧密码立即失效。</Typography><TextField autoFocus fullWidth type="password" label="新密码" helperText="8–72 个字符" value={newPassword} onChange={event => setNewPassword(event.target.value)} /></DialogContent><DialogActions><Button onClick={() => setResetUser(null)}>取消</Button><Button variant="contained" disabled={saving} onClick={() => void submitPassword()}>确认重置</Button></DialogActions></Dialog>

    <Dialog open={Boolean(balanceUser)} onClose={() => !saving && setBalanceUser(null)} fullWidth maxWidth="xs"><DialogTitle>调整会员余额</DialogTitle><DialogContent><Alert severity="info" sx={{ mb: 2 }}>当前余额：{money(balanceUser?.balance ?? 0)}。正数增加，负数扣减，操作会写入审计流水。</Alert><Stack gap={2}><TextField autoFocus type="number" label="调整金额" placeholder="例如 100 或 -50" value={balanceAmount} onChange={event => setBalanceAmount(event.target.value)} /><TextField required multiline minRows={3} label="调整原因" value={balanceRemark} onChange={event => setBalanceRemark(event.target.value)} /></Stack></DialogContent><DialogActions><Button onClick={() => setBalanceUser(null)}>取消</Button><Button variant="contained" disabled={saving} onClick={() => void submitBalance()}>确认调整</Button></DialogActions></Dialog>

    <Drawer anchor="right" open={Boolean(detailUser)} onClose={closeDetail} slotProps={{ paper: { sx: { width: { xs: '100%', sm: 720, lg: 860 }, p: { xs: 1.5, sm: 2.5 } } } }}>
      <Stack direction="row" justifyContent="space-between" alignItems="center">
        <Box>
          <Typography variant="overline" color="primary">会员详情</Typography>
          <Typography variant="h6" fontWeight={850}>{detailUser?.nickname || detailUser?.username}</Typography>
        </Box>
        <IconButton aria-label="关闭会员详情" onClick={closeDetail}><CloseRounded /></IconButton>
      </Stack>
      {detailUser && (
        <>
          <Stack direction="row" gap={1.5} alignItems="center" mt={2}>
            <Avatar sx={{ width: 54, height: 54, bgcolor: 'primary.main' }}>{(detailUser.nickname || detailUser.username)[0]}</Avatar>
            <Box>
              <Typography fontWeight={800}>@{detailUser.username}</Typography>
              <Typography variant="caption" color="text.secondary">ID {detailUser.id} · {roleLabels[detailUser.role]} · {flyModeLabel(detailUser.fly_mode)}</Typography>
            </Box>
            <Stack direction="row" gap={.7} sx={{ ml: 'auto' }}><UserPresenceChip online={detailUser.online === true} /><Chip size="small" color={detailUser.status === 1 ? 'success' : 'default'} variant="outlined" label={detailUser.status === 1 ? '账号正常' : '账号停用'} /></Stack>
          </Stack>
          <Card variant="outlined" sx={{ mt: 2 }}>
            <CardContent>
              <Stack gap={1}>
                {[['账户余额', money(detailUser.balance)], ['邮箱', detailUser.email || '未填写'], ['手机号', detailUser.phone || '未填写'], ['风控等级', riskLabels[detailUser.risk_level]], ['登录次数', String(detailUser.login_count)], ['最后登录', dateTime(detailUser.last_login_at)], ['创建时间', dateTime(detailUser.created_at)]].map(([label, value]) => (
                  <Stack direction="row" justifyContent="space-between" gap={2} key={label}>
                    <Typography variant="caption" color="text.secondary">{label}</Typography>
                    <Typography variant="caption" textAlign="right" fontWeight={700}>{value}</Typography>
                  </Stack>
                ))}
              </Stack>
              {detailUser.remark && <><Divider sx={{ my: 1.5 }} /><Typography variant="caption" color="text.secondary">管理备注</Typography><Typography variant="body2" mt={.5}>{detailUser.remark}</Typography></>}
            </CardContent>
          </Card>

          {detailUser.role === 'member' && <><Typography fontWeight={800} mt={2.5} mb={1}>会员交易配置</Typography>
          <Card variant="outlined">
            <CardContent>
              {tradingError && <Alert severity="error" sx={{ mb: 1 }}>{tradingError}</Alert>}
              {tradingLoading && <Typography variant="caption" color="text.secondary">加载交易配置中…</Typography>}
              {!trading ? (
                !tradingLoading && <Button onClick={() => {
                  if (renderedDetailRequest === detailRequestRef.current && renderedTradingRequest === tradingRequestRef.current && !tradingRef.current) void loadTrading(detailUser.id)
                }}>重新读取交易配置</Button>
              ) : (
                <Stack gap={1.5}>
                  <Alert severity="info" sx={{ py: 0.5 }}>
                    仅平台后台已配置且启用的玩法可设置覆盖。赔率优先级：会员单独赔率 → 当前房间赔率 × 会员倍率 → 平台已配置赔率 × 会员倍率。会员进入其他房间后，只使用新房间内的会员配置。
                  </Alert>

                  <Paper variant="outlined" sx={{ p: 1.25, borderRadius: 2.2 }}>
                    <Stack direction={{ xs: 'column', md: 'row' }} justifyContent="space-between" gap={1.2} alignItems={{ md: 'center' }}>
                      <Box>
                        <Typography fontSize={12.5} fontWeight={900}>会员赔率倍率</Typography>
                        <Typography fontSize={10} color="text.secondary">作用于该会员继承的房间赔率；玩法单独赔率仍然优先。</Typography>
                      </Box>
                      <TextField
                        size="small"
                        type="number"
                        label="倍率"
                        disabled={tradingUnavailable}
                        value={trading.odds_multiplier ?? 1}
                        onChange={event => {
                          const raw = event.target.value
                          editTrading(current => ({ ...current, odds_multiplier: raw === '' ? undefined : Number(raw) }))
                        }}
                        inputProps={{ min: 0.5, max: 1.5, step: 0.01 }}
                        sx={{ width: { xs: '100%', md: 130 } }}
                      />
                    </Stack>
                    <Stack direction="row" gap={.65} mt={1} sx={{ overflowX: 'auto', pb: .25 }}>
                      {[0.8, 0.9, 1, 1.1, 1.2].map(value => <Button
                        key={value}
                        size="small"
                        disabled={tradingUnavailable}
                        variant={Math.abs((trading.odds_multiplier ?? 1) - value) < 0.0001 ? 'contained' : 'outlined'}
                        onClick={() => editTrading(current => ({ ...current, odds_multiplier: value }))}
                        sx={{ minWidth: 66, flex: '0 0 auto' }}
                      >{value.toFixed(2)} 倍</Button>)}
                    </Stack>
                  </Paper>

                  <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'repeat(2,minmax(0,1fr))' }, gap: 1 }}>
                    <Paper variant="outlined" sx={{ p: 1.15, borderRadius: 2.2 }}>
                      <Typography fontSize={12} fontWeight={900} mb={1}>飞单设置</Typography>
                      <Stack gap={1}>
                        <TextField
                          select
                          size="small"
                          label="飞单模式"
                          disabled={tradingUnavailable}
                          value={trading.fly.mode}
                          onChange={event => editTrading(current => ({ ...current, fly: { ...current.fly, mode: event.target.value } }))}
                        >
                          <MenuItem value="inherit">跟随房间（当前 {trading.room_fly_rate}%）</MenuItem>
                          <MenuItem value="custom">单独比例</MenuItem>
                          <MenuItem value="off">不飞单</MenuItem>
                        </TextField>
                        <TextField
                          size="small"
                          type="number"
                          label="单独飞单比例 %"
                          disabled={tradingUnavailable || trading.fly.mode !== 'custom'}
                          value={trading.fly.rate}
                          onChange={event => editTrading(current => ({ ...current, fly: { ...current.fly, rate: Number(event.target.value) } }))}
                          inputProps={{ min: 0, max: 100, step: 0.01 }}
                        />
                      </Stack>
                    </Paper>
                    <Paper variant="outlined" sx={{ p: 1.15, borderRadius: 2.2 }}>
                      <Typography fontSize={12} fontWeight={900} mb={1}>返水设置</Typography>
                      <Stack gap={1}>
                        <TextField select size="small" label="返水模式" disabled={tradingUnavailable} value={trading.rebate.mode} onChange={event => editTrading(current => ({ ...current, rebate: { ...current.rebate, mode: event.target.value } }))}>
                          <MenuItem value="inherit">跟随房间（当前 {trading.room_rebate_rate}%）</MenuItem>
                          <MenuItem value="custom">用户单独返水</MenuItem>
                          <MenuItem value="off">关闭返水</MenuItem>
                        </TextField>
                        <TextField size="small" type="number" label="用户返水比例 %" disabled={tradingUnavailable || trading.rebate.mode !== 'custom'} value={trading.rebate.rate} onChange={event => editTrading(current => ({ ...current, rebate: { ...current.rebate, rate: Number(event.target.value) } }))} inputProps={{ min: 0, max: 100, step: 0.01 }} />
                      </Stack>
                    </Paper>
                  </Box>

                  <GameOddsNavigation
                    games={games.map(game => ({ ...game, enabled: true }))}
                    gameId={trading.game_id}
                    onSelect={next => {
                      if (!isCurrentTradingEditor() || tradingWriteRef.current !== null) return
                      if (tradingDirtyRef.current) {
                        showMessage('当前游戏有未保存修改，请先保存再切换', 'warning')
                        return
                      }
                      if (next === tradingRef.current?.game_id && !tradingLoadingRef.current) return
                      void loadTrading(detailUser.id, next)
                    }}
                  />
                  <Box component="fieldset" disabled={tradingUnavailable} sx={{ border: 0, m: 0, p: 0, minWidth: 0 }}><OddsOverrideGrid
                    items={trading.odds}
                    level="member"
                    onChange={odds => editTrading(current => ({
                        ...current,
                        odds: odds.map(item => ({ ...item, room_odds: item.room_odds ?? item.base_odds })),
                      }))}
                  /></Box>
                  <Stack direction="row" justifyContent="flex-end">
                    <Button variant="contained" disabled={tradingUnavailable || !tradingDirty} onClick={() => void saveTrading()}>{tradingSaving ? '保存中…' : tradingDirty ? '保存会员交易配置' : '已保存'}</Button>
                  </Stack>
                </Stack>
              )}
            </CardContent>
          </Card></>}

          <Typography fontWeight={800} mt={2.5} mb={1}>余额流水</Typography>
          <Stack gap={1}>
            {history.map(record => (
              <Paper variant="outlined" sx={{ p: 1.2 }} key={record.id}>
                <Stack direction="row" justifyContent="space-between">
                  <Typography fontSize={12} fontWeight={800} color={record.amount >= 0 ? 'success.main' : 'error.main'}>{record.amount >= 0 ? '+' : ''}{money(record.amount)}</Typography>
                  <Typography fontSize={10} color="text.secondary">{dateTime(record.created_at)}</Typography>
                </Stack>
                <Typography fontSize={10} mt={.5}>{record.remark}</Typography>
                <Typography fontSize={9} color="text.secondary">{money(record.before)} → {money(record.after)} · {record.operator}</Typography>
              </Paper>
            ))}
            {!history.length && <Typography variant="caption" color="text.secondary">暂无余额调整记录</Typography>}
          </Stack>
        </>
      )}
    </Drawer>
  </Box>
}
