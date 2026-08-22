import {
  Alert,
  Box,
  Button,
  Card,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  MenuItem,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  TextField,
  Typography,
} from '@mui/material'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import { useCallback, useEffect, useState } from 'react'
import { adminApi, type AgentItem, type SpecialOverview } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'

const money = (value: number) => new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)

export function AgentsPage() {
  const [items, setItems] = useState<AgentItem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState(20)
  const [query, setQuery] = useState('')
  const [applied, setApplied] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [assignOpen, setAssignOpen] = useState(false)
  const [special, setSpecial] = useState<SpecialOverview | null>(null)
  const [resourceId, setResourceId] = useState(0)
  const [userId, setUserId] = useState('')
  const [saving, setSaving] = useState(false)
  const { showMessage } = useFeedback()

  const load = useCallback(async (notify = false) => {
    setLoading(true)
    setError('')
    try {
      const list = await adminApi.agents({ query: applied, page: page + 1, pageSize })
      setItems(list.items)
      setTotal(list.total)
      if (notify) showMessage('代理列表已刷新')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取代理失败')
    } finally {
      setLoading(false)
    }
  }, [applied, page, pageSize, showMessage])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])

  const openAssign = async () => {
    setAssignOpen(true)
    try {
      const overview = await adminApi.specialOverview()
      setSpecial(overview)
      const first = overview.resources.find(item => item.status === 'available')
      setResourceId(first?.id ?? 0)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取房间号池失败')
    }
  }

  const assign = async () => {
    if (!resourceId || !Number(userId)) {
      setError('请选择可用房间号并填写用户 ID')
      return
    }
    setSaving(true)
    try {
      await adminApi.assignAgentRoom({ resource_id: resourceId, user_id: Number(userId) })
      showMessage('房间号已分配，用户已升为代理')
      setAssignOpen(false)
      setUserId('')
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '分配失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Box p={{ xs: 2, lg: 2.5 }}>
      <PageHeader
        eyebrow="业务管理 / 代理"
        title="代理管理"
        description="代理通过购买/获发靓号房间号运营；用户输入房间号进入对应代理房间。"
        actions={
          <>
            <Button variant="outlined" startIcon={<RefreshRounded />} onClick={() => void load(true)}>刷新</Button>
            <Button variant="contained" onClick={() => void openAssign()}>分配房间号</Button>
          </>
        }
      />
      {error && <Alert severity="error" sx={{ mt: 2 }} onClose={() => setError('')}>{error}</Alert>}
      <Card sx={{ mt: 2.5, p: 2 }}>
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.5}>
          <TextField size="small" fullWidth placeholder="搜索代理用户名 / 昵称 / 房间号" value={query} onChange={e => setQuery(e.target.value)} onKeyDown={e => { if (e.key === 'Enter') { setPage(0); setApplied(query.trim()) } }} />
          <Button variant="contained" startIcon={<SearchRounded />} onClick={() => { setPage(0); setApplied(query.trim()) }}>查询</Button>
        </Stack>
      </Card>
      <Card sx={{ mt: 1.5 }}>
        {loading && <Box px={2} py={1}><CircularProgress size={18} /></Box>}
        <TableContainer>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>代理</TableCell>
                <TableCell>房间号（靓号）</TableCell>
                <TableCell align="right">余额</TableCell>
                <TableCell align="right">下级会员</TableCell>
                <TableCell>状态</TableCell>
                <TableCell>创建时间</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {items.map(item => (
                <TableRow key={item.id} hover>
                  <TableCell>
                    <Typography fontSize={12} fontWeight={800}>{item.nickname || item.username}</Typography>
                    <Typography variant="caption" color="text.secondary">@{item.username} · ID {item.id}</Typography>
                  </TableCell>
                  <TableCell>
                    {item.room_code ? <Chip size="small" color="primary" label={item.room_code} /> : <Typography variant="caption" color="text.secondary">未分配</Typography>}
                  </TableCell>
                  <TableCell align="right">{money(item.balance)}</TableCell>
                  <TableCell align="right">{item.member_count}</TableCell>
                  <TableCell><Chip size="small" color={item.status === 1 ? 'success' : 'default'} label={item.status === 1 ? '正常' : '停用'} /></TableCell>
                  <TableCell>{item.created_at}</TableCell>
                </TableRow>
              ))}
              {!loading && !items.length && <TableRow><TableCell colSpan={6} align="center" sx={{ py: 6, color: 'text.secondary' }}>暂无代理，可先在用户管理设为代理，或通过「分配房间号」发放靓号</TableCell></TableRow>}
            </TableBody>
          </Table>
        </TableContainer>
        <TablePagination component="div" count={total} page={page} onPageChange={(_, next) => setPage(next)} rowsPerPage={pageSize} onRowsPerPageChange={e => { setPageSize(Number(e.target.value)); setPage(0) }} rowsPerPageOptions={[10, 20, 50]} labelRowsPerPage="每页" />
      </Card>

      <Dialog open={assignOpen} onClose={() => setAssignOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>分配靓号房间号给代理</DialogTitle>
        <DialogContent>
          <Alert severity="info" sx={{ mb: 2 }}>发放后：用户角色升为代理，房间号写入代理资料；前端输入该号即可进入代理房间。</Alert>
          <Stack gap={1.5} mt={1}>
            <TextField select label="可用房间号" value={resourceId || ''} onChange={e => setResourceId(Number(e.target.value))}>
              {(special?.resources ?? []).filter(item => item.status === 'available').map(item => (
                <MenuItem key={item.id} value={item.id}>{item.number} · {item.level}</MenuItem>
              ))}
            </TextField>
            <TextField type="number" label="用户 ID" value={userId} onChange={e => setUserId(e.target.value)} helperText="普通会员或已有代理均可；发放后绑定该房间号" />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setAssignOpen(false)}>取消</Button>
          <Button variant="contained" disabled={saving} onClick={() => void assign()}>{saving ? '分配中…' : '确认分配'}</Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}
