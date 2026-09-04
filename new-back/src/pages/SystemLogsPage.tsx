import {
  Alert, Box, Button, Card, CardContent, Chip, CircularProgress, InputAdornment, LinearProgress, MenuItem,
  Paper, Stack, Tab, Table, TableBody, TableCell, TableContainer, TableHead, TableRow, Tabs, TextField, Typography,
} from '@mui/material'
import ErrorOutlineRounded from '@mui/icons-material/ErrorOutlineRounded'
import HistoryRounded from '@mui/icons-material/HistoryRounded'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import TaskAltRounded from '@mui/icons-material/TaskAltRounded'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { adminApi, type AdminGame, type AuditLog, type SystemLogItem, type SystemLogParams } from '../api'
import {
  dateBoundaryISO, filterAuditLogs, mergeLogPage, sourceLogMatchesQuery, systemLogEventLabel, systemLogStatusLabel,
} from '../systemLogs'

type LogTab = 'source' | 'operation'
type SourceFilters = {
  query: string
  category: string
  type: string
  status: string
  gameId: string
  sourceGroup: string
  start: string
  end: string
}

const pad = (value: number) => String(value).padStart(2, '0')
const localDate = (date: Date) => `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
const initialSourceFilters = (): SourceFilters => {
  const end = new Date()
  const start = new Date(end)
  start.setDate(start.getDate() - 6)
  return { query: '', category: '', type: '', status: '', gameId: '', sourceGroup: '', start: localDate(start), end: localDate(end) }
}
const formatTime = (value?: string) => {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false })
}
const errorText = (reason: unknown, fallback: string) => reason instanceof Error ? reason.message : fallback
const sourceStatusColor = (status: string): 'error' | 'success' | 'warning' | 'info' | 'default' => status === 'error' ? 'error' : status === 'ok' ? 'success' : ['standby', 'stopped'].includes(status) ? 'warning' : status === 'started' ? 'info' : 'default'
const auditStatusColor = (status: number): 'success' | 'warning' | 'error' => status < 400 ? 'success' : status < 500 ? 'warning' : 'error'

export function SystemLogsPage() {
  const [tab, setTab] = useState<LogTab>('source')
  const [games, setGames] = useState<AdminGame[]>([])
  const [sourceDraft, setSourceDraft] = useState<SourceFilters>(initialSourceFilters)
  const [sourceFilters, setSourceFilters] = useState<SourceFilters>(initialSourceFilters)
  const [sourceRows, setSourceRows] = useState<SystemLogItem[]>([])
  const [sourceHasMore, setSourceHasMore] = useState(false)
  const [sourceLoading, setSourceLoading] = useState(true)
  const [sourceLoadingMore, setSourceLoadingMore] = useState(false)
  const [sourceError, setSourceError] = useState('')
  const [sourceFilterError, setSourceFilterError] = useState('')
  const [auditRows, setAuditRows] = useState<AuditLog[]>([])
  const [auditHasMore, setAuditHasMore] = useState(false)
  const [auditLoading, setAuditLoading] = useState(true)
  const [auditLoadingMore, setAuditLoadingMore] = useState(false)
  const [auditError, setAuditError] = useState('')
  const [auditQuery, setAuditQuery] = useState('')
  const [auditStatus, setAuditStatus] = useState('')
  const sourceSentinel = useRef<HTMLDivElement | null>(null)
  const auditSentinel = useRef<HTMLDivElement | null>(null)
  const sourceVersion = useRef(0)
  const auditVersion = useRef(0)
  const sourceMorePending = useRef(false)
  const auditMorePending = useRef(false)
  const sourceNextRef = useRef<number | undefined>(undefined)
  const sourceHasMoreRef = useRef(false)
  const auditNextRef = useRef<number | undefined>(undefined)
  const auditHasMoreRef = useRef(false)

  const gameNames = useMemo(() => new Map(games.map(game => [game.id, game.name])), [games])
  const visibleSourceRows = useMemo(() => sourceRows.filter(row => sourceLogMatchesQuery(row, sourceFilters.query, gameNames.get(row.game_id ?? '') ?? '')), [gameNames, sourceFilters.query, sourceRows])
  const visibleAuditRows = useMemo(() => filterAuditLogs(auditRows, auditQuery, auditStatus), [auditQuery, auditRows, auditStatus])

  useEffect(() => {
    let active = true
    void adminApi.games().then(value => { if (active) setGames(Array.isArray(value) ? value : []) }).catch(() => { if (active) setGames([]) })
    return () => { active = false }
  }, [])

  const sourceParams = useCallback((beforeId?: number): SystemLogParams => ({
    beforeId, limit: 50, category: sourceFilters.category, type: sourceFilters.type, status: sourceFilters.status,
    gameId: sourceFilters.gameId, sourceGroup: sourceFilters.sourceGroup.trim(),
    from: dateBoundaryISO(sourceFilters.start), to: dateBoundaryISO(sourceFilters.end, true),
    query: sourceFilters.query.trim().slice(0, 80),
  }), [sourceFilters])

  const loadSource = useCallback(async (reset: boolean) => {
    if (!reset && (!sourceHasMoreRef.current || !sourceNextRef.current || sourceMorePending.current)) return
    const version = reset ? ++sourceVersion.current : sourceVersion.current
    if (reset) { setSourceLoading(true); setSourceError('') }
    else { sourceMorePending.current = true; setSourceLoadingMore(true) }
    try {
      const page = await adminApi.systemLogs(sourceParams(reset ? undefined : sourceNextRef.current))
      if (version !== sourceVersion.current) return
      const incoming = Array.isArray(page?.items) ? page.items : []
      setSourceRows(current => mergeLogPage(current, incoming, reset))
      const hasMore = Boolean(page?.has_more && page?.next_before_id)
      sourceHasMoreRef.current = hasMore; sourceNextRef.current = page?.next_before_id
      setSourceHasMore(hasMore)
      setSourceError('')
    } catch (reason) {
      if (version === sourceVersion.current) setSourceError(errorText(reason, '读取开奖源日志失败'))
    } finally {
      if (reset && version === sourceVersion.current) setSourceLoading(false)
      if (!reset) { sourceMorePending.current = false; if (version === sourceVersion.current) setSourceLoadingMore(false) }
    }
  }, [sourceParams])

  const loadAudit = useCallback(async (reset: boolean) => {
    if (!reset && (!auditHasMoreRef.current || !auditNextRef.current || auditMorePending.current)) return
    const version = reset ? ++auditVersion.current : auditVersion.current
    if (reset) { setAuditLoading(true); setAuditError('') }
    else { auditMorePending.current = true; setAuditLoadingMore(true) }
    try {
      const page = await adminApi.auditLogs(reset ? undefined : auditNextRef.current, 50)
      if (version !== auditVersion.current) return
      const incoming = Array.isArray(page?.items) ? page.items : []
      setAuditRows(current => mergeLogPage(current, incoming, reset))
      const hasMore = Boolean(page?.has_more && page?.next_before_id)
      auditHasMoreRef.current = hasMore; auditNextRef.current = page?.next_before_id
      setAuditHasMore(hasMore)
      setAuditError('')
    } catch (reason) {
      if (version === auditVersion.current) setAuditError(errorText(reason, '读取操作日志失败'))
    } finally {
      if (reset && version === auditVersion.current) setAuditLoading(false)
      if (!reset) { auditMorePending.current = false; if (version === auditVersion.current) setAuditLoadingMore(false) }
    }
  }, [])

  useEffect(() => { const timer = window.setTimeout(() => void loadSource(true), 0); return () => window.clearTimeout(timer) }, [loadSource])
  useEffect(() => { const timer = window.setTimeout(() => void loadAudit(true), 0); return () => window.clearTimeout(timer) }, [loadAudit])

  useEffect(() => {
    const target = sourceSentinel.current
    if (tab !== 'source' || !sourceHasMore || !target || typeof IntersectionObserver === 'undefined') return
    let consumed = false
    const observer = new IntersectionObserver(entries => {
      if (consumed || !entries.some(entry => entry.isIntersecting)) return
      consumed = true; observer.disconnect(); void loadSource(false)
    }, { rootMargin: '260px 0px' })
    observer.observe(target)
    return () => observer.disconnect()
  }, [loadSource, sourceHasMore, sourceRows.length, tab])

  useEffect(() => {
    const target = auditSentinel.current
    if (tab !== 'operation' || !auditHasMore || !target || typeof IntersectionObserver === 'undefined') return
    let consumed = false
    const observer = new IntersectionObserver(entries => {
      if (consumed || !entries.some(entry => entry.isIntersecting)) return
      consumed = true; observer.disconnect(); void loadAudit(false)
    }, { rootMargin: '260px 0px' })
    observer.observe(target)
    return () => observer.disconnect()
  }, [auditHasMore, auditRows.length, loadAudit, tab])

  const applySourceFilters = () => {
    if (sourceDraft.start && sourceDraft.end && sourceDraft.start > sourceDraft.end) {
      setSourceFilterError('结束日期不能早于开始日期')
      return
    }
    setSourceFilterError('')
    setSourceFilters({ ...sourceDraft, query: sourceDraft.query.trim(), sourceGroup: sourceDraft.sourceGroup.trim() })
  }
  const resetSourceFilters = () => {
    const next = initialSourceFilters()
    setSourceDraft(next); setSourceFilters(next); setSourceFilterError('')
  }
  const abnormalLoaded = sourceRows.filter(row => row.status === 'error').length
  const recoveredLoaded = sourceRows.filter(row => row.status === 'ok').length

  return <Box p={{ xs: 1.5, md: 2.5 }}>
    <Card variant="outlined">
      <CardContent>
        <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" alignItems={{ sm: 'center' }} gap={1.5}>
          <Box>
            <Typography variant="h5" fontWeight={900}>日志</Typography>
            <Typography variant="body2" color="text.secondary" mt={.4}>集中查看开奖源异常、恢复、调度状态与后台操作凭证。</Typography>
          </Box>
          <Button variant="outlined" startIcon={<RefreshRounded />} onClick={() => void (tab === 'source' ? loadSource(true) : loadAudit(true))} disabled={tab === 'source' ? sourceLoading : auditLoading}>刷新当前</Button>
        </Stack>
      </CardContent>
    </Card>

    <Paper variant="outlined" sx={{ mt: 1.5 }}>
      <Tabs value={tab} onChange={(_, value: LogTab) => setTab(value)} variant="scrollable" scrollButtons="auto">
        <Tab value="source" label="开奖源日志" icon={<HistoryRounded fontSize="small" />} iconPosition="start" />
        <Tab value="operation" label="操作日志" icon={<TaskAltRounded fontSize="small" />} iconPosition="start" />
      </Tabs>
    </Paper>

    {tab === 'source' ? <>
      <Alert severity="info" sx={{ mt: 1.5 }}>只记录状态转换：首次异常、异常恢复、调度启动和待命；正常的高频轮询不重复写日志。</Alert>
      <Paper variant="outlined" sx={{ p: 1.4, mt: 1.25 }}>
        <Stack direction={{ xs: 'column', xl: 'row' }} gap={1}>
          <TextField size="small" label="原因" placeholder="搜索异常原因或状态说明" value={sourceDraft.query} onChange={event => setSourceDraft(current => ({ ...current, query: event.target.value }))} sx={{ minWidth: 230, flex: 1 }} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> }, htmlInput: { maxLength: 80 } }} />
          <TextField select size="small" label="分类" value={sourceDraft.category} onChange={event => setSourceDraft(current => ({ ...current, category: event.target.value }))} sx={{ minWidth: 130 }}><MenuItem value="">全部</MenuItem><MenuItem value="source">开奖源</MenuItem><MenuItem value="scheduler">调度器</MenuItem></TextField>
          <TextField select size="small" label="状态" value={sourceDraft.status} onChange={event => setSourceDraft(current => ({ ...current, status: event.target.value }))} sx={{ minWidth: 130 }}><MenuItem value="">全部</MenuItem><MenuItem value="error">异常</MenuItem><MenuItem value="ok">已恢复</MenuItem><MenuItem value="started">已启动</MenuItem><MenuItem value="standby">待命</MenuItem><MenuItem value="stopped">已停止</MenuItem></TextField>
          <TextField select size="small" label="事件" value={sourceDraft.type} onChange={event => setSourceDraft(current => ({ ...current, type: event.target.value }))} sx={{ minWidth: 155 }}><MenuItem value="">全部</MenuItem><MenuItem value="sync_error">开奖源异常</MenuItem><MenuItem value="sync_recovered">开奖源恢复</MenuItem><MenuItem value="scheduler_error">调度异常</MenuItem><MenuItem value="scheduler_recovered">调度恢复</MenuItem><MenuItem value="scheduler_started">调度启动</MenuItem><MenuItem value="scheduler_stopped">调度停止</MenuItem><MenuItem value="standby">调度待命</MenuItem></TextField>
        </Stack>
        <Stack direction={{ xs: 'column', lg: 'row' }} gap={1} mt={1}>
          <TextField select size="small" label="彩种" value={sourceDraft.gameId} onChange={event => setSourceDraft(current => ({ ...current, gameId: event.target.value }))} sx={{ minWidth: 210 }}><MenuItem value="">全部彩种</MenuItem>{games.map(game => <MenuItem key={game.id} value={game.id}>{game.name} · {game.id}</MenuItem>)}</TextField>
          <TextField size="small" label="来源组" placeholder="例如 pc28-163" value={sourceDraft.sourceGroup} onChange={event => setSourceDraft(current => ({ ...current, sourceGroup: event.target.value }))} sx={{ minWidth: 170 }} />
          <TextField size="small" type="date" label="开始日期" value={sourceDraft.start} onChange={event => setSourceDraft(current => ({ ...current, start: event.target.value }))} slotProps={{ inputLabel: { shrink: true } }} />
          <TextField size="small" type="date" label="结束日期" value={sourceDraft.end} onChange={event => setSourceDraft(current => ({ ...current, end: event.target.value }))} slotProps={{ inputLabel: { shrink: true } }} />
          <Stack direction="row" gap={1}><Button variant="contained" onClick={applySourceFilters}>查询</Button><Button onClick={resetSourceFilters}>重置</Button></Stack>
        </Stack>
        {sourceFilterError && <Typography color="error" variant="caption" display="block" mt={1}>{sourceFilterError}</Typography>}
      </Paper>

      <Stack direction="row" useFlexGap flexWrap="wrap" gap={1} my={1.4}>
        <Chip label={`已加载 ${sourceRows.length} 条`} />
        <Chip color="error" variant="outlined" icon={<ErrorOutlineRounded />} label={`异常 ${abnormalLoaded}`} />
        <Chip color="success" variant="outlined" icon={<TaskAltRounded />} label={`恢复 ${recoveredLoaded}`} />
        {sourceFilters.query && <Chip variant="outlined" label={`关键字命中 ${visibleSourceRows.length}`} />}
      </Stack>
      {sourceError && <Alert severity="error" action={<Button color="inherit" size="small" onClick={() => void loadSource(true)}>重试</Button>} sx={{ mb: 1 }}>{sourceError}</Alert>}
      <Card>{sourceLoading && <LinearProgress />}
        <TableContainer><Table size="small" sx={{ minWidth: 1050 }}>
          <TableHead><TableRow><TableCell>时间</TableCell><TableCell>事件</TableCell><TableCell>彩种 / 任务</TableCell><TableCell>来源组</TableCell><TableCell>状态</TableCell><TableCell>原因 / 说明</TableCell><TableCell>期号</TableCell><TableCell align="right">导入</TableCell><TableCell align="right">连错</TableCell></TableRow></TableHead>
          <TableBody>{visibleSourceRows.map(row => <TableRow hover key={row.id} sx={row.status === 'error' ? { bgcolor: 'error.main', '& td': { bgcolor: 'rgba(255,255,255,.94)' } } : undefined}>
            <TableCell><Typography variant="caption" noWrap>{formatTime(row.created_at)}</Typography></TableCell>
            <TableCell><Typography fontSize={13} fontWeight={800} noWrap>{systemLogEventLabel(row.event_type)}</Typography><Typography variant="caption" color="text.secondary">{row.category === 'scheduler' ? '调度' : '来源'}</Typography></TableCell>
            <TableCell><Typography fontSize={13} fontWeight={800}>{row.game_id ? gameNames.get(row.game_id) ?? row.game_id : row.job_id || '—'}</Typography>{row.game_id && <Typography variant="caption" color="text.secondary">{row.game_id}</Typography>}</TableCell>
            <TableCell><Typography fontSize={12.5}>{row.source_group || '—'}</Typography></TableCell>
            <TableCell><Chip size="small" color={sourceStatusColor(row.status)} label={systemLogStatusLabel(row.status)} /></TableCell>
            <TableCell sx={{ maxWidth: 410 }}><Typography fontSize={12.5} sx={{ whiteSpace: 'normal', overflowWrap: 'anywhere' }}>{row.message || '—'}</Typography></TableCell>
            <TableCell><Typography fontSize={12.5}>{row.latest_issue || '—'}</Typography></TableCell>
            <TableCell align="right">{row.imported || 0}</TableCell><TableCell align="right">{row.consecutive_errors || 0}</TableCell>
          </TableRow>)}
          {!sourceLoading && !visibleSourceRows.length && <TableRow><TableCell colSpan={9}><Stack minHeight={220} alignItems="center" justifyContent="center" color="text.secondary"><HistoryRounded sx={{ fontSize: 42, opacity: .4 }} /><Typography mt={1}>当前条件暂无日志</Typography></Stack></TableCell></TableRow>}</TableBody>
        </Table></TableContainer>
        <Box ref={sourceSentinel} data-testid="source-log-load-more" textAlign="center" py={1.25}>
          {sourceLoadingMore ? <CircularProgress size={20} /> : sourceHasMore ? <Button size="small" onClick={() => void loadSource(false)}>加载更早日志</Button> : sourceRows.length > 0 ? <Typography variant="caption" color="text.secondary">已到最早记录</Typography> : null}
        </Box>
      </Card>
    </> : <>
      <Paper variant="outlined" sx={{ p: 1.4, mt: 1.5 }}>
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={1}>
          <TextField size="small" label="搜索已加载记录" placeholder="操作人、路径、请求ID或IP" value={auditQuery} onChange={event => setAuditQuery(event.target.value)} sx={{ flex: 1 }} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> } }} />
          <TextField select size="small" label="结果" value={auditStatus} onChange={event => setAuditStatus(event.target.value)} sx={{ minWidth: 145 }}><MenuItem value="">全部</MenuItem><MenuItem value="success">成功</MenuItem><MenuItem value="error">失败</MenuItem></TextField>
        </Stack>
        <Typography variant="caption" color="text.secondary" display="block" mt={1}>操作日志按游标持续加载；搜索只筛选当前已加载的 {auditRows.length} 条记录。</Typography>
      </Paper>
      {auditError && <Alert severity="error" action={<Button color="inherit" size="small" onClick={() => void loadAudit(true)}>重试</Button>} sx={{ mt: 1.25 }}>{auditError}</Alert>}
      <Card sx={{ mt: 1.25 }}>{auditLoading && <LinearProgress />}
        <TableContainer><Table size="small" sx={{ minWidth: 920 }}><TableHead><TableRow><TableCell>时间</TableCell><TableCell>操作人</TableCell><TableCell>角色</TableCell><TableCell>方法</TableCell><TableCell>路径</TableCell><TableCell>结果</TableCell><TableCell>请求 ID</TableCell><TableCell>IP</TableCell></TableRow></TableHead>
          <TableBody>{visibleAuditRows.map(row => <TableRow hover key={row.id}><TableCell><Typography variant="caption" noWrap>{formatTime(row.created_at)}</Typography></TableCell><TableCell><Typography fontSize={13} fontWeight={800}>{row.actor_name || '—'}</Typography></TableCell><TableCell>{row.actor_role || '—'}</TableCell><TableCell><Chip size="small" variant="outlined" label={row.method || '—'} /></TableCell><TableCell sx={{ maxWidth: 390 }}><Typography fontSize={12.5} sx={{ overflowWrap: 'anywhere' }}>{row.path || '—'}</Typography></TableCell><TableCell><Chip size="small" color={auditStatusColor(row.status_code)} label={row.status_code || '—'} /></TableCell><TableCell><Typography variant="caption">{row.request_id || '—'}</Typography></TableCell><TableCell><Typography variant="caption">{row.ip || '—'}</Typography></TableCell></TableRow>)}
          {!auditLoading && !visibleAuditRows.length && <TableRow><TableCell colSpan={8}><Stack minHeight={220} alignItems="center" justifyContent="center" color="text.secondary"><TaskAltRounded sx={{ fontSize: 42, opacity: .4 }} /><Typography mt={1}>当前条件暂无操作记录</Typography></Stack></TableCell></TableRow>}</TableBody></Table></TableContainer>
        <Box ref={auditSentinel} data-testid="audit-log-load-more" textAlign="center" py={1.25}>{auditLoadingMore ? <CircularProgress size={20} /> : auditHasMore ? <Button size="small" onClick={() => void loadAudit(false)}>加载更早日志</Button> : auditRows.length > 0 ? <Typography variant="caption" color="text.secondary">已到最早记录</Typography> : null}</Box>
      </Card>
    </>}
  </Box>
}
