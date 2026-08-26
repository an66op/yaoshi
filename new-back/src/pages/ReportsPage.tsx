import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
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
  Tab,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material'
import AccountBalanceWalletRounded from '@mui/icons-material/AccountBalanceWalletRounded'
import ArrowDownwardRounded from '@mui/icons-material/ArrowDownwardRounded'
import ArrowUpwardRounded from '@mui/icons-material/ArrowUpwardRounded'
import DownloadRounded from '@mui/icons-material/DownloadRounded'
import GroupsRounded from '@mui/icons-material/GroupsRounded'
import InfoOutlined from '@mui/icons-material/InfoOutlined'
import PendingActionsRounded from '@mui/icons-material/PendingActionsRounded'
import ReceiptLongRounded from '@mui/icons-material/ReceiptLongRounded'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import TrendingUpRounded from '@mui/icons-material/TrendingUpRounded'
import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { adminApi, type FinancialRecord, type FinancialReport } from '../api'
import { PageHeader } from '../components/PageHeader'
import { OperatingReportPanel } from '../components/OperatingReportPanel'
import { useFeedback } from '../components/feedback'

const ledgerLabels: Record<string, string> = {
  manual: '人工调整',
  application_credit: '申请上分',
  application_debit: '申请下分',
  bet: '下注扣款',
  bet_cancel: '撤单退款',
  settlement: '开奖派彩',
  rebate: '回水入账',
  checkin: '签到奖励',
  redpacket: '红包奖励',
  invite: '邀请奖励',
  agent_share: '代理利润分账',
}

const categoryLabels: Record<string, string> = {
  finance: '资金操作',
  betting: '投注结算',
  welfare: '福利活动',
  share: '代理分账',
  other: '其他',
}

const ledgerColors: Record<string, 'default' | 'primary' | 'success' | 'warning' | 'info' | 'secondary'> = {
  manual: 'primary',
  application_credit: 'success',
  application_debit: 'warning',
  bet: 'info',
  bet_cancel: 'info',
  settlement: 'info',
  rebate: 'secondary',
  checkin: 'secondary',
  redpacket: 'secondary',
  invite: 'secondary',
  agent_share: 'success',
}

const money = (value: number) => new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)
const dateTime = (value: string) => new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(new Date(value))
const dayText = (value: string) => value.slice(5).replace('-', '/')
const today = () => new Date().toISOString().slice(0, 10)
const daysAgo = (days: number) => { const date = new Date(); date.setDate(date.getDate() - days); return date.toISOString().slice(0, 10) }

type ReportFilters = { query: string; type: string; start: string; end: string }
const defaultFilters = (): ReportFilters => ({ query: '', type: 'all', start: daysAgo(6), end: today() })

