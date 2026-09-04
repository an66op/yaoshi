import { Alert, Box, Button, Card, CardContent, Chip, CircularProgress, Collapse, Divider, LinearProgress, MenuItem, Stack, Tab, Tabs, TextField, Typography } from '@mui/material'
import PlayArrowRounded from '@mui/icons-material/PlayArrowRounded'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import { useCallback, useEffect, useRef, useState } from 'react'
import { adminApi, type FeedStatus } from '../api'
import { diagnosticBatch, filterDiagnosticGames, filterDiagnosticSources, gameSources, SOURCE_BATCH_SIZE, sourceHasWarning, sourceRelationForGame, type SourceDiagnosticFilter, type SourceDiagnosticGame, type SourceDiagnosticRelation, type SourceDiagnosticSource, type SourceDiagnostics, type SourceProbeResult, type SourceProbeResults } from '../sourceDiagnostics'

function formatTime(value?: string | null) {
  if (!value) return '—'
  const date = new Date(value)
  if (!Number.isFinite(date.getTime()) || date.getTime() <= 0) return '—'
  return new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(date)
}

const probeLabels = { success: '获取成功', error: '请求或数据异常', empty: '返回空数据', stale: '开奖数据过期' } as const

const relationMeta: Record<SourceDiagnosticRelation, { label: string; color: 'default' | 'primary' | 'success' | 'warning' | 'error' | 'info' }> = {
  production: { label: '生产来源', color: 'primary' },
  historical: { label: '历史核对', color: 'default' },
  same_product_candidate: { label: '同款候选', color: 'success' },
  different_product: { label: '不同产品', color: 'error' },
  cross_check_only: { label: '仅交叉核查', color: 'info' },
  unverified_candidate: { label: '候选待验', color: 'warning' },
  unavailable: { label: '有ID但不可用', color: 'error' },
  catalog_only: { label: '目录项', color: 'default' },
}

const source163Meta: Record<SourceDiagnosticGame['source_163_status'], { label: string; color: 'default' | 'primary' | 'success' | 'warning' | 'error' | 'info' }> = {
  current: { label: '163当前母源', color: 'primary' },
  verified_candidate: { label: '163同款候选已核', color: 'success' },
  candidate_unverified: { label: '163候选待验', color: 'warning' },
  unavailable: { label: '163有ID但过期/空', color: 'error' },
  not_found: { label: '163目录未找到同款', color: 'warning' },
  not_assessed: { label: '163目录未评估', color: 'default' },
}

function ProbeDetails({ result }: { result: SourceProbeResult }) {
  return <Box sx={{ mt: 1, p: 1.5, bgcolor: 'action.hover', borderRadius: 1.5 }} aria-live="polite">
    <Typography variant="body2" fontWeight={750}>本次测试结果（只读，不入库）</Typography>
    <Typography variant="caption" color="text.secondary" display="block">
      北京时间 {formatTime(result.checked_at)} · 耗时 {result.duration_ms} ms · HTTP {result.http_status ?? '—'} · 历史返回 {result.history_count} 期
    </Typography>
    <Typography variant="body2" sx={{ mt: 1 }}>最新期号：{result.issue || '—'} · 开奖时间：{formatTime(result.draw_at)}</Typography>
    <Stack direction="row" useFlexGap flexWrap="wrap" gap={.5} sx={{ mt: 1 }} aria-label="测试返回的原始号码">
      {result.numbers.length > 0 ? result.numbers.map((number, index) => <Box component="span" key={index} sx={{ bgcolor: 'background.paper', border: '1px solid', borderColor: 'divider', borderRadius: 1, minWidth: 30, p: .5, textAlign: 'center', fontWeight: 750 }}>{number}</Box>) : <Typography variant="body2" color="text.secondary">未获取到号码</Typography>}
    </Stack>
    <Typography variant="body2" color={result.status === 'success' ? 'text.secondary' : 'error.main'} sx={{ mt: 1, overflowWrap: 'anywhere' }}>{result.message}</Typography>
  </Box>
}

