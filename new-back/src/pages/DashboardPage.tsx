import { Alert, Box, Button, Card, CardContent, Chip, CircularProgress, FormControlLabel, IconButton, InputAdornment, Paper, Stack, Switch, Table, TableBody, TableCell, TableContainer, TableHead, TableRow, TextField, Typography } from '@mui/material'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import AccountBalanceWalletRounded from '@mui/icons-material/AccountBalanceWalletRounded'
import TrendingUpRounded from '@mui/icons-material/TrendingUpRounded'
import QueryStatsRounded from '@mui/icons-material/QueryStatsRounded'
import SavingsRounded from '@mui/icons-material/SavingsRounded'
import ShowChartRounded from '@mui/icons-material/ShowChartRounded'
import PendingActionsRounded from '@mui/icons-material/PendingActionsRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { adminApi, type AdminGame, type DashboardData } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'
import { useServerClock } from '../hooks/useServerClock'

const stats = [
  ['用户余额', 'user_balance', AccountBalanceWalletRounded, '#5b7cec'], ['今日流水', 'today_turnover', TrendingUpRounded, '#e4a23e'], ['今日盈亏', 'today_profit', QueryStatsRounded, '#35a7c8'],
  ['今日回水', 'today_rebate', SavingsRounded, '#2eaf7b'], ['总盈亏', 'total_profit', ShowChartRounded, '#df746a'], ['未结算金额', 'pending_settlement', PendingActionsRounded, '#8a70df'],
] as const

const number = (value = 0) => new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)
function left(value: string, now: number) { if (!now) return '--:--'; const seconds = Math.max(0, Math.floor((new Date(value).getTime() - now) / 1000)); return `${String(Math.floor(seconds / 60)).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}` }

