import {
  Alert,
  Box,
  Button,
  Card,
  Chip,
  CircularProgress,
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
import { adminApi, type AdminBet, type AdminGame } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'

const money = (value: number) => new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)
const statusLabel: Record<string, string> = { pending: '待结算', won: '中奖', lost: '未中', cancelled: '已撤单' }
const statusColor = (status: string): 'warning' | 'success' | 'default' | 'error' => {
  if (status === 'pending') return 'warning'
  if (status === 'won') return 'success'
  if (status === 'cancelled') return 'error'
  return 'default'
}

export function BetsPage() {
  const [items, setItems] = useState<AdminBet[]>([])
  const [games, setGames] = useState<AdminGame[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState(20)
  const [query, setQuery] = useState('')
  const [issue, setIssue] = useState('')
  const [userId, setUserId] = useState('')
  const [gameId, setGameId] = useState('all')
  const [status, setStatus] = useState('all')
  const [applied, setApplied] = useState({ query: '', issue: '', userId: '', gameId: 'all', status: 'all' })
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const { showMessage } = useFeedback()

  const load = useCallback(async (notify = false) => {
    setLoading(true)
    setError('')
    try {
      const [list, dashboard] = await Promise.all([
        adminApi.bets({
          query: applied.query,
          issue: applied.issue,
          gameId: applied.gameId,
          status: applied.status,
          userId: applied.userId ? Number(applied.userId) : undefined,
          page: page + 1,
          pageSize,
        }),
        adminApi.dashboard(),
      ])
      setItems(list.items)
      setTotal(list.total)
      setGames(dashboard.games ?? [])
      if (notify) showMessage('注单列表已刷新')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取注单失败')
    } finally {
      setLoading(false)
    }
  }, [applied, page, pageSize, showMessage])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])

  const cancel = async (id: number) => {
    try {
      await adminApi.cancelBet(id)
      showMessage('注单已撤销')
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '撤单失败')
    }
  }

  return (
    <Box p={{ xs: 2, lg: 2.5 }}>
      <PageHeader
        eyebrow="游戏运营 / 注单"
        title="注单管理"
        description="按用户、期号、彩种查询注单，对待结算订单执行撤单。"
        actions={<Button variant="outlined" startIcon={<RefreshRounded />} onClick={() => void load(true)} disabled={loading}>刷新</Button>}
      />
      {error && <Alert severity="error" sx={{ mt: 2 }}>{error}</Alert>}
      <Card sx={{ mt: 2.5, p: 2 }}>
        <Stack direction={{ xs: 'column', md: 'row' }} gap={1.5} flexWrap="wrap">
          <TextField size="small" label="关键词" placeholder="用户名 / 期号 / 玩法" value={query} onChange={e => setQuery(e.target.value)} sx={{ minWidth: 180 }} />
          <TextField size="small" label="期号" value={issue} onChange={e => setIssue(e.target.value)} sx={{ minWidth: 140 }} />
          <TextField size="small" label="用户 ID" value={userId} onChange={e => setUserId(e.target.value)} sx={{ minWidth: 120 }} />
          <TextField size="small" select label="彩种" value={gameId} onChange={e => setGameId(e.target.value)} sx={{ minWidth: 160 }}>
            <MenuItem value="all">全部彩种</MenuItem>
            {games.map(game => <MenuItem key={game.id} value={game.id}>{game.name}</MenuItem>)}
          </TextField>
          <TextField size="small" select label="状态" value={status} onChange={e => setStatus(e.target.value)} sx={{ minWidth: 120 }}>
            <MenuItem value="all">全部</MenuItem>
            <MenuItem value="pending">待结算</MenuItem>
            <MenuItem value="won">中奖</MenuItem>
            <MenuItem value="lost">未中</MenuItem>
            <MenuItem value="cancelled">已撤单</MenuItem>
          </TextField>
          <Button variant="contained" startIcon={<SearchRounded />} onClick={() => { setPage(0); setApplied({ query, issue, userId, gameId, status }) }}>查询</Button>
        </Stack>
      </Card>
      <Card sx={{ mt: 1.5 }}>
        {loading && <Box px={2} py={1}><CircularProgress size={18} /></Box>}
        <TableContainer>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>ID</TableCell>
                <TableCell>用户</TableCell>
                <TableCell>彩种/期号</TableCell>
                <TableCell>玩法</TableCell>
                <TableCell align="right">金额</TableCell>
                <TableCell align="right">赔率</TableCell>
                <TableCell align="right">飞单</TableCell>
                <TableCell>状态</TableCell>
                <TableCell align="right">派彩</TableCell>
                <TableCell>时间</TableCell>
                <TableCell align="right">操作</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {items.map(item => (
                <TableRow key={item.id} hover>
                  <TableCell>{item.id}</TableCell>
                  <TableCell>
                    <Typography fontSize={12} fontWeight={700}>{item.username}</Typography>
                    <Typography variant="caption" color="text.secondary">#{item.user_id}</Typography>
                  </TableCell>
                  <TableCell>
                    <Typography fontSize={12}>{item.game_id}</Typography>
                    <Typography variant="caption" color="text.secondary">{item.issue}</Typography>
                  </TableCell>
                  <TableCell>{item.play_name || item.play_code} / {item.selection}</TableCell>
                  <TableCell align="right">{money(item.amount)}</TableCell>
                  <TableCell align="right">{item.odds}</TableCell>
                  <TableCell align="right">{money(item.fly_amount)}</TableCell>
                  <TableCell><Chip size="small" color={statusColor(item.status)} label={statusLabel[item.status] ?? item.status} /></TableCell>
                  <TableCell align="right">{money(item.payout)}</TableCell>
                  <TableCell>{new Date(item.created_at).toLocaleString('zh-CN', { hour12: false })}</TableCell>
                  <TableCell align="right">
                    {item.status === 'pending' ? <Button size="small" color="error" onClick={() => void cancel(item.id)}>撤单</Button> : '—'}
                  </TableCell>
                </TableRow>
              ))}
              {!loading && items.length === 0 && (
                <TableRow><TableCell colSpan={11}><Typography textAlign="center" color="text.secondary" py={4}>暂无注单</Typography></TableCell></TableRow>
              )}
            </TableBody>
          </Table>
        </TableContainer>
        <TablePagination
          component="div"
          count={total}
          page={page}
          onPageChange={(_, next) => setPage(next)}
          rowsPerPage={pageSize}
          onRowsPerPageChange={event => { setPageSize(Number(event.target.value)); setPage(0) }}
          rowsPerPageOptions={[10, 20, 50]}
          labelRowsPerPage="每页"
        />
      </Card>
    </Box>
  )
}
