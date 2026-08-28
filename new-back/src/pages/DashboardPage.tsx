import { Alert, Avatar, Box, Card, CardContent, Chip, CircularProgress, FormControlLabel, IconButton, InputAdornment, Paper, Stack, Switch, Table, TableBody, TableCell, TableContainer, TableHead, TableRow, TextField, ToggleButton, ToggleButtonGroup, Typography } from '@mui/material'
import AccountBalanceWalletRounded from '@mui/icons-material/AccountBalanceWalletRounded'
import TrendingUpRounded from '@mui/icons-material/TrendingUpRounded'
import QueryStatsRounded from '@mui/icons-material/QueryStatsRounded'
import SavingsRounded from '@mui/icons-material/SavingsRounded'
import ShowChartRounded from '@mui/icons-material/ShowChartRounded'
import PendingActionsRounded from '@mui/icons-material/PendingActionsRounded'
import PriceCheckRounded from '@mui/icons-material/PriceCheckRounded'
import RedeemRounded from '@mui/icons-material/RedeemRounded'
import AccountTreeRounded from '@mui/icons-material/AccountTreeRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import PeopleAltRounded from '@mui/icons-material/PeopleAltRounded'
import SupportAgentRounded from '@mui/icons-material/SupportAgentRounded'
import ForumRounded from '@mui/icons-material/ForumRounded'
import FactCheckRounded from '@mui/icons-material/FactCheckRounded'
import BusinessRounded from '@mui/icons-material/BusinessRounded'
import WarningAmberRounded from '@mui/icons-material/WarningAmberRounded'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { adminApi, type AdminGame, type DashboardData } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'
import { gameLogo } from '../gameLogos'
import { useServerClock } from '../hooks/useServerClock'
import { normalizeDashboardData } from '../utils/dashboardData'

const stats = [
  ['用户余额', 'user_balance', AccountBalanceWalletRounded, '#5b7cec'],
  ['今日流水', 'today_turnover', TrendingUpRounded, '#e4a23e'],
  ['今日已结算流水', 'today_settled_turnover', PriceCheckRounded, '#268cb2'],
  ['今日毛利', 'today_gross_profit', QueryStatsRounded, '#35a7c8'],
  ['今日净利', 'today_net_profit', ShowChartRounded, '#df746a'],
  ['今日回水', 'today_rebate', SavingsRounded, '#2eaf7b'],
  ['今日福利', 'today_welfare', RedeemRounded, '#ee7b58'],
  ['今日代理分成', 'today_agent_share', AccountTreeRounded, '#826fd2'],
  ['未结算金额', 'pending_settlement', PendingActionsRounded, '#8a70df'],
] as const

