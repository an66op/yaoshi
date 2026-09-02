import AddRounded from '@mui/icons-material/AddRounded'
import BadgeRounded from '@mui/icons-material/BadgeRounded'
import EditRounded from '@mui/icons-material/EditRounded'
import GroupsRounded from '@mui/icons-material/GroupsRounded'
import KeyRounded from '@mui/icons-material/KeyRounded'
import PersonAddAlt1Rounded from '@mui/icons-material/PersonAddAlt1Rounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import StorefrontRounded from '@mui/icons-material/StorefrontRounded'
import SettingsSuggestRounded from '@mui/icons-material/SettingsSuggestRounded'
import TuneRounded from '@mui/icons-material/TuneRounded'
import AccountBalanceWalletRounded from '@mui/icons-material/AccountBalanceWalletRounded'
import { Alert, Box, Button, Card, Chip, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, Grid, IconButton, InputAdornment, MenuItem, Stack, Switch, Table, TableBody, TableCell, TableContainer, TableHead, TablePagination, TableRow, TextField, Tooltip, Typography } from '@mui/material'
import { useCallback, useEffect, useRef, useState } from 'react'
import { adminApi, type AdminGame, type AgentItem, type AgentListResponse, type RoomTradingConfig, type TenantItem } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'
import { RoomOperationsDialog } from '../components/RoomOperationsDialog'
import { WorkspaceAdminAccountFields, WorkspaceAdminCreatedDialog } from '../components/WorkspaceAdminAccount'
import { createdWorkspaceAdmin, validateWorkspaceAdminAccount, type CreatedWorkspaceAdmin } from '../utils/workspaceAdminAccount'

const money = (value: number) => new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)
const emptyAgent = () => ({ username: '', password: '', nickname: '', email: '', phone: '', room_code: '', room_name: '', room_logo: '', rebate_rate: 0, profit_share_rate: 0, remark: '', status: 1, tenant_id: 0 })
const validRoom = (room: string) => /^\d{5,12}$/.test(room.trim())

function StatCard({ icon, label, value, tone = 'primary' }: { icon: React.ReactNode; label: string; value: string | number; tone?: 'primary' | 'success' | 'warning' | 'secondary' }) {
  return <Card variant="outlined" sx={{ p: 1.5, height: '100%' }}><Stack direction="row" alignItems="center" gap={1.2}><Box sx={{ width: 38, height: 38, borderRadius: 2, display: 'grid', placeItems: 'center', color: `${tone}.main`, bgcolor: `${tone}.lighter` }}>{icon}</Box><Box><Typography variant="caption" color="text.secondary">{label}</Typography><Typography fontWeight={900} fontSize={19}>{value}</Typography></Box></Stack></Card>
}