export function ReportsPage() {
  const [view, setView] = useState<'operating' | 'ledger'>('operating')
  const [draft, setDraft] = useState<ReportFilters>(defaultFilters)
  const [applied, setApplied] = useState<ReportFilters>(defaultFilters)
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState(20)
  const [data, setData] = useState<FinancialReport | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const { showMessage } = useFeedback()

  const load = useCallback(async (notify = false) => {
    setLoading(true)
    setError('')
    try {
      const result = await adminApi.financialReport({ ...applied, page: page + 1, pageSize })
      setData(result)
      if (notify) showMessage('财务报表已刷新')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取财务报表失败')
    } finally {
      setLoading(false)
    }
  }, [applied, page, pageSize, showMessage])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])

  const apply = () => {
    if (draft.start && draft.end && draft.start > draft.end) {
      setError('结束日期不能早于开始日期')
      return
    }
    setError('')
    setPage(0)
    setApplied({ ...draft, query: draft.query.trim() })
  }
  const selectPeriod = (days: number) => {
    const next = { ...draft, start: daysAgo(days - 1), end: today() }
    setDraft(next)
    setApplied({ ...next, query: next.query.trim() })
    setPage(0)
  }
  const reset = () => {
    const next = defaultFilters()
    setDraft(next)
    setApplied(next)
    setPage(0)
  }
  const exportCurrentPage = () => {
    const records = data?.items ?? []
    const escape = (value: unknown) => `"${String(value).replaceAll('"', '""')}"`
    const rows = records.map(record => [record.id, record.username, record.nickname, ledgerLabels[record.type] ?? record.type, record.amount.toFixed(2), record.before.toFixed(2), record.after.toFixed(2), record.remark, record.operator, dateTime(record.created_at)])
    const csv = [['流水编号', '用户名', '昵称', '类型', '变动金额', '变动前', '变动后', '备注', '操作人', '发生时间'], ...rows].map(row => row.map(escape).join(',')).join('\n')
    const link = document.createElement('a')
    link.href = URL.createObjectURL(new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8' }))
    link.download = `财务流水_${data?.summary.period_start ?? today()}_${data?.summary.period_end ?? today()}.csv`
    link.click()
    URL.revokeObjectURL(link.href)
  }

  const summary = data?.summary
  const statCards = [
    ['当前用户余额', summary?.total_balance ?? 0, AccountBalanceWalletRounded, '#238dae', '所有未删除账户的实时可用余额'],
    ['区间入账', summary?.credit_amount ?? 0, ArrowUpwardRounded, '#2ba87c', `${summary?.record_count ?? 0} 笔余额流水`],
    ['区间出账', summary?.debit_amount ?? 0, ArrowDownwardRounded, '#dc786d', `${summary?.active_users ?? 0} 个发生变动的账户`],
    ['区间净变化', summary?.net_change ?? 0, TrendingUpRounded, (summary?.net_change ?? 0) >= 0 ? '#4f7edc' : '#d86868', `待审核申请 ${summary?.pending_applications ?? 0} 笔`],
  ] as const
  const chartPoints = useMemo(() => data?.trend.length && data.trend.length > 14 ? data.trend.slice(-14) : data?.trend ?? [], [data])
  const chartMax = Math.max(1, ...chartPoints.flatMap(point => [point.credit, point.debit]))

  const reportTabs = <Paper variant="outlined" sx={{ mb: 1.5 }}><Tabs value={view} onChange={(_, value: 'operating' | 'ledger') => setView(value)} sx={{ px: 1 }}><Tab value="operating" label="经营利润" /><Tab value="ledger" label="余额流水" /></Tabs></Paper>

  if (view === 'operating') return <Box p={{ xs: 2, lg: 2.5 }}>
    <PageHeader eyebrow="数据中心 / 经营" title="经营与分账报表" description="从有效投注穿透到房间、代理、会员和逐笔注单，统一核算毛利、回水、福利成本、代理分成与平台净利润。" />
    {reportTabs}
    <OperatingReportPanel />
  </Box>

  return <Box p={{ xs: 2, lg: 2.5 }}>
    <PageHeader eyebrow="数据中心 / 财务" title="数据报表" description={summary ? `统计区间：${summary.period_start} 至 ${summary.period_end} · 数据来自不可篡改的余额流水` : '统一查询账户余额、上下分和人工调整流水。'} actions={<><Button variant="outlined" startIcon={<DownloadRounded />} disabled={!data?.items.length} onClick={exportCurrentPage}>导出当前页</Button><Button variant="outlined" startIcon={loading ? <CircularProgress size={16} /> : <RefreshRounded />} disabled={loading} onClick={() => void load(true)}>刷新</Button></>} />
    {reportTabs}
    {error && <Alert severity="error" sx={{ mt: 2 }} onClose={() => setError('')}>{error}</Alert>}
    <Alert icon={<InfoOutlined />} severity="info" sx={{ mt: 2 }}>统计全部余额流水：资金操作（上下分/人工调整）、投注结算（下注/撤单/派彩）、福利活动（回水/签到/红包/邀请）和已实际入账的代理利润分账。</Alert>
    <Paper variant="outlined" sx={{ p: 1.5, mt: 1.5 }}><Stack direction={{ xs: 'column', lg: 'row' }} gap={1} alignItems={{ lg: 'center' }}><TextField placeholder="搜索用户名、备注或操作人" value={draft.query} onChange={event => setDraft(current => ({ ...current, query: event.target.value }))} onKeyDown={event => { if (event.key === 'Enter') apply() }} sx={{ flex: 1, minWidth: { lg: 220 } }} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> } }} /><TextField select label="流水类型" value={draft.type} onChange={event => setDraft(current => ({ ...current, type: event.target.value }))} sx={{ minWidth: 170 }}><MenuItem value="all">全部流水</MenuItem><MenuItem value="credit">全部入账</MenuItem><MenuItem value="debit">全部出账</MenuItem><MenuItem value="finance">资金操作</MenuItem><MenuItem value="betting">投注结算</MenuItem><MenuItem value="welfare">福利活动</MenuItem><MenuItem value="share">代理分账</MenuItem><MenuItem value="manual">人工调整</MenuItem><MenuItem value="application_credit">申请上分</MenuItem><MenuItem value="application_debit">申请下分</MenuItem><MenuItem value="bet">下注扣款</MenuItem><MenuItem value="bet_cancel">撤单退款</MenuItem><MenuItem value="settlement">开奖派彩</MenuItem><MenuItem value="rebate">回水</MenuItem><MenuItem value="agent_share">代理利润分账</MenuItem><MenuItem value="checkin">签到</MenuItem><MenuItem value="redpacket">红包</MenuItem><MenuItem value="invite">邀请</MenuItem></TextField><TextField type="date" label="开始日期" value={draft.start} onChange={event => setDraft(current => ({ ...current, start: event.target.value }))} slotProps={{ inputLabel: { shrink: true } }} sx={{ minWidth: 150 }} /><TextField type="date" label="结束日期" value={draft.end} onChange={event => setDraft(current => ({ ...current, end: event.target.value }))} slotProps={{ inputLabel: { shrink: true } }} sx={{ minWidth: 150 }} /><Button variant="contained" onClick={apply}>查询</Button><Button variant="text" onClick={reset}>重置</Button></Stack><Stack direction="row" gap={.75} flexWrap="wrap" mt={1.25}><Typography variant="caption" color="text.secondary" sx={{ alignSelf: 'center', mr: .25 }}>快捷区间</Typography><Button size="small" variant="outlined" onClick={() => selectPeriod(1)}>今日</Button><Button size="small" variant="outlined" onClick={() => selectPeriod(7)}>近 7 天</Button><Button size="small" variant="outlined" onClick={() => selectPeriod(30)}>近 30 天</Button><Typography variant="caption" color="text.secondary" sx={{ alignSelf: 'center', ml: { sm: 'auto' } }}>单次最多查询 92 天</Typography></Stack></Paper>
    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2,1fr)', lg: 'repeat(4,1fr)' }, gap: 1.25, mt: 1.5 }}>{statCards.map(([label, value, Icon, color, hint]) => <Card key={label}><CardContent sx={{ p: '15px !important' }}><Stack direction="row" justifyContent="space-between" alignItems="flex-start" gap={1}><Box minWidth={0}><Typography variant="caption" color="text.secondary">{label}</Typography><Typography fontSize={{ xs: 18, sm: 23 }} fontWeight={850} mt={.4} noWrap color={label === '区间净变化' && Number(value) < 0 ? 'error.main' : 'text.primary'}>{label === '区间净变化' && Number(value) > 0 ? '+' : ''}{money(Number(value))}</Typography><Typography variant="caption" color="text.secondary" noWrap>{hint}</Typography></Box><Box sx={{ width: 39, height: 39, borderRadius: 2.5, flex: '0 0 auto', display: 'grid', placeItems: 'center', color: '#fff', bgcolor: color }}><Icon fontSize="small" /></Box></Stack></CardContent></Card>)}</Box>
    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', xl: 'minmax(0,1fr) 310px' }, gap: 1.5, mt: 1.5 }}><Card><CardContent><Stack direction="row" alignItems="center" justifyContent="space-between"><Box><Typography fontWeight={850}>资金趋势</Typography><Typography variant="caption" color="text.secondary">绿色为入账，红色为出账；展示区间内最近 {chartPoints.length} 天</Typography></Box><Chip size="small" icon={<ReceiptLongRounded />} label={`${summary?.record_count ?? 0} 条流水`} /></Stack><Box sx={{ display: 'grid', gridTemplateColumns: `repeat(${Math.max(chartPoints.length, 1)}, minmax(22px, 1fr))`, height: 212, gap: { xs: .55, sm: 1 }, alignItems: 'end', mt: 2, pt: 2, borderBottom: 1, borderColor: 'divider' }}>{chartPoints.map(point => <Tooltip key={point.date} arrow title={<><div>{point.date}</div><div>入账：{money(point.credit)}</div><div>出账：{money(point.debit)}</div><div>流水：{point.record_count} 笔</div></>}><Stack height="100%" justifyContent="flex-end" alignItems="center" gap={.4} sx={{ cursor: 'default', minWidth: 0 }}><Stack direction="row" alignItems="flex-end" justifyContent="center" gap={.25} flex={1} width="100%"><Box sx={{ width: { xs: 7, sm: 11 }, minHeight: point.credit ? 4 : 0, height: `${Math.max(2, point.credit / chartMax * 100)}%`, borderRadius: '5px 5px 1px 1px', bgcolor: 'success.main', opacity: .86 }} /><Box sx={{ width: { xs: 7, sm: 11 }, minHeight: point.debit ? 4 : 0, height: `${Math.max(2, point.debit / chartMax * 100)}%`, borderRadius: '5px 5px 1px 1px', bgcolor: 'error.main', opacity: .78 }} /></Stack><Typography fontSize={9} color="text.secondary" noWrap>{dayText(point.date)}</Typography></Stack></Tooltip>)}{!chartPoints.length && <Stack gridColumn="1 / -1" height="100%" alignItems="center" justifyContent="center" color="text.secondary"><ReceiptLongRounded sx={{ fontSize: 36, opacity: .45 }} /><Typography variant="caption" mt={1}>当前区间暂无余额流水</Typography></Stack>}</Box><Stack direction="row" gap={1.5} justifyContent="flex-end" mt={1.2}><Legend color="success.main" label="入账" /><Legend color="error.main" label="出账" /></Stack></CardContent></Card><Card><CardContent><Typography fontWeight={850}>分类汇总</Typography><Stack gap={1.3} mt={1.5}><Metric icon={<GroupsRounded fontSize="small" />} label="变动账户" value={`${summary?.active_users ?? 0} 个`} /><Metric icon={<ReceiptLongRounded fontSize="small" />} label="流水笔数" value={`${summary?.record_count ?? 0} 笔`} /><Metric icon={<PendingActionsRounded fontSize="small" />} label="待审核申请" value={`${summary?.pending_applications ?? 0} 笔`} /><Metric icon={<ArrowUpwardRounded fontSize="small" />} label="资金入账" value={money(summary?.finance_credit ?? 0)} /><Metric icon={<ArrowDownwardRounded fontSize="small" />} label="资金出账" value={money(summary?.finance_debit ?? 0)} /><Metric icon={<TrendingUpRounded fontSize="small" />} label="投注派彩" value={money(summary?.betting_credit ?? 0)} /><Metric icon={<TrendingUpRounded fontSize="small" />} label="投注扣款" value={money(summary?.betting_debit ?? 0)} /><Metric icon={<TrendingUpRounded fontSize="small" />} label="福利发放" value={money(summary?.welfare_credit ?? 0)} /><Metric icon={<AccountBalanceWalletRounded fontSize="small" />} label="代理分账入账" value={money(summary?.agent_share_credit ?? 0)} /></Stack><Alert severity="warning" icon={false} sx={{ mt: 1.75, py: .5 }}><Typography fontSize={11}>当前余额是实时值，不受筛选日期影响；分类汇总按当前筛选区间计算。</Typography></Alert></CardContent></Card></Box>
    <Card sx={{ mt: 1.5 }}>{loading && <Box px={2} py={1}><CircularProgress size={18} /></Box>}<Stack direction={{ xs: 'column', sm: 'row' }} gap={1} justifyContent="space-between" alignItems={{ sm: 'center' }} p={2} pb={1}><Box><Typography fontWeight={850}>余额流水明细</Typography><Typography variant="caption" color="text.secondary">每笔变动均保留变动前后金额和操作来源</Typography></Box><Chip size="small" label={`共 ${data?.total ?? 0} 条`} variant="outlined" /></Stack><TableContainer><Table size="small" sx={{ minWidth: 1030 }}><TableHead><TableRow><TableCell>用户</TableCell><TableCell>来源</TableCell><TableCell align="right">变动金额</TableCell><TableCell align="right">余额变化</TableCell><TableCell>备注</TableCell><TableCell>操作人</TableCell><TableCell>发生时间</TableCell></TableRow></TableHead><TableBody>{data?.items.map(record => <FinancialRow key={record.id} record={record} />)}{!loading && !data?.items.length && <TableRow><TableCell colSpan={7}><Stack minHeight={190} alignItems="center" justifyContent="center" color="text.secondary"><ReceiptLongRounded sx={{ fontSize: 40, opacity: .45 }} /><Typography fontWeight={750} mt={1}>当前筛选条件下暂无余额流水</Typography><Typography variant="caption">审核上分、下分或人工调整后，数据会自动出现在这里</Typography></Stack></TableCell></TableRow>}</TableBody></Table></TableContainer><TablePagination component="div" count={data?.total ?? 0} page={page} onPageChange={(_, next) => setPage(next)} rowsPerPage={pageSize} onRowsPerPageChange={event => { setPageSize(Number(event.target.value)); setPage(0) }} rowsPerPageOptions={[10, 20, 50]} labelRowsPerPage="每页" /></Card>
  </Box>
}

