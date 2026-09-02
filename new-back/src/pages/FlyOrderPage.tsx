import {
  Alert, Avatar, Box, Button, Chip, CircularProgress, Dialog, DialogActions, DialogContent,
  DialogTitle, FormControl, InputLabel, MenuItem, Paper, Select, Stack, Table, TableBody,
  TableCell, TableContainer, TableHead, TablePagination, TableRow, TextField, Typography,
} from '@mui/material'
import FlightTakeoffRounded from '@mui/icons-material/FlightTakeoffRounded'
import LinkOffRounded from '@mui/icons-material/LinkOffRounded'
import ManageAccountsRounded from '@mui/icons-material/ManageAccountsRounded'
import RestartAltRounded from '@mui/icons-material/RestartAltRounded'
import SaveRounded from '@mui/icons-material/SaveRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  adminApi, agentApi, tenantApi, type AdminUser, type UpdateUserTradingPayload,
  type UserListResponse, type UserTradingConfig, type WorkspaceMember, type WorkspaceMemberList,
} from '../api'
import { useFeedback } from '../components/feedback'

type FlyOrderRole = 'admin' | 'tenant' | 'agent'
type FlyMember = AdminUser | WorkspaceMember
const isWorkspaceMember = (member: FlyMember): member is WorkspaceMember => 'in_current_room' in member
const canConfigure = (role: FlyOrderRole, member?: FlyMember | null) => Boolean(member && (role === 'admin' || (isWorkspaceMember(member) && member.in_current_room === true && member.can_manage === true)))
const visibleMember = (role: FlyOrderRole, member: FlyMember): FlyMember => canConfigure(role, member) ? member : {
  id: member.id, public_id: member.public_id, username: member.username, nickname: member.nickname,
  avatar: member.avatar, public_title: member.public_title, badge: member.badge, role: 'member',
  in_current_room: isWorkspaceMember(member) && member.in_current_room === true,
  can_manage: false, balance: null, online: null, status: null,
}

const modeLabel: Record<string, string> = {
  inherit: '跟随房间',
  custom: '单独比例',
  off: '已停用',
}

function listMembers(role: FlyOrderRole, query: string, status: string, page: number, pageSize: number): Promise<UserListResponse | WorkspaceMemberList> {
  if (role === 'admin') return adminApi.users({ query, status, role: 'member', kind: 'member', page, pageSize })
  if (role === 'tenant') return tenantApi.users({ query, status, page, pageSize })
  return agentApi.users({ query, status, page, pageSize })
}

function getTrading(role: FlyOrderRole, userID: number) {
  if (role === 'admin') return adminApi.userTrading(userID)
  if (role === 'tenant') return tenantApi.userTrading(userID)
  return agentApi.userTrading(userID)
}

function updateTrading(role: FlyOrderRole, userID: number, payload: UpdateUserTradingPayload) {
  if (role === 'admin') return adminApi.updateUserTrading(userID, payload)
  if (role === 'tenant') return tenantApi.updateUserTrading(userID, payload)
  return agentApi.updateUserTrading(userID, payload)
}

function updateExternal(config: UserTradingConfig | null, patch: Partial<UserTradingConfig['external_follow']>) {
  return config ? { ...config, external_follow: { ...config.external_follow, ...patch } } : config
}