type SourceCardProps = {
  source: SourceDiagnosticSource
  relationOverride?: SourceDiagnosticRelation
  result?: SourceProbeResult
  current?: boolean
  testing: string
  expanded: string
  onTest: (source: SourceDiagnosticSource) => void
  onExpand: (key: string) => void
}

export function SourceDiagnosticCard({ source, relationOverride, result, current, testing, expanded, onTest, onExpand }: SourceCardProps) {
  const pending = testing === source.key
  const persistentRisk = Boolean(result?.status === 'success' && source.warning && source.warning_persistent)
  const relationKey = relationOverride ?? source.relation
  const relation = relationMeta[relationKey]
  return <Box sx={{ p: 1.5, border: '1px solid', borderColor: 'divider', borderRadius: 2, minWidth: 0 }}>
    <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" gap={1}>
      <Box sx={{ minWidth: 0 }}>
        <Stack direction="row" flexWrap="wrap" useFlexGap alignItems="center" gap={.75}>
          <Typography variant="body2" fontWeight={800}>{source.name}</Typography>
          <Chip size="small" color={current ? 'primary' : relation.color} variant="outlined" label={current ? '当前使用' : current === false && relationKey === 'production' ? '生产母源组件' : relation.label} />
          {source.upstream_game_id != null && <Chip size="small" variant="outlined" label={`ID ${source.upstream_game_id}`} />}
          <Chip size="small" color={pending ? 'info' : persistentRisk ? 'warning' : result ? result.status === 'success' ? 'success' : 'error' : source.warning ? 'warning' : 'default'} label={pending ? '测试中…' : persistentRisk ? '接口可达 · 风险仍在' : result ? probeLabels[result.status] : source.warning ? '有核查提示 · 待复测' : '未测试'} />
        </Stack>
        <Typography variant="caption" color="text.secondary" display="block" sx={{ mt: .4 }}>{source.provider} · {source.groups.join(' / ')}</Typography>
        <Typography variant="caption" color="text.secondary" display="block" sx={{ overflowWrap: 'anywhere' }}>{source.endpoint}</Typography>
      </Box>
      <Stack direction="row" gap={.5} alignItems="start" flexShrink={0}>
        {result && <Button size="small" onClick={() => onExpand(expanded === source.key ? '' : source.key)} aria-expanded={expanded === source.key}>{expanded === source.key ? '收起结果' : '查看结果'}</Button>}
        <Button size="small" variant="outlined" startIcon={pending ? <CircularProgress size={14} /> : <PlayArrowRounded />} disabled={Boolean(testing)} onClick={() => onTest(source)} aria-label={`测试 ${source.name} ${source.key}`}>{pending ? '测试中' : '点击测试'}</Button>
      </Stack>
    </Stack>
    {source.warning && <Typography variant="caption" color="warning.main" display="block" sx={{ mt: .75, overflowWrap: 'anywhere' }}>核查提示{source.warning_checked_at ? `（${formatTime(source.warning_checked_at)}）` : ''}：{source.warning}</Typography>}
    {result && <Typography variant="caption" color="text.secondary" display="block" sx={{ mt: .75 }}>最近测试 {formatTime(result.checked_at)} · {result.duration_ms} ms · 期号 {result.issue || '—'}</Typography>}
    <Collapse in={Boolean(result && expanded === source.key)} unmountOnExit>{result && <ProbeDetails result={result} />}</Collapse>
  </Box>
}

