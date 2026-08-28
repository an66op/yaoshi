import {
  Alert, Box, Button, Card, CardContent, Chip, CircularProgress, Divider, InputAdornment, MenuItem,
  Paper, Stack, Table, TableBody, TableCell, TableContainer, TableHead, TablePagination, TableRow,
  TextField, Typography,
} from '@mui/material'
import AssessmentRounded from '@mui/icons-material/AssessmentRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  adminApi, agentApi, tenantApi, type AgentItem, type ReportCenterParams, type ReportCenterResult,
  type ReportDefinition, type TenantItem,
} from '../api'
import { getStoredUser } from '../auth'
import { PageHeader } from '../components/PageHeader'
import { normalizeReportResult } from '../utils/reportData'

const reportGroups = ['经营分析', '财务结算', '风控会员', '系统审计']
const pad = (value: number) => String(value).padStart(2, '0')
const dateValue = (date: Date) => `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
const today = () => dateValue(new Date())
const daysAgo = (days: number) => { const value = new Date(); value.setDate(value.getDate() - days); return dateValue(value) }
const defaultCatalog: ReportDefinition[] = [
  ['summary', '总报表', '经营分析'], ['users', '用户报表', '经营分析'], ['entertainment', '娱乐报表', '经营分析'], ['28', '28报表', '经营分析'], ['categories', '分类报表', '经营分析'], ['unsettled', '未结报表', '经营分析'],
  ['financial', '财务报表', '财务结算'], ['commission', '返佣报表', '财务结算'], ['redpackets', '红包报表', '财务结算'], ['rebates', '回水报表', '财务结算'], ['entertainment-rebates', '娱乐回水', '财务结算'], ['28-rebates', '28回水', '财务结算'],
  ['alerts', '告警报表', '风控会员'], ['new-members', '新会员统计', '风控会员'], ['daily-members', '当日会员概要', '风控会员'], ['logs', '日志报表', '系统审计'],
].map(([key, title, group]) => ({ key, title, group }))

type Filters = { query: string; start: string; end: string; workspaceId: number; gameId: string; category: string; issue: string; status: string }
const initialFilters = (): Filters => {
  const query = new URLSearchParams(window.location.search)
  return {
    query: query.get('query') ?? '', start: query.get('start') ?? daysAgo(6), end: query.get('end') ?? today(),
    workspaceId: Number(query.get('workspace_id') ?? 0), gameId: query.get('game_id') ?? '', category: query.get('category') ?? '',
    issue: query.get('issue') ?? '', status: query.get('status') ?? 'all',
  }
}

const roleApi = (role: string) => role === 'agent' ? agentApi : role === 'tenant' ? tenantApi : adminApi
const formatCell = (value: unknown) => {
  if (value === null || value === undefined || value === '') return '—'
  if (typeof value === 'boolean') return value ? '是' : '否'
  if (typeof value === 'number') return Number.isInteger(value) ? value.toLocaleString('zh-CN') : value.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
  if (typeof value === 'string' && /^\d{4}-\d{2}-\d{2}T/.test(value)) return new Date(value).toLocaleString('zh-CN', { hour12: false })
  return String(value)
}

export function ReportsPage({ initialReport }: { initialReport?: string } = {}) {
  const role = getStoredUser()?.role ?? 'admin'
  const api = useMemo(() => roleApi(role), [role])
  const url = new URLSearchParams(window.location.search)
  const [catalog, setCatalog] = useState<ReportDefinition[]>(defaultCatalog)
  const [reportKey, setReportKey] = useState(url.get('report') ?? initialReport ?? 'summary')
  const [draft, setDraft] = useState<Filters>(initialFilters)
  const [filters, setFilters] = useState<Filters>(initialFilters)
  const [data, setData] = useState<ReportCenterResult | null>(null)
  const [workspaces, setWorkspaces] = useState<Array<{ id: number; label: string }>>([])
  const [page, setPage] = useState(Math.max(0, Number(url.get('page') ?? 1) - 1))
  const [pageSize, setPageSize] = useState(Number(url.get('page_size') ?? 20))
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    void api.reportCatalog().then(items => setCatalog(Array.isArray(items) && items.length ? items : defaultCatalog)).catch(() => setCatalog(defaultCatalog))
    if (role !== 'admin') return
    void Promise.all([adminApi.tenants({ pageSize: 100 }), adminApi.agents({ pageSize: 100 })]).then(([tenants, agents]) => {
      const tenantRooms = (Array.isArray(tenants.items) ? tenants.items : []).filter((item: TenantItem) => item.workspace_id).map((item: TenantItem) => ({ id: item.workspace_id, label: `租户直属 · ${item.room_code || '未分配'} · ${item.room_name || item.nickname || item.username}` }))
      const agentRooms = (Array.isArray(agents.items) ? agents.items : []).filter((item: AgentItem) => item.workspace_id).map((item: AgentItem) => ({ id: item.workspace_id, label: `代理房间 · ${item.room_code} · ${item.room_name || item.nickname || item.username}` }))
      setWorkspaces([...tenantRooms, ...agentRooms])
    }).catch(() => setWorkspaces([]))
  }, [api, role])

  const params = useMemo<ReportCenterParams>(() => ({
    query: filters.query, start: filters.start, end: filters.end, workspaceId: role === 'admin' ? filters.workspaceId : undefined,
    gameId: filters.gameId, category: filters.category, issue: filters.issue, status: filters.status,
    page: page + 1, pageSize,
  }), [filters, page, pageSize, role])

  const syncUrl = useCallback((nextKey: string, next: Filters, nextPage: number, nextSize: number) => {
    const query = new URLSearchParams({ report: nextKey, start: next.start, end: next.end, page: String(nextPage + 1), page_size: String(nextSize) })
    if (next.query) query.set('query', next.query)
    if (next.workspaceId) query.set('workspace_id', String(next.workspaceId))
    if (next.gameId) query.set('game_id', next.gameId)
    if (next.category) query.set('category', next.category)
    if (next.issue) query.set('issue', next.issue)
    if (next.status !== 'all') query.set('status', next.status)
    window.history.replaceState({}, '', `${window.location.pathname}?${query}`)
  }, [])

  const load = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const result = normalizeReportResult(await api.reportCenter(reportKey, params))
      setData(result)
      syncUrl(reportKey, filters, page, pageSize)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取报表失败')
    } finally { setLoading(false) }
  }, [api, filters, page, pageSize, params, reportKey, setData, syncUrl])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])
  const selectReport = (key: string) => { setReportKey(key); setPage(0); setData(null) }
  const apply = () => {
    if (draft.start > draft.end) { setError('结束日期不能早于开始日期'); return }
    setFilters({ ...draft, query: draft.query.trim(), issue: draft.issue.trim() }); setPage(0)
  }
  const period = (days: number, offset = 0) => {
    const end = daysAgo(offset); const start = daysAgo(offset + days - 1)
    const next = { ...draft, start, end }; setDraft(next); setFilters(next); setPage(0)
  }
  const definition = catalog.find(item => item.key === reportKey) ?? defaultCatalog[0]

  return <Box p={{ xs: 1.5, md: 2.5 }}>
    <PageHeader eyebrow="数据中心" title="报表中心" description="" />
    {error && <Alert severity="error" action={<Button color="inherit" size="small" onClick={() => void load()}>重试</Button>} sx={{ mt: 1.5 }}>{error}</Alert>}
    <TextField select fullWidth size="small" label="选择报表" value={reportKey} onChange={event => selectReport(event.target.value)} sx={{ display: { xs: 'flex', lg: 'none' }, mt: 1.5 }}>
      {reportGroups.map(group => [<MenuItem key={`${group}-head`} disabled>{group}</MenuItem>, ...catalog.filter(item => item.group === group).map(item => <MenuItem key={item.key} value={item.key}>{item.title}</MenuItem>)])}
    </TextField>
    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'minmax(0,1fr)', lg: '252px minmax(0,1fr)' }, alignItems: 'start', gap: 1.8, mt: 1.5 }}>
      <Paper variant="outlined" sx={{ display: { xs: 'none', lg: 'block' }, p: 1.25, position: 'sticky', top: 88, borderRadius: 2.5 }}>
        <Typography fontSize={16} fontWeight={850} px={1} pt={.35} pb={1}>报表目录</Typography>
        {reportGroups.map(group => {
          const groupItems = catalog.filter(item => item.group === group)
          return <Box key={group} mb={1.25}>
            <Typography variant="caption" color="text.secondary" fontWeight={850} display="block" px={1} mb={.55} sx={{ borderLeft: 3, borderColor: 'primary.main', lineHeight: 1.2 }}>{group}</Typography>
            <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(2,minmax(0,1fr))', gap: .65 }}>
              {groupItems.map((item, index) => {
                const spansRow = groupItems.length % 2 === 1 && index === groupItems.length - 1
                const selected = reportKey === item.key
                return <Button key={item.key} fullWidth onClick={() => selectReport(item.key)} variant={selected ? 'contained' : 'outlined'} sx={{ gridColumn: spansRow ? '1 / -1' : 'auto', minHeight: 40, px: .7, justifyContent: 'center', borderRadius: 1.6, borderColor: selected ? 'primary.main' : 'divider', color: selected ? 'primary.contrastText' : 'text.primary', bgcolor: selected ? undefined : 'background.default', fontSize: 13.5, fontWeight: selected ? 850 : 650, whiteSpace: 'nowrap' }}>{item.title}</Button>
              })}
            </Box>
          </Box>
        })}
      </Paper>
      <Box minWidth={0}>
        <Paper variant="outlined" sx={{ p: 1.4 }}><Stack direction={{ xs: 'column', xl: 'row' }} gap={1}><TextField size="small" placeholder="搜索用户、期号或记录" value={draft.query} onChange={event => setDraft(current => ({ ...current, query: event.target.value }))} sx={{ flex: 1, minWidth: 190 }} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> } }} />{role === 'admin' && <TextField select size="small" label="房间" value={draft.workspaceId} onChange={event => setDraft(current => ({ ...current, workspaceId: Number(event.target.value) }))} sx={{ minWidth: 230 }}><MenuItem value={0}>全部房间</MenuItem>{workspaces.map(item => <MenuItem key={item.id} value={item.id}>{item.label}</MenuItem>)}</TextField>}<TextField size="small" type="date" label="开始" value={draft.start} onChange={event => setDraft(current => ({ ...current, start: event.target.value }))} slotProps={{ inputLabel: { shrink: true } }} /><TextField size="small" type="date" label="结束" value={draft.end} onChange={event => setDraft(current => ({ ...current, end: event.target.value }))} slotProps={{ inputLabel: { shrink: true } }} /><Button variant="contained" onClick={apply}>查询</Button></Stack><Stack direction="row" gap={.6} flexWrap="wrap" mt={1}><Button size="small" onClick={() => period(1)}>今日</Button><Button size="small" onClick={() => period(1, 1)}>昨日</Button><Button size="small" onClick={() => period(7)}>近 7 天</Button><Button size="small" onClick={() => period(30)}>近 30 天</Button><Divider orientation="vertical" flexItem /><TextField size="small" placeholder="彩种 ID" value={draft.gameId} onChange={event => setDraft(current => ({ ...current, gameId: event.target.value }))} sx={{ width: 130 }} /><TextField size="small" placeholder="分类" value={draft.category} onChange={event => setDraft(current => ({ ...current, category: event.target.value }))} sx={{ width: 110 }} /><TextField size="small" placeholder="期号" value={draft.issue} onChange={event => setDraft(current => ({ ...current, issue: event.target.value }))} sx={{ width: 135 }} /><TextField select size="small" value={draft.status} onChange={event => setDraft(current => ({ ...current, status: event.target.value }))} sx={{ width: 120 }}><MenuItem value="all">全部状态</MenuItem><MenuItem value="pending">待处理</MenuItem><MenuItem value="settling">结算中</MenuItem><MenuItem value="won">已中奖</MenuItem><MenuItem value="lost">未中奖</MenuItem><MenuItem value="abnormal">异常</MenuItem></TextField></Stack></Paper>
        <Stack direction="row" justifyContent="space-between" alignItems="center" mt={1.6} mb={1}><Box><Typography variant="h6" fontWeight={850}>{definition.title}</Typography><Typography variant="caption" color="text.secondary">{data ? `${data.period_start} 至 ${data.period_end}` : '正在加载统计区间'}</Typography></Box><Chip icon={<AssessmentRounded />} label={`共 ${data?.total ?? 0} 条`} variant="outlined" /></Stack>
        <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2,minmax(0,1fr))', md: 'repeat(3,minmax(0,1fr))', xl: 'repeat(6,minmax(0,1fr))' }, gap: 1 }}>{(data?.metrics ?? []).map(metric => <Card key={metric.key} variant="outlined"><CardContent sx={{ p: '13px !important' }}><Typography variant="caption" color="text.secondary">{metric.label}</Typography><Typography fontSize={{ xs: 17, md: 20 }} fontWeight={850} mt={.4} noWrap>{formatCell(metric.value)}</Typography></CardContent></Card>)}</Box>
        <Card sx={{ mt: 1.3 }}>{loading && <Box px={2} pt={1}><CircularProgress size={18} /></Box>}<TableContainer><Table size="small" sx={{ minWidth: Math.max(720, (data?.columns ?? []).length * 145) }}><TableHead><TableRow>{(data?.columns ?? []).map(column => <TableCell key={column.key}>{column.label}</TableCell>)}</TableRow></TableHead><TableBody>{(data?.items ?? []).map((row, index) => <TableRow hover key={String(row.id ?? `${reportKey}-${index}`)}>{(data?.columns ?? []).map(column => <TableCell key={column.key}><Typography fontSize={11.5} fontWeight={column.key === 'username' || column.key === 'id' ? 750 : 500} noWrap>{formatCell(row[column.key])}</Typography></TableCell>)}</TableRow>)}{!loading && !(data?.items ?? []).length && <TableRow><TableCell colSpan={Math.max(1, (data?.columns ?? []).length)}><Stack minHeight={220} alignItems="center" justifyContent="center" color="text.secondary"><AssessmentRounded sx={{ fontSize: 42, opacity: .4 }} /><Typography mt={1}>当前条件暂无记录</Typography></Stack></TableCell></TableRow>}</TableBody></Table></TableContainer><TablePagination component="div" count={data?.total ?? 0} page={page} onPageChange={(_, next) => setPage(next)} rowsPerPage={pageSize} onRowsPerPageChange={event => { setPageSize(Number(event.target.value)); setPage(0) }} rowsPerPageOptions={[10, 20, 50, 100]} labelRowsPerPage="每页" /></Card>
      </Box>
    </Box>
  </Box>
}
