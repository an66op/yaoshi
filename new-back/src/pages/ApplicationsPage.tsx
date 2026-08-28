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
  InputAdornment,
  MenuItem,
  Paper,
  Stack,
  Tab,
  Tabs,
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
import VisibilityRounded from '@mui/icons-material/VisibilityRounded'
import FactCheckRounded from '@mui/icons-material/FactCheckRounded'
import PendingActionsRounded from '@mui/icons-material/PendingActionsRounded'
import TaskAltRounded from '@mui/icons-material/TaskAltRounded'
import CancelRounded from '@mui/icons-material/CancelRounded'
import PaymentsRounded from '@mui/icons-material/PaymentsRounded'
import CloseRounded from '@mui/icons-material/CloseRounded'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { adminApi, agentApi, tenantApi, type AdminApplication, type AdminUser, type AgentItem, type ApplicationPayload, type ApplicationStats, type ManagementWsEvent, type TenantItem } from '../api'
import { getStoredUser } from '../auth'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'
import { MANAGEMENT_WS_EVENT } from '../hooks/useManagementWebSocket'

const typeLabels: Record<AdminApplication['request_type'], string> = { credit: '上分', debit: '下分', agent: '代理申请', join: '入房申请' }
const statusLabels: Record<AdminApplication['status'], string> = { pending: '待审核', approved: '已通过', rejected: '已拒绝' }
const roleLabels: Record<string, string> = { member: '普通会员', agent: '代理', admin: '管理员' }
const paymentLabels: Record<string, string> = { manual: '人工处理', bank: '银行卡', alipay: '支付宝', wechat: '微信', usdt: 'USDT' }
type ApplicationCategory = 'wallet' | 'join' | 'entertainment'
const applicationCategories: Array<{ value: ApplicationCategory; label: string; hint: string }> = [
  { value: 'join', label: '入房申请', hint: '会员进入房间' },
  { value: 'wallet', label: '上下分申请', hint: '会员账户上下分' },
  { value: 'entertainment', label: '娱乐上下分', hint: '娱乐平台额度' },
]
const money = (value: number) => new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)
const dateTime = (value?: string | null) => value ? new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(new Date(value)) : '—'
const emptyForm: ApplicationPayload = { user_id: 0, request_type: 'credit', payment_type: 'manual', game_id: '', amount: 0, remark: '' }
const oddsMultiplierPresets = [0.8, 0.9, 1, 1.1, 1.2]

