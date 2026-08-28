import { AccountBalanceWalletRounded, AddRounded, EditRounded, KeyRounded, PhotoCameraRounded, SettingsSuggestRounded, TuneRounded } from '@mui/icons-material'
import { Alert, Avatar, Box, Button, Card, Chip, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, MenuItem, Stack, Switch, Table, TableBody, TableCell, TableContainer, TableHead, TableRow, TextField, Typography } from '@mui/material'
import { useCallback, useEffect, useState } from 'react'
import { adminApi, type AdminGame, type RoomTradingConfig, type TenantItem } from '../api'
import { useFeedback } from '../components/feedback'
import { RoomOperationsDialog } from '../components/RoomOperationsDialog'
import { prepareRoomLogo } from '../utils/roomLogo'

const emptyForm = { username: '', password: '', email: '', nickname: '', phone: '', room_code: '', room_name: '', room_logo: '', remark: '', status: 1 }
const money = (value: number) => new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)
const validRoomCode = (value: string) => /^\d{5,12}$/.test(value.trim())

export function TenantsPage() {
  const { showMessage } = useFeedback()
  const [items, setItems] = useState<TenantItem[]>([])
  const [summary, setSummary] = useState({ total: 0, active: 0, agents: 0, members: 0 })
  const [query, setQuery] = useState('')
  const [error, setError] = useState('')
  const [editing, setEditing] = useState<TenantItem | null | undefined>(undefined)
  const [form, setForm] = useState(emptyForm)
  const [passwordTarget, setPasswordTarget] = useState<TenantItem | null>(null)
  const [password, setPassword] = useState('')
	const [tradingTenant, setTradingTenant] = useState<TenantItem | null>(null)
	const [trading, setTrading] = useState<RoomTradingConfig | null>(null)
	const [games, setGames] = useState<AdminGame[]>([])
	const [tradingSaving, setTradingSaving] = useState(false)
	const [operationsTenant, setOperationsTenant] = useState<TenantItem | null>(null)
	const [balanceTenant, setBalanceTenant] = useState<TenantItem | null>(null)
	const [balanceForm, setBalanceForm] = useState({ amount: '', remark: '直属房间红包及运营备用金' })

  const load = useCallback(async () => {
    try { const result = await adminApi.tenants({ query }); setItems(Array.isArray(result?.items) ? result.items : []); setSummary({ total: result?.total ?? 0, active: result?.active ?? 0, agents: result?.agents ?? 0, members: result?.members ?? 0 }); setError('') }
    catch (reason) { setError(reason instanceof Error ? reason.message : '读取租户失败') }
  }, [query])
  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])

  const open = (row?: TenantItem) => {
    setEditing(row ?? null)
    setForm(row ? { username: row.username, password: '', email: row.email, nickname: row.nickname, phone: row.phone, room_code: row.room_code, room_name: row.room_name, room_logo: row.room_logo, remark: row.remark, status: row.status } : emptyForm)
  }
  const save = async () => {
    if (!validRoomCode(form.room_code)) {
      showMessage('房间号须为 5–12 位数字', 'error')
      return
    }
    try {
      if (editing) await adminApi.updateTenant(editing.id, form)
      else await adminApi.createTenant(form)
      showMessage(editing ? '租户资料已保存' : '租户账号已创建'); setEditing(undefined); await load()
    } catch (reason) { showMessage(reason instanceof Error ? reason.message : '保存失败', 'error') }
  }
	const openTrading = async (tenant: TenantItem) => {
		setTradingTenant(tenant); setTrading(null); setError('')
		try {
			const [roomTrading, allGames] = await Promise.all([adminApi.tenantRoomTrading(tenant.id), adminApi.games()])
			setTrading(roomTrading); setGames(allGames)
		} catch (reason) { setError(reason instanceof Error ? reason.message : '读取直属房间赔率失败') }
	}
	const loadTrading = async (gameId: string) => {
		if (!tradingTenant) return
		try { setTrading(await adminApi.tenantRoomTrading(tradingTenant.id, gameId)) }
		catch (reason) { setError(reason instanceof Error ? reason.message : '读取直属房间赔率失败') }
	}
	const saveTrading = async () => {
		if (!tradingTenant || !trading) return
		setTradingSaving(true)
		try {
			setTrading(await adminApi.updateTenantRoomTrading(tradingTenant.id, { rebate_rate: trading.rebate_rate, game_id: trading.game_id, odds: trading.odds.map(item => ({ play_code: item.play_code, override: item.has_override ? item.override : null })) }))
			showMessage(`租户直属房间 ${tradingTenant.room_code} 的赔率与返水已保存`)
		} catch (reason) { showMessage(reason instanceof Error ? reason.message : '保存失败', 'error') }
		finally { setTradingSaving(false) }
	}
	const adjustRoomBalance = async () => {
		if (!balanceTenant) return
		const amount = Number(balanceForm.amount)
		if (!Number.isFinite(amount) || amount === 0 || !balanceForm.remark.trim()) { setError('请填写非零调整金额和原因'); return }
		try {
			await adminApi.adjustUserBalance(balanceTenant.id, amount, balanceForm.remark.trim())
			showMessage(amount > 0 ? '直属房间运营余额已补充' : '直属房间运营余额已扣减')
			setBalanceTenant(null); setBalanceForm({ amount: '', remark: '直属房间红包及运营备用金' }); await load()
		} catch (reason) { setError(reason instanceof Error ? reason.message : '调整房间余额失败') }
	}

  return <Box p={{ xs: 1.5, md: 2.5 }}>
    <Stack direction="row" justifyContent="flex-end" mb={1.5}>
      <Button variant="contained" startIcon={<AddRounded />} onClick={() => open()}>新增租户</Button>
    </Stack>
    <Box display="grid" gridTemplateColumns={{ xs: '1fr 1fr', md: 'repeat(4,1fr)' }} gap={1.2} mb={2}>{[['租户总数', summary.total], ['正常租户', summary.active], ['所属代理', summary.agents], ['所属会员', summary.members]].map(([label, value]) => <Card key={label}><Box p={1.7}><Typography variant="caption" color="text.secondary">{label}</Typography><Typography variant="h6" fontWeight={850}>{value}</Typography></Box></Card>)}</Box>
    {error && <Alert severity="error" sx={{ mb: 1.5 }}>{error}</Alert>}
    <Card><Box p={1.2}><TextField size="small" fullWidth placeholder="搜索租户账号、名称、邮箱或手机" value={query} onChange={event => setQuery(event.target.value)} /></Box>
      <TableContainer><Table size="small"><TableHead><TableRow><TableCell>租户</TableCell><TableCell>直属房间</TableCell><TableCell>业务规模</TableCell><TableCell align="right">运营余额</TableCell><TableCell>状态</TableCell><TableCell>最后登录</TableCell><TableCell align="right">操作</TableCell></TableRow></TableHead><TableBody>{items.map(row => <TableRow key={row.id} hover><TableCell><Typography fontWeight={750}>{row.nickname || row.username}</Typography><Typography variant="caption" color="text.secondary">@{row.username} · ID {row.public_id}</Typography></TableCell><TableCell><Stack direction="row" alignItems="center" gap={1}><Avatar src={row.room_logo || undefined} variant="rounded" sx={{ width: 34, height: 34, fontSize: 12 }}>{(row.room_name || '房').slice(0, 1)}</Avatar><Box><Typography fontWeight={750} fontSize={12}>{row.room_name || '未命名房间'}</Typography><Typography variant="caption" color="text.secondary">房间号 {row.room_code || '未分配'}</Typography></Box></Stack></TableCell><TableCell><Chip size="small" label={`${row.agent_count} 代理`} sx={{ mr: .6 }} /><Chip size="small" variant="outlined" label={`${row.member_count} 会员`} /></TableCell><TableCell align="right"><Typography fontWeight={800}>{money(row.balance)}</Typography></TableCell><TableCell><Chip size="small" color={row.status === 1 ? 'success' : 'default'} label={row.status === 1 ? '正常' : '停用'} /></TableCell><TableCell>{row.last_login_at || '尚未登录'}</TableCell><TableCell align="right"><Button size="small" startIcon={<AccountBalanceWalletRounded />} onClick={() => { setBalanceTenant(row); setBalanceForm({ amount: '', remark: '直属房间红包及运营备用金' }) }}>余额</Button><Button size="small" startIcon={<SettingsSuggestRounded />} onClick={() => setOperationsTenant(row)}>经营</Button><Button size="small" startIcon={<TuneRounded />} onClick={() => void openTrading(row)}>赔率</Button><Button size="small" startIcon={<EditRounded />} onClick={() => open(row)}>编辑</Button><Button size="small" startIcon={<KeyRounded />} onClick={() => { setPasswordTarget(row); setPassword('') }}>重置密码</Button></TableCell></TableRow>)}</TableBody></Table></TableContainer>
    </Card>
    <Dialog open={editing !== undefined} onClose={() => setEditing(undefined)} fullWidth maxWidth="sm"><DialogTitle>{editing ? `编辑租户 · ${editing.username}` : '新增租户'}</DialogTitle><DialogContent><Stack gap={1.6} pt={1}><TextField label="登录账号" disabled={Boolean(editing)} required value={form.username} onChange={event => setForm(current => ({ ...current, username: event.target.value }))} />{!editing && <TextField label="初始密码" type="password" required helperText="8–72 个字符" value={form.password} onChange={event => setForm(current => ({ ...current, password: event.target.value }))} />}<Stack direction={{ xs: 'column', sm: 'row' }} gap={1.3}><TextField fullWidth label="租户名称" value={form.nickname} onChange={event => setForm(current => ({ ...current, nickname: event.target.value }))} /><TextField fullWidth label="联系电话" value={form.phone} onChange={event => setForm(current => ({ ...current, phone: event.target.value }))} /></Stack><TextField label="邮箱" type="email" value={form.email} onChange={event => setForm(current => ({ ...current, email: event.target.value }))} /><Typography fontWeight={850}>租户直属房间</Typography><Stack direction={{ xs: 'column', sm: 'row' }} gap={1.3}><TextField fullWidth required label="公开房间号" value={form.room_code} onChange={event => setForm(current => ({ ...current, room_code: event.target.value.replace(/\D/g, '') }))} helperText="全平台唯一，5–12 位数字" /><TextField fullWidth required label="房间名称" value={form.room_name} onChange={event => setForm(current => ({ ...current, room_name: event.target.value }))} /></Stack><Stack direction="row" alignItems="center" gap={1.5}><Avatar src={form.room_logo || undefined} variant="rounded" sx={{ width: 52, height: 52 }}>{(form.room_name || '房').slice(0, 1)}</Avatar><Button component="label" variant="outlined" startIcon={<PhotoCameraRounded />}>选择房间 Logo<input hidden type="file" accept="image/png,image/jpeg,image/webp" onChange={event => { const file = event.target.files?.[0]; if (!file) return; void prepareRoomLogo(file).then(room_logo => setForm(current => ({ ...current, room_logo }))).catch(reason => showMessage(reason instanceof Error ? reason.message : '图片处理失败', 'error')); event.target.value = '' }} /></Button>{form.room_logo && <Button color="error" onClick={() => setForm(current => ({ ...current, room_logo: '' }))}>移除</Button>}</Stack><TextField label="备注" multiline minRows={2} value={form.remark} onChange={event => setForm(current => ({ ...current, remark: event.target.value }))} /><Stack direction="row" alignItems="center" justifyContent="space-between"><Typography>启用租户</Typography><Switch checked={form.status === 1} onChange={event => setForm(current => ({ ...current, status: event.target.checked ? 1 : 0 }))} /></Stack></Stack></DialogContent><DialogActions><Button onClick={() => setEditing(undefined)}>取消</Button><Button variant="contained" disabled={!form.username.trim() || !validRoomCode(form.room_code) || form.room_name.trim().length < 2 || (!editing && form.password.length < 8)} onClick={() => void save()}>保存</Button></DialogActions></Dialog>
    <Dialog open={Boolean(passwordTarget)} onClose={() => setPasswordTarget(null)} fullWidth maxWidth="xs"><DialogTitle>重置租户密码</DialogTitle><DialogContent><TextField autoFocus fullWidth type="password" label="新密码" value={password} onChange={event => setPassword(event.target.value)} sx={{ mt: 1 }} /></DialogContent><DialogActions><Button onClick={() => setPasswordTarget(null)}>取消</Button><Button variant="contained" disabled={password.length < 8} onClick={() => { if (!passwordTarget) return; void adminApi.resetTenantPassword(passwordTarget.id, password).then(() => { showMessage('密码已重置'); setPasswordTarget(null) }).catch(reason => showMessage(reason instanceof Error ? reason.message : '重置失败', 'error')) }}>确认</Button></DialogActions></Dialog>
		<Dialog open={Boolean(tradingTenant)} onClose={() => !tradingSaving && setTradingTenant(null)} fullWidth maxWidth="md"><DialogTitle>直属房间赔率与返水 · {tradingTenant?.room_code}</DialogTitle><DialogContent>{!trading ? <Box py={6} display="grid" sx={{ placeItems: 'center' }}><CircularProgress size={24} /></Box> : <Stack gap={1.5} mt={1}><Alert severity="info">平台可代为配置租户直属房间；租户登录后也能维护同一份配置。</Alert><Stack direction={{ xs: 'column', sm: 'row' }} gap={1.2}><TextField fullWidth type="number" label="房间默认返水 %" value={trading.rebate_rate} onChange={event => setTrading(current => current ? { ...current, rebate_rate: Number(event.target.value) } : current)} inputProps={{ min: 0, max: 100, step: 0.01 }} /><TextField select fullWidth label="彩种赔率" value={trading.game_id} onChange={event => void loadTrading(event.target.value)}>{games.map(game => <MenuItem key={game.id} value={game.id}>{game.name}</MenuItem>)}</TextField></Stack><TableContainer sx={{ maxHeight: 430, border: 1, borderColor: 'divider', borderRadius: 2 }}><Table size="small" stickyHeader><TableHead><TableRow><TableCell>玩法</TableCell><TableCell align="right">平台默认</TableCell><TableCell align="right">房间设置</TableCell></TableRow></TableHead><TableBody>{trading.odds.map((item, index) => <TableRow key={item.play_code}><TableCell><Typography fontSize={11} fontWeight={800}>{item.play_name}</Typography><Typography fontSize={9} color="text.secondary">{item.play_code}</Typography></TableCell><TableCell align="right">{item.base_odds}</TableCell><TableCell align="right"><TextField size="small" type="number" placeholder="继承平台" value={item.has_override ? (item.override ?? '') : ''} onChange={event => { const raw = event.target.value; setTrading(current => { if (!current) return current; const odds = [...current.odds]; odds[index] = raw.trim() === '' ? { ...odds[index], override: null, has_override: false, effective: odds[index].base_odds } : { ...odds[index], override: Number(raw), has_override: true, effective: Number(raw) }; return { ...current, odds } }) }} sx={{ width: 125 }} /></TableCell></TableRow>)}</TableBody></Table></TableContainer></Stack>}</DialogContent><DialogActions><Button onClick={() => setTradingTenant(null)}>取消</Button><Button variant="contained" disabled={!trading || tradingSaving} onClick={() => void saveTrading()}>{tradingSaving ? '保存中…' : '保存房间配置'}</Button></DialogActions></Dialog>
        <RoomOperationsDialog open={Boolean(operationsTenant)} title={`经营配置 · ${operationsTenant?.room_code || ''}`} target={operationsTenant ? { kind: 'tenant', id: operationsTenant.id } : null} onClose={() => setOperationsTenant(null)} onSaved={showMessage} />
        <Dialog open={Boolean(balanceTenant)} onClose={() => setBalanceTenant(null)} fullWidth maxWidth="xs"><DialogTitle>直属房间运营余额 · {balanceTenant?.room_code}</DialogTitle><DialogContent><Stack gap={1.5} mt={1}><Alert severity="info">红包发送时会先预留总金额；红包关闭或过期后，未领取部分自动退回该余额。</Alert><Typography fontWeight={800}>当前余额：{money(balanceTenant?.balance ?? 0)}</Typography><TextField autoFocus type="number" label="调整金额" value={balanceForm.amount} onChange={event => setBalanceForm(current => ({ ...current, amount: event.target.value }))} helperText="正数为补充，负数为扣减" /><TextField label="调整原因" value={balanceForm.remark} onChange={event => setBalanceForm(current => ({ ...current, remark: event.target.value }))} /></Stack></DialogContent><DialogActions><Button onClick={() => setBalanceTenant(null)}>取消</Button><Button variant="contained" disabled={!balanceForm.amount || Number(balanceForm.amount) === 0} onClick={() => void adjustRoomBalance()}>确认调整</Button></DialogActions></Dialog>
  </Box>
}