const overviewStats = [
  ['会员', 'member_count', 'active_member_count', PeopleAltRounded, '#3c91cc'],
  ['代理', 'agent_count', 'active_agent_count', BusinessRounded, '#7a6fd1'],
  ['待审核', 'pending_application_count', '', FactCheckRounded, '#e6a13d'],
  ['客服会话', 'service_conversation_count', '', SupportAgentRounded, '#24a7a2'],
  ['房间聊天室', 'group_conversation_count', '', ForumRounded, '#a266bf'],
  ['开奖源异常', 'source_error_count', '', WarningAmberRounded, '#df6c67'],
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
  const [gameStatus, setGameStatus] = useState<'enabled' | 'disabled' | 'all'>('enabled')
  const { showMessage } = useFeedback()
  const load = useCallback(async (notify = false) => { setError(''); setLoading(true); try { setData(normalizeDashboardData(await adminApi.dashboard())); if (notify) showMessage('运营数据已刷新') } catch (reason) { setError(reason instanceof Error ? reason.message : '服务连接失败') } finally { setLoading(false) } }, [showMessage])
  useEffect(() => { void Promise.resolve().then(() => load()) }, [load])
  const gameCounts = useMemo(() => ({
    enabled: data?.games.filter(game => game.enabled).length ?? 0,
    disabled: data?.games.filter(game => !game.enabled).length ?? 0,
    all: data?.games.length ?? 0,
  }), [data])
  const filteredGames = useMemo(() => data?.games.filter(game => {
    const matchesStatus = gameStatus === 'all' || (gameStatus === 'enabled' ? game.enabled : !game.enabled)
    return matchesStatus && `${game.name}${game.code}${game.issue}`.toLowerCase().includes(query.trim().toLowerCase())
  }) ?? [], [data, gameStatus, query])
  const toggle = async (game: AdminGame) => { setUpdating(game.id); try { const next = await adminApi.updateGameStatus(game.id, !game.enabled); setData(current => current ? { ...current, games: current.games.map(item => item.id === next.id ? next : item) } : current); showMessage(`${game.name}已${next.enabled ? '开启' : '关闭'}`) } catch (reason) { setError(reason instanceof Error ? reason.message : '更新失败') } finally { setUpdating('') } }

  return <Box p={{ xs: 2, lg: 2.5 }}><PageHeader eyebrow="总览 / 实时运营" title="运营首页" description="" />
    {error && <Alert severity="error" sx={{ mt: 2 }}>{error}，请确认 backend 已在 8080 端口启动。</Alert>}
    {!data && loading && <Stack alignItems="center" py={10}><CircularProgress size={30} /></Stack>}
    {data && <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2,1fr)', md: 'repeat(3,1fr)', xl: 'repeat(6,1fr)' }, gap: 1.2, mt: 2.5 }}>
      {overviewStats.map(([label, key, activeKey, Icon, color]) => <Paper variant="outlined" key={key} sx={{ p: 1.4, display: 'flex', alignItems: 'center', gap: 1.2 }}>
        <Box sx={{ width: 34, height: 34, display: 'grid', placeItems: 'center', borderRadius: 2, color: '#fff', bgcolor: color, flexShrink: 0 }}><Icon sx={{ fontSize: 19 }} /></Box>
        <Box minWidth={0}><Typography variant="caption" color="text.secondary" noWrap>{label}</Typography><Typography fontSize={18} fontWeight={850}>{data.overview?.[key] ?? 0}{activeKey && <Typography component="span" fontSize={9} color="success.main" ml={.5}>启用 {data.overview?.[activeKey] ?? 0}</Typography>}</Typography></Box>
      </Paper>)}
    </Box>}
    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', xl: '300px minmax(0,1fr)' }, gap: 2, mt: 2 }}>
      <Card sx={{ order: { xs: 2, xl: 1 } }}><CardContent sx={{ p: 0 }}><Stack direction="row" justifyContent="space-between" alignItems="center" px={2} py={1.6} borderBottom={1} borderColor="divider"><Box><Typography fontWeight={750}>游戏状态</Typography><Typography variant="caption" color="text.secondary">运行 {gameCounts.enabled} · 已关闭 {gameCounts.disabled}</Typography></Box><Chip size="small" color="success" label="实时" /></Stack><Box px={1.5} pt={1.5}><TextField fullWidth placeholder="搜索游戏或期号" value={query} onChange={event => setQuery(event.target.value)} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> } }} /><ToggleButtonGroup exclusive fullWidth size="small" value={gameStatus} onChange={(_, value) => { if (value) setGameStatus(value) }} sx={{ mt: 1 }}><ToggleButton value="enabled">运行中 {gameCounts.enabled}</ToggleButton><ToggleButton value="disabled">已关闭 {gameCounts.disabled}</ToggleButton><ToggleButton value="all">全部 {gameCounts.all}</ToggleButton></ToggleButtonGroup></Box><Box sx={{ maxHeight: { xl: 'calc(100vh - 360px)' }, overflow: 'auto', p: 1 }}>{filteredGames.map(game => <Stack key={game.id} direction="row" alignItems="center" gap={1.2} p={1} borderRadius={2} sx={{ '&:hover': { bgcolor: 'action.hover' } }}><Avatar src={gameLogo(game.id)} alt={`${game.name} Logo`} sx={{ width: 40, height: 40, flex: '0 0 40px', bgcolor: game.badge_color || 'primary.main', border: 1, borderColor: 'divider', boxShadow: '0 2px 8px rgba(25,80,105,.13)', fontSize: 9, fontWeight: 800 }}>{game.badge.slice(0, 3)}</Avatar><Box minWidth={0} flex={1}><Stack direction="row" alignItems="center" gap={.6}><Typography fontSize={12} fontWeight={700} noWrap>{game.name}</Typography>{game.source_kind === 'official' && <Chip size="small" color="success" label="官方" sx={{ height: 17, fontSize: 8 }} />}</Stack><Typography fontSize={9} color="text.secondary" noWrap>{game.issue || '等待同步'}</Typography></Box><FormControlLabel sx={{ m: 0 }} control={<Switch inputProps={{ 'aria-label': `${game.name}运行状态` }} size="small" checked={game.enabled} disabled={updating === game.id} onChange={() => toggle(game)} />} label="" /></Stack>)}{filteredGames.length === 0 && <Typography textAlign="center" color="text.secondary" fontSize={12} py={5}>当前筛选下没有游戏</Typography>}</Box></CardContent></Card>
      <Box sx={{ minWidth: 0, order: { xs: 1, xl: 2 } }}><Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2,1fr)', md: 'repeat(3,1fr)' }, gap: 1.4 }}>{stats.map(([label, key, Icon, color]) => <Card key={key}><CardContent sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', p: '16px !important' }}><Box><Typography variant="caption" color="text.secondary">{label}</Typography><Typography mt={.5} fontSize={{ xs: 16, md: 20 }} fontWeight={800}>{data ? number(data.stats[key]) : '—'}</Typography></Box><IconButton disableRipple sx={{ color: '#fff', bgcolor: color, '&:hover': { bgcolor: color } }}><Icon /></IconButton></CardContent></Card>)}</Box>
        <Card sx={{ mt: 2 }}><Stack direction="row" justifyContent="space-between" alignItems="center" p={2} borderBottom={1} borderColor="divider"><Box><Typography fontWeight={750}>游戏运行数据</Typography><Typography variant="caption" color="text.secondary">流水不含撤单；盈亏按已结算注单计算（毛利）</Typography></Box><Chip size="small" variant="outlined" label={`${filteredGames.length} 个游戏`} /></Stack><TableContainer component={Paper} elevation={0} sx={{ maxHeight: 'calc(100vh - 390px)' }}><Table stickyHeader size="small" sx={{ minWidth: 760 }}><TableHead><TableRow><TableCell>游戏</TableCell><TableCell>最新期号</TableCell><TableCell>状态</TableCell><TableCell align="right">今日流水</TableCell><TableCell align="right">今日毛利</TableCell><TableCell align="right">开奖节奏</TableCell></TableRow></TableHead><TableBody>{filteredGames.map(game => <TableRow hover key={game.id}><TableCell><Stack direction="row" alignItems="center" gap={1}><Avatar src={gameLogo(game.id)} alt={`${game.name} Logo`} sx={{ width: 32, height: 32, bgcolor: game.badge_color || 'primary.main', border: 1, borderColor: 'divider', fontSize: 8, fontWeight: 800 }}>{game.badge.slice(0, 2)}</Avatar><Box><Typography fontSize={12} fontWeight={700}>{game.name}</Typography><Typography fontSize={9} color="text.secondary">{game.category}</Typography></Box></Stack></TableCell><TableCell sx={{ fontSize: 11 }}>{game.issue}</TableCell><TableCell><Chip size="small" color={game.enabled ? 'success' : 'default'} label={game.enabled ? '运行中' : '已关闭'} sx={{ fontSize: 9 }} /></TableCell><TableCell align="right">{number(game.turnover)}</TableCell><TableCell align="right">{number(game.profit)}</TableCell><TableCell align="right">{game.enabled ? game.schedule_mode === 'official-feed' ? <Chip size="small" color="info" variant="outlined" label="跟随官方" /> : left(game.next_draw_at, now) : '--:--'}</TableCell></TableRow>)}</TableBody></Table></TableContainer></Card>
      </Box>
    </Box>
  </Box>
}