export function AgentsPage() {
  const [data, setData] = useState<AgentListResponse | null>(null)
  const [page, setPage] = useState(0); const [pageSize, setPageSize] = useState(20)
  const [query, setQuery] = useState(''); const [applied, setApplied] = useState('')
  const [loading, setLoading] = useState(true); const [error, setError] = useState(''); const [saving, setSaving] = useState(false)
  const [createOpen, setCreateOpen] = useState(false); const [promoteOpen, setPromoteOpen] = useState(false)
  const [editing, setEditing] = useState<AgentItem | null>(null); const [resetting, setResetting] = useState<AgentItem | null>(null)
  const [tradingAgent, setTradingAgent] = useState<AgentItem | null>(null); const [trading, setTrading] = useState<RoomTradingConfig | null>(null); const [games, setGames] = useState<AdminGame[]>([]); const [tradingSaving, setTradingSaving] = useState(false)
  const [operationsAgent, setOperationsAgent] = useState<AgentItem | null>(null)
  const [balanceAgent, setBalanceAgent] = useState<AgentItem | null>(null)
  const [balanceForm, setBalanceForm] = useState({ amount: '', remark: '房间红包及运营备用金' })
  const [form, setForm] = useState(emptyAgent); const [promote, setPromote] = useState({ user_id: '', room_code: '' }); const [newPassword, setNewPassword] = useState('')
  const [tenants, setTenants] = useState<TenantItem[]>([])
  const [createdAdmin, setCreatedAdmin] = useState<CreatedWorkspaceAdmin | null>(null)
  const [formError, setFormError] = useState('')
  const creating = useRef(false)
  const { showMessage } = useFeedback(); const items = Array.isArray(data?.items) ? data.items : []; const summary = data?.summary ?? { total: 0, active: 0, disabled: 0, members: 0 }

  const load = useCallback(async (notify = false) => {
    setLoading(true); setError('')
    try { const result = await adminApi.agents({ query: applied, page: page + 1, pageSize }); setData(result); if (notify) showMessage('代理数据已刷新') }
    catch (reason) { setError(reason instanceof Error ? reason.message : '读取代理失败') }
    finally { setLoading(false) }
  }, [applied, page, pageSize, showMessage])
  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])
  useEffect(() => { void adminApi.tenants({ pageSize: 100 }).then(result => setTenants(Array.isArray(result?.items) ? result.items : [])).catch(() => setTenants([])) }, [])

  const openCreate = () => { setForm(emptyAgent()); setFormError(''); setCreateOpen(true) }
  const closeForm = () => { if (saving || creating.current) return; setCreateOpen(false); setEditing(null); setForm(emptyAgent()); setFormError('') }
  const openEdit = (agent: AgentItem) => { setFormError(''); setForm({ username: agent.username, password: '', nickname: agent.nickname, email: agent.email, phone: agent.phone, room_code: agent.room_code, room_name: agent.room_name ?? '', room_logo: agent.room_logo ?? '', rebate_rate: agent.rebate_rate ?? 0, profit_share_rate: agent.profit_share_rate ?? 0, remark: agent.remark, status: agent.status, tenant_id: agent.tenant_id ?? 0 }); setEditing(agent) }
  const saveNew = async () => {
    if (creating.current) return
    const validationError = validateWorkspaceAdminAccount(form.username, form.password)
    if (validationError || !validRoom(form.room_code)) { setFormError(validationError || '房间号须为 5–12 位数字'); return }
    creating.current = true
    setSaving(true); setFormError('')
    try {
      const account = await adminApi.createAgent({ ...form, username: form.username.trim(), room_code: form.room_code.trim(), tenant_id: form.tenant_id || undefined })
      setCreatedAdmin(createdWorkspaceAdmin('agent', account))
      showMessage('代理账号已创建'); setCreateOpen(false); setForm(emptyAgent()); await load()
    }
    catch (reason) { setFormError(reason instanceof Error ? reason.message : '创建代理失败') } finally { creating.current = false; setSaving(false) }
  }
  const saveEdit = async () => {
    if (!editing || creating.current) return
    if (!validRoom(form.room_code)) { setFormError('房间号须为 5–12 位数字'); return }
    creating.current = true
    setSaving(true); setFormError('')
    try { await adminApi.updateAgent(editing.id, { email: form.email, nickname: form.nickname, phone: form.phone, room_code: form.room_code.trim(), room_name: form.room_name.trim(), room_logo: form.room_logo, rebate_rate: form.rebate_rate, profit_share_rate: form.profit_share_rate, remark: form.remark, status: form.status, tenant_id: form.tenant_id || undefined }); showMessage('代理资料、租户归属与房间配置已保存'); setEditing(null); setForm(emptyAgent()); await load() }
    catch (reason) { setFormError(reason instanceof Error ? reason.message : '保存代理失败') } finally { creating.current = false; setSaving(false) }
  }
  const promoteExisting = async () => {
    const id = Number(promote.user_id)
    if (!id || !validRoom(promote.room_code)) { setError('请填写已有会员内部 ID 和 5–12 位数字房间号'); return }
    setSaving(true)
    try { await adminApi.promoteAgent(id, promote.room_code.trim()); showMessage('已有会员已转为代理，房间号已分配'); setPromoteOpen(false); setPromote({ user_id: '', room_code: '' }); await load() }
    catch (reason) { setError(reason instanceof Error ? reason.message : '转换代理失败') } finally { setSaving(false) }
  }
  const resetPassword = async () => {
    if (!resetting || new TextEncoder().encode(newPassword).length < 8) { setError('新密码需要 8–72 个字符'); return }
    setSaving(true)
    try { await adminApi.resetAgentPassword(resetting.id, newPassword); showMessage('代理登录密码已重置'); setResetting(null); setNewPassword('') }
    catch (reason) { setError(reason instanceof Error ? reason.message : '重置密码失败') } finally { setSaving(false) }
  }
  const adjustRoomBalance = async () => {
    if (!balanceAgent) return
    const amount = Number(balanceForm.amount)
    if (!Number.isFinite(amount) || amount === 0 || !balanceForm.remark.trim()) { setError('请填写非零调整金额和原因'); return }
    setSaving(true)
    try {
      await adminApi.adjustUserBalance(balanceAgent.id, amount, balanceForm.remark.trim())
      showMessage(amount > 0 ? '房间运营余额已补充' : '房间运营余额已扣减')
      setBalanceAgent(null); setBalanceForm({ amount: '', remark: '房间红包及运营备用金' }); await load()
    } catch (reason) { setError(reason instanceof Error ? reason.message : '调整房间余额失败') }
    finally { setSaving(false) }
  }
  const openRoomTrading = async (agent: AgentItem) => {
    setTradingAgent(agent); setTrading(null)
    try {
      const [dashboard, config] = await Promise.all([adminApi.dashboard(), adminApi.roomTrading(agent.id)])
      setGames(dashboard.games ?? []); setTrading(config)
    } catch (reason) { setError(reason instanceof Error ? reason.message : '读取房间赔率失败') }
  }
  const loadRoomTrading = async (gameId: string) => {
    if (!tradingAgent) return
    try { setTrading(await adminApi.roomTrading(tradingAgent.id, gameId)) } catch (reason) { setError(reason instanceof Error ? reason.message : '读取房间赔率失败') }
  }
  const saveRoomTrading = async () => {
    if (!tradingAgent || !trading) return
    setTradingSaving(true)
    try {
      const next = await adminApi.updateRoomTrading(tradingAgent.id, { rebate_rate: trading.rebate_rate, game_id: trading.game_id, odds: trading.odds.map(item => ({ play_code: item.play_code, override: item.has_override ? item.override : null })) })
      setTrading(next); showMessage(`房间 ${next.room_code} 的赔率与返水已保存`); await load()
    } catch (reason) { setError(reason instanceof Error ? reason.message : '保存房间配置失败') } finally { setTradingSaving(false) }
  }
  const stats = [
    { label: '代理总数', value: summary.total, icon: <BadgeRounded fontSize="small" /> },
    { label: '正常运营', value: summary.active, icon: <StorefrontRounded fontSize="small" />, tone: 'success' as const },
    { label: '已停用', value: summary.disabled, icon: <StorefrontRounded fontSize="small" />, tone: 'warning' as const },
    { label: '归属会员', value: summary.members, icon: <GroupsRounded fontSize="small" />, tone: 'secondary' as const },
  ]

  return <Box p={{ xs: 2, lg: 2.5 }}>
    <PageHeader eyebrow="业务管理 / 代理" title="代理管理" description="在这里开通代理登录账号、维护资料和重置密码；房间是代理账号的关联配置。" actions={<><Button variant="outlined" startIcon={<PersonAddAlt1Rounded />} onClick={() => setPromoteOpen(true)}>已有会员转代理</Button><Button variant="contained" startIcon={<AddRounded />} onClick={openCreate}>开通代理账号</Button></>} />
    {error && <Alert severity="error" sx={{ mt: 2 }} onClose={() => setError('')}>{error}</Alert>}
    <Grid container spacing={1.5} sx={{ mt: 1 }}>{stats.map(stat => <Grid size={{ xs: 6, md: 3 }} key={stat.label}><StatCard {...stat} /></Grid>)}</Grid>
    <Card variant="outlined" sx={{ mt: 2, p: 1.5 }}><Stack direction={{ xs: 'column', sm: 'row' }} gap={1.25}><TextField size="small" fullWidth placeholder="搜索登录账号、昵称、手机号或房间号" value={query} onChange={event => setQuery(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') { setPage(0); setApplied(query.trim()) } }} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> } }} /><Button variant="contained" onClick={() => { setPage(0); setApplied(query.trim()) }}>查询</Button></Stack></Card>
    <Card sx={{ mt: 1.5 }}>{loading && <Box px={2} py={1}><CircularProgress size={18} /></Box>}<TableContainer><Table size="small" sx={{ minWidth: 1120 }}><TableHead><TableRow><TableCell>代理账号</TableCell><TableCell>所属租户</TableCell><TableCell>关联房间</TableCell><TableCell>联系方式</TableCell><TableCell align="right">归属会员</TableCell><TableCell align="right">账户余额</TableCell><TableCell>最近登录</TableCell><TableCell>运营状态</TableCell><TableCell align="right">操作</TableCell></TableRow></TableHead><TableBody>
      {items.map(item => <TableRow hover key={item.id}><TableCell><Typography fontWeight={850} fontSize={12}>{item.nickname || item.username}</Typography><Typography fontSize={10} color="text.secondary">@{item.username} · ID {item.public_id || item.id}</Typography></TableCell><TableCell><Chip size="small" variant={item.tenant_id ? 'filled' : 'outlined'} color={item.tenant_id ? 'secondary' : 'default'} label={item.tenant_name || '待分配'} /></TableCell><TableCell><Typography fontWeight={800} fontSize={11.5}>{item.room_name || `${item.nickname || item.username}的房间`}</Typography><Chip size="small" color="primary" label={item.room_code || '未分配'} /><Typography fontSize={9} color="text.secondary" mt={.5}>返水 {item.rebate_rate ?? 0}% · 分成 {item.profit_share_rate ?? 0}%</Typography></TableCell><TableCell><Typography fontSize={11}>{item.phone || '未填写手机'}</Typography><Typography fontSize={10} color="text.secondary">{item.email || '未填写邮箱'}</Typography></TableCell><TableCell align="right"><Typography fontWeight={800}>{item.member_count}</Typography></TableCell><TableCell align="right">{money(item.balance)}</TableCell><TableCell><Typography fontSize={10}>{item.last_login_at || '尚未登录'}</Typography><Typography fontSize={9} color="text.secondary">累计 {item.login_count} 次</Typography></TableCell><TableCell><Stack direction="row" alignItems="center" gap={.5}><Switch size="small" checked={item.status === 1} onChange={() => openEdit({ ...item, status: item.status === 1 ? 0 : 1 })} /><Typography fontSize={10}>{item.status === 1 ? '正常' : '已停用'}</Typography></Stack></TableCell><TableCell align="right"><Tooltip title="调整房间运营余额"><IconButton size="small" onClick={() => { setBalanceAgent(item); setBalanceForm({ amount: '', remark: '房间红包及运营备用金' }) }}><AccountBalanceWalletRounded fontSize="small" /></IconButton></Tooltip><Tooltip title="房间营业、审核与游戏"><IconButton size="small" onClick={() => setOperationsAgent(item)}><SettingsSuggestRounded fontSize="small" /></IconButton></Tooltip><Tooltip title="房间赔率与返水"><IconButton size="small" onClick={() => void openRoomTrading(item)}><TuneRounded fontSize="small" /></IconButton></Tooltip><IconButton size="small" color="primary" aria-label={`设置 @${item.username} 代理账号`} onClick={() => openEdit(item)}><EditRounded fontSize="small" /></IconButton><Tooltip title="重置登录密码"><IconButton size="small" aria-label={`重置 @${item.username} 登录密码`} onClick={() => { setResetting(item); setNewPassword('') }}><KeyRounded fontSize="small" /></IconButton></Tooltip></TableCell></TableRow>)}
      {!loading && !items.length && <TableRow><TableCell colSpan={9} align="center" sx={{ py: 7, color: 'text.secondary' }}>暂无代理。可开通代理账号，或将会员管理中的已有会员转为代理。</TableCell></TableRow>}
    </TableBody></Table></TableContainer><TablePagination component="div" count={data?.total ?? 0} page={page} onPageChange={(_, next) => setPage(next)} rowsPerPage={pageSize} onRowsPerPageChange={event => { setPageSize(Number(event.target.value)); setPage(0) }} rowsPerPageOptions={[10, 20, 50]} labelRowsPerPage="每页" /></Card>
    <Dialog open={createOpen || Boolean(editing)} onClose={closeForm} fullWidth maxWidth="sm"><DialogTitle>{editing ? `代理账号设置 · ${editing.username}` : '开通代理账号'}</DialogTitle><DialogContent>{editing && <Alert severity="info" sx={{ mt: 1, mb: 2 }}>在这里维护代理的账号资料和关联房间；登录账号不可修改，密码可在代理列表中重置。</Alert>}{formError && <Alert severity="error" sx={{ my: 1.5 }}>{formError}</Alert>}<Stack gap={1.5}><WorkspaceAdminAccountFields role="agent" username={form.username} password={form.password} editing={Boolean(editing)} disabled={saving} onUsernameChange={username => setForm(current => ({ ...current, username }))} onPasswordChange={password => setForm(current => ({ ...current, password }))} /><TextField select fullWidth label="所属租户" value={form.tenant_id} onChange={event => setForm(current => ({ ...current, tenant_id: Number(event.target.value) }))} helperText="未分配租户的代理只有总管理员可管理"><MenuItem value={0}>暂不分配</MenuItem>{tenants.map(tenant => <MenuItem key={tenant.id} value={tenant.id}>{tenant.nickname || tenant.username} · @{tenant.username}</MenuItem>)}</TextField><Stack direction={{ xs: 'column', sm: 'row' }} gap={1.5}><TextField fullWidth label="代理昵称" value={form.nickname} onChange={event => setForm(current => ({ ...current, nickname: event.target.value }))} /><TextField fullWidth label="房间名称" value={form.room_name} onChange={event => setForm(current => ({ ...current, room_name: event.target.value }))} helperText="代理可在自己的工作台再次修改" inputProps={{ maxLength: 30 }} /></Stack><TextField required fullWidth label="房间号" value={form.room_code} onChange={event => setForm(current => ({ ...current, room_code: event.target.value.replace(/\D/g, '') }))} helperText="5–12 位数字，必须唯一" /><Stack direction={{ xs: 'column', sm: 'row' }} gap={1.5}><TextField fullWidth label="手机号" value={form.phone} onChange={event => setForm(current => ({ ...current, phone: event.target.value }))} /><TextField fullWidth type="email" label="邮箱" value={form.email} onChange={event => setForm(current => ({ ...current, email: event.target.value }))} /></Stack><Stack direction={{ xs: 'column', sm: 'row' }} gap={1.5}><TextField fullWidth type="number" label="房间默认返水比例 %" value={form.rebate_rate} onChange={event => setForm(current => ({ ...current, rebate_rate: Number(event.target.value) }))} inputProps={{ min: 0, max: 100, step: 0.01 }} /><TextField fullWidth type="number" label="代理利润分成比例 %" value={form.profit_share_rate} onChange={event => setForm(current => ({ ...current, profit_share_rate: Number(event.target.value) }))} inputProps={{ min: 0, max: 100, step: 0.01 }} helperText="逐注正毛利 × 比例，亏损注不抵扣；手动结算" /></Stack><TextField label="运营备注" multiline minRows={2} value={form.remark} onChange={event => setForm(current => ({ ...current, remark: event.target.value }))} /><Stack direction="row" alignItems="center" gap={1}><Switch checked={form.status === 1} onChange={(_, checked) => setForm(current => ({ ...current, status: checked ? 1 : 0 }))} /><Typography fontSize={13}>{form.status === 1 ? '账号正常启用' : '账号创建后停用'}</Typography></Stack></Stack></DialogContent><DialogActions><Button disabled={saving} onClick={closeForm}>取消</Button><Button variant="contained" disabled={saving} onClick={() => void (editing ? saveEdit() : saveNew())}>{saving ? '保存中…' : editing ? '保存修改' : '开通代理账号'}</Button></DialogActions></Dialog>
    <WorkspaceAdminCreatedDialog account={createdAdmin} onClose={() => setCreatedAdmin(null)} />
    <Dialog open={Boolean(tradingAgent)} onClose={() => !tradingSaving && setTradingAgent(null)} fullWidth maxWidth="md"><DialogTitle>房间赔率与返水 · {tradingAgent?.room_code}</DialogTitle><DialogContent>{!trading ? <Box py={6} textAlign="center"><CircularProgress size={24} /></Box> : <Stack gap={1.5} mt={1}><Alert severity="info">本页设置整个房间的默认值；会员若有单独配置，将优先使用会员配置。</Alert><Stack direction={{ xs: 'column', sm: 'row' }} gap={1}><TextField fullWidth type="number" label="房间默认返水 %" value={trading.rebate_rate} onChange={event => setTrading(current => current ? { ...current, rebate_rate: Number(event.target.value) } : current)} inputProps={{ min: 0, max: 100, step: 0.01 }} /><TextField select fullWidth label="彩种赔率" value={trading.game_id} onChange={event => void loadRoomTrading(event.target.value)}>{games.map(game => <MenuItem key={game.id} value={game.id}>{game.name}</MenuItem>)}{!games.some(game => game.id === trading.game_id) && <MenuItem value={trading.game_id}>{trading.game_name}</MenuItem>}</TextField></Stack><TableContainer sx={{ maxHeight: 430 }}><Table size="small" stickyHeader><TableHead><TableRow><TableCell>玩法</TableCell><TableCell align="right">平台赔率</TableCell><TableCell align="right">房间赔率</TableCell></TableRow></TableHead><TableBody>{trading.odds.map((item, index) => <TableRow key={item.play_code}><TableCell><Typography fontSize={11} fontWeight={700}>{item.play_name}</Typography><Typography fontSize={9} color="text.secondary">{item.play_code}</Typography></TableCell><TableCell align="right">{item.base_odds}</TableCell><TableCell align="right"><TextField size="small" type="number" placeholder="继承平台" value={item.has_override ? (item.override ?? '') : ''} onChange={event => { const raw = event.target.value; setTrading(current => { if (!current) return current; const odds = [...current.odds]; odds[index] = !raw.trim() ? { ...odds[index], override: null, has_override: false, effective: odds[index].base_odds } : { ...odds[index], override: Number(raw), has_override: true, effective: Number(raw) }; return { ...current, odds } }) }} sx={{ width: 120 }} inputProps={{ min: 1.001, step: 0.001 }} /></TableCell></TableRow>)}</TableBody></Table></TableContainer></Stack>}</DialogContent><DialogActions><Button onClick={() => setTradingAgent(null)}>取消</Button><Button variant="contained" disabled={!trading || tradingSaving} onClick={() => void saveRoomTrading()}>{tradingSaving ? '保存中…' : '保存房间配置'}</Button></DialogActions></Dialog>
    <RoomOperationsDialog open={Boolean(operationsAgent)} title={`经营配置 · ${operationsAgent?.room_code || ''}`} target={operationsAgent ? { kind: 'agent', id: operationsAgent.id } : null} onClose={() => setOperationsAgent(null)} onSaved={showMessage} />
    <Dialog open={Boolean(balanceAgent)} onClose={() => !saving && setBalanceAgent(null)} fullWidth maxWidth="xs"><DialogTitle>房间运营余额 · {balanceAgent?.room_code}</DialogTitle><DialogContent><Stack gap={1.5} mt={1}><Alert severity="info">红包发送时会先从房间所有者余额预留总金额；红包关闭或过期后，未领取部分自动退回。</Alert><Typography fontWeight={800}>当前余额：{money(balanceAgent?.balance ?? 0)}</Typography><TextField autoFocus type="number" label="调整金额" value={balanceForm.amount} onChange={event => setBalanceForm(current => ({ ...current, amount: event.target.value }))} helperText="正数为补充，负数为扣减" /><TextField label="调整原因" value={balanceForm.remark} onChange={event => setBalanceForm(current => ({ ...current, remark: event.target.value }))} /></Stack></DialogContent><DialogActions><Button onClick={() => setBalanceAgent(null)}>取消</Button><Button variant="contained" disabled={saving || !balanceForm.amount || Number(balanceForm.amount) === 0} onClick={() => void adjustRoomBalance()}>{saving ? '处理中…' : '确认调整'}</Button></DialogActions></Dialog>
    <Dialog open={promoteOpen} onClose={() => !saving && setPromoteOpen(false)} fullWidth maxWidth="xs"><DialogTitle>已有会员转为代理</DialogTitle><DialogContent><Alert severity="info" sx={{ mt: 1, mb: 2 }}>输入会员管理中的内部 ID（不是公开 ID）。转换后，该会员账号变为代理账号，使用原账号和密码登录管理后台。</Alert><Stack gap={1.5}><TextField required type="number" label="已有会员内部 ID" value={promote.user_id} onChange={event => setPromote(current => ({ ...current, user_id: event.target.value }))} /><TextField required label="分配房间号" value={promote.room_code} onChange={event => setPromote(current => ({ ...current, room_code: event.target.value.replace(/\D/g, '') }))} helperText="5–12 位数字且不可与其他代理重复" /></Stack></DialogContent><DialogActions><Button onClick={() => setPromoteOpen(false)}>取消</Button><Button variant="contained" disabled={saving} onClick={() => void promoteExisting()}>{saving ? '处理中…' : '确认转换'}</Button></DialogActions></Dialog>
    <Dialog open={Boolean(resetting)} onClose={() => !saving && setResetting(null)} fullWidth maxWidth="xs"><DialogTitle>重置代理登录密码</DialogTitle><DialogContent><Typography variant="body2" color="text.secondary" sx={{ mt: 1, mb: 2 }}>将为 @{resetting?.username} 设置新的登录密码。</Typography><TextField autoFocus fullWidth type="password" label="新登录密码" value={newPassword} onChange={event => setNewPassword(event.target.value)} helperText="8–72 字节，中文等字符占多个字节" /></DialogContent><DialogActions><Button onClick={() => setResetting(null)}>取消</Button><Button variant="contained" disabled={saving} onClick={() => void resetPassword()}>{saving ? '重置中…' : '确认重置'}</Button></DialogActions></Dialog>
  </Box>
}
