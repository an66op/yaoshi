import {
  Alert, Box, Button, Card, CardContent, Chip, CircularProgress, InputAdornment, MenuItem, Paper, Stack,
  Table, TableBody, TableCell, TableContainer, TableHead, TablePagination, TableRow, TextField, Typography,
} from '@mui/material'
import DownloadRounded from '@mui/icons-material/DownloadRounded'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import { useCallback, useEffect, useState } from 'react'
import { adminApi, agentApi, tenantApi, type OperatingReport } from '../api'
import { useFeedback } from './feedback'
import { ProfitSharePanel } from './ProfitSharePanel'

const money = (value: number) => new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)
const dateTime = (value?: string) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—'
const today = () => new Date().toISOString().slice(0, 10)
const daysAgo = (days: number) => { const date = new Date(); date.setDate(date.getDate() - days); return date.toISOString().slice(0, 10) }

type Filters = { query: string; start: string; end: string; roomScope: string; gameId: string; dimension: 'room' | 'game' | 'user' }
const initialFilters = (agent: boolean): Filters => ({ query: '', start: daysAgo(6), end: today(), roomScope: '', gameId: '', dimension: agent ? 'game' : 'room' })

export function OperatingReportPanel({ agent = false, tenantAgentId }: { agent?: boolean; tenantAgentId?: number }) {
  const [draft, setDraft] = useState(() => initialFilters(agent))
  const [applied, setApplied] = useState(() => initialFilters(agent))
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState(20)
  const [data, setData] = useState<OperatingReport | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const { showMessage } = useFeedback()

  const load = useCallback(async (notify = false) => {
    setLoading(true); setError('')
    try {
      const params = { ...applied, page: page + 1, pageSize }
      const result = tenantAgentId
        ? await tenantApi.roomOperatingReport(tenantAgentId, params)
        : await (agent ? agentApi : adminApi).operatingReport(params)
      setData(result)
      if (notify) showMessage('经营账单已刷新')
    } catch (reason) { setError(reason instanceof Error ? reason.message : '读取经营账单失败') }
    finally { setLoading(false) }
  }, [agent, applied, page, pageSize, showMessage, tenantAgentId])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])
  const apply = () => {
    if (draft.start && draft.end && draft.start > draft.end) { setError('结束日期不能早于开始日期'); return }
    setPage(0); setApplied({ ...draft, query: draft.query.trim(), gameId: draft.gameId.trim(), roomScope: draft.roomScope.trim() })
  }
  const selectPeriod = (days: number) => {
    const next = { ...draft, start: daysAgo(days - 1), end: today() }
    setDraft(next); setApplied(next); setPage(0)
  }
  const reset = () => { const next = initialFilters(agent); setDraft(next); setApplied(next); setPage(0) }

  const exportItems = () => {
    const escape = (value: unknown) => `"${String(value ?? '').replaceAll('"', '""')}"`
    const rows = (data?.items ?? []).map(row => [row.id, row.room_scope, row.game_name, row.issue, row.username, row.play_name, row.selection, row.stake, row.payout, row.gross_profit, row.rebate, row.agent_share, row.platform_profit, dateTime(row.settled_at)])
    const csv = [['注单', '房间', '彩种', '期号', '会员', '玩法', '选择', '有效投注', '派彩', '毛利', '回水', '代理分成', '平台净利', '结算时间'], ...rows].map(row => row.map(escape).join(',')).join('\n')
    const href = URL.createObjectURL(new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8' }))
    const link = document.createElement('a'); link.href = href; link.download = `经营账单_${data?.summary.period_start ?? today()}_${data?.summary.period_end ?? today()}.csv`; link.click(); URL.revokeObjectURL(href)
  }

  const summary = data?.summary
  const cards = [
    ['有效投注', summary?.settled_turnover ?? 0, `${summary?.settled_tickets ?? 0} 笔已结算`],
    ['派彩金额', summary?.payout ?? 0, `会员净输赢 ${money(summary?.member_net ?? 0)}`],
    ['经营毛利', summary?.gross_profit ?? 0, `毛利率 ${summary?.gross_margin ?? 0}%`],
    ['回水成本', summary?.rebate_accrued ?? 0, '按下注时冻结比例'],
    ['福利成本', summary?.welfare_cost ?? 0, '签到、红包、邀请'],
    ['代理分成', summary?.agent_share ?? 0, '按结算毛利计算'],
    ['平台净利润', summary?.platform_net_profit ?? 0, `净利率 ${summary?.net_margin ?? 0}%`],
    ['待结算敞口', summary?.pending_turnover ?? 0, `${summary?.pending_tickets ?? 0} 笔待结算`],
  ] as const
  const chartMax = Math.max(1, ...(data?.trend ?? []).flatMap(row => [Math.abs(row.gross_profit), Math.abs(row.platform_profit)]))
  const breakdownTitle = applied.dimension === 'room' ? '房间分账' : applied.dimension === 'game' ? '彩种盈利' : '会员贡献'

  return <Box>
    {error && <Alert severity="error" sx={{ mb: 1.5 }} onClose={() => setError('')}>{error}</Alert>}
    <Alert severity="info" sx={{ mb: 1.5 }}>平台净利润 = 有效投注 − 派彩 − 回水 − 福利 − 代理分成。仅统计已结算注单；待结算投注单独展示，不提前计入盈利。</Alert>
    <Paper variant="outlined" sx={{ p: 1.5 }}>
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,minmax(0,1fr))', lg: agent ? 'minmax(260px,2fr) repeat(4,minmax(135px,1fr))' : 'minmax(260px,2fr) repeat(5,minmax(125px,1fr))' }, gap: 1, alignItems: 'center' }}>
        <TextField size="small" placeholder="会员、期号、玩法或选择" value={draft.query} onChange={event => setDraft(current => ({ ...current, query: event.target.value }))} onKeyDown={event => { if (event.key === 'Enter') apply() }} sx={{ gridColumn: { sm: '1 / -1', lg: 'auto' } }} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> } }} />
        {!agent && <TextField size="small" label="房间范围" placeholder="如 agent:23" value={draft.roomScope} onChange={event => setDraft(current => ({ ...current, roomScope: event.target.value }))} />}
        <TextField size="small" label="彩种编号" placeholder="全部彩种" value={draft.gameId} onChange={event => setDraft(current => ({ ...current, gameId: event.target.value }))} />
        <TextField size="small" select label="汇总维度" value={draft.dimension} onChange={event => setDraft(current => ({ ...current, dimension: event.target.value as Filters['dimension'] }))}><MenuItem value="room" disabled={agent}>按房间</MenuItem><MenuItem value="game">按彩种</MenuItem><MenuItem value="user">按会员</MenuItem></TextField>
        <TextField size="small" type="date" label="开始" value={draft.start} onChange={event => setDraft(current => ({ ...current, start: event.target.value }))} slotProps={{ inputLabel: { shrink: true } }} />
        <TextField size="small" type="date" label="结束" value={draft.end} onChange={event => setDraft(current => ({ ...current, end: event.target.value }))} slotProps={{ inputLabel: { shrink: true } }} />
      </Box>
      <Stack direction="row" gap={.75} flexWrap="wrap" mt={1.2} alignItems="center">
        <Button size="small" variant="outlined" onClick={() => selectPeriod(1)}>今日</Button><Button size="small" variant="outlined" onClick={() => selectPeriod(7)}>近 7 天</Button><Button size="small" variant="outlined" onClick={() => selectPeriod(30)}>近 30 天</Button>
        <Box flex={1} />
        <Button size="small" onClick={reset}>重置</Button><Button size="small" variant="contained" onClick={apply} sx={{ minWidth: 88 }}>查询</Button><Button size="small" startIcon={<DownloadRounded />} disabled={!data?.items.length} onClick={exportItems}>导出当前页</Button><Button size="small" startIcon={loading ? <CircularProgress size={14} /> : <RefreshRounded />} disabled={loading} onClick={() => void load(true)}>刷新</Button>
      </Stack>
    </Paper>

    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2,1fr)', lg: 'repeat(4,1fr)' }, gap: 1.2, mt: 1.5 }}>{cards.map(([label, value, hint]) => <Card key={label}><CardContent sx={{ p: '14px !important' }}><Typography variant="caption" color="text.secondary">{label}</Typography><Typography fontSize={{ xs: 18, md: 23 }} fontWeight={900} mt={.35} color={(label === '经营毛利' || label === '平台净利润') && value < 0 ? 'error.main' : 'text.primary'}>{value > 0 && (label === '经营毛利' || label === '平台净利润') ? '+' : ''}{money(value)}</Typography><Typography variant="caption" color="text.secondary">{hint}</Typography></CardContent></Card>)}</Box>

    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', xl: 'minmax(0,1fr) minmax(430px,.95fr)' }, gap: 1.5, mt: 1.5 }}>
      <Card><CardContent><Typography fontWeight={850}>每日盈利趋势</Typography><Typography variant="caption" color="text.secondary">毛利与扣除回水、福利、代理分成后的平台净利润</Typography><Stack direction="row" alignItems="flex-end" gap={1} height={220} mt={2} sx={{ overflowX: 'auto', borderBottom: 1, borderColor: 'divider', pb: 1 }}>{(data?.trend ?? []).map(row => <Stack key={row.date} minWidth={38} height="100%" justifyContent="flex-end" alignItems="center"><Stack direction="row" gap={.35} alignItems="flex-end" flex={1}><Box sx={{ width: 10, height: `${Math.max(3, Math.abs(row.gross_profit) / chartMax * 100)}%`, bgcolor: row.gross_profit >= 0 ? 'primary.main' : 'error.main', borderRadius: '5px 5px 0 0' }} /><Box sx={{ width: 10, height: `${Math.max(3, Math.abs(row.platform_profit) / chartMax * 100)}%`, bgcolor: row.platform_profit >= 0 ? 'success.main' : 'error.dark', borderRadius: '5px 5px 0 0' }} /></Stack><Typography fontSize={9} color="text.secondary">{row.date.slice(5).replace('-', '/')}</Typography></Stack>)}{!data?.trend.length && <Stack width="100%" height="100%" justifyContent="center" alignItems="center"><Typography color="text.secondary">当前区间暂无已结算数据</Typography></Stack>}</Stack></CardContent></Card>
      <Card><CardContent><Stack direction="row" justifyContent="space-between"><Box><Typography fontWeight={850}>{breakdownTitle}</Typography><Typography variant="caption" color="text.secondary">每一层都可核对投注、派彩、三项成本与净利</Typography></Box><Chip size="small" label={`${data?.breakdown.length ?? 0} 项`} /></Stack><TableContainer sx={{ mt: 1, maxHeight: 245 }}><Table stickyHeader size="small"><TableHead><TableRow><TableCell>对象</TableCell><TableCell align="right">投注</TableCell><TableCell align="right">毛利</TableCell><TableCell align="right">回水</TableCell><TableCell align="right">福利</TableCell><TableCell align="right">代理分成</TableCell><TableCell align="right">净利</TableCell></TableRow></TableHead><TableBody>{data?.breakdown.map(row => <TableRow key={row.key} hover><TableCell><Typography fontSize={12} fontWeight={800}>{row.label}</Typography><Typography fontSize={9} color="text.secondary">{row.tickets} 注 · {row.key}</Typography></TableCell><TableCell align="right">{money(row.turnover)}</TableCell><TableCell align="right">{money(row.gross_profit)}</TableCell><TableCell align="right">{money(row.rebate)}</TableCell><TableCell align="right">{money(row.welfare)}</TableCell><TableCell align="right">{money(row.agent_share)}</TableCell><TableCell align="right"><Typography fontWeight={850} color={row.platform_profit >= 0 ? 'success.main' : 'error.main'}>{money(row.platform_profit)}</Typography></TableCell></TableRow>)}</TableBody></Table></TableContainer></CardContent></Card>
    </Box>

    {!tenantAgentId && <ProfitSharePanel key={`${agent ? 'agent' : 'admin'}:${applied.end || today()}`} agent={agent} initialDate={applied.end || today()} />}

    <Card sx={{ mt: 1.5 }}><Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" gap={1} p={2} pb={1}><Box><Typography fontWeight={850}>逐笔注单利润明细</Typography><Typography variant="caption" color="text.secondary">赔率、回水率和分成率均为下注时冻结快照，可直接追账</Typography></Box><Chip size="small" variant="outlined" label={`共 ${data?.total ?? 0} 笔`} /></Stack><TableContainer><Table size="small" sx={{ minWidth: 1260 }}><TableHead><TableRow><TableCell>注单 / 房间</TableCell><TableCell>会员</TableCell><TableCell>彩种 / 期号</TableCell><TableCell>玩法</TableCell><TableCell align="right">投注</TableCell><TableCell align="right">派彩</TableCell><TableCell align="right">毛利</TableCell><TableCell align="right">回水</TableCell><TableCell align="right">代理分成</TableCell><TableCell align="right">平台净利</TableCell><TableCell>结算时间</TableCell></TableRow></TableHead><TableBody>{data?.items.map(row => <TableRow key={row.id} hover><TableCell><Typography fontWeight={800}>#{row.id}</Typography><Typography fontSize={9} color="text.secondary">{row.room_scope || 'lobby'}</Typography></TableCell><TableCell><Typography fontSize={12}>{row.username}</Typography><Typography fontSize={9} color="text.secondary">ID {row.user_id}</Typography></TableCell><TableCell><Typography fontSize={12} fontWeight={700}>{row.game_name}</Typography><Typography fontSize={9} color="text.secondary">{row.issue}</Typography></TableCell><TableCell><Typography fontSize={11}>{row.play_name}</Typography><Typography fontSize={9} color="text.secondary">{row.selection}</Typography></TableCell><TableCell align="right">{money(row.stake)}</TableCell><TableCell align="right">{money(row.payout)}</TableCell><TableCell align="right">{money(row.gross_profit)}</TableCell><TableCell align="right"><Typography fontSize={11}>{money(row.rebate)}</Typography><Typography fontSize={9} color="text.secondary">{row.rebate_rate}%</Typography></TableCell><TableCell align="right"><Typography fontSize={11}>{money(row.agent_share)}</Typography><Typography fontSize={9} color="text.secondary">{row.agent_share_rate}%</Typography></TableCell><TableCell align="right"><Typography fontWeight={850} color={row.platform_profit >= 0 ? 'success.main' : 'error.main'}>{money(row.platform_profit)}</Typography></TableCell><TableCell>{dateTime(row.settled_at)}</TableCell></TableRow>)}{!loading && !data?.items.length && <TableRow><TableCell colSpan={11} align="center" sx={{ py: 8, color: 'text.secondary' }}>当前条件没有已结算注单</TableCell></TableRow>}</TableBody></Table></TableContainer><TablePagination component="div" count={data?.total ?? 0} page={page} onPageChange={(_, next) => setPage(next)} rowsPerPage={pageSize} onRowsPerPageChange={event => { setPageSize(Number(event.target.value)); setPage(0) }} rowsPerPageOptions={[10, 20, 50, 100]} labelRowsPerPage="每页" /></Card>
  </Box>
}