export function FlyOrderPage({ role }: { role: FlyOrderRole }) {
  const { showMessage } = useFeedback()
  const [members, setMembers] = useState<FlyMember[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState(20)
  const [query, setQuery] = useState('')
  const [appliedQuery, setAppliedQuery] = useState('')
  const [status, setStatus] = useState('all')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selected, setSelected] = useState<FlyMember | null>(null)
  const [config, setConfig] = useState<UserTradingConfig | null>(null)
  const [configLoading, setConfigLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [dirty, setDirty] = useState(false)
  const configRequestRef = useRef(0)
  const selectedMemberIDRef = useRef<number | null>(null)
  const savingRef = useRef(false)
  const knownMembers = useRef(new Map<number, FlyMember>())
  const memberVersions = useRef(new Map<number, number>())
  const membershipSequence = useRef(0)
  const alive = useRef(true)
  const activeRole = useRef(role)

  const invalidateDialog = useCallback(() => {
    configRequestRef.current += 1
    selectedMemberIDRef.current = null
    setSelected(null); setConfig(null); setConfigLoading(false); setDirty(false)
  }, [])
  const acceptMember = useCallback((member: FlyMember, version: number) => {
    if ((memberVersions.current.get(member.id) ?? 0) > version) return knownMembers.current.get(member.id) ?? visibleMember(role, member)
    const next = visibleMember(role, member)
    memberVersions.current.set(member.id, version)
    knownMembers.current.set(member.id, next)
    if (!canConfigure(role, next) && selectedMemberIDRef.current === next.id) invalidateDialog()
    return next
  }, [invalidateDialog, role])
  useEffect(() => {
    alive.current = true
    activeRole.current = role
    const cache = knownMembers.current
    const versions = memberVersions.current
    return () => {
      alive.current = false
      configRequestRef.current += 1
      selectedMemberIDRef.current = null
      cache.clear(); versions.clear()
    }
  }, [role])

  const revalidateMember = async (member: FlyMember) => {
    if (!canConfigure(role, knownMembers.current.get(member.id) ?? member)) return null
    if (role === 'admin') return member
    const version = ++membershipSequence.current
    const result = await (role === 'tenant' ? tenantApi : agentApi).users({ userId: member.id, page: 1, pageSize: 1 })
    if (!alive.current || activeRole.current !== role) return null
    const found = result.items.find(item => item.id === member.id)
    const next = acceptMember(found ?? {
      id: member.id, public_id: member.public_id, username: member.username, nickname: member.nickname,
      avatar: member.avatar, public_title: member.public_title, badge: member.badge, role: 'member',
      in_current_room: false, can_manage: false, balance: null, status: null, online: null,
    }, version)
    setMembers(current => current.map(item => item.id === member.id ? next : item))
    if (!canConfigure(role, next)) {
      showMessage('该会员已不在本房间或无管理权限，请刷新会员列表', 'warning')
      return null
    }
    return next
  }

  useEffect(() => {
    let cancelled = false
    const version = ++membershipSequence.current
    void listMembers(role, appliedQuery, status, page + 1, pageSize)
      .then(result => {
        if (cancelled) return
        setMembers((result.items ?? []).map(member => acceptMember(member, version)))
        setTotal(result.total ?? 0)
				setError('')
      })
      .catch(reason => {
        if (!cancelled) setError(reason instanceof Error ? reason.message : '读取会员飞单配置失败')
      })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [acceptMember, appliedQuery, page, pageSize, role, status])

  const summary = useMemo(() => ({
    custom: members.filter(item => canConfigure(role, item) && item.fly_mode === 'custom').length,
    disabled: members.filter(item => canConfigure(role, item) && item.fly_mode === 'off').length,
  }), [members, role])

  const openMember = async (member: FlyMember) => {
    if (!canConfigure(role, member) || !canConfigure(role, knownMembers.current.get(member.id) ?? member) || savingRef.current) return
    const requestID = ++configRequestRef.current
    selectedMemberIDRef.current = member.id
    setSelected(member)
    setConfig(null)
    setDirty(false)
    setConfigLoading(true)
    try {
      const current = await revalidateMember(member)
      if (!current || requestID !== configRequestRef.current || selectedMemberIDRef.current !== member.id) return
      const next = await getTrading(role, member.id)
      if (requestID !== configRequestRef.current || selectedMemberIDRef.current !== member.id || !alive.current) return
      const latest = await revalidateMember(current)
      if (!latest || requestID !== configRequestRef.current || selectedMemberIDRef.current !== member.id) return
      setSelected(latest)
      setConfig(next)
    } catch (reason) {
      if (requestID !== configRequestRef.current || selectedMemberIDRef.current !== member.id) return
      showMessage(reason instanceof Error ? reason.message : '读取会员飞单配置失败', 'error')
      setConfigLoading(false)
      selectedMemberIDRef.current = null
      setSelected(null)
    } finally {
      if (requestID === configRequestRef.current && selectedMemberIDRef.current === member.id) setConfigLoading(false)
    }
  }

  const closeDialog = () => {
    if (savingRef.current) return
    invalidateDialog()
  }

  const save = async () => {
    if (!selected || !config || !canConfigure(role, selected) || savingRef.current) return
    const memberID = selected.id
    const configSnapshot = config
    if (selectedMemberIDRef.current !== memberID) return
    if (configSnapshot.fly.mode === 'custom' && (!Number.isFinite(configSnapshot.fly.rate) || configSnapshot.fly.rate < 0 || configSnapshot.fly.rate > 100)) {
      showMessage('单独飞单比例需在 0–100% 之间', 'warning')
      return
    }
    const external = configSnapshot.external_follow
    if (external.single_limit > 0 && external.daily_limit > 0 && external.daily_limit < external.single_limit) {
      showMessage('每日上限不能小于单笔上限', 'warning')
      return
    }
    savingRef.current = true
    setSaving(true)
    try {
      const current = await revalidateMember(selected)
      if (!current || selectedMemberIDRef.current !== memberID) return
      const next = await updateTrading(role, memberID, {
        fly_mode: configSnapshot.fly.mode,
        fly_rate: configSnapshot.fly.rate,
        external_follow: {
          target_platform: external.target_platform,
          target_account: external.target_account,
          endpoint_label: external.endpoint_label,
          single_limit: external.single_limit,
          daily_limit: external.daily_limit,
          remark: external.remark,
        },
        rebate_mode: configSnapshot.rebate.mode,
        rebate_rate: configSnapshot.rebate.rate,
        // This page owns fly-order fields only. An empty game avoids rewriting
        // any per-play odds while preserving the existing rebate configuration.
        game_id: '',
        odds: [],
      })
      if (selectedMemberIDRef.current !== memberID || !alive.current) return
      const latest = await revalidateMember(current)
      if (!latest || selectedMemberIDRef.current !== memberID) return
      setConfig(next)
      setMembers(current => current.map(item => item.id === memberID
        ? { ...item, fly_mode: next.fly.mode, fly_rate: next.fly.rate }
        : item))
      setDirty(false)
      showMessage('会员飞单配置已保存；外部平台仍为未连接', 'success')
    } catch (reason) {
      if (selectedMemberIDRef.current === memberID) showMessage(reason instanceof Error ? reason.message : '保存会员飞单配置失败', 'error')
    } finally {
      savingRef.current = false
      if (alive.current) setSaving(false)
    }
  }

  return <Stack gap={2.2} sx={{ p: { xs: 1.25, sm: 1.75, md: 2 } }}>
    <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'flex-start', sm: 'center' }} justifyContent="space-between" gap={1.5}>
      <Box>
        <Typography color="primary.main" fontSize={11} fontWeight={900} letterSpacing=".13em">内容与服务</Typography>
        <Stack direction="row" alignItems="center" gap={1} mt={0.3}>
          <FlightTakeoffRounded color="primary" />
          <Typography variant="h5" fontWeight={950}>飞单管理</Typography>
        </Stack>
        <Typography color="text.secondary" fontSize={12} mt={0.5}>按会员独立维护站内飞单比例，并准备未来外部跟单连接资料。</Typography>
      </Box>
      <Chip icon={<LinkOffRounded />} color="warning" variant="outlined" label="外部跟单未连接" />
    </Stack>

    <Alert severity="warning" variant="outlined">
      当前没有安装外部平台连接器。目标平台、账号和端点仅保存为非敏感配置准备，不会发送真实外部订单，也不会显示虚假的成功状态；请勿填写密码、Token 或 API Key。
    </Alert>

    <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.2}>
      {[
        [role === 'admin' ? '权限内会员' : '曾加入房间会员', total, role === 'admin' ? '当前账号只能查看所属权限范围' : '已切换会员仅保留公开身份，不再提供配置'],
        ['本页单独比例', summary.custom, '已覆盖房间默认比例'],
        ['本页已停用', summary.disabled, '不会计算站内飞单金额'],
      ].map(([label, value, note]) => <Paper key={String(label)} variant="outlined" sx={{ flex: 1, p: 1.6, borderRadius: 1.5 }}>
        <Typography color="text.secondary" fontSize={10.5} fontWeight={800}>{label}</Typography>
        <Typography fontSize={23} fontWeight={950} lineHeight={1.25}>{value}</Typography>
        <Typography color="text.secondary" fontSize={9.5}>{note}</Typography>
      </Paper>)}
    </Stack>

    <Paper variant="outlined" sx={{ borderRadius: 1.5, overflow: 'hidden' }}>
      <Stack component="form" direction={{ xs: 'column', sm: 'row' }} gap={1} p={1.5} onSubmit={event => { event.preventDefault(); const next = query.trim(); if (next !== appliedQuery || page !== 0) setLoading(true); setPage(0); setAppliedQuery(next) }}>
        <TextField fullWidth size="small" label="搜索会员" placeholder="账号、昵称或会员号" value={query} onChange={event => setQuery(event.target.value)} />
        <FormControl size="small" sx={{ minWidth: 138 }}>
          <InputLabel id="fly-member-status">会员状态</InputLabel>
          <Select labelId="fly-member-status" label="会员状态" value={status} onChange={event => { setLoading(true); setStatus(event.target.value); setPage(0) }}>
            <MenuItem value="all">全部状态</MenuItem>
            <MenuItem value="active">正常</MenuItem>
            <MenuItem value="disabled">停用</MenuItem>
          </Select>
        </FormControl>
        <Button type="submit" variant="contained" startIcon={<SearchRounded />} sx={{ minWidth: 104 }}>查询</Button>
      </Stack>
      {error && <Alert severity="error" sx={{ borderRadius: 0 }}>{error}</Alert>}
      <TableContainer sx={{ minHeight: 280 }}>
        <Table size="small" sx={{ minWidth: 760 }}>
          <TableHead><TableRow>
            <TableCell>会员</TableCell>
            <TableCell>{role === 'admin' ? '所属房间' : '房间状态'}</TableCell>
            <TableCell>站内飞单</TableCell>
            <TableCell>外部连接</TableCell>
            <TableCell align="right">操作</TableCell>
          </TableRow></TableHead>
          <TableBody>
            {loading ? <TableRow><TableCell colSpan={5} align="center" sx={{ py: 7 }}><CircularProgress size={26} /></TableCell></TableRow>
              : members.length === 0 ? <TableRow><TableCell colSpan={5} align="center" sx={{ py: 7, color: 'text.secondary' }}>当前范围内没有匹配会员</TableCell></TableRow>
                : members.map(member => <TableRow hover key={member.id}>
                  <TableCell><Stack direction="row" alignItems="center" gap={1}>
                    <Avatar src={member.avatar} sx={{ width: 34, height: 34, fontSize: 12 }}>{(member.nickname || member.username).slice(0, 1)}</Avatar>
                    <Box minWidth={0}><Typography fontSize={12} fontWeight={850} noWrap>{member.nickname || member.username}</Typography><Typography color="text.secondary" fontSize={9.5} noWrap>{member.username} · #{member.public_id}</Typography></Box>
                  </Stack></TableCell>
                  <TableCell><Typography fontSize={11}>{role === 'admin' ? ('room_code' in member ? member.room_code || '直属会员' : '—') : isWorkspaceMember(member) && member.in_current_room === true ? '在本房间' : '已切换'}</Typography></TableCell>
                  <TableCell>{canConfigure(role, member) ? <Chip size="small" color={member.fly_mode === 'off' ? 'default' : member.fly_mode === 'custom' ? 'primary' : 'info'} variant="outlined" label={`${modeLabel[member.fly_mode ?? 'inherit'] ?? member.fly_mode}${member.fly_mode === 'custom' ? ` ${member.fly_rate ?? 0}%` : ''}`} /> : '—'}</TableCell>
                  <TableCell>{canConfigure(role, member) ? <Chip size="small" icon={<LinkOffRounded />} color="warning" variant="outlined" label="未连接" /> : '—'}</TableCell>
                  <TableCell align="right">{canConfigure(role, member) ? <Button size="small" startIcon={<ManageAccountsRounded />} onClick={() => void openMember(member)}>独立配置</Button> : '—'}</TableCell>
                </TableRow>)}
          </TableBody>
        </Table>
      </TableContainer>
      <TablePagination component="div" count={total} page={page} rowsPerPage={pageSize} rowsPerPageOptions={[10, 20, 50]} labelRowsPerPage="每页" labelDisplayedRows={({ from, to, count }) => `${from}–${to} / ${count}`} onPageChange={(_, next) => { setLoading(true); setPage(next) }} onRowsPerPageChange={event => { setLoading(true); setPageSize(Number(event.target.value)); setPage(0) }} />
    </Paper>

    <Dialog open={Boolean(selected)} onClose={closeDialog} fullWidth maxWidth="md" slotProps={{ paper: { sx: { borderRadius: 1.5 } } }}>
      <DialogTitle sx={{ pb: 1 }}>
        <Stack direction="row" alignItems="center" justifyContent="space-between" gap={1}>
          <Box><Typography fontSize={17} fontWeight={950}>会员独立飞单</Typography><Typography color="text.secondary" fontSize={10.5}>{selected?.nickname || selected?.username} · {selected?.username}</Typography></Box>
          <Chip size="small" icon={<LinkOffRounded />} color="warning" variant="outlined" label="外部未连接" />
        </Stack>
      </DialogTitle>
      <DialogContent dividers>
        {!canConfigure(role, selected) || configLoading || !config ? <Box py={8} display="grid" sx={{ placeItems: 'center' }}><CircularProgress size={28} /></Box> : <Stack gap={2}>
          <Paper variant="outlined" sx={{ p: 1.6, borderRadius: 1.5 }}>
            <Typography fontSize={13} fontWeight={900}>站内飞单规则</Typography>
            <Typography color="text.secondary" fontSize={10} mb={1.3}>该设置复用现有注单计算逻辑，保存后真实影响此会员的站内飞单金额。</Typography>
            <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.2}>
              <TextField select fullWidth size="small" label="飞单模式" value={config.fly.mode} onChange={event => { setConfig(current => current ? { ...current, fly: { ...current.fly, mode: event.target.value } } : current); setDirty(true) }}>
                <MenuItem value="inherit">跟随房间（当前 {config.room_fly_rate}%）</MenuItem>
                <MenuItem value="custom">会员单独比例</MenuItem>
                <MenuItem value="off">停用飞单</MenuItem>
              </TextField>
              <TextField fullWidth size="small" type="number" label="会员飞单比例 %" disabled={config.fly.mode !== 'custom'} value={config.fly.rate} inputProps={{ min: 0, max: 100, step: .01 }} onChange={event => { setConfig(current => current ? { ...current, fly: { ...current.fly, rate: Number(event.target.value) } } : current); setDirty(true) }} />
            </Stack>
          </Paper>

          <Paper variant="outlined" sx={{ p: 1.6, borderRadius: 1.5 }}>
            <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'flex-start', sm: 'center' }} justifyContent="space-between" gap={1} mb={1.3}>
              <Box><Typography fontSize={13} fontWeight={900}>外部跟单连接预配置</Typography><Typography color="text.secondary" fontSize={10}>只保存非敏感标识和未来限额；当前不参与注单发送。</Typography></Box>
              <Button size="small" color="inherit" startIcon={<RestartAltRounded />} onClick={() => { setConfig(current => updateExternal(current, { target_platform: '', target_account: '', endpoint_label: '', single_limit: 0, daily_limit: 0, remark: '' })); setDirty(true) }}>清空预配置</Button>
            </Stack>
            <Alert severity="info" icon={<LinkOffRounded />} sx={{ mb: 1.4 }}>连接能力：待接入。系统不会收集密码、Token 或 API Key，也不会将“保存配置”当作连接成功。</Alert>
            <Stack gap={1.2}>
              <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.2}>
                <TextField fullWidth size="small" label="目标平台" placeholder="例如：待接入平台 A" value={config.external_follow.target_platform} inputProps={{ maxLength: 80 }} onChange={event => { setConfig(current => updateExternal(current, { target_platform: event.target.value })); setDirty(true) }} />
                <TextField fullWidth size="small" label="目标账号标识" placeholder="只填账号别名，不填密码" value={config.external_follow.target_account} inputProps={{ maxLength: 120 }} onChange={event => { setConfig(current => updateExternal(current, { target_account: event.target.value })); setDirty(true) }} />
              </Stack>
              <TextField fullWidth size="small" label="端点标识" placeholder="例如：华东线路 A；不是密钥或带凭据 URL" value={config.external_follow.endpoint_label} inputProps={{ maxLength: 160 }} onChange={event => { setConfig(current => updateExternal(current, { endpoint_label: event.target.value })); setDirty(true) }} />
              <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.2}>
                <TextField fullWidth size="small" type="number" label="预配置单笔上限" helperText="0 表示暂未设置" value={config.external_follow.single_limit} inputProps={{ min: 0, max: 100000000, step: .01 }} onChange={event => { setConfig(current => updateExternal(current, { single_limit: Number(event.target.value) })); setDirty(true) }} />
                <TextField fullWidth size="small" type="number" label="预配置每日上限" helperText="应不小于单笔上限；0 表示暂未设置" value={config.external_follow.daily_limit} inputProps={{ min: 0, max: 100000000, step: .01 }} onChange={event => { setConfig(current => updateExternal(current, { daily_limit: Number(event.target.value) })); setDirty(true) }} />
              </Stack>
              <TextField fullWidth multiline minRows={2} size="small" label="连接准备备注" placeholder="记录对接负责人、平台确认事项等；不要粘贴任何密钥" value={config.external_follow.remark} inputProps={{ maxLength: 500 }} onChange={event => { setConfig(current => updateExternal(current, { remark: event.target.value })); setDirty(true) }} />
            </Stack>
          </Paper>
        </Stack>}
      </DialogContent>
      <DialogActions sx={{ p: 1.5 }}>
        <Button onClick={closeDialog}>关闭</Button>
        <Button variant="contained" startIcon={<SaveRounded />} disabled={!canConfigure(role, selected) || !config || !dirty || saving} onClick={() => void save()}>{saving ? '保存中…' : dirty ? '保存会员配置' : '已保存'}</Button>
      </DialogActions>
    </Dialog>
  </Stack>
}
