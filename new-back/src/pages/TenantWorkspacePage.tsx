import { AddRounded, DeleteOutlineRounded, EditRounded, KeyRounded, PhotoCameraRounded } from '@mui/icons-material'
import {
  Alert, Avatar, Box, Button, Card, Chip, Dialog, DialogActions, DialogContent, DialogTitle, Divider,
  Stack, Switch, Table, TableBody, TableCell, TableContainer, TableHead, TablePagination, TableRow, TextField, Typography,
} from '@mui/material'
import { useCallback, useEffect, useRef, useState, type ChangeEvent } from 'react'
import { tenantApi, type AgentItem, type TenantDashboard } from '../api'
import { useFeedback } from '../components/feedback'
import { prepareRoomLogo } from '../utils/roomLogo'
import { WorkspaceAdminAccountFields, WorkspaceAdminCreatedDialog } from '../components/WorkspaceAdminAccount'
import { createdWorkspaceAdmin, validateWorkspaceAdminAccount, type CreatedWorkspaceAdmin } from '../utils/workspaceAdminAccount'
import { utf8ByteLength } from '../loginLimits'

const blank = { username: '', password: '', email: '', nickname: '', phone: '', room_code: '', room_name: '', room_logo: '', rebate_rate: 0, profit_share_rate: 0, robot_quota: 0, remark: '', status: 1 }
const validRoomCode = (value: string) => /^\d{5,12}$/.test(value.trim())

type TenantSection = 'dashboard' | 'agents'

