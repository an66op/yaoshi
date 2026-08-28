import { AddRounded, DeleteOutlineRounded, EditRounded, KeyRounded, PhotoCameraRounded } from '@mui/icons-material'
import {
  Alert, Avatar, Box, Button, Card, Chip, Dialog, DialogActions, DialogContent, DialogTitle,
  Stack, Switch, Table, TableBody, TableCell, TableContainer, TableHead, TableRow, TextField, Typography,
} from '@mui/material'
import { useCallback, useEffect, useState, type ChangeEvent } from 'react'
import { tenantApi, type AgentItem, type TenantDashboard } from '../api'
import { useFeedback } from '../components/feedback'
import { prepareRoomLogo } from '../utils/roomLogo'

const blank = { username: '', password: '', email: '', nickname: '', phone: '', room_code: '', room_name: '', room_logo: '', rebate_rate: 0, profit_share_rate: 0, remark: '', status: 1 }
const validRoomCode = (value: string) => /^\d{5,12}$/.test(value.trim())

type TenantSection = 'dashboard' | 'agents'

export function TenantWorkspacePage({ section }: { section: TenantSection }) {
  const { showMessage } = useFeedback()
  const [dashboard, setDashboard] = useState<TenantDashboard | null>(null)
  const [agents, setAgents] = useState<AgentItem[]>([])
  const [query, setQuery] = useState('')
  const [error, setError] = useState('')
  const [editing, setEditing] = useState<AgentItem | null | undefined>(undefined)
  const [form, setForm] = useState(blank)
  const [passwordTarget, setPasswordTarget] = useState<AgentItem | null>(null)
  const [password, setPassword] = useState('')

  const load = useCallback(async () => {
    try {
      const [summary, roomResult] = await Promise.all([
        tenantApi.dashboard(),
        tenantApi.agents({ query: section === 'agents' ? query : '', pageSize: 200 }),
      ])
      setDashboard(summary)
      setAgents(roomResult.items)
      setError('')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取租户数据失败')
    }
  }, [query, section])

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), section === 'agents' ? 180 : 0)
    return () => window.clearTimeout(timer)
  }, [load, section])

  const open = (row?: AgentItem) => {
    setEditing(row ?? null)
    setForm(row ? {
      username: row.username, password: '', email: row.email, nickname: row.nickname, phone: row.phone,
      room_code: row.room_code, room_name: row.room_name ?? '', room_logo: row.room_logo ?? '', rebate_rate: row.rebate_rate,
      profit_share_rate: row.profit_share_rate, remark: row.remark, status: row.status,
    } : blank)
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
    if (!validRoomCode(form.room_code)) {
      showMessage('房间号须为 5–12 位数字', 'error')
      return
    }
    try {
      if (editing) await tenantApi.updateAgent(editing.id, form)
      else await tenantApi.createAgent(form)
      showMessage(editing ? '房间资料已保存' : '房间已开通')
      setEditing(undefined)
      await load()
    } catch (reason) {
      showMessage(reason instanceof Error ? reason.message : '保存失败', 'error')
    }
  }

  return <Box p={{ xs: 1.5, md: 2.5 }}>
    {section === 'agents' && <Stack direction="row" justifyContent="flex-end" mb={1.5}><Button variant="contained" startIcon={<AddRounded />} onClick={() => open()}>开通房间</Button></Stack>}
    {error && <Alert severity="error" sx={{ mb: 2 }} action={<Button onClick={() => void load()}>重试</Button>}>{error}</Alert>}

    {section === 'dashboard' && <>
      <Box display="grid" gridTemplateColumns={{ xs: '1fr', sm: 'repeat(3,1fr)' }} gap={1.5}>{[
        ['房间总数', dashboard?.agent_count ?? 0], ['正常房间', dashboard?.active_agent_count ?? 0], ['所属会员', dashboard?.member_count ?? 0],
      ].map(([label, value]) => <Card key={label}><Box p={2}><Typography color="text.secondary" variant="body2">{label}</Typography><Typography variant="h4" fontWeight={850}>{value}</Typography></Box></Card>)}</Box>
      <Card sx={{ mt: 1.5 }}><Box p={2}><Typography fontWeight={850} mb={1}>我的房间</Typography><Stack direction="row" flexWrap="wrap" gap={1}>{agents.map(row => <Chip key={row.id} color={row.status === 1 ? 'primary' : 'default'} variant="outlined" label={`${row.room_name || '聊天室'} · ${row.room_code}`} />)}{!agents.length && <Typography color="text.secondary">还没有房间，请到“房间管理”开通。</Typography>}</Stack></Box></Card>
    </>}

    {section === 'agents' && <Card>
      <Box p={1.2}><TextField fullWidth size="small" placeholder="搜索房间、管理员账号或手机号" value={query} onChange={event => setQuery(event.target.value)} /></Box>
      <TableContainer><Table size="small"><TableHead><TableRow><TableCell>房间管理员</TableCell><TableCell>房间</TableCell><TableCell>会员</TableCell><TableCell>返水 / 分成</TableCell><TableCell>状态</TableCell><TableCell align="right">操作</TableCell></TableRow></TableHead><TableBody>{agents.map(row => <TableRow key={row.id} hover><TableCell><Typography fontWeight={750}>{row.nickname || row.username}</Typography><Typography variant="caption" color="text.secondary">@{row.username}</Typography></TableCell><TableCell><Stack direction="row" alignItems="center" gap={1}><Avatar src={row.room_logo || undefined} variant="rounded" sx={{ width: 36, height: 36, bgcolor: 'primary.main', fontSize: 15, fontWeight: 850 }}>{(row.room_name || '房').slice(0, 1)}</Avatar><Box><Typography fontWeight={750}>{row.room_name || '聊天室'}</Typography><Typography variant="caption" color="text.secondary">房间号 {row.room_code}</Typography></Box></Stack></TableCell><TableCell>{row.member_count}</TableCell><TableCell>{row.rebate_rate}% / {row.profit_share_rate}%</TableCell><TableCell><Chip size="small" color={row.status === 1 ? 'success' : 'default'} label={row.status === 1 ? '正常' : '停用'} /></TableCell><TableCell align="right"><Button size="small" startIcon={<EditRounded />} onClick={() => open(row)}>编辑</Button><Button size="small" startIcon={<KeyRounded />} onClick={() => { setPasswordTarget(row); setPassword('') }}>密码</Button></TableCell></TableRow>)}</TableBody></Table></TableContainer>
    </Card>}

    <Dialog open={editing !== undefined} onClose={() => setEditing(undefined)} fullWidth maxWidth="sm"><DialogTitle>{editing ? `编辑房间 · ${editing.room_code}` : '开通新房间'}</DialogTitle><DialogContent><Stack gap={1.4} pt={1}><Stack direction={{ xs: 'column', sm: 'row' }} gap={1.2}><TextField fullWidth label="房间管理员账号" disabled={Boolean(editing)} value={form.username} onChange={event => setForm(current => ({ ...current, username: event.target.value }))} /><TextField fullWidth label="房间号" value={form.room_code} onChange={event => setForm(current => ({ ...current, room_code: event.target.value.replace(/\D/g, '') }))} helperText="5–12 位数字，全平台唯一" /></Stack>{!editing && <TextField label="初始密码" type="password" helperText="8–72 个字符" value={form.password} onChange={event => setForm(current => ({ ...current, password: event.target.value }))} />}<Stack direction={{ xs: 'column', sm: 'row' }} gap={1.2}><TextField fullWidth label="管理员名称" value={form.nickname} onChange={event => setForm(current => ({ ...current, nickname: event.target.value }))} /><TextField fullWidth label="房间名称" value={form.room_name} onChange={event => setForm(current => ({ ...current, room_name: event.target.value }))} inputProps={{ maxLength: 30 }} /></Stack><Stack direction="row" alignItems="center" gap={1.3}><Avatar src={form.room_logo || undefined} variant="rounded" sx={{ width: 58, height: 58, bgcolor: 'primary.main', fontWeight: 900 }}>{(form.room_name || '房').slice(0, 1)}</Avatar><Button component="label" variant="outlined" startIcon={<PhotoCameraRounded />}>{form.room_logo ? '更换 Logo' : '选择 Logo'}<input hidden type="file" accept="image/png,image/jpeg,image/webp" onChange={chooseRoomLogo} /></Button>{form.room_logo && <Button color="error" startIcon={<DeleteOutlineRounded />} onClick={() => setForm(current => ({ ...current, room_logo: '' }))}>移除</Button>}</Stack><TextField fullWidth label="手机号" value={form.phone} onChange={event => setForm(current => ({ ...current, phone: event.target.value }))} /><Stack direction={{ xs: 'column', sm: 'row' }} gap={1.2}><TextField fullWidth type="number" label="房间返水 %" value={form.rebate_rate} onChange={event => setForm(current => ({ ...current, rebate_rate: Number(event.target.value) }))} /><TextField fullWidth type="number" label="管理员分成 %" value={form.profit_share_rate} onChange={event => setForm(current => ({ ...current, profit_share_rate: Number(event.target.value) }))} /></Stack><TextField label="邮箱" value={form.email} onChange={event => setForm(current => ({ ...current, email: event.target.value }))} /><TextField label="备注" value={form.remark} onChange={event => setForm(current => ({ ...current, remark: event.target.value }))} /><Stack direction="row" justifyContent="space-between" alignItems="center"><Typography>启用房间</Typography><Switch checked={form.status === 1} onChange={event => setForm(current => ({ ...current, status: event.target.checked ? 1 : 0 }))} /></Stack></Stack></DialogContent><DialogActions><Button onClick={() => setEditing(undefined)}>取消</Button><Button variant="contained" disabled={!form.username.trim() || !validRoomCode(form.room_code) || (!editing && form.password.length < 8)} onClick={() => void save()}>保存</Button></DialogActions></Dialog>
    <Dialog open={Boolean(passwordTarget)} onClose={() => setPasswordTarget(null)} fullWidth maxWidth="xs"><DialogTitle>重置房间管理员密码</DialogTitle><DialogContent><TextField autoFocus fullWidth type="password" label="新密码" value={password} onChange={event => setPassword(event.target.value)} sx={{ mt: 1 }} /></DialogContent><DialogActions><Button onClick={() => setPasswordTarget(null)}>取消</Button><Button variant="contained" disabled={password.length < 8} onClick={() => { if (!passwordTarget) return; void tenantApi.resetAgentPassword(passwordTarget.id, password).then(() => { showMessage('密码已重置'); setPasswordTarget(null) }).catch(reason => showMessage(reason instanceof Error ? reason.message : '重置失败', 'error')) }}>确认</Button></DialogActions></Dialog>
  </Box>
}
