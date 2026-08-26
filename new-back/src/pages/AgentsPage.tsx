import AddRounded from '@mui/icons-material/AddRounded'
import BadgeRounded from '@mui/icons-material/BadgeRounded'
import EditRounded from '@mui/icons-material/EditRounded'
import GroupsRounded from '@mui/icons-material/GroupsRounded'
import KeyRounded from '@mui/icons-material/KeyRounded'
import PersonAddAlt1Rounded from '@mui/icons-material/PersonAddAlt1Rounded'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import StorefrontRounded from '@mui/icons-material/StorefrontRounded'
import TuneRounded from '@mui/icons-material/TuneRounded'
import { Alert, Box, Button, Card, Chip, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, Grid, IconButton, InputAdornment, MenuItem, Stack, Switch, Table, TableBody, TableCell, TableContainer, TableHead, TablePagination, TableRow, TextField, Tooltip, Typography } from '@mui/material'
import { useCallback, useEffect, useState } from 'react'
import { adminApi, type AdminGame, type AgentItem, type AgentListResponse, type RoomTradingConfig, type TenantItem } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'

const money = (value: number) => new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)
const emptyAgent = () => ({ username: '', password: '', nickname: '', email: '', phone: '', room_code: '', room_name: '', room_logo: '', rebate_rate: 0, profit_share_rate: 0, remark: '', status: 1, tenant_id: 0 })
const validRoom = (room: string) => /^\d{4,12}$/.test(room.trim())

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
  const [form, setForm] = useState(emptyAgent); const [promote, setPromote] = useState({ user_id: '', room_code: '' }); const [newPassword, setNewPassword] = useState('')
  const [tenants, setTenants] = useState<TenantItem[]>([])
  const { showMessage } = useFeedback(); const items = data?.items ?? []; const summary = data?.summary ?? { total: 0, active: 0, disabled: 0, members: 0 }

  const load = useCallback(async (notify = false) => {
    setLoading(true); setError('')
    try { const result = await adminApi.agents({ query: applied, page: page + 1, pageSize }); setData(result); if (notify) showMessage('代理数据已刷新') }
    catch (reason) { setError(reason instanceof Error ? reason.message : '读取代理失败') }
    finally { setLoading(false) }
  }, [applied, page, pageSize, showMessage])
  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])
  useEffect(() => { void adminApi.tenants({ pageSize: 100 }).then(result => setTenants(result.items)).catch(() => setTenants([])) }, [])

  const openCreate = () => { setForm(emptyAgent()); setCreateOpen(true) }
  const openEdit = (agent: AgentItem) => { setForm({ username: agent.username, password: '', nickname: agent.nickname, email: agent.email, phone: agent.phone, room_code: agent.room_code, room_name: agent.room_name ?? '', room_logo: agent.room_logo ?? '', rebate_rate: agent.rebate_rate ?? 0, profit_share_rate: agent.profit_share_rate ?? 0, remark: agent.remark, status: agent.status, tenant_id: agent.tenant_id ?? 0 }); setEditing(agent) }
  const saveNew = async () => {
    if (!form.username.trim() || new TextEncoder().encode(form.password).length < 8 || !validRoom(form.room_code)) { setError('请填写登录账号、8–72 位密码和 4–12 位数字房间号'); return }
    setSaving(true)
    try { await adminApi.createAgent({ ...form, username: form.username.trim(), room_code: form.room_code.trim(), tenant_id: form.tenant_id || undefined }); showMessage('代理账号已创建，可使用该账号登录会员端'); setCreateOpen(false); await load() }
    catch (reason) { setError(reason instanceof Error ? reason.message : '创建代理失败') } finally { setSaving(false) }
  }
  const saveEdit = async () => {
    if (!editing) return
    if (!validRoom(form.room_code)) { setError('房间号须为 4–12 位数字'); return }
    setSaving(true)
    try { await adminApi.updateAgent(editing.id, { email: form.email, nickname: form.nickname, phone: form.phone, room_code: form.room_code.trim(), room_name: form.room_name.trim(), room_logo: form.room_logo, rebate_rate: form.rebate_rate, profit_share_rate: form.profit_share_rate, remark: form.remark, status: form.status, tenant_id: form.tenant_id || undefined }); showMessage('代理资料、租户归属与房间配置已保存'); setEditing(null); await load() }
    catch (reason) { setError(reason instanceof Error ? reason.message : '保存代理失败') } finally { setSaving(false) }
  }
  const promoteExisting = async () => {
    const id = Number(promote.user_id)
    if (!id || !validRoom(promote.room_code)) { setError('请填写已有用户 ID 和 4–12 位数字房间号'); return }
    setSaving(true)
    try { await adminApi.promoteAgent(id, promote.room_code.trim()); showMessage('已有用户已升为代理，房间号已分配'); setPromoteOpen(false); setPromote({ user_id: '', room_code: '' }); await load() }
    catch (reason) { setError(reason instanceof Error ? reason.message : '转换代理失败') } finally { setSaving(false) }
  }
  const resetPassword = async () => {
    if (!resetting || new TextEncoder().encode(newPassword).length < 8) { setError('新密码需要 8–72 个字符'); return }
    setSaving(true)
    try { await adminApi.resetAgentPassword(resetting.id, newPassword); showMessage('代理登录密码已重置'); setResetting(null); setNewPassword('') }
    catch (reason) { setError(reason instanceof Error ? reason.message : '重置密码失败') } finally { setSaving(false) }
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
    <PageHeader eyebrow="业务管理 / 代理" title="代理管理" description="完整管理代理登录账号、房间号与运营状态。修改房间号后，已归属成员仍留在该代理名下。" actions={<><Button variant="outlined" startIcon={<RefreshRounded />} onClick={() => void load(true)}>刷新</Button><Button variant="outlined" startIcon={<PersonAddAlt1Rounded />} onClick={() => setPromoteOpen(true)}>已有用户转代理</Button><Button variant="contained" startIcon={<AddRounded />} onClick={openCreate}>新增代理</Button></>} />
    {error && <Alert severity="error" sx={{ mt: 2 }} onClose={() => setError('')}>{error}</Alert>}
    <Grid container spacing={1.5} sx={{ mt: 1 }}>{stats.map(stat => <Grid size={{ xs: 6, md: 3 }} key={stat.label}><StatCard {...stat} /></Grid>)}</Grid>
    <Card variant="outlined" sx={{ mt: 2, p: 1.5 }}><Stack direction={{ xs: 'column', sm: 'row' }} gap={1.25}><TextField size="small" fullWidth placeholder="搜索登录账号、昵称、手机号或房间号" value={query} onChange={event => setQuery(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') { setPage(0); setApplied(query.trim()) } }} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> } }} /><Button variant="contained" onClick={() => { setPage(0); setApplied(query.trim()) }}>查询</Button></Stack></Card>
    <Card sx={{ mt: 1.5 }}>{loading && <Box px={2} py={1}><CircularProgress size={18} /></Box>}<TableContainer><Table size="small" sx={{ minWidth: 1120 }}><TableHead><TableRow><TableCell>代理登录账号</TableCell><TableCell>所属租户</TableCell><TableCell>房间号</TableCell><TableCell>联系方式</TableCell><TableCell align="right">归属会员</TableCell><TableCell align="right">账户余额</TableCell><TableCell>最近登录</TableCell><TableCell>运营状态</TableCell><TableCell align="right">操作</TableCell></TableRow></TableHead><TableBody>
      {items.map(item => <TableRow hover key={item.id}><TableCell><Typography fontWeight={850} fontSize={12}>{item.nickname || item.username}</Typography><Typography fontSize={10} color="text.secondary">@{item.username} · ID {item.public_id || item.id}</Typography></TableCell><TableCell><Chip size="small" variant={item.tenant_id ? 'filled' : 'outlined'} color={item.tenant_id ? 'secondary' : 'default'} label={item.tenant_name || '待分配'} /></TableCell><TableCell><Typography fontWeight={800} fontSize={11.5}>{item.room_name || `${item.nickname || item.username}的房间`}</Typography><Chip size="small" color="primary" label={item.room_code || '未分配'} /><Typography fontSize={9} color="text.secondary" mt={.5}>返水 {item.rebate_rate ?? 0}% · 分成 {item.profit_share_rate ?? 0}%</Typography></TableCell><TableCell><Typography fontSize={11}>{item.phone || '未填写手机'}</Typography><Typography fontSize={10} color="text.secondary">{item.email || '未填写邮箱'}</Typography></TableCell><TableCell align="right"><Typography fontWeight={800}>{item.member_count}</Typography></TableCell><TableCell align="right">{money(item.balance)}</TableCell><TableCell><Typography fontSize={10}>{item.last_login_at || '尚未登录'}</Typography><Typography fontSize={9} color="text.secondary">累计 {item.login_count} 次</Typography></TableCell><TableCell><Stack direction="row" alignItems="center" gap={.5}><Switch size="small" checked={item.status === 1} onChange={() => openEdit({ ...item, status: item.status === 1 ? 0 : 1 })} /><Typography fontSize={10}>{item.status === 1 ? '正常' : '已停用'}</Typography></Stack></TableCell><TableCell align="right"><Tooltip title="房间赔率与返水"><IconButton size="small" onClick={() => void openRoomTrading(item)}><TuneRounded fontSize="small" /></IconButton></Tooltip><Tooltip title="编辑账号与房间"><IconButton size="small" onClick={() => openEdit(item)}><EditRounded fontSize="small" /></IconButton></Tooltip><Tooltip title="重置登录密码"><IconButton size="small" onClick={() => { setResetting(item); setNewPassword('') }}><KeyRounded fontSize="small" /></IconButton></Tooltip></TableCell></TableRow>)}
      {!loading && !items.length && <TableRow><TableCell colSpan={9} align="center" sx={{ py: 7, color: 'text.secondary' }}>暂无代理。可新建代理账号，或将用户管理中的已有用户转为代理。</TableCell></TableRow>}
    </TableBody></Table></TableContainer><TablePagination component="div" count={data?.total ?? 0} page={page} onPageChange={(_, next) => setPage(next)} rowsPerPage={pageSize} onRowsPerPageChange={event => { setPageSize(Number(event.target.value)); setPage(0) }} rowsPerPageOptions={[10, 20, 50]} labelRowsPerPage="每页" /></Card>
    <Dialog open={createOpen || Boolean(editing)} onClose={() => !saving && (setCreateOpen(false), setEditing(null))} fullWidth maxWidth="sm"><DialogTitle>{editing ? `编辑代理 · ${editing.username}` : '新增代理账号'}</DialogTitle><DialogContent><Alert severity="info" sx={{ mt: 1, mb: 2 }}>{editing ? '房间名称由代理使用，也可由总管理员代为修改；房间号仍是用户进入房间的唯一编号。' : '创建后可直接使用“登录账号 + 初始密码”登录会员端，房间号立即开通。'}</Alert><Stack gap={1.5}>{!editing && <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.5}><TextField required fullWidth label="登录账号" value={form.username} onChange={event => setForm(current => ({ ...current, username: event.target.value }))} helperText="3–50 个字符，不可重复" /><TextField required fullWidth type="password" label="初始登录密码" value={form.password} onChange={event => setForm(current => ({ ...current, password: event.target.value }))} helperText="8–72 个字符" /></Stack>}<TextField select fullWidth label="所属租户" value={form.tenant_id} onChange={event => setForm(current => ({ ...current, tenant_id: Number(event.target.value) }))} helperText="未分配租户的代理只有总管理员可管理"><MenuItem value={0}>暂不分配</MenuItem>{tenants.map(tenant => <MenuItem key={tenant.id} value={tenant.id}>{tenant.nickname || tenant.username} · @{tenant.username}</MenuItem>)}</TextField><Stack direction={{ xs: 'column', sm: 'row' }} gap={1.5}><TextField fullWidth label="代理昵称" value={form.nickname} onChange={event => setForm(current => ({ ...current, nickname: event.target.value }))} /><TextField fullWidth label="房间名称" value={form.room_name} onChange={event => setForm(current => ({ ...current, room_name: event.target.value }))} helperText="代理可在自己的工作台再次修改" inputProps={{ maxLength: 30 }} /></Stack><TextField required fullWidth label="房间号" value={form.room_code} onChange={event => setForm(current => ({ ...current, room_code: event.target.value.replace(/\D/g, '') }))} helperText="4–12 位数字，必须唯一" /><Stack direction={{ xs: 'column', sm: 'row' }} gap={1.5}><TextField fullWidth label="手机号" value={form.phone} onChange={event => setForm(current => ({ ...current, phone: event.target.value }))} /><TextField fullWidth type="email" label="邮箱" value={form.email} onChange={event => setForm(current => ({ ...current, email: event.target.value }))} /></Stack><Stack direction={{ xs: 'column', sm: 'row' }} gap={1.5}><TextField fullWidth type="number" label="房间默认返水比例 %" value={form.rebate_rate} onChange={event => setForm(current => ({ ...current, rebate_rate: Number(event.target.value) }))} inputProps={{ min: 0, max: 100, step: 0.01 }} /><TextField fullWidth type="number" label="代理利润分成比例 %" value={form.profit_share_rate} onChange={event => setForm(current => ({ ...current, profit_share_rate: Number(event.target.value) }))} inputProps={{ min: 0, max: 100, step: 0.01 }} helperText="按每注实际毛利（投注－派彩）结算" /></Stack><TextField label="运营备注" multiline minRows={2} value={form.remark} onChange={event => setForm(current => ({ ...current, remark: event.target.value }))} /><Stack direction="row" alignItems="center" gap={1}><Switch checked={form.status === 1} onChange={(_, checked) => setForm(current => ({ ...current, status: checked ? 1 : 0 }))} /><Typography fontSize={13}>{form.status === 1 ? '账号正常启用' : '账号创建后停用'}</Typography></Stack></Stack></DialogContent><DialogActions><Button disabled={saving} onClick={() => { setCreateOpen(false); setEditing(null) }}>取消</Button><Button variant="contained" disabled={saving} onClick={() => void (editing ? saveEdit() : saveNew())}>{saving ? '保存中…' : '保存并开通'}</Button></DialogActions></Dialog>
    <Dialog open={Boolean(tradingAgent)} onClose={() => !tradingSaving && setTradingAgent(null)} fullWidth maxWidth="md"><DialogTitle>房间赔率与返水 · {tradingAgent?.room_code}</DialogTitle><DialogContent>{!trading ? <Box py={6} textAlign="center"><CircularProgress size={24} /></Box> : <Stack gap={1.5} mt={1}><Alert severity="info">本页设置整个房间的默认值；会员若有单独配置，将优先使用会员配置。</Alert><Stack direction={{ xs: 'column', sm: 'row' }} gap={1}><TextField fullWidth type="number" label="房间默认返水 %" value={trading.rebate_rate} onChange={event => setTrading(current => current ? { ...current, rebate_rate: Number(event.target.value) } : current)} inputProps={{ min: 0, max: 100, step: 0.01 }} /><TextField select fullWidth label="彩种赔率" value={trading.game_id} onChange={event => void loadRoomTrading(event.target.value)}>{games.map(game => <MenuItem key={game.id} value={game.id}>{game.name}</MenuItem>)}{!games.some(game => game.id === trading.game_id) && <MenuItem value={trading.game_id}>{trading.game_name}</MenuItem>}</TextField></Stack><TableContainer sx={{ maxHeight: 430 }}><Table size="small" stickyHeader><TableHead><TableRow><TableCell>玩法</TableCell><TableCell align="right">平台赔率</TableCell><TableCell align="right">房间赔率</TableCell></TableRow></TableHead><TableBody>{trading.odds.map((item, index) => <TableRow key={item.play_code}><TableCell><Typography fontSize={11} fontWeight={700}>{item.play_name}</Typography><Typography fontSize={9} color="text.secondary">{item.play_code}</Typography></TableCell><TableCell align="right">{item.base_odds}</TableCell><TableCell align="right"><TextField size="small" type="number" placeholder="继承平台" value={item.has_override ? (item.override ?? '') : ''} onChange={event => { const raw = event.target.value; setTrading(current => { if (!current) return current; const odds = [...current.odds]; odds[index] = !raw.trim() ? { ...odds[index], override: null, has_override: false, effective: odds[index].base_odds } : { ...odds[index], override: Number(raw), has_override: true, effective: Number(raw) }; return { ...current, odds } }) }} sx={{ width: 120 }} inputProps={{ min: 1.001, step: 0.001 }} /></TableCell></TableRow>)}</TableBody></Table></TableContainer></Stack>}</DialogContent><DialogActions><Button onClick={() => setTradingAgent(null)}>取消</Button><Button variant="contained" disabled={!trading || tradingSaving} onClick={() => void saveRoomTrading()}>{tradingSaving ? '保存中…' : '保存房间配置'}</Button></DialogActions></Dialog>
    <Dialog open={promoteOpen} onClose={() => !saving && setPromoteOpen(false)} fullWidth maxWidth="xs"><DialogTitle>已有用户转为代理</DialogTitle><DialogContent><Alert severity="info" sx={{ mt: 1, mb: 2 }}>输入用户管理里的内部用户 ID。转换后，该用户可继续用原登录账号和密码进入会员端。</Alert><Stack gap={1.5}><TextField required type="number" label="已有用户 ID" value={promote.user_id} onChange={event => setPromote(current => ({ ...current, user_id: event.target.value }))} /><TextField required label="分配房间号" value={promote.room_code} onChange={event => setPromote(current => ({ ...current, room_code: event.target.value.replace(/\D/g, '') }))} helperText="4–12 位数字且不可与其他代理重复" /></Stack></DialogContent><DialogActions><Button onClick={() => setPromoteOpen(false)}>取消</Button><Button variant="contained" disabled={saving} onClick={() => void promoteExisting()}>{saving ? '处理中…' : '确认转换'}</Button></DialogActions></Dialog>
    <Dialog open={Boolean(resetting)} onClose={() => !saving && setResetting(null)} fullWidth maxWidth="xs"><DialogTitle>重置代理登录密码</DialogTitle><DialogContent><Typography variant="body2" color="text.secondary" sx={{ mt: 1, mb: 2 }}>将为 @{resetting?.username} 设置新的会员端登录密码。</Typography><TextField autoFocus fullWidth type="password" label="新登录密码" value={newPassword} onChange={event => setNewPassword(event.target.value)} helperText="8–72 个字符" /></DialogContent><DialogActions><Button onClick={() => setResetting(null)}>取消</Button><Button variant="contained" disabled={saving} onClick={() => void resetPassword()}>{saving ? '重置中…' : '确认重置'}</Button></DialogActions></Dialog>
  </Box>
}