export function DashboardPage() {
  const [data, setData] = useState<DashboardData | null>(null)
  const [error, setError] = useState('')
  const { now } = useServerClock()
  const [updating, setUpdating] = useState('')
  const [loading, setLoading] = useState(true)
  const [query, setQuery] = useState('')
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null)
  const { showMessage } = useFeedback()
  const load = useCallback(async (notify = false) => { setError(''); setLoading(true); try { setData(await adminApi.dashboard()); setUpdatedAt(new Date()); if (notify) showMessage('运营数据已刷新') } catch (reason) { setError(reason instanceof Error ? reason.message : '无法连接后端') } finally { setLoading(false) } }, [showMessage])
  useEffect(() => { void Promise.resolve().then(() => load()) }, [load])
  const filteredGames = useMemo(() => data?.games.filter(game => `${game.name}${game.code}${game.issue}`.toLowerCase().includes(query.trim().toLowerCase())) ?? [], [data, query])
  const toggle = async (game: AdminGame) => { setUpdating(game.id); try { const next = await adminApi.updateGameStatus(game.id, !game.enabled); setData(current => current ? { ...current, games: current.games.map(item => item.id === next.id ? next : item) } : current); showMessage(`${game.name}已${next.enabled ? '开启' : '关闭'}`) } catch (reason) { setError(reason instanceof Error ? reason.message : '更新失败') } finally { setUpdating('') } }

  return <Box p={{ xs: 2, lg: 2.5 }}><PageHeader eyebrow="总览 / 实时运营" title="运营首页" description={updatedAt ? `实时掌握游戏状态与经营数据 · 更新于 ${updatedAt.toLocaleTimeString('zh-CN', { hour12: false })}` : '实时掌握游戏运行状态、开奖周期和核心经营数据。'} actions={<Button variant="outlined" startIcon={loading ? <CircularProgress size={16} /> : <RefreshRounded />} disabled={loading} onClick={() => load(true)}>刷新数据</Button>} />
    {error && <Alert severity="error" sx={{ mt: 2 }}>{error}，请确认 backend 已在 8080 端口启动。</Alert>}
    {!data && loading && <Stack alignItems="center" py={10}><CircularProgress size={30} /></Stack>}
    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', xl: '300px minmax(0,1fr)' }, gap: 2, mt: 2.5 }}>
      <Card sx={{ order: { xs: 2, xl: 1 } }}><CardContent sx={{ p: 0 }}><Stack direction="row" justifyContent="space-between" alignItems="center" px={2} py={1.6} borderBottom={1} borderColor="divider"><Box><Typography fontWeight={750}>游戏状态</Typography><Typography variant="caption" color="text.secondary">{data?.games.filter(game => game.enabled).length ?? 0} 个运行中</Typography></Box><Chip size="small" color="success" label="实时" /></Stack><Box px={1.5} pt={1.5}><TextField fullWidth placeholder="搜索游戏或期号" value={query} onChange={event => setQuery(event.target.value)} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> } }} /></Box><Box sx={{ maxHeight: { xl: 'calc(100vh - 310px)' }, overflow: 'auto', p: 1 }}>{filteredGames.map(game => <Stack key={game.id} direction="row" alignItems="center" gap={1.2} p={1} borderRadius={2} sx={{ '&:hover': { bgcolor: 'action.hover' } }}><Box sx={{ width: 40, height: 40, flex: '0 0 auto', display: 'grid', placeItems: 'center', borderRadius: 2.2, color: '#fff', fontSize: 9, fontWeight: 800, background: game.badge_color || 'linear-gradient(145deg,#1686ad,#27b5ac)' }}>{game.badge.slice(0, 3)}</Box><Box minWidth={0} flex={1}><Stack direction="row" alignItems="center" gap={.6}><Typography fontSize={12} fontWeight={700} noWrap>{game.name}</Typography>{game.source_kind === 'official' && <Chip size="small" color="success" label="官方" sx={{ height: 17, fontSize: 8 }} />}</Stack><Typography fontSize={9} color="text.secondary" noWrap>{game.issue || '等待同步'}</Typography></Box><FormControlLabel sx={{ m: 0 }} control={<Switch inputProps={{ 'aria-label': `${game.name}运行状态` }} size="small" checked={game.enabled} disabled={updating === game.id} onChange={() => toggle(game)} />} label="" /></Stack>)}{filteredGames.length === 0 && <Typography textAlign="center" color="text.secondary" fontSize={12} py={5}>未找到匹配的游戏</Typography>}</Box></CardContent></Card>
      <Box sx={{ minWidth: 0, order: { xs: 1, xl: 2 } }}><Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2,1fr)', md: 'repeat(3,1fr)' }, gap: 1.4 }}>{stats.map(([label, key, Icon, color]) => <Card key={key}><CardContent sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', p: '16px !important' }}><Box><Typography variant="caption" color="text.secondary">{label}</Typography><Typography mt={.5} fontSize={{ xs: 16, md: 20 }} fontWeight={800}>{data ? number(data.stats[key]) : '—'}</Typography></Box><IconButton disableRipple sx={{ color: '#fff', bgcolor: color, '&:hover': { bgcolor: color } }}><Icon /></IconButton></CardContent></Card>)}</Box>
        <Card sx={{ mt: 2 }}><Stack direction="row" justifyContent="space-between" alignItems="center" p={2} borderBottom={1} borderColor="divider"><Box><Typography fontWeight={750}>游戏运行数据</Typography><Typography variant="caption" color="text.secondary">数据来自 Go backend</Typography></Box><Chip size="small" variant="outlined" label={`${data?.games.length ?? 0} 个游戏`} /></Stack><TableContainer component={Paper} elevation={0} sx={{ maxHeight: 'calc(100vh - 390px)' }}><Table stickyHeader size="small" sx={{ minWidth: 760 }}><TableHead><TableRow><TableCell>游戏</TableCell><TableCell>最新期号</TableCell><TableCell>状态</TableCell><TableCell align="right">当期流水</TableCell><TableCell align="right">今日盈亏</TableCell><TableCell align="right">开奖节奏</TableCell></TableRow></TableHead><TableBody>{data?.games.map(game => <TableRow hover key={game.id}><TableCell><Typography fontSize={12} fontWeight={700}>{game.name}</Typography><Typography fontSize={9} color="text.secondary">{game.category}</Typography></TableCell><TableCell sx={{ fontSize: 11 }}>{game.issue}</TableCell><TableCell><Chip size="small" color={game.enabled ? 'success' : 'default'} label={game.enabled ? '运行中' : '已关闭'} sx={{ fontSize: 9 }} /></TableCell><TableCell align="right">{number(game.turnover)}</TableCell><TableCell align="right">{number(game.profit)}</TableCell><TableCell align="right">{game.enabled ? game.schedule_mode === 'official-feed' ? <Chip size="small" color="info" variant="outlined" label="跟随官方" /> : left(game.next_draw_at, now) : '--:--'}</TableCell></TableRow>)}</TableBody></Table></TableContainer></Card>
      </Box>
    </Box>
  </Box>
}
