import {
  Alert,
  Box,
  Button,
  Card,
  Chip,
  CircularProgress,
  InputAdornment,
  MenuItem,
  Paper,
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
import DownloadRounded from '@mui/icons-material/DownloadRounded'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import InboxRounded from '@mui/icons-material/InboxRounded'
import { useCallback, useEffect, useState } from 'react'
import { adminApi, type AdminGame, type BoardReportRow } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'

const money = (value: number) => new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)
const dateTime = (value?: string | null) => value ? new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(new Date(value)) : '—'

export function BoardReportPage() {
  const [games, setGames] = useState<AdminGame[]>([])
  const [items, setItems] = useState<BoardReportRow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState(20)
  const [query, setQuery] = useState('')
  const [gameId, setGameId] = useState('all')
  const [applied, setApplied] = useState({ query: '', gameId: 'all' })
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const { showMessage } = useFeedback()

  const load = useCallback(async (notify = false) => {
    setLoading(true)
    setError('')
    try {
      const result = await adminApi.boardReport({ query: applied.query, gameId: applied.gameId, page: page + 1, pageSize })
      setItems(result.items)
      setTotal(result.total)
      if (notify) showMessage('打盘报表已刷新')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取打盘报表失败')
    } finally {
      setLoading(false)
    }
  }, [applied, page, pageSize, showMessage])

  useEffect(() => {
    void adminApi.games().then(setGames).catch(() => undefined)
  }, [])

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0)
    return () => window.clearTimeout(timer)
  }, [load])

  const exportCsv = () => {
    if (!items.length) {
      showMessage('当前没有可导出的记录', 'warning')
      return
    }
    const rows = items.map(item => [item.game_name, item.issue, item.bet_count, item.total_amount.toFixed(2), item.fly_amount.toFixed(2), item.status, dateTime(item.draw_at), item.draw_result])
    const escape = (value: unknown) => `"${String(value).replaceAll('"', '""')}"`
    const csv = [['游戏类型', '游戏期数', '总注单', '下注总金额', '飞单总金额', '状态', '开奖时间', '开奖结果'], ...rows].map(row => row.map(escape).join(',')).join('\n')
    const link = document.createElement('a')
    link.href = URL.createObjectURL(new Blob(['\uFEFF' + csv], { type: 'text/csv;charset=utf-8;' }))
    link.download = `board-report-${Date.now()}.csv`
    link.click()
    showMessage('打盘报表已导出')
  }

  return (
    <Box p={{ xs: 2, lg: 2.5 }}>
      <PageHeader
        eyebrow="数据中心 / 打盘"
        title="打盘报表"
        description="查看飞单执行状态与异常记录。"
        actions={
          <>
            <Button variant="contained" startIcon={<DownloadRounded />} onClick={exportCsv}>导出记录</Button>
            <Button variant="outlined" startIcon={<RefreshRounded />} disabled={loading} onClick={() => void load(true)}>刷新</Button>
          </>
        }
      />
      {error && <Alert severity="error" sx={{ mt: 2 }}>{error}</Alert>}
      <Stack gap={1.5} mt={2.5}>
        <Paper variant="outlined" sx={{ p: 1.5 }}>
          <Stack direction={{ xs: 'column', sm: 'row' }} gap={1} flexWrap="wrap">
            <TextField placeholder="搜索期号" value={query} onChange={event => setQuery(event.target.value)} sx={{ minWidth: { sm: 200 }, flex: { xs: 1, lg: 0 } }} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> } }} />
            <TextField select value={gameId} onChange={event => setGameId(event.target.value)} sx={{ minWidth: 180 }}>
              <MenuItem value="all">全部游戏</MenuItem>
              {games.map(game => <MenuItem key={game.id} value={game.id}>{game.name}</MenuItem>)}
            </TextField>
            <Button variant="contained" onClick={() => { setPage(0); setApplied({ query: query.trim(), gameId }) }}>查询</Button>
            <Button variant="outlined" onClick={() => { setQuery(''); setGameId('all'); setPage(0); setApplied({ query: '', gameId: 'all' }); showMessage('筛选条件已重置', 'info') }}>重置</Button>
          </Stack>
        </Paper>
        <Card>
          {loading && <Box px={2} py={1}><CircularProgress size={18} /></Box>}
          <TableContainer>
            <Table size="small" sx={{ minWidth: 980 }}>
              <TableHead>
                <TableRow>
                  {['游戏类型', '游戏期数', '总注单', '下注总金额', '飞单总金额', '状态', '开奖时间', '开奖结果'].map(column => <TableCell key={column}>{column}</TableCell>)}
                </TableRow>
              </TableHead>
              <TableBody>
                {items.map(item => (
                  <TableRow hover key={`${item.game_id}-${item.issue}`}>
                    <TableCell>{item.game_name}</TableCell>
                    <TableCell>{item.issue}</TableCell>
                    <TableCell>{item.bet_count}</TableCell>
                    <TableCell>{money(item.total_amount)}</TableCell>
                    <TableCell>{money(item.fly_amount)}</TableCell>
                    <TableCell><Chip size="small" color={item.status.includes('待') ? 'warning' : 'success'} label={item.status} /></TableCell>
                    <TableCell>{dateTime(item.draw_at)}</TableCell>
                    <TableCell>{item.draw_result || '—'}</TableCell>
                  </TableRow>
                ))}
                {!loading && !items.length && (
                  <TableRow>
                    <TableCell colSpan={8}>
                      <Stack minHeight={240} alignItems="center" justifyContent="center" color="text.secondary">
                        <InboxRounded />
                        <Typography mt={1} fontSize={13} fontWeight={700}>暂无打盘记录</Typography>
                        <Typography variant="caption">可在现场监控生成演示注单后再查看</Typography>
                      </Stack>
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>
          <TablePagination component="div" count={total} page={page} onPageChange={(_, next) => setPage(next)} rowsPerPage={pageSize} onRowsPerPageChange={event => { setPageSize(Number(event.target.value)); setPage(0) }} rowsPerPageOptions={[10, 20, 50]} labelRowsPerPage="每页" />
        </Card>
      </Stack>
    </Box>
  )
}
