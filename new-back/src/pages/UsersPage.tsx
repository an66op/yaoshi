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
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import DownloadRounded from '@mui/icons-material/DownloadRounded'
import EditRounded from '@mui/icons-material/EditRounded'
import KeyRounded from '@mui/icons-material/KeyRounded'
import AccountBalanceWalletRounded from '@mui/icons-material/AccountBalanceWalletRounded'
import VisibilityRounded from '@mui/icons-material/VisibilityRounded'
import PeopleAltRounded from '@mui/icons-material/PeopleAltRounded'
import PersonAddAltRounded from '@mui/icons-material/PersonAddAltRounded'
import BlockRounded from '@mui/icons-material/BlockRounded'
import AdminPanelSettingsRounded from '@mui/icons-material/AdminPanelSettingsRounded'
import CloseRounded from '@mui/icons-material/CloseRounded'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { adminApi, type AdminGame, type AdminUser, type BalanceRecord, type UserPayload, type UserStats, type UserTradingConfig } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'

const roleLabels: Record<AdminUser['role'], string> = { member: '普通会员', agent: '代理', admin: '管理员' }
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
  const [role, setRole] = useState('all')
  const [applied, setApplied] = useState({ query: '', status: 'all', role: 'all' })
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
  const [trading, setTrading] = useState<UserTradingConfig | null>(null)
  const [tradingGameId, setTradingGameId] = useState('')
  const [tradingSaving, setTradingSaving] = useState(false)
  const { showMessage } = useFeedback()

  const load = useCallback(async (notify = false) => {
    setLoading(true)
    setError('')
    try {
      const [list, nextStats] = await Promise.all([
        adminApi.users({ ...applied, page: page + 1, pageSize }),
        adminApi.userStats(),
      ])
      setUsers(list.items)
      setTotal(list.total)
      setStats(nextStats)
      if (notify) showMessage('用户数据已刷新')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取用户数据失败')
    } finally {
      setLoading(false)
    }
  }, [applied, page, pageSize, showMessage])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])

  const openCreate = () => {
    setEditing(null)
    setForm(emptyForm)
    setFormOpen(true)
  }

  const openEdit = (user: AdminUser) => {
    setEditing(user)
    setForm({ email: user.email, nickname: user.nickname, phone: user.phone, role: user.role, remark: user.remark, risk_level: user.risk_level, status: user.status })
    setFormOpen(true)
  }

  const submitUser = async () => {
    if (!editing && (!form.username?.trim() || (form.password?.length ?? 0) < 6)) {
      setError('请填写用户名，并设置至少 6 位密码')
      return
    }
    setSaving(true)
    setError('')
    try {
      if (editing) await adminApi.updateUser(editing.id, form)
      else await adminApi.createUser(form)
      setFormOpen(false)
      showMessage(editing ? '用户资料已更新' : '用户创建成功')
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
    if (!resetUser || newPassword.length < 6) {
      setError('新密码至少需要 6 个字符')
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
      const nextStats = await adminApi.userStats()
      setStats(nextStats)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '余额调整失败')
    } finally {
      setSaving(false)
    }
  }

  const openDetail = async (user: AdminUser) => {
    setDetailUser(user)
    setHistory([])
    setTrading(null)
    try {
      const [nextHistory, dashboard, nextTrading] = await Promise.all([
        adminApi.userBalanceHistory(user.id),
        adminApi.dashboard(),
        adminApi.userTrading(user.id),
      ])
      setHistory(nextHistory)
      setGames(dashboard.games ?? [])
      setTrading(nextTrading)
      setTradingGameId(nextTrading.game_id)
    } catch {
      setHistory([])
    }
  }

  const loadTrading = async (userId: number, gameId: string) => {
    try {
      const next = await adminApi.userTrading(userId, gameId)
      setTrading(next)
      setTradingGameId(next.game_id)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取飞单与赔率失败')
    }
  }

  const saveTrading = async () => {
    if (!detailUser || !trading) return
    setTradingSaving(true)
    try {
      const next = await adminApi.updateUserTrading(detailUser.id, {
        fly_mode: trading.fly.mode,
        fly_rate: trading.fly.rate,
        game_id: trading.game_id,
        odds: trading.odds.map(item => ({ play_code: item.play_code, override: item.has_override ? item.override : null })),
      })
      setTrading(next)
      setDetailUser(current => current ? { ...current, fly_mode: next.fly.mode, fly_rate: next.fly.rate } : current)
      setUsers(current => current.map(item => item.id === detailUser.id ? { ...item, fly_mode: next.fly.mode, fly_rate: next.fly.rate } : item))
      showMessage('飞单与单独赔率已保存')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存失败')
    } finally {
      setTradingSaving(false)
    }
  }

  const flyModeLabel = (mode?: string) => ({ inherit: '跟随房间', custom: '单独比例', off: '不飞单' }[mode ?? 'inherit'] ?? mode)

  const applyFilters = () => { setPage(0); setApplied({ query: query.trim(), status, role }) }
  const resetFilters = () => { setQuery(''); setStatus('all'); setRole('all'); setPage(0); setApplied({ query: '', status: 'all', role: 'all' }) }
  const exportCsv = () => {
    const rows = users.map(user => [user.public_id, user.username, user.nickname, roleLabels[user.role], user.email, user.phone, user.balance.toFixed(2), user.status === 1 ? '启用' : '停用', dateTime(user.created_at)])
    const csv = [['用户 ID', '用户名', '昵称', '角色', '邮箱', '手机', '余额', '状态', '创建时间'], ...rows].map(row => row.join(',')).join('\n')
    const link = document.createElement('a')
    link.href = URL.createObjectURL(new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8' }))
    link.download = '用户列表.csv'
    link.click()
    URL.revokeObjectURL(link.href)
  }

  const statCards = useMemo(() => [
    ['用户总数', stats?.total ?? 0, PeopleAltRounded, '#4f7edc'],
    ['正常用户', stats?.active ?? 0, PersonAddAltRounded, '#2eaf7b'],
    ['停用用户', stats?.disabled ?? 0, BlockRounded, '#df746a'],
    ['今日新增', stats?.new_today ?? 0, AdminPanelSettingsRounded, '#8a70df'],
  ] as const, [stats])

  return <Box p={{ xs: 2, lg: 2.5 }}>
    <PageHeader eyebrow="业务管理 / 用户" title="用户管理" description="管理会员资料、权限状态、风控标记和账户余额。" actions={<><Button variant="outlined" startIcon={<DownloadRounded />} disabled={!users.length} onClick={exportCsv}>导出当前页</Button><Button variant="outlined" startIcon={loading ? <CircularProgress size={16} /> : <RefreshRounded />} disabled={loading} onClick={() => void load(true)}>刷新</Button><Button variant="contained" startIcon={<AddRounded />} onClick={openCreate}>新增用户</Button></>} />
    {error && <Alert severity="error" onClose={() => setError('')} sx={{ mt: 2 }}>{error}</Alert>}
    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2,1fr)', lg: 'repeat(4,1fr)' }, gap: 1.25, mt: 2.5 }}>{statCards.map(([label, value, Icon, color]) => <Card key={label}><CardContent sx={{ p: '15px !important' }}><Stack direction="row" alignItems="center" justifyContent="space-between"><Box><Typography variant="caption" color="text.secondary">{label}</Typography><Typography fontSize={{ xs: 20, sm: 24 }} fontWeight={850} mt={.4}>{value}</Typography></Box><Box sx={{ width: 40, height: 40, borderRadius: 2.5, display: 'grid', placeItems: 'center', color: '#fff', bgcolor: color }}><Icon fontSize="small" /></Box></Stack></CardContent></Card>)}</Box>
    <Paper variant="outlined" sx={{ p: 1.5, mt: 1.5 }}><Stack direction={{ xs: 'column', md: 'row' }} gap={1}><TextField placeholder="搜索用户名、昵称、邮箱、手机或备注" value={query} onChange={event => setQuery(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') applyFilters() }} sx={{ flex: 1, minWidth: { md: 280 } }} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> } }} /><TextField select label="账号状态" value={status} onChange={event => setStatus(event.target.value)} sx={{ minWidth: 140 }}><MenuItem value="all">全部状态</MenuItem><MenuItem value="active">正常</MenuItem><MenuItem value="disabled">已停用</MenuItem></TextField><TextField select label="账号角色" value={role} onChange={event => setRole(event.target.value)} sx={{ minWidth: 140 }}><MenuItem value="all">全部角色</MenuItem><MenuItem value="member">普通会员</MenuItem><MenuItem value="agent">代理</MenuItem><MenuItem value="admin">管理员</MenuItem></TextField><Button variant="contained" onClick={applyFilters}>查询</Button><Button variant="text" onClick={resetFilters}>重置</Button></Stack></Paper>
    <Card sx={{ mt: 1.5 }}>{loading && <Box px={2} py={1}><CircularProgress size={18} /></Box>}<TableContainer><Table size="small" sx={{ minWidth: 1080 }}><TableHead><TableRow><TableCell>用户</TableCell><TableCell>角色</TableCell><TableCell align="right">余额</TableCell><TableCell>联系方式</TableCell><TableCell>风控</TableCell><TableCell>状态</TableCell><TableCell>最后登录</TableCell><TableCell>创建时间</TableCell><TableCell align="right">操作</TableCell></TableRow></TableHead><TableBody>{users.map(user => <TableRow hover key={user.id}><TableCell><Stack direction="row" alignItems="center" gap={1}><Avatar sx={{ width: 34, height: 34, fontSize: 13, bgcolor: user.role === 'admin' ? 'secondary.main' : 'primary.main' }}>{(user.nickname || user.username).slice(0, 1).toUpperCase()}</Avatar><Box><Typography fontSize={12} fontWeight={800}>{user.nickname || user.username}</Typography><Typography fontSize={10} color="text.secondary">@{user.username} · ID {user.public_id}</Typography></Box></Stack></TableCell><TableCell><Chip size="small" variant="outlined" color={user.role === 'admin' ? 'secondary' : user.role === 'agent' ? 'info' : 'default'} label={roleLabels[user.role]} /></TableCell><TableCell align="right"><Typography fontWeight={800}>{money(user.balance)}</Typography></TableCell><TableCell><Typography fontSize={11}>{user.phone || '未填写手机'}</Typography><Typography fontSize={9} color="text.secondary">{user.email || '未填写邮箱'}</Typography></TableCell><TableCell><Chip size="small" color={user.risk_level === 'normal' ? 'success' : user.risk_level === 'watch' ? 'warning' : 'error'} variant="outlined" label={riskLabels[user.risk_level]} /></TableCell><TableCell><Stack direction="row" alignItems="center" gap={.5}><Switch size="small" checked={user.status === 1} onChange={() => void toggleStatus(user)} /><Typography fontSize={10}>{user.status === 1 ? '正常' : '停用'}</Typography></Stack></TableCell><TableCell><Typography fontSize={10}>{dateTime(user.last_login_at)}</Typography><Typography fontSize={9} color="text.secondary">登录 {user.login_count} 次</Typography></TableCell><TableCell sx={{ fontSize: 10 }}>{dateTime(user.created_at)}</TableCell><TableCell align="right"><Stack direction="row" justifyContent="flex-end"><Tooltip title="查看详情"><IconButton size="small" onClick={() => void openDetail(user)}><VisibilityRounded fontSize="small" /></IconButton></Tooltip><Tooltip title="编辑资料"><IconButton size="small" onClick={() => openEdit(user)}><EditRounded fontSize="small" /></IconButton></Tooltip><Tooltip title="调整余额"><IconButton size="small" onClick={() => { setBalanceUser(user); setBalanceAmount(''); setBalanceRemark('') }}><AccountBalanceWalletRounded fontSize="small" /></IconButton></Tooltip><Tooltip title="重置密码"><IconButton size="small" onClick={() => { setResetUser(user); setNewPassword('') }}><KeyRounded fontSize="small" /></IconButton></Tooltip></Stack></TableCell></TableRow>)}{!loading && !users.length && <TableRow><TableCell colSpan={9} align="center" sx={{ py: 8, color: 'text.secondary' }}>没有找到符合条件的用户</TableCell></TableRow>}</TableBody></Table></TableContainer><TablePagination component="div" count={total} page={page} onPageChange={(_, next) => setPage(next)} rowsPerPage={pageSize} onRowsPerPageChange={event => setPageSize(Number(event.target.value))} rowsPerPageOptions={[10, 20, 50]} labelRowsPerPage="每页" /></Card>

    <Dialog open={formOpen} onClose={() => !saving && setFormOpen(false)} fullWidth maxWidth="md"><DialogTitle>{editing ? `编辑用户 · ${editing.username}` : '新增用户'}</DialogTitle><DialogContent><Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,1fr)' }, gap: 2, pt: 1 }}>{!editing && <><TextField required label="用户名" value={form.username ?? ''} onChange={event => setForm(current => ({ ...current, username: event.target.value }))} /><TextField required type="password" label="初始密码" helperText="至少 6 个字符" value={form.password ?? ''} onChange={event => setForm(current => ({ ...current, password: event.target.value }))} /></>}<TextField label="昵称" value={form.nickname} onChange={event => setForm(current => ({ ...current, nickname: event.target.value }))} /><TextField type="email" label="邮箱" value={form.email} onChange={event => setForm(current => ({ ...current, email: event.target.value }))} /><TextField label="手机号" value={form.phone} onChange={event => setForm(current => ({ ...current, phone: event.target.value }))} /><TextField select label="账号角色" value={form.role} onChange={event => setForm(current => ({ ...current, role: event.target.value as AdminUser['role'] }))}><MenuItem value="member">普通会员</MenuItem><MenuItem value="agent">代理</MenuItem><MenuItem value="admin">管理员</MenuItem></TextField><TextField select label="风控等级" value={form.risk_level} onChange={event => setForm(current => ({ ...current, risk_level: event.target.value as AdminUser['risk_level'] }))}><MenuItem value="normal">正常</MenuItem><MenuItem value="watch">重点关注</MenuItem><MenuItem value="restricted">限制账号</MenuItem></TextField><TextField select label="账号状态" value={form.status} onChange={event => setForm(current => ({ ...current, status: Number(event.target.value) as 0 | 1 }))}><MenuItem value={1}>正常</MenuItem><MenuItem value={0}>停用</MenuItem></TextField><TextField multiline minRows={3} label="管理备注" value={form.remark} onChange={event => setForm(current => ({ ...current, remark: event.target.value }))} sx={{ gridColumn: { sm: '1/-1' } }} /></Box></DialogContent><DialogActions><Button disabled={saving} onClick={() => setFormOpen(false)}>取消</Button><Button variant="contained" disabled={saving} onClick={() => void submitUser()}>{saving ? '保存中…' : '保存用户'}</Button></DialogActions></Dialog>

    <Dialog open={Boolean(resetUser)} onClose={() => !saving && setResetUser(null)} fullWidth maxWidth="xs"><DialogTitle>重置密码</DialogTitle><DialogContent><Typography variant="body2" color="text.secondary" mb={2}>为 {resetUser?.username} 设置新密码。保存后旧密码立即失效。</Typography><TextField autoFocus fullWidth type="password" label="新密码" helperText="至少 6 个字符" value={newPassword} onChange={event => setNewPassword(event.target.value)} /></DialogContent><DialogActions><Button onClick={() => setResetUser(null)}>取消</Button><Button variant="contained" disabled={saving} onClick={() => void submitPassword()}>确认重置</Button></DialogActions></Dialog>

    <Dialog open={Boolean(balanceUser)} onClose={() => !saving && setBalanceUser(null)} fullWidth maxWidth="xs"><DialogTitle>调整用户余额</DialogTitle><DialogContent><Alert severity="info" sx={{ mb: 2 }}>当前余额：{money(balanceUser?.balance ?? 0)}。正数增加，负数扣减，操作会写入审计流水。</Alert><Stack gap={2}><TextField autoFocus type="number" label="调整金额" placeholder="例如 100 或 -50" value={balanceAmount} onChange={event => setBalanceAmount(event.target.value)} /><TextField required multiline minRows={3} label="调整原因" value={balanceRemark} onChange={event => setBalanceRemark(event.target.value)} /></Stack></DialogContent><DialogActions><Button onClick={() => setBalanceUser(null)}>取消</Button><Button variant="contained" disabled={saving} onClick={() => void submitBalance()}>确认调整</Button></DialogActions></Dialog>

    <Drawer anchor="right" open={Boolean(detailUser)} onClose={() => setDetailUser(null)} slotProps={{ paper: { sx: { width: { xs: '100%', sm: 480 }, p: 2.5 } } }}>
      <Stack direction="row" justifyContent="space-between" alignItems="center">
        <Box>
          <Typography variant="overline" color="primary">用户详情</Typography>
          <Typography variant="h6" fontWeight={850}>{detailUser?.nickname || detailUser?.username}</Typography>
        </Box>
        <IconButton onClick={() => setDetailUser(null)}><CloseRounded /></IconButton>
      </Stack>
      {detailUser && (
        <>
          <Stack direction="row" gap={1.5} alignItems="center" mt={2}>
            <Avatar sx={{ width: 54, height: 54, bgcolor: 'primary.main' }}>{(detailUser.nickname || detailUser.username)[0]}</Avatar>
            <Box>
              <Typography fontWeight={800}>@{detailUser.username}</Typography>
              <Typography variant="caption" color="text.secondary">ID {detailUser.id} · {roleLabels[detailUser.role]} · {flyModeLabel(detailUser.fly_mode)}</Typography>
            </Box>
            <Chip sx={{ ml: 'auto' }} size="small" color={detailUser.status === 1 ? 'success' : 'default'} label={detailUser.status === 1 ? '正常' : '停用'} />
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

          <Typography fontWeight={800} mt={2.5} mb={1}>飞单与单独赔率</Typography>
          <Card variant="outlined">
            <CardContent>
              {!trading ? (
                <Typography variant="caption" color="text.secondary">加载交易配置中…</Typography>
              ) : (
                <Stack gap={1.5}>
                  <Alert severity="info" sx={{ py: 0.5 }}>赔率优先级：用户单独赔率 → 房间玩法赔率。飞单：不传显式金额时按本策略自动计算。</Alert>
                  <TextField
                    select
                    size="small"
                    label="飞单模式"
                    value={trading.fly.mode}
                    onChange={event => setTrading(current => current ? { ...current, fly: { ...current.fly, mode: event.target.value } } : current)}
                  >
                    <MenuItem value="inherit">跟随房间（当前 {trading.room_fly_rate}%）</MenuItem>
                    <MenuItem value="custom">单独比例</MenuItem>
                    <MenuItem value="off">不飞单</MenuItem>
                  </TextField>
                  <TextField
                    size="small"
                    type="number"
                    label="单独飞单比例 %"
                    disabled={trading.fly.mode !== 'custom'}
                    value={trading.fly.rate}
                    onChange={event => setTrading(current => current ? { ...current, fly: { ...current.fly, rate: Number(event.target.value) } } : current)}
                  />
                  <TextField
                    select
                    size="small"
                    label="彩种（单独赔率）"
                    value={tradingGameId}
                    onChange={event => {
                      const next = event.target.value
                      setTradingGameId(next)
                      void loadTrading(detailUser.id, next)
                    }}
                  >
                    {games.map(game => <MenuItem key={game.id} value={game.id}>{game.name}</MenuItem>)}
                    {!games.find(game => game.id === trading.game_id) && <MenuItem value={trading.game_id}>{trading.game_name || trading.game_id}</MenuItem>}
                  </TextField>
                  <TableContainer>
                    <Table size="small">
                      <TableHead>
                        <TableRow>
                          <TableCell>玩法</TableCell>
                          <TableCell align="right">房间</TableCell>
                          <TableCell align="right">单独</TableCell>
                        </TableRow>
                      </TableHead>
                      <TableBody>
                        {trading.odds.map((item, index) => (
                          <TableRow key={item.play_code}>
                            <TableCell>
                              <Typography fontSize={11} fontWeight={700}>{item.play_name}</Typography>
                              <Typography fontSize={9} color="text.secondary">{item.play_code}</Typography>
                            </TableCell>
                            <TableCell align="right">{item.room_odds}</TableCell>
                            <TableCell align="right">
                              <TextField
                                size="small"
                                type="number"
                                placeholder="继承"
                                value={item.has_override ? (item.override ?? '') : ''}
                                onChange={event => {
                                  const raw = event.target.value
                                  setTrading(current => {
                                    if (!current) return current
                                    const odds = [...current.odds]
                                    if (!raw.trim()) {
                                      odds[index] = { ...odds[index], override: null, has_override: false, effective: odds[index].room_odds }
                                    } else {
                                      const value = Number(raw)
                                      odds[index] = { ...odds[index], override: value, has_override: true, effective: value }
                                    }
                                    return { ...current, odds }
                                  })
                                }}
                                sx={{ width: 96 }}
                                inputProps={{ step: 0.001, min: 1.001 }}
                              />
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </TableContainer>
                  <Button variant="contained" disabled={tradingSaving} onClick={() => void saveTrading()}>{tradingSaving ? '保存中…' : '保存飞单/赔率'}</Button>
                </Stack>
              )}
            </CardContent>
          </Card>

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