export function NetworkPage() {
  const [data, setData] = useState<SourceDiagnostics | null>(null)
  const [feed, setFeed] = useState<FeedStatus | null>(null)
  const [tab, setTab] = useState<'games' | 'catalog' | 'jobs'>('games')
  const [filter, setFilter] = useState<SourceDiagnosticFilter>('all')
  const [query, setQuery] = useState('')
  const [provider, setProvider] = useState('')
  const [visibleCount, setVisibleCount] = useState(SOURCE_BATCH_SIZE)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState('')
  const [feedError, setFeedError] = useState('')
  const [results, setResults] = useState<SourceProbeResults>({})
  const [testing, setTesting] = useState('')
  const [expanded, setExpanded] = useState('')
  const loadRequest = useRef<AbortController | null>(null)
  const probeRequest = useRef<AbortController | null>(null)
  const loadMoreRef = useRef<HTMLDivElement | null>(null)

  const load = useCallback(async () => {
    if (loadRequest.current) return
    const controller = new AbortController()
    loadRequest.current = controller
    setRefreshing(true)
    try {
      const value = await adminApi.sourceDiagnostics(controller.signal)
      if (!controller.signal.aborted) { setData(value); setError('') }
    } catch (reason) {
      if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : '读取来源配置失败')
    } finally {
      if (loadRequest.current === controller) loadRequest.current = null
      if (!controller.signal.aborted) { setLoading(false); setRefreshing(false) }
    }
  }, [])

  useEffect(() => {
    const initial = window.setTimeout(() => void load(), 0)
    // This refresh reads local configuration only. It does not probe upstreams.
    const poll = window.setInterval(() => { if (document.visibilityState !== 'hidden') void load() }, 30_000)
    return () => {
      window.clearTimeout(initial); window.clearInterval(poll)
      loadRequest.current?.abort(); loadRequest.current = null
      probeRequest.current?.abort(); probeRequest.current = null
    }
  }, [load])

  useEffect(() => {
    if (tab !== 'jobs') return
    const controller = new AbortController()
    let pending = false
    const read = async () => {
      if (pending || document.visibilityState === 'hidden') return
      pending = true
      try {
        const status = await adminApi.feedStatus(controller.signal)
        if (!controller.signal.aborted) { setFeed(status); setFeedError('') }
      } catch (reason) {
        if (!controller.signal.aborted) setFeedError(reason instanceof Error ? reason.message : '读取调度状态失败')
      } finally { pending = false }
    }
    const initial = window.setTimeout(() => void read(), 0)
    const poll = window.setInterval(() => void read(), 30_000)
    return () => { controller.abort(); window.clearTimeout(initial); window.clearInterval(poll) }
  }, [tab])

  const testSource = async (source: SourceDiagnosticSource) => {
    if (probeRequest.current) return
    const controller = new AbortController()
    probeRequest.current = controller
    setTesting(source.key)
    const started = Date.now()
    try {
      const result = await adminApi.probeSource(source.key, controller.signal)
      if (result.source_key !== source.key) throw new Error('测试返回的来源与请求不一致')
      if (!controller.signal.aborted) {
        setResults(previous => ({ ...previous, [source.key]: result }))
        setExpanded(source.key)
      }
    } catch (reason) {
      if (!controller.signal.aborted) {
        setResults(previous => ({ ...previous, [source.key]: {
          source_key: source.key, status: 'error', checked_at: new Date().toISOString(), duration_ms: Date.now() - started,
          http_status: null, issue: null, draw_at: null, numbers: [], history_count: 0,
          message: reason instanceof Error ? reason.message : '测试请求失败，请重试',
        } }))
        setExpanded(source.key)
      }
    } finally {
      if (probeRequest.current === controller) probeRequest.current = null
      if (!controller.signal.aborted) setTesting('')
    }
  }

  const cancelProbe = () => { probeRequest.current?.abort(); probeRequest.current = null; setTesting('') }
  const sources = data?.catalog ?? []
  const games = data?.games ?? []
  const providers = [...new Set(sources.map(source => source.provider))]
  const sourceList = diagnosticBatch(filterDiagnosticSources(sources, results, query, provider, filter), visibleCount)
  const gameList = diagnosticBatch(filterDiagnosticGames(games, sources, results, query, provider, filter), visibleCount)
  const activeList = tab === 'catalog' ? sourceList : gameList
  const canLoadMore = tab !== 'jobs' && activeList.hasMore
  const loadMore = useCallback(() => {
    setVisibleCount(count => Math.min(count + SOURCE_BATCH_SIZE, activeList.total))
  }, [activeList.total])

  useEffect(() => {
    const target = loadMoreRef.current
    if (!canLoadMore || !target || typeof IntersectionObserver === 'undefined') return
    let active = true
    let consumed = false
    const observer = new IntersectionObserver(entries => {
      if (!active || consumed || !entries.some(entry => entry.isIntersecting)) return
      consumed = true
      observer.disconnect()
      loadMore()
    }, { rootMargin: '240px 0px' })
    observer.observe(target)
    return () => { active = false; observer.disconnect() }
  }, [canLoadMore, loadMore, visibleCount, tab, query, provider, filter])

  const warningCount = sources.filter(source => sourceHasWarning(source, results)).length
  const runtimeErrors = games.filter(game => game.enabled && (game.last_sync_error || ['error', 'stale', 'paused'].includes(game.sync_status))).length
  const source163NotFound = games.filter(game => game.source_163_status === 'not_found').length
  const source163Unverified = games.filter(game => game.source_163_status === 'candidate_unverified').length
  const source163Unavailable = games.filter(game => game.source_163_status === 'unavailable').length
  const tested = Object.keys(results).length
  const renderSource = (source: SourceDiagnosticSource, current?: boolean, gameID?: string) => <SourceDiagnosticCard key={source.key} source={source} relationOverride={gameID ? sourceRelationForGame(source, gameID) : undefined} current={current} result={results[source.key]} testing={testing} expanded={expanded} onTest={source => void testSource(source)} onExpand={setExpanded} />

  return <Box p={{ xs: 2, lg: 2.5 }}>
    <Card>
      {refreshing && <LinearProgress />}
      <CardContent>
        <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" gap={2}>
          <Box>
            <Typography fontWeight={850}>开奖源与接口诊断</Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mt: .5 }}>当前绑定以每个彩种的“当前使用”为准；163当前88项目录快照分别标出已接入、同款候选、候选待验、有ID但过期/空和目录未找到。“163官方彩”只是上游分组名。测试只读取上游，不切换来源、不导入开奖。</Typography>
          </Box>
          <Button startIcon={<RefreshRounded />} onClick={() => void load()} disabled={refreshing} sx={{ flexShrink: 0 }}>刷新配置</Button>
        </Stack>
        <Stack direction="row" useFlexGap flexWrap="wrap" gap={1} sx={{ mt: 2 }}>
          <Chip label={`当前彩种 ${data ? games.length : '—'}`} />
          <Chip label={`来源目录 ${data ? sources.length : '—'}`} />
          <Chip color={runtimeErrors ? 'error' : 'default'} label={`同步异常 ${data ? runtimeErrors : '未知'}`} />
          <Chip color={warningCount ? 'warning' : 'default'} label={`需核查来源 ${data ? warningCount : '未知'}`} onClick={() => { setTab('catalog'); setFilter('abnormal'); setVisibleCount(SOURCE_BATCH_SIZE) }} />
          <Chip color={source163NotFound ? 'warning' : 'default'} variant="outlined" label={`163目录未找到同款 ${data ? source163NotFound : '未知'}`} />
          <Chip color={source163Unverified ? 'warning' : 'default'} variant="outlined" label={`163候选待验 ${data ? source163Unverified : '未知'}`} />
          <Chip color={source163Unavailable ? 'error' : 'default'} variant="outlined" label={`163有ID但过期/空 ${data ? source163Unavailable : '未知'}`} />
          <Chip variant="outlined" label={`本页会话已测 ${tested}`} />
        </Stack>
      </CardContent>
      <Tabs value={tab} onChange={(_, value: typeof tab) => { setTab(value); setVisibleCount(SOURCE_BATCH_SIZE) }} variant="scrollable" scrollButtons="auto" aria-label="接口诊断分类">
        <Tab label="彩种与玩法来源" value="games" /><Tab label="数据源目录 / 单项测试" value="catalog" /><Tab label="运行中的同步任务" value="jobs" />
      </Tabs>
    </Card>

    {error && <Alert severity="error" sx={{ mt: 1.5 }}>{error}{data ? '；以下为上次读取的配置，非实时状态。' : '；来源状态未知，请重试。'}</Alert>}
    {testing && <Alert severity="info" sx={{ mt: 1.5 }} action={<Button color="inherit" size="small" onClick={cancelProbe}>取消测试</Button>}>正在读取 {sources.find(source => source.key === testing)?.name ?? testing}，结果将显示在该来源下方。</Alert>}
    {loading && <Box sx={{ p: 4, textAlign: 'center' }}><CircularProgress size={28} /></Box>}

    {tab !== 'jobs' && data && <>
      <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.5} sx={{ my: 2 }}>
        <TextField size="small" label="搜索彩种、玩法或来源ID" value={query} onChange={event => { setQuery(event.target.value); setVisibleCount(SOURCE_BATCH_SIZE) }} sx={{ flex: 1 }} />
        <TextField select size="small" label="提供方" value={provider} onChange={event => { setProvider(event.target.value); setVisibleCount(SOURCE_BATCH_SIZE) }} sx={{ minWidth: 150 }}>
          <MenuItem value="">全部提供方</MenuItem>{providers.map(value => <MenuItem key={value} value={value}>{value}</MenuItem>)}
        </TextField>
        <TextField select size="small" label="检查状态" value={filter} onChange={event => { setFilter(event.target.value as SourceDiagnosticFilter); setVisibleCount(SOURCE_BATCH_SIZE) }} sx={{ minWidth: 150 }}>
          <MenuItem value="all">全部状态</MenuItem><MenuItem value="abnormal">异常 / 需核查</MenuItem><MenuItem value="untested">本次尚未测试</MenuItem>
        </TextField>
      </Stack>
      <Typography variant="caption" color="text.secondary" display="block" sx={{ mb: 1.5 }}>下滑自动追加12项，展开才显示详情；不会自动批量请求上游。过期按各来源时效阈值初筛，不代表开奖日历已验收；核查提示与本次测试结果分开展示。</Typography>
      {activeList.items.length === 0 && <Alert severity="info">没有符合筛选条件的记录。</Alert>}
      {tab === 'catalog' && <Stack gap={1.5}>{sourceList.items.map(source => <Card key={source.key}><CardContent>
        {renderSource(source)}
        <Typography variant="caption" color="text.secondary" display="block" sx={{ mt: 1 }}>对应彩种：{source.game_ids.map(id => games.find(game => game.game_id === id)?.name ?? id).join('、') || '尚无本地彩种映射，仅列入来源目录'}</Typography>
      </CardContent></Card>)}</Stack>}
      {tab === 'games' && <Stack gap={1.5}>{gameList.items.map(game => {
        const related = gameSources(game, sources)
        return <Card key={game.game_id}><CardContent>
          <Stack direction="row" useFlexGap flexWrap="wrap" gap={1} alignItems="center">
            <Typography fontWeight={850}>{game.name}</Typography>
            <Chip size="small" variant="outlined" label={game.lobby_category || '未分类'} />
            <Chip size="small" color={source163Meta[game.source_163_status].color} variant="outlined" label={source163Meta[game.source_163_status].label} />
            {!game.enabled && <Chip size="small" label="已停用" />}
            {(game.last_sync_error || ['error', 'stale', 'paused'].includes(game.sync_status)) && <Chip size="small" color="error" label="当前同步异常" />}
          </Stack>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>{game.rules_message || '该彩种各玩法共用同一组原始开奖，玩法与赔率仍按后台规则配置。'}</Typography>
          <Typography variant="caption" color="text.secondary" display="block" sx={{ mt: .5 }}>{game.game_id} · 规则 {game.rule_version || '尚未绑定'} · 当前来源 {game.source_name || (game.source_kind === 'platform' ? '平台自开' : '未配置')} · 最近同步 {formatTime(game.last_sync_at)}</Typography>
          <Typography variant="caption" color={['candidate_unverified', 'unavailable', 'not_found'].includes(game.source_163_status) ? 'warning.main' : 'text.secondary'} display="block" sx={{ mt: .5 }}>{game.source_163_message}</Typography>
          {game.last_sync_error && <Alert severity="error" sx={{ my: 1 }}>{game.last_sync_error}</Alert>}
          <Stack gap={1} sx={{ mt: 1.5 }}>{related.map(source => renderSource(source, source.key === game.source_key, game.game_id))}</Stack>
          {!related.length && <Alert severity="info" sx={{ mt: 1.5 }}>{game.source_163_status === 'not_found' ? game.source_163_message : '本次固定诊断目录尚未登记可单项测试的接口，不能据此认定来源正常或断言其他渠道不存在。'}</Alert>}
          {related.length > 0 && !game.source_key && <Typography variant="caption" color="warning.main" display="block" sx={{ mt: 1 }}>现用来源尚无独立测试入口；以上映射只用于候选核查，不代表已切换。</Typography>}
        </CardContent></Card>
      })}</Stack>}
      <Box ref={loadMoreRef} data-testid="source-diagnostics-load-more" sx={{ mt: 2, py: 1, textAlign: 'center' }}>
        <Typography variant="caption" color="text.secondary" display="block" aria-live="polite">已显示 {activeList.items.length} / {activeList.total} 项{!canLoadMore && activeList.total > 0 ? ' · 已全部加载' : ''}</Typography>
        {canLoadMore && <>
          <Typography variant="caption" color="text.secondary" display="block" sx={{ mt: .5 }}>继续下滑自动加载，也可点击加载更多</Typography>
          <Button size="small" onClick={loadMore} sx={{ mt: .5 }}>加载更多</Button>
        </>}
      </Box>
    </>}

    {tab === 'jobs' && <Box sx={{ mt: 2 }}>
      {feedError && <Alert severity="error" sx={{ mb: 1.5 }}>{feedError}；调度状态可能不是最新。</Alert>}
      {!feed && !feedError && <CircularProgress size={28} />}
      {feed && <>
        <Alert severity={feed.running ? 'info' : 'warning'} sx={{ mb: 1.5 }}>{feed.running ? '调度服务运行中' : '调度服务未启动'} · 服务器时间 {formatTime(feed.server_time)} · 下方是现有同步任务，不是本页只读测试结果。</Alert>
        <Stack gap={1.5}>{feed.jobs.map(job => <Card key={job.id}><CardContent>
          <Stack direction="row" alignItems="center" useFlexGap flexWrap="wrap" gap={1}>
            <Typography fontWeight={800}>{job.name}</Typography>
            <Chip size="small" color={job.last_error || job.consecutive_errors ? 'error' : !feed.running ? 'warning' : job.running ? 'info' : 'default'} label={job.last_error || job.consecutive_errors ? '同步异常' : !feed.running ? '调度未启动' : job.running ? '正在执行' : '等待调度'} />
          </Stack>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>{job.game_ids.map(id => games.find(game => game.game_id === id)?.name ?? id).join('、')}</Typography>
          <Divider sx={{ my: 1 }} />
          <Typography variant="caption" color="text.secondary">最后成功 {formatTime(job.last_success_at)} · 下次执行 {formatTime(job.next_run_at)} · 最近期号 {job.latest_issue || '—'} · 连续失败 {job.consecutive_errors}</Typography>
          {job.last_error && <Typography variant="body2" color="error.main" sx={{ mt: 1, overflowWrap: 'anywhere' }}>{job.last_error}</Typography>}
        </CardContent></Card>)}</Stack>
      </>}
    </Box>}
  </Box>
}