export function ApplicationsPage({ initialCategory = 'join' }: { initialCategory?: ApplicationCategory }) {
  const role = getStoredUser()?.role ?? 'admin'
  const resolvedInitialCategory: ApplicationCategory = role === 'admin' && initialCategory === 'join' ? 'wallet' : initialCategory
  const visibleApplicationCategories = role === 'admin'
    ? applicationCategories.filter(item => item.value !== 'join')
    : applicationCategories
  const [category, setCategory] = useState<ApplicationCategory>(resolvedInitialCategory)
  const joinOnly = category === 'join'
  const entertainmentOnly = category === 'entertainment'
  const [items, setItems] = useState<AdminApplication[]>([])
  const [stats, setStats] = useState<ApplicationStats | null>(null)
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState(20)
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('all')
  const [date, setDate] = useState('')
  const [workspaceId, setWorkspaceId] = useState(0)
  const [workspaces, setWorkspaces] = useState<Array<{ id: number; label: string }>>([])
  const [applied, setApplied] = useState({ query: '', status: 'all', type: resolvedInitialCategory, date: '', workspaceId: 0 })
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [formOpen, setFormOpen] = useState(false)
  const [form, setForm] = useState<ApplicationPayload>(emptyForm)
  const [users, setUsers] = useState<AdminUser[]>([])
  const [reviewItem, setReviewItem] = useState<AdminApplication | null>(null)
  const [decision, setDecision] = useState<'approved' | 'rejected'>('approved')
  const [receivedAmount, setReceivedAmount] = useState('')
  const [oddsMultiplier, setOddsMultiplier] = useState('1')
  const [reviewRemark, setReviewRemark] = useState('')
  const [detail, setDetail] = useState<AdminApplication | null>(null)
  const { showMessage } = useFeedback()

  const load = useCallback(async (notify = false) => {
    setLoading(true)
    setError('')
    try {
      const params = { query: applied.query, status: applied.status, type: applied.type, date: applied.date, start: applied.date, end: applied.date, page: page + 1, pageSize }
      const listPromise = role === 'tenant' ? tenantApi.applications(params) : role === 'agent' ? agentApi.applications(params) : adminApi.applications({ ...params, workspaceId: applied.workspaceId })
      const statsPromise = role === 'tenant' ? tenantApi.applicationStats() : role === 'agent' ? agentApi.applicationStats() : adminApi.applicationStats(applied.workspaceId)
      const [list, nextStats] = await Promise.all([listPromise, statsPromise])
      setItems(Array.isArray(list?.items) ? list.items : [])
      setTotal(list.total)
      setStats(nextStats)
      if (notify) showMessage('申请数据已刷新')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取申请数据失败')
    } finally {
      setLoading(false)
    }
  }, [applied, page, pageSize, role, showMessage])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])

  useEffect(() => {
    if (role !== 'admin') return
    void Promise.all([adminApi.tenants({ pageSize: 100 }), adminApi.agents({ pageSize: 100 })]).then(([tenants, agents]) => {
      const tenantRooms = (Array.isArray(tenants?.items) ? tenants.items : []).filter((item: TenantItem) => item.workspace_id).map((item: TenantItem) => ({ id: item.workspace_id, label: `租户直属 · ${item.room_code || '未分配'} · ${item.room_name || item.nickname || item.username}` }))
      const agentRooms = (Array.isArray(agents?.items) ? agents.items : []).filter((item: AgentItem) => item.workspace_id).map((item: AgentItem) => ({ id: item.workspace_id, label: `代理房间 · ${item.room_code} · ${item.room_name || item.nickname || item.username}` }))
      setWorkspaces([...tenantRooms, ...agentRooms])
    }).catch(() => setWorkspaces([]))
  }, [role])

  useEffect(() => {
    const onRealtime = (event: Event) => {
      const payload = (event as CustomEvent<ManagementWsEvent>).detail
      if (payload?.type !== 'application') return
      if (role === 'admin' && applied.workspaceId && payload.workspace_id && payload.workspace_id !== applied.workspaceId) return
      void load()
    }
    window.addEventListener(MANAGEMENT_WS_EVENT, onRealtime)
    return () => window.removeEventListener(MANAGEMENT_WS_EVENT, onRealtime)
  }, [applied.workspaceId, load, role])

  const openCreate = async () => {
    setError('')
    setForm({ ...emptyForm, workspace_id: applied.workspaceId || undefined, game_id: entertainmentOnly ? '' : undefined })
    setFormOpen(true)
    if (role !== 'admin') return
    try {
      const list = await adminApi.users({ status: 'active', workspaceId: applied.workspaceId || undefined, page: 1, pageSize: 100 })
      const nextUsers = Array.isArray(list?.items) ? list.items : []
      setUsers(nextUsers)
      if (nextUsers.length) setForm(current => ({ ...current, user_id: nextUsers[0].id }))
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取用户失败')
    }
  }

  const submitCreate = async () => {
    const needsAmount = form.request_type === 'credit' || form.request_type === 'debit'
    if (!form.user_id || (needsAmount && (!Number.isFinite(form.amount) || form.amount <= 0)) || (entertainmentOnly && !form.game_id?.trim())) {
      setError(entertainmentOnly && !form.game_id?.trim() ? '请填写娱乐平台或游戏标识' : needsAmount ? '请选择用户并填写大于 0 的申请金额' : '请选择申请用户')
      return
    }
    setSaving(true)
    setError('')
    try {
      await adminApi.createApplication({ ...form, amount: needsAmount ? form.amount : 0 })
      setFormOpen(false)
      showMessage('申请创建成功，已进入待审核队列')
      setPage(0)
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '创建申请失败')
    } finally {
      setSaving(false)
    }
  }

  const openReview = (item: AdminApplication) => {
    setReviewItem(item)
    setDecision('approved')
    setReceivedAmount(item.requested_amount ? String(item.requested_amount) : '')
    setOddsMultiplier(String(item.odds_multiplier || 1))
    setReviewRemark('')
    setError('')
  }

  const submitReview = async () => {
    if (!reviewItem) return
    const received = decision === 'approved' && (reviewItem.request_type === 'credit' || reviewItem.request_type === 'debit') ? Number(receivedAmount) : 0
    const multiplier = Number(oddsMultiplier)
    if (decision === 'approved' && (reviewItem.request_type === 'credit' || reviewItem.request_type === 'debit') && (!Number.isFinite(received) || received <= 0)) {
      setError('请输入大于 0 的到账金额')
      return
    }
    if (decision === 'approved' && reviewItem.request_type === 'join' && (!Number.isFinite(multiplier) || multiplier < 0.5 || multiplier > 1.5)) {
      setError('会员赔率倍率需在 0.50–1.50 之间')
      return
    }
    if (decision === 'rejected' && !reviewRemark.trim()) {
      setError('拒绝申请时请填写审核原因')
      return
    }
    setSaving(true)
    setError('')
    try {
      const payload = { decision, received_amount: received, odds_multiplier: reviewItem.request_type === 'join' ? multiplier : undefined, remark: reviewRemark.trim() }
      const next = role === 'tenant' ? await tenantApi.reviewApplication(reviewItem.id, payload) : role === 'agent' ? await agentApi.reviewApplication(reviewItem.id, payload) : await adminApi.reviewApplication(reviewItem.id, payload)
      setReviewItem(null)
      setItems(current => current.map(item => item.id === next.id ? next : item))
      showMessage(decision === 'approved' ? '申请已通过并完成账户处理' : '申请已拒绝')
      void load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '审核失败')
    } finally {
      setSaving(false)
    }
  }

  const applyFilters = () => { setPage(0); setApplied({ query: query.trim(), status, type: category, date, workspaceId }) }
  const resetFilters = () => { setQuery(''); setStatus('all'); setDate(''); setWorkspaceId(0); setPage(0); setApplied({ query: '', status: 'all', type: category, date: '', workspaceId: 0 }) }
  const selectCategory = (next: ApplicationCategory) => {
    setCategory(next)
    setQuery('')
    setStatus('all')
    setDate('')
    setPage(0)
    setApplied({ query: '', status: 'all', type: next, date: '', workspaceId })
  }
  const statCards = useMemo(() => [
    ['待审核', stats?.pending ?? 0, PendingActionsRounded, '#e99c35'],
    ['今日通过', stats?.approved_today ?? 0, TaskAltRounded, '#2eaf7b'],
    ['今日拒绝', stats?.rejected_today ?? 0, CancelRounded, '#df746a'],
    ['今日申请金额', money(stats?.today_amount ?? 0), PaymentsRounded, '#4f7edc'],
  ] as const, [stats])
  const moneyRequest = reviewItem?.request_type === 'credit' || reviewItem?.request_type === 'debit'

  return <Box p={{ xs: 2, lg: 2.5 }}>
    <PageHeader eyebrow="申请与审核" title="申请管理" description="" actions={role === 'admin' && !joinOnly ? <Button variant="contained" startIcon={<AddRounded />} onClick={() => void openCreate()}>新增申请</Button> : undefined} />
    {error && <Alert severity="error" onClose={() => setError('')} sx={{ mt: 2 }}>{error}</Alert>}
    <Paper variant="outlined" sx={{ mt: 2.25, borderRadius: 2.5, overflow: 'hidden' }}><Tabs value={category} onChange={(_, next: ApplicationCategory) => selectCategory(next)} variant="fullWidth" sx={{ minHeight: 62, '& .MuiTab-root': { minHeight: 62, py: 1, fontWeight: 800, fontSize: { xs: 13, sm: 15 } } }}>{visibleApplicationCategories.map(item => <Tab key={item.value} value={item.value} label={<Box><Stack direction="row" justifyContent="center" alignItems="center" gap={.65}><Typography component="span" fontWeight={850} fontSize="inherit">{item.label}</Typography>{Boolean(stats?.pending_by_category?.[item.value]) && <Chip size="small" color="warning" label={stats?.pending_by_category?.[item.value]} sx={{ height: 20, '& .MuiChip-label': { px: .7, fontWeight: 850 } }} />}</Stack><Typography component="span" display={{ xs: 'none', sm: 'block' }} fontSize={10.5} color="text.secondary" mt={.2}>{item.hint}</Typography></Box>} />)}</Tabs></Paper>
    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2,1fr)', lg: 'repeat(4,1fr)' }, gap: 1.25, mt: 1.5 }}>{statCards.map(([label, value, Icon, color]) => <Card key={label}><CardContent sx={{ p: '15px !important' }}><Stack direction="row" alignItems="center" justifyContent="space-between" gap={1}><Box minWidth={0}><Typography variant="caption" color="text.secondary">{label}</Typography><Typography fontSize={{ xs: 19, sm: 24 }} fontWeight={850} mt={.4} noWrap>{value}</Typography></Box><Box sx={{ width: 40, height: 40, flex: '0 0 auto', borderRadius: 2.5, display: 'grid', placeItems: 'center', color: '#fff', bgcolor: color }}><Icon fontSize="small" /></Box></Stack></CardContent></Card>)}</Box>
    <Paper variant="outlined" sx={{ p: 1.5, mt: 1.5 }}><Stack direction={{ xs: 'column', lg: 'row' }} gap={1}><TextField placeholder="搜索会员账号、申请编号或备注" value={query} onChange={event => setQuery(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') applyFilters() }} sx={{ flex: 1, minWidth: { lg: 250 } }} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> } }} />{role === 'admin' && <TextField select label="房间" value={workspaceId} onChange={event => setWorkspaceId(Number(event.target.value))} sx={{ minWidth: 230 }}><MenuItem value={0}>全部房间</MenuItem>{workspaces.map(item => <MenuItem key={item.id} value={item.id}>{item.label}</MenuItem>)}</TextField>}<TextField select label="审核状态" value={status} onChange={event => setStatus(event.target.value)} sx={{ minWidth: 135 }}><MenuItem value="all">全部状态</MenuItem><MenuItem value="pending">待审核</MenuItem><MenuItem value="approved">已通过</MenuItem><MenuItem value="rejected">已拒绝</MenuItem></TextField><TextField type="date" label="申请日期" value={date} onChange={event => setDate(event.target.value)} slotProps={{ inputLabel: { shrink: true } }} sx={{ minWidth: 155 }} /><Button variant="contained" onClick={applyFilters}>查询</Button><Button variant="text" onClick={resetFilters}>重置</Button></Stack></Paper>
    <Card sx={{ mt: 1.5 }}>{loading && <Box px={2} py={1}><CircularProgress size={18} /></Box>}<TableContainer><Table size="small" sx={{ minWidth: 1180 }}><TableHead><TableRow><TableCell>申请会员</TableCell><TableCell>申请类型</TableCell><TableCell>{joinOnly ? '目标房间' : entertainmentOnly ? '娱乐平台' : '支付方式'}</TableCell><TableCell align="right">申请金额</TableCell><TableCell align="right">到账金额</TableCell><TableCell>申请时间</TableCell><TableCell>审核信息</TableCell><TableCell>状态</TableCell><TableCell align="right">操作</TableCell></TableRow></TableHead><TableBody>{items.map(item => <TableRow hover key={item.id}><TableCell><Stack direction="row" alignItems="center" gap={1}><Avatar sx={{ width: 34, height: 34, fontSize: 13, bgcolor: item.request_type === 'credit' ? 'success.main' : 'primary.main' }}>{item.username.slice(0, 1).toUpperCase()}</Avatar><Box><Typography fontSize={12} fontWeight={800}>{item.username}</Typography><Typography fontSize={10} color="text.secondary">会员 ID {item.user_id} · {roleLabels[item.account_type] ?? item.account_type}</Typography></Box></Stack></TableCell><TableCell><Chip size="small" label={typeLabels[item.request_type]} color={item.request_type === 'credit' ? 'success' : item.request_type === 'debit' ? 'warning' : 'info'} variant="outlined" /></TableCell><TableCell>{item.request_type === 'join' ? <Chip size="small" color="primary" label={item.target_room_code || '房间待确认'} /> : entertainmentOnly ? <Chip size="small" color="secondary" variant="outlined" label={item.game_id || '未标记平台'} /> : paymentLabels[item.payment_type] ?? item.payment_type}</TableCell><TableCell align="right"><Typography fontWeight={800}>{item.requested_amount ? money(item.requested_amount) : '—'}</Typography></TableCell><TableCell align="right">{item.received_amount ? money(item.received_amount) : '—'}</TableCell><TableCell>{dateTime(item.created_at)}</TableCell><TableCell><Typography fontSize={11} fontWeight={700}>{item.operator || '尚未审核'}</Typography><Typography fontSize={10} color="text.secondary">{dateTime(item.reviewed_at)}</Typography></TableCell><TableCell><Chip size="small" label={statusLabels[item.status]} color={item.status === 'approved' ? 'success' : item.status === 'rejected' ? 'error' : 'warning'} /></TableCell><TableCell align="right"><Stack direction="row" justifyContent="flex-end"><Tooltip title="查看详情"><Button size="small" startIcon={<VisibilityRounded />} onClick={() => setDetail(item)}>详情</Button></Tooltip>{item.status === 'pending' && <Button size="small" variant="contained" startIcon={<FactCheckRounded />} onClick={() => openReview(item)}>审核</Button>}</Stack></TableCell></TableRow>)}{!loading && !items.length && <TableRow><TableCell colSpan={9}><Stack minHeight={210} alignItems="center" justifyContent="center" color="text.secondary"><PendingActionsRounded sx={{ fontSize: 42, opacity: .55 }} /><Typography fontWeight={750} mt={1}>暂无符合条件的申请</Typography><Typography variant="caption">{joinOnly ? '会员提交入房申请后会显示在这里' : entertainmentOnly ? '娱乐上下分申请会单独显示在这里' : '新增申请后会进入待审核队列'}</Typography></Stack></TableCell></TableRow>}</TableBody></Table></TableContainer><TablePagination component="div" count={total} page={page} onPageChange={(_, next) => setPage(next)} rowsPerPage={pageSize} onRowsPerPageChange={event => { setPageSize(Number(event.target.value)); setPage(0) }} rowsPerPageOptions={[10, 20, 50]} labelRowsPerPage="每页" /></Card>

    <Dialog open={formOpen} onClose={() => !saving && setFormOpen(false)} fullWidth maxWidth="sm"><DialogTitle>新增{entertainmentOnly ? '娱乐上下分' : '上下分'}申请</DialogTitle><DialogContent><Stack gap={2} pt={1}><TextField select label="申请用户" value={form.user_id || ''} onChange={event => setForm(current => ({ ...current, user_id: Number(event.target.value) }))} required disabled={!users.length}>{users.map(user => <MenuItem key={user.id} value={user.id}>{user.nickname || user.username}（@{user.username} · 余额 {money(user.balance)}）</MenuItem>)}</TextField>{!users.length && <Alert severity="warning">没有可用的正常用户，请先在用户管理中创建或启用用户。</Alert>}<Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,1fr)' }, gap: 2 }}><TextField select label="申请类型" value={form.request_type} onChange={event => setForm(current => ({ ...current, request_type: event.target.value as 'credit' | 'debit', amount: 0 }))}><MenuItem value="credit">上分</MenuItem><MenuItem value="debit">下分</MenuItem></TextField><TextField select label="支付方式" value={form.payment_type} onChange={event => setForm(current => ({ ...current, payment_type: event.target.value }))}>{Object.entries(paymentLabels).map(([value, label]) => <MenuItem key={value} value={value}>{label}</MenuItem>)}</TextField></Box>{entertainmentOnly && <TextField label="娱乐平台或游戏标识" value={form.game_id || ''} onChange={event => setForm(current => ({ ...current, game_id: event.target.value }))} placeholder="例如 FB体育、捕鱼大厅" inputProps={{ maxLength: 40 }} required />}<TextField type="number" label="申请金额" value={form.amount || ''} onChange={event => setForm(current => ({ ...current, amount: Number(event.target.value) }))} slotProps={{ htmlInput: { min: .01, step: .01 } }} required /><TextField label="申请备注" value={form.remark} onChange={event => setForm(current => ({ ...current, remark: event.target.value }))} multiline minRows={3} inputProps={{ maxLength: 500 }} /></Stack></DialogContent><DialogActions><Button onClick={() => setFormOpen(false)} disabled={saving}>取消</Button><Button variant="contained" onClick={() => void submitCreate()} disabled={saving || !users.length}>{saving ? '保存中…' : '创建申请'}</Button></DialogActions></Dialog>

    <Dialog open={Boolean(reviewItem)} onClose={() => !saving && setReviewItem(null)} fullWidth maxWidth="sm"><DialogTitle>审核申请 #{reviewItem?.id}</DialogTitle><DialogContent>{reviewItem && <Stack gap={2} pt={1}><Paper variant="outlined" sx={{ p: 1.5 }}><Stack direction="row" justifyContent="space-between" alignItems="center"><Box><Typography fontWeight={800}>{reviewItem.username} · {typeLabels[reviewItem.request_type]}</Typography><Typography variant="caption" color="text.secondary">{reviewItem.request_type === 'join' ? `目标房间 ${reviewItem.target_room_code || '待确认'}` : `申请金额 ${reviewItem.requested_amount ? money(reviewItem.requested_amount) : '—'}`} · {dateTime(reviewItem.created_at)}</Typography></Box><Chip size="small" color="warning" label="待审核" /></Stack>{reviewItem.remark && <Typography fontSize={12} mt={1.25}>{reviewItem.remark}</Typography>}</Paper><Tabs value={decision} onChange={(_, value: 'approved' | 'rejected') => setDecision(value)} variant="fullWidth"><Tab value="approved" icon={<TaskAltRounded />} iconPosition="start" label="通过申请" /><Tab value="rejected" icon={<CancelRounded />} iconPosition="start" label="拒绝申请" /></Tabs>{decision === 'approved' && reviewItem.request_type === 'join' && <Paper variant="outlined" sx={{ p: 1.5, borderColor: 'primary.light', bgcolor: 'action.hover' }}><Typography fontSize={13} fontWeight={850}>会员赔率倍率</Typography><Typography variant="caption" color="text.secondary">只对目标房间生效；用户单独玩法赔率优先，无单独赔率时按房间或平台赔率 × 倍率。</Typography><Stack direction="row" gap={.75} flexWrap="wrap" mt={1.25}>{oddsMultiplierPresets.map(value => <Button key={value} size="small" variant={Number(oddsMultiplier) === value ? 'contained' : 'outlined'} onClick={() => setOddsMultiplier(String(value))}>{value.toFixed(2)}×</Button>)}</Stack><TextField fullWidth type="number" label="自定义倍率" value={oddsMultiplier} onChange={event => setOddsMultiplier(event.target.value)} sx={{ mt: 1.5 }} slotProps={{ htmlInput: { min: .5, max: 1.5, step: .01 } }} helperText="允许范围 0.50–1.50；1.00 为跟随房间原赔率" required /></Paper>}{decision === 'approved' && moneyRequest && <><TextField type="number" label={reviewItem.request_type === 'credit' ? '实际到账金额' : '实际出款金额'} value={receivedAmount} onChange={event => setReceivedAmount(event.target.value)} helperText={reviewItem.request_type === 'credit' ? '通过后将按实际到账金额增加用户余额' : '通过后将按申请金额扣减余额，并记录实际出款金额'} slotProps={{ htmlInput: { min: .01, step: .01 } }} required /><Paper variant="outlined" sx={{ p: 1.35 }}><Stack direction="row" justifyContent="space-between"><Typography fontSize={12} color="text.secondary">变动前余额</Typography><Typography fontSize={13} fontWeight={800}>{money(reviewItem.user_balance ?? 0)}</Typography></Stack><Stack direction="row" justifyContent="space-between" mt={.8}><Typography fontSize={12} color="text.secondary">预计变动后余额</Typography><Typography fontSize={15} fontWeight={900} color="primary.main">{money((reviewItem.user_balance ?? 0) + (reviewItem.request_type === 'credit' ? (Number(receivedAmount) || 0) : -reviewItem.requested_amount))}</Typography></Stack></Paper></>}<TextField label={decision === 'rejected' ? '拒绝原因' : '审核备注'} value={reviewRemark} onChange={event => setReviewRemark(event.target.value)} multiline minRows={3} required={decision === 'rejected'} inputProps={{ maxLength: 500 }} /><Alert severity={decision === 'approved' ? 'info' : 'warning'}>{decision === 'approved' ? reviewItem.request_type === 'join' ? `确认后会员将绑定至房间 ${reviewItem.target_room_code || ''}，赔率倍率为 ${(Number(oddsMultiplier) || 1).toFixed(2)}×。` : moneyRequest ? '确认后会立即处理用户余额并写入资金流水，操作不可重复。' : '确认后申请状态将变为已通过。' : '拒绝后不会发生账户或房间变更。'}</Alert></Stack>}</DialogContent><DialogActions><Button onClick={() => setReviewItem(null)} disabled={saving}>取消</Button><Button variant="contained" color={decision === 'approved' ? 'success' : 'error'} onClick={() => void submitReview()} disabled={saving}>{saving ? '处理中…' : decision === 'approved' ? '确认通过' : '确认拒绝'}</Button></DialogActions></Dialog>

    <Drawer anchor="right" open={Boolean(detail)} onClose={() => setDetail(null)} slotProps={{ paper: { sx: { width: { xs: '100%', sm: 430 }, p: 2.5 } } }}>{detail && <><Stack direction="row" alignItems="center" justifyContent="space-between"><Box><Typography variant="overline" color="primary">申请 #{detail.id}</Typography><Typography variant="h6" fontWeight={850}>申请详情</Typography></Box><Button onClick={() => setDetail(null)} startIcon={<CloseRounded />}>关闭</Button></Stack><Divider sx={{ my: 2 }} /><Stack gap={1.4}><DetailRow label="申请用户" value={`${detail.username}（ID ${detail.user_id}）`} /><DetailRow label="当前余额" value={money(detail.user_balance ?? 0)} />{detail.balance_before !== undefined && <DetailRow label="审核前余额" value={money(detail.balance_before)} />}{detail.balance_after !== undefined && <DetailRow label="审核后余额" value={money(detail.balance_after)} />}<DetailRow label="所属房间" value={`${detail.room_code || detail.target_room_code || '—'}${detail.room_name ? ` · ${detail.room_name}` : ''}`} /><DetailRow label="账号类型" value={roleLabels[detail.account_type] ?? detail.account_type} /><DetailRow label="申请类型" value={typeLabels[detail.request_type]} />{detail.request_type === 'join' && <DetailRow label="会员赔率倍率" value={`${(detail.odds_multiplier || 1).toFixed(2)}×`} />}{detail.game_id && <DetailRow label="娱乐平台" value={detail.game_id} />}<DetailRow label="支付方式" value={detail.payment_account_label || paymentLabels[detail.payment_type] || detail.payment_type} /><DetailRow label="申请金额" value={detail.requested_amount ? money(detail.requested_amount) : '—'} /><DetailRow label="到账金额" value={detail.received_amount ? money(detail.received_amount) : '—'} />{detail.request_id && <DetailRow label="请求编号" value={detail.request_id} />}{detail.chat_message_id ? <DetailRow label="关联聊天" value={`消息 #${detail.chat_message_id}`} /> : null}<DetailRow label="审核状态" value={statusLabels[detail.status]} /><DetailRow label="申请时间" value={dateTime(detail.created_at)} /><DetailRow label="审核时间" value={dateTime(detail.reviewed_at)} /><DetailRow label="操作人" value={detail.operator || '—'} /></Stack><Typography fontWeight={800} mt={2.5} mb={1}>处理时间线</Typography><Paper variant="outlined" sx={{ p: 1.5 }}><Stack gap={1.1}><Typography fontSize={12}><b>提交申请</b> · {dateTime(detail.created_at)}</Typography><Typography fontSize={12} color={detail.reviewed_at ? 'text.primary' : 'text.secondary'}><b>{detail.status === 'pending' ? '等待审核' : detail.status === 'approved' ? '审核通过' : '审核拒绝'}</b>{detail.reviewed_at ? ` · ${dateTime(detail.reviewed_at)}` : ''}</Typography></Stack></Paper><Typography fontWeight={800} mt={2.5} mb={1}>申请备注</Typography><Paper variant="outlined" sx={{ p: 1.5, minHeight: 70 }}><Typography fontSize={12}>{detail.remark || '无'}</Typography></Paper><Typography fontWeight={800} mt={2.5} mb={1}>审核备注</Typography><Paper variant="outlined" sx={{ p: 1.5, minHeight: 70 }}><Typography fontSize={12}>{detail.review_remark || '无'}</Typography></Paper>{detail.status === 'pending' && <Button variant="contained" startIcon={<FactCheckRounded />} sx={{ mt: 2 }} onClick={() => { setDetail(null); openReview(detail) }}>立即审核</Button>}</>}</Drawer>
  </Box>
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return <Stack direction="row" justifyContent="space-between" gap={2}><Typography fontSize={12} color="text.secondary">{label}</Typography><Typography fontSize={12} fontWeight={750} textAlign="right">{value}</Typography></Stack>
}
