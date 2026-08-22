import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  MenuItem,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import AutoAwesomeRounded from '@mui/icons-material/AutoAwesomeRounded'
import CasinoRounded from '@mui/icons-material/CasinoRounded'
import TaskAltRounded from '@mui/icons-material/TaskAltRounded'
import { useCallback, useEffect, useState } from 'react'
import { adminApi, type AdminGame, type MonitorSnapshot } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'

const money = (value: number) => new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)
const columns = ['第一球', '第二球', '第三球', '第四球', '第五球', '总和']

export function MonitorPage() {
  const [games, setGames] = useState<AdminGame[]>([])
  const [gameId, setGameId] = useState('')
  const [data, setData] = useState<MonitorSnapshot | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const { showMessage } = useFeedback()

  const load = useCallback(async (id: string, notify = false) => {
    setLoading(true)
    setError('')
    try {
      const snapshot = await adminApi.monitor(id || undefined)
      setData(snapshot)
      setGameId(snapshot.game_id)
      if (notify) showMessage('现场监控已刷新')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取现场监控失败')
    } finally {
      setLoading(false)
    }
  }, [showMessage])

  useEffect(() => {
    let active = true
    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          const list = await adminApi.games()
          if (!active) return
          setGames(list)
          const preferred = list.find(game => game.enabled)?.id || list[0]?.id || ''
          await load(preferred)
        } catch (reason) {
          if (!active) return
          setError(reason instanceof Error ? reason.message : '读取游戏列表失败')
          setLoading(false)
        }
      })()
    }, 0)
    return () => { active = false; window.clearTimeout(timer) }
  }, [load])

  useEffect(() => {
    if (!gameId) return
    const poll = window.setInterval(() => void load(gameId), 15_000)
    return () => window.clearInterval(poll)
  }, [gameId, load])

  const run = async (action: 'seed' | 'publish' | 'settle') => {
    if (!gameId || !data) return
    setBusy(action)
    setError('')
    try {
      if (action === 'seed') {
        const snapshot = await adminApi.seedMonitor(gameId)
        setData(snapshot)
        showMessage('演示注单已生成')
      } else if (action === 'publish') {
        const result = await adminApi.publishDraw(gameId, { issue: data.issue })
        showMessage(`${result.game_name} ${result.issue} 已开奖结算：中 ${result.won} / 未中 ${result.lost}，派彩 ${money(result.payout_amount)}`)
        await load(gameId)
      } else {
        const result = await adminApi.settleIssue(gameId, data.issue)
        showMessage(`结算完成：中 ${result.won} / 未中 ${result.lost}，派彩 ${money(result.payout_amount)}`)
        await load(gameId)
      }
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '操作失败')
    } finally {
      setBusy('')
    }
  }

  const settlement = data?.settlement

  return (
    <Box p={{ xs: 2, lg: 2.5 }}>
      <PageHeader
        eyebrow="游戏运营 / 实时"
        title="现场监控"
        description="监控当期注单分布，支持开奖并自动结算派彩。"
        actions={
          <>
            <TextField select size="small" label="彩种" value={gameId} onChange={event => { setGameId(event.target.value); void load(event.target.value) }} sx={{ minWidth: 180 }} disabled={!games.length || loading}>
              {games.map(game => <MenuItem key={game.id} value={game.id}>{game.name}</MenuItem>)}
            </TextField>
            <Button variant="outlined" startIcon={<RefreshRounded />} disabled={loading || Boolean(busy)} onClick={() => void load(gameId, true)}>刷新</Button>
            <Button variant="outlined" startIcon={<AutoAwesomeRounded />} disabled={!gameId || loading || Boolean(busy)} onClick={() => void run('seed')}>{busy === 'seed' ? '生成中…' : '生成演示注单'}</Button>
            <Button variant="contained" startIcon={<CasinoRounded />} disabled={!gameId || !data || loading || Boolean(busy)} onClick={() => void run('publish')}>{busy === 'publish' ? '开奖中…' : '开奖并结算'}</Button>
            <Button variant="contained" color="secondary" startIcon={<TaskAltRounded />} disabled={!gameId || !data || loading || Boolean(busy) || !settlement?.has_draw || settlement.pending === 0} onClick={() => void run('settle')}>{busy === 'settle' ? '结算中…' : '仅结算'}</Button>
            <Chip color="success" label="实时更新" />
          </>
        }
      />
      {error && <Alert severity="error" sx={{ mt: 2 }}>{error}</Alert>}
      {settlement && (
        <Alert severity={settlement.settled ? 'success' : settlement.has_draw ? 'warning' : 'info'} sx={{ mt: 2 }}>
          期号 {settlement.issue}
          {settlement.has_draw ? ` · 开奖号码 ${settlement.numbers.join(',')}` : ' · 尚未开奖'}
          {` · 待结算 ${settlement.pending} · 中 ${settlement.won} · 未中 ${settlement.lost} · 派彩 ${money(settlement.payout_amount)}`}
        </Alert>
      )}
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2,1fr)', md: 'repeat(4,1fr)' }, gap: 1.2, mt: 2.5 }}>
        {[
          ['当前期号', data?.issue || '—'],
          ['总金额', data ? money(data.total_amount) : '—'],
          ['参与人数', data ? String(data.bettor_count) : '—'],
          ['开奖时间', data?.draw_at_label || '—'],
        ].map(([label, value]) => (
          <Card key={label}><CardContent><Typography variant="caption" color="text.secondary">{label}</Typography><Typography fontWeight={800} mt={.5}>{value}</Typography></CardContent></Card>
        ))}
      </Box>
      <Card sx={{ mt: 1.5 }}>
        {loading && <Box px={2} py={1}><CircularProgress size={18} /></Box>}
        <TableContainer>
          <Table size="small" sx={{ minWidth: 760 }}>
            <TableHead>
              <TableRow>
                <TableCell>号码</TableCell>
                {columns.map(column => <TableCell align="center" key={column}>{column}</TableCell>)}
              </TableRow>
            </TableHead>
            <TableBody>
              {Array.from({ length: 10 }, (_, n) => (
                <TableRow key={n}>
                  <TableCell><Chip label={n} color="primary" size="small" /></TableCell>
                  {Array.from({ length: 6 }, (_, col) => {
                    const amount = data?.matrix?.[n]?.[col] ?? 0
                    return <TableCell align="center" key={col}>{amount > 0 ? money(amount) : '0'}</TableCell>
                  })}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
        {data && (
          <Stack direction="row" justifyContent="space-between" px={2} py={1.2}>
            <Typography variant="caption" color="text.secondary">{data.game_name} · 注单 {data.bet_count} 笔</Typography>
            <Typography variant="caption" color="text.secondary">更新于 {new Date(data.updated_at).toLocaleTimeString('zh-CN', { hour12: false })}</Typography>
          </Stack>
        )}
      </Card>
    </Box>
  )
}
