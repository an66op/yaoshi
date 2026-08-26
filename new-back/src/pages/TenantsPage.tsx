import { AddRounded, EditRounded, KeyRounded } from '@mui/icons-material'
import { Alert, Box, Button, Card, Chip, Dialog, DialogActions, DialogContent, DialogTitle, Stack, Switch, Table, TableBody, TableCell, TableContainer, TableHead, TableRow, TextField, Typography } from '@mui/material'
import { useCallback, useEffect, useState } from 'react'
import { adminApi, type TenantItem } from '../api'
import { useFeedback } from '../components/feedback'

const emptyForm = { username: '', password: '', email: '', nickname: '', phone: '', remark: '', status: 1 }

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

  const load = useCallback(async () => {
    try { const result = await adminApi.tenants({ query }); setItems(result.items); setSummary({ total: result.total, active: result.active, agents: result.agents, members: result.members }); setError('') }
    catch (reason) { setError(reason instanceof Error ? reason.message : '读取租户失败') }
  }, [query])
  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])

  const open = (row?: TenantItem) => {
    setEditing(row ?? null)
    setForm(row ? { username: row.username, password: '', email: row.email, nickname: row.nickname, phone: row.phone, remark: row.remark, status: row.status } : emptyForm)
  }
  const save = async () => {
    try {
      if (editing) await adminApi.updateTenant(editing.id, form)
      else await adminApi.createTenant(form)
      showMessage(editing ? '租户资料已保存' : '租户账号已创建'); setEditing(undefined); await load()
    } catch (reason) { showMessage(reason instanceof Error ? reason.message : '保存失败', 'error') }
  }

  return <Box p={{ xs: 1.5, md: 2.5 }}>
    <Stack direction="row" justifyContent="flex-end" mb={1.5}>
      <Button variant="contained" startIcon={<AddRounded />} onClick={() => open()}>新增租户</Button>
    </Stack>
    <Box display="grid" gridTemplateColumns={{ xs: '1fr 1fr', md: 'repeat(4,1fr)' }} gap={1.2} mb={2}>{[['租户总数', summary.total], ['正常租户', summary.active], ['所属代理', summary.agents], ['所属会员', summary.members]].map(([label, value]) => <Card key={label}><Box p={1.7}><Typography variant="caption" color="text.secondary">{label}</Typography><Typography variant="h6" fontWeight={850}>{value}</Typography></Box></Card>)}</Box>
    {error && <Alert severity="error" sx={{ mb: 1.5 }}>{error}</Alert>}
    <Card><Box p={1.2}><TextField size="small" fullWidth placeholder="搜索租户账号、名称、邮箱或手机" value={query} onChange={event => setQuery(event.target.value)} /></Box>
      <TableContainer><Table size="small"><TableHead><TableRow><TableCell>租户</TableCell><TableCell>业务规模</TableCell><TableCell>状态</TableCell><TableCell>最后登录</TableCell><TableCell align="right">操作</TableCell></TableRow></TableHead><TableBody>{items.map(row => <TableRow key={row.id} hover><TableCell><Typography fontWeight={750}>{row.nickname || row.username}</Typography><Typography variant="caption" color="text.secondary">@{row.username} · ID {row.public_id}</Typography></TableCell><TableCell><Chip size="small" label={`${row.agent_count} 代理`} sx={{ mr: .6 }} /><Chip size="small" variant="outlined" label={`${row.member_count} 会员`} /></TableCell><TableCell><Chip size="small" color={row.status === 1 ? 'success' : 'default'} label={row.status === 1 ? '正常' : '停用'} /></TableCell><TableCell>{row.last_login_at || '尚未登录'}</TableCell><TableCell align="right"><Button size="small" startIcon={<EditRounded />} onClick={() => open(row)}>编辑</Button><Button size="small" startIcon={<KeyRounded />} onClick={() => { setPasswordTarget(row); setPassword('') }}>重置密码</Button></TableCell></TableRow>)}</TableBody></Table></TableContainer>
    </Card>
    <Dialog open={editing !== undefined} onClose={() => setEditing(undefined)} fullWidth maxWidth="sm"><DialogTitle>{editing ? `编辑租户 · ${editing.username}` : '新增租户'}</DialogTitle><DialogContent><Stack gap={1.6} pt={1}><TextField label="登录账号" disabled={Boolean(editing)} required value={form.username} onChange={event => setForm(current => ({ ...current, username: event.target.value }))} />{!editing && <TextField label="初始密码" type="password" required helperText="8–72 个字符" value={form.password} onChange={event => setForm(current => ({ ...current, password: event.target.value }))} />}<Stack direction={{ xs: 'column', sm: 'row' }} gap={1.3}><TextField fullWidth label="租户名称" value={form.nickname} onChange={event => setForm(current => ({ ...current, nickname: event.target.value }))} /><TextField fullWidth label="联系电话" value={form.phone} onChange={event => setForm(current => ({ ...current, phone: event.target.value }))} /></Stack><TextField label="邮箱" type="email" value={form.email} onChange={event => setForm(current => ({ ...current, email: event.target.value }))} /><TextField label="备注" multiline minRows={2} value={form.remark} onChange={event => setForm(current => ({ ...current, remark: event.target.value }))} /><Stack direction="row" alignItems="center" justifyContent="space-between"><Typography>启用租户</Typography><Switch checked={form.status === 1} onChange={event => setForm(current => ({ ...current, status: event.target.checked ? 1 : 0 }))} /></Stack></Stack></DialogContent><DialogActions><Button onClick={() => setEditing(undefined)}>取消</Button><Button variant="contained" disabled={!form.username.trim() || (!editing && form.password.length < 8)} onClick={() => void save()}>保存</Button></DialogActions></Dialog>
    <Dialog open={Boolean(passwordTarget)} onClose={() => setPasswordTarget(null)} fullWidth maxWidth="xs"><DialogTitle>重置租户密码</DialogTitle><DialogContent><TextField autoFocus fullWidth type="password" label="新密码" value={password} onChange={event => setPassword(event.target.value)} sx={{ mt: 1 }} /></DialogContent><DialogActions><Button onClick={() => setPasswordTarget(null)}>取消</Button><Button variant="contained" disabled={password.length < 8} onClick={() => { if (!passwordTarget) return; void adminApi.resetTenantPassword(passwordTarget.id, password).then(() => { showMessage('密码已重置'); setPasswordTarget(null) }).catch(reason => showMessage(reason instanceof Error ? reason.message : '重置失败', 'error')) }}>确认</Button></DialogActions></Dialog>
  </Box>
}