export function TenantWorkspacePage({ section }: { section: TenantSection }) {
  const { showMessage } = useFeedback()
  const [dashboard, setDashboard] = useState<TenantDashboard | null>(null)
  const [agents, setAgents] = useState<AgentItem[]>([])
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState<AgentItem | null | undefined>(undefined)
  const [form, setForm] = useState(blank)
  const [passwordTarget, setPasswordTarget] = useState<AgentItem | null>(null)
  const [password, setPassword] = useState('')
  const [passwordError, setPasswordError] = useState('')
  const [resettingPassword, setResettingPassword] = useState(false)
  const [createdAdmin, setCreatedAdmin] = useState<CreatedWorkspaceAdmin | null>(null)
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')
  const savePending = useRef(false)
  const resetPending = useRef(false)
  const loadGeneration = useRef(0)

  const load = useCallback(async () => {
    const generation = ++loadGeneration.current
    setLoading(true)
    try {
      const [summary, agentResult] = await Promise.all([
        tenantApi.dashboard(),
        tenantApi.agents({ query: section === 'agents' ? query : '', page: section === 'agents' ? page + 1 : 1, pageSize }),
      ])
      if (generation !== loadGeneration.current) return
      setDashboard(summary)
      setAgents(agentResult.items)
      setTotal(agentResult.total)
      setError('')
    } catch (reason) {
      if (generation !== loadGeneration.current) return
      setError(reason instanceof Error ? reason.message : '读取租户数据失败')
    } finally { if (generation === loadGeneration.current) setLoading(false) }
  }, [query, section, page, pageSize])

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), section === 'agents' ? 180 : 0)
    return () => { window.clearTimeout(timer); loadGeneration.current += 1 }
  }, [load, section])

  const open = (row?: AgentItem) => {
    setFormError('')
    setEditing(row ?? null)
    setForm(row ? {
      username: row.username, password: '', email: row.email, nickname: row.nickname, phone: row.phone,
      room_code: row.room_code, room_name: row.room_name ?? '', room_logo: row.room_logo ?? '', rebate_rate: row.rebate_rate,
      profit_share_rate: row.profit_share_rate, robot_quota: row.robot_quota ?? 0, remark: row.remark, status: row.status,
    } : blank)
  }
  const closeForm = () => {
    if (savePending.current) return
    setEditing(undefined); setForm(blank); setFormError('')
  }
  const chooseRoomLogo = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    try {
      const roomLogo = await prepareRoomLogo(file)
      setForm(current => ({ ...current, room_logo: roomLogo }))
    }
    catch (reason) { showMessage(reason instanceof Error ? reason.message : '处理房间 Logo 失败', 'error') }
  }
  const save = async () => {
    if (savePending.current) return
    const validationError = editing ? '' : validateWorkspaceAdminAccount(form.username, form.password)
    if (validationError || !validRoomCode(form.room_code)) {
      setFormError(validationError || '房间号须为 5–12 位数字')
      return
    }
    savePending.current = true
    setSaving(true); setFormError('')
    try {
      const payload = { ...form, username: form.username.trim(), room_code: form.room_code.trim() }
      if (editing) await tenantApi.updateAgent(editing.id, payload)
      else {
        const account = await tenantApi.createAgent(payload)
        setCreatedAdmin(createdWorkspaceAdmin('agent', account))
      }
      showMessage(editing ? '代理账号资料已保存' : '代理账号已创建')
      setEditing(undefined)
      setForm(blank)
      await load()
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : '保存失败')
    } finally { savePending.current = false; setSaving(false) }
  }

  const openPasswordReset = (row: AgentItem) => {
    if (resetPending.current) return
    setPasswordTarget(row); setPassword(''); setPasswordError('')
  }
  const closePasswordReset = () => {
    if (resetPending.current) return
    setPasswordTarget(null); setPassword(''); setPasswordError('')
  }
  const resetPassword = async () => {
    if (!passwordTarget || resetPending.current) return
    if (Array.from(password).length < 8 || utf8ByteLength(password) > 72) {
      setPasswordError('新密码至少 8 个字符，且不超过 72 字节')
      return
    }
    resetPending.current = true
    setResettingPassword(true); setPasswordError('')
    try {
      await tenantApi.resetAgentPassword(passwordTarget.id, password)
      showMessage(`代理账号 @${passwordTarget.username} 的密码已重置`)
      setPasswordTarget(null); setPassword('')
    } catch (reason) {
      setPasswordError(reason instanceof Error ? reason.message : '重置密码失败')
    } finally { resetPending.current = false; setResettingPassword(false) }
  }

  return <Box p={{ xs: 1.5, md: 2.5 }}>
    {section === 'agents' && <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" alignItems={{ sm: 'center' }} gap={1.5} mb={1.5}>
      <Box><Typography variant="h6" fontWeight={850}>代理账号管理</Typography><Typography variant="body2" color="text.secondary">管理下级代理的账号资料、登录密码与分成设置，关联房间资料可在编辑时维护。</Typography></Box>
      <Button variant="contained" startIcon={<AddRounded />} onClick={() => open()} sx={{ flexShrink: 0 }}>开通代理账号</Button>
    </Stack>}
    {error && <Alert severity="error" sx={{ mb: 2 }} action={<Button onClick={() => void load()}>重试</Button>}>{error}</Alert>}

    {section === 'dashboard' && <>
      <Box display="grid" gridTemplateColumns={{ xs: '1fr', sm: 'repeat(3,1fr)' }} gap={1.5}>{[
        ['代理账号总数', dashboard?.agent_count ?? 0], ['正常代理账号', dashboard?.active_agent_count ?? 0], ['所属会员', dashboard?.member_count ?? 0],
      ].map(([label, value]) => <Card key={label}><Box p={2}><Typography color="text.secondary" variant="body2">{label}</Typography><Typography variant="h4" fontWeight={850}>{value}</Typography></Box></Card>)}</Box>
      <Card sx={{ mt: 1.5 }}><Box p={2}>
        <Typography fontWeight={850} mb={1}>下级代理</Typography>
        <Stack direction="row" flexWrap="wrap" gap={1}>{agents.map(row => <Chip key={row.id} color={row.status === 1 ? 'primary' : 'default'} variant="outlined" label={`${row.nickname || row.username} · @${row.username}`} />)}</Stack>
        {!loading && !agents.length && <Typography color="text.secondary">还没有代理账号，请到“代理管理”开通。</Typography>}
        {total > agents.length && <Typography variant="body2" color="text.secondary" mt={1}>展示最近 {agents.length} 个代理账号；全部 {total} 个账号可在“代理管理”查看。</Typography>}
      </Box></Card>
    </>}

    {section === 'agents' && <Card>
      <Box p={1.5}><TextField fullWidth size="small" placeholder="搜索代理账号、昵称、手机号或房间号" value={query} onChange={event => { setQuery(event.target.value); setPage(0); setLoading(true) }} /></Box>
      <TableContainer><Table size="small" sx={{ minWidth: 1100 }} aria-label="代理账号列表"><TableHead><TableRow>
        <TableCell>代理账号</TableCell><TableCell>联系方式</TableCell><TableCell>账号状态</TableCell><TableCell>上次登录</TableCell><TableCell>分成率</TableCell><TableCell>机器人名额</TableCell><TableCell>关联房间</TableCell><TableCell align="right">账号操作</TableCell>
      </TableRow></TableHead><TableBody>
        {agents.map(row => <TableRow key={row.id} hover>
          <TableCell sx={{ minWidth: 170 }}><Typography fontWeight={850}>{row.nickname || row.username}</Typography><Typography variant="body2">@{row.username}</Typography><Typography variant="caption" color="text.secondary">公开 ID {row.public_id || '未分配'}</Typography></TableCell>
          <TableCell><Typography variant="body2">{row.phone || '未填写手机号'}</Typography><Typography variant="caption" color="text.secondary" sx={{ overflowWrap: 'anywhere' }}>{row.email || '未填写邮箱'}</Typography></TableCell>
          <TableCell><Chip size="small" color={row.status === 1 ? 'success' : 'default'} label={row.status === 1 ? '正常' : '停用'} /></TableCell>
          <TableCell><Typography variant="body2" sx={{ whiteSpace: 'nowrap' }}>{row.last_login_at || '尚未登录'}</Typography><Typography variant="caption" color="text.secondary">累计登录 {row.login_count ?? 0} 次</Typography></TableCell>
          <TableCell><Typography fontWeight={750}>{row.profit_share_rate}%</Typography></TableCell>
          <TableCell><Chip size="small" color={row.robot_quota ? 'primary' : 'default'} variant="outlined" label={`${row.robot_quota ?? 0}/10`} /></TableCell>
          <TableCell><Stack direction="row" alignItems="center" gap={1}><Avatar src={row.room_logo || undefined} variant="rounded" sx={{ width: 30, height: 30, fontSize: 12 }}>{(row.room_name || '房').slice(0, 1)}</Avatar><Box><Typography variant="body2" color="text.secondary">{row.room_name || '未命名房间'}</Typography><Typography variant="caption" color="text.secondary">房间号 {row.room_code || '未分配'}</Typography></Box></Stack><Typography variant="caption" color="text.secondary">{row.member_count} 位会员 · 返水 {row.rebate_rate}%</Typography></TableCell>
          <TableCell align="right"><Stack direction="row" justifyContent="flex-end" gap={.5} sx={{ whiteSpace: 'nowrap' }}><Button size="small" variant="outlined" startIcon={<EditRounded />} onClick={() => open(row)}>编辑账号</Button><Button size="small" startIcon={<KeyRounded />} onClick={() => openPasswordReset(row)}>重置密码</Button></Stack></TableCell>
        </TableRow>)}
        {!agents.length && <TableRow><TableCell colSpan={8} align="center" sx={{ py: 6, color: 'text.secondary' }}>{loading ? '正在读取代理账号…' : query.trim() ? '没有找到匹配的代理账号' : '还没有代理账号，点击“开通代理账号”开始。'}</TableCell></TableRow>}
      </TableBody></Table></TableContainer>
      <TablePagination component="div" count={total} page={page} onPageChange={(_, next) => { setPage(next); setLoading(true) }} rowsPerPage={pageSize} onRowsPerPageChange={event => { setPageSize(Number(event.target.value)); setPage(0); setLoading(true) }} rowsPerPageOptions={[20, 50, 100]} labelRowsPerPage="每页账号" />
    </Card>}

    <Dialog open={editing !== undefined} onClose={closeForm} fullWidth maxWidth="sm"><DialogTitle>{editing ? `编辑代理账号 · @${editing.username}` : '开通代理账号'}</DialogTitle><DialogContent>
      {formError && <Alert severity="error" sx={{ my: 1.5 }}>{formError}</Alert>}
      <Stack gap={1.5} pt={1}>
        <Typography fontWeight={800}>账号资料</Typography>
        {editing && <Typography variant="body2" color="text.secondary">公开 ID {editing.public_id || '未分配'} · 登录账号不可修改；密码请在列表使用“重置密码”。</Typography>}
        <WorkspaceAdminAccountFields role="agent" username={form.username} password={form.password} editing={Boolean(editing)} disabled={saving} onUsernameChange={username => setForm(current => ({ ...current, username }))} onPasswordChange={password => setForm(current => ({ ...current, password }))} />
        <TextField fullWidth label="代理昵称" value={form.nickname} disabled={saving} onChange={event => setForm(current => ({ ...current, nickname: event.target.value }))} />
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.2}>
          <TextField fullWidth label="手机号" value={form.phone} disabled={saving} onChange={event => setForm(current => ({ ...current, phone: event.target.value }))} />
          <TextField fullWidth label="邮箱" value={form.email} disabled={saving} onChange={event => setForm(current => ({ ...current, email: event.target.value }))} />
        </Stack>
        <TextField label="备注" value={form.remark} disabled={saving} onChange={event => setForm(current => ({ ...current, remark: event.target.value }))} />
        <Stack direction="row" justifyContent="space-between" alignItems="center"><Typography>启用代理账号及关联房间</Typography><Switch checked={form.status === 1} disabled={saving} inputProps={{ 'aria-label': '启用代理账号及关联房间' }} onChange={event => setForm(current => ({ ...current, status: event.target.checked ? 1 : 0 }))} /></Stack>
        <Divider />
        <Typography fontWeight={800}>分成与机器人</Typography>
        <TextField fullWidth type="number" label="代理分成 %" value={form.profit_share_rate} disabled={saving} onChange={event => setForm(current => ({ ...current, profit_share_rate: Number(event.target.value) }))} helperText="逐注正毛利 × 比例，亏损注不抵扣；手动结算" />
        <TextField fullWidth type="number" label="机器人名额" value={form.robot_quota} disabled={saving} onChange={event => setForm(current => ({ ...current, robot_quota: Math.max(0, Math.min(10, Number(event.target.value))) }))} slotProps={{ htmlInput: { min: 0, max: 10, step: 1 } }} helperText="由租户分配给该代理，可设 0–10 个；减少名额会自动停用超出部分" />
        <Divider />
        <Box><Typography fontWeight={800}>关联房间资料</Typography><Typography variant="body2" color="text.secondary">房间号用于进入房间，与代理登录账号不同。</Typography></Box>
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.2}>
          <TextField required fullWidth label="房间号" value={form.room_code} disabled={saving} onChange={event => setForm(current => ({ ...current, room_code: event.target.value.replace(/\D/g, '') }))} helperText="5–12 位数字，全平台唯一" />
          <TextField fullWidth label="房间名称" value={form.room_name} disabled={saving} onChange={event => setForm(current => ({ ...current, room_name: event.target.value }))} inputProps={{ maxLength: 30 }} />
        </Stack>
        <Stack direction="row" alignItems="center" gap={1.3}>
          <Avatar src={form.room_logo || undefined} variant="rounded" sx={{ width: 58, height: 58, bgcolor: 'primary.main', fontWeight: 900 }}>{(form.room_name || '房').slice(0, 1)}</Avatar>
          <Button component="label" variant="outlined" disabled={saving} startIcon={<PhotoCameraRounded />}>{form.room_logo ? '更换 Logo' : '选择 Logo'}<input hidden type="file" accept="image/png,image/jpeg,image/webp" disabled={saving} onChange={chooseRoomLogo} /></Button>
          {form.room_logo && <Button color="error" disabled={saving} startIcon={<DeleteOutlineRounded />} onClick={() => setForm(current => ({ ...current, room_logo: '' }))}>移除</Button>}
        </Stack>
        <TextField fullWidth type="number" label="房间返水 %" value={form.rebate_rate} disabled={saving} onChange={event => setForm(current => ({ ...current, rebate_rate: Number(event.target.value) }))} />
      </Stack>
    </DialogContent><DialogActions><Button disabled={saving} onClick={closeForm}>取消</Button><Button variant="contained" disabled={saving} onClick={() => void save()}>{saving ? '保存中…' : editing ? '保存修改' : '开通代理账号'}</Button></DialogActions></Dialog>
    <WorkspaceAdminCreatedDialog account={createdAdmin} onClose={() => setCreatedAdmin(null)} />
    <Dialog open={Boolean(passwordTarget)} onClose={closePasswordReset} fullWidth maxWidth="xs"><DialogTitle>重置代理账号密码</DialogTitle><DialogContent>
      {passwordTarget && <Box mb={1.5}><Typography fontWeight={800}>{passwordTarget.nickname || passwordTarget.username}</Typography><Typography variant="body2">@{passwordTarget.username}</Typography><Typography variant="caption" color="text.secondary">公开 ID {passwordTarget.public_id || '未分配'}</Typography></Box>}
      {passwordError && <Alert severity="error" sx={{ mb: 1.5 }}>{passwordError}</Alert>}
      <TextField autoFocus fullWidth type="password" label="新密码" autoComplete="new-password" value={password} disabled={resettingPassword} onChange={event => setPassword(event.target.value)} helperText="至少 8 个字符，不超过 72 字节；中文等字符占多个字节" sx={{ mt: 1 }} />
    </DialogContent><DialogActions><Button disabled={resettingPassword} onClick={closePasswordReset}>取消</Button><Button variant="contained" disabled={resettingPassword || Array.from(password).length < 8 || utf8ByteLength(password) > 72} onClick={() => void resetPassword()}>{resettingPassword ? '重置中…' : '确认重置'}</Button></DialogActions></Dialog>
  </Box>
}