function FinancialRow({ record }: { record: FinancialRecord }) {
  const positive = record.amount >= 0
  return <TableRow hover><TableCell><Typography fontSize={12} fontWeight={800}>{record.nickname || record.username}</Typography><Typography fontSize={10} color="text.secondary">@{record.username} · ID {record.user_id}</Typography></TableCell><TableCell><Stack direction="row" gap={.5} flexWrap="wrap"><Chip size="small" label={categoryLabels[record.category ?? 'other'] ?? '其他'} variant="outlined" /><Chip size="small" label={ledgerLabels[record.type] ?? record.type} color={ledgerColors[record.type] ?? 'default'} variant="outlined" /></Stack></TableCell><TableCell align="right"><Typography fontWeight={850} color={positive ? 'success.main' : 'error.main'}>{positive ? '+' : ''}{money(record.amount)}</Typography></TableCell><TableCell align="right"><Typography fontSize={12}>{money(record.before)} <Box component="span" color="text.secondary">→</Box> {money(record.after)}</Typography></TableCell><TableCell><Typography fontSize={11} sx={{ maxWidth: 250 }} noWrap>{record.remark || '—'}</Typography></TableCell><TableCell><Typography fontSize={11}>{record.operator || '系统'}</Typography></TableCell><TableCell><Typography fontSize={11}>{dateTime(record.created_at)}</Typography></TableCell></TableRow>
}

function Legend({ color, label }: { color: string; label: string }) {
  return <Stack direction="row" alignItems="center" gap={.5}><Box sx={{ width: 8, height: 8, borderRadius: 1, bgcolor: color }} /><Typography variant="caption" color="text.secondary">{label}</Typography></Stack>
}

function Metric({ icon, label, value }: { icon: ReactNode; label: string; value: string }) {
  return <Stack direction="row" alignItems="center" gap={1.1}><Box sx={{ width: 32, height: 32, borderRadius: 2, display: 'grid', placeItems: 'center', color: 'primary.main', bgcolor: 'primary.light', opacity: .9 }}>{icon}</Box><Typography variant="caption" color="text.secondary" flex={1}>{label}</Typography><Typography fontSize={13} fontWeight={850}>{value}</Typography></Stack>
}
