import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  LinearProgress,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material'
import CloudSyncRounded from '@mui/icons-material/CloudSyncRounded'
import PlayArrowRounded from '@mui/icons-material/PlayArrowRounded'
import PublicRounded from '@mui/icons-material/PublicRounded'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { adminApi, type AdminGame, type FeedStatus } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'
import { buildInterfaceHealthLines, summarizeInterfaceHealthLines, type InterfaceHealthLine, type SourceInterfaceStatus, type SourceOverallStatus, type SourceSchedulerStatus } from '../utils/interfaceHealth'

function formatTime(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime()) || date.getTime() <= 0) return '—'
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  }).format(date)
}

function sourceHost(value: string) {
  try {
    return new URL(value).hostname
  } catch {
    return value
  }
}

function typeLabel(line: InterfaceHealthLine) {
  const labels = line.sourceKinds.map(kind => kind === 'official' ? '官方源' : kind === 'external' ? '外部源' : kind === 'platform' ? '平台源' : kind)
  return [...new Set(labels)].join(' / ') || '未知类型'
}

function overallChip(status: SourceOverallStatus) {
  if (status === 'healthy') return <Chip size="small" color="success" label="正常" />
  if (status === 'checking') return <Chip size="small" color="info" label="测试中" />
  if (status === 'pending') return <Chip size="small" color="warning" label="待首次成功" />
  if (status === 'disabled') return <Chip size="small" label="未启用" />
  return <Chip size="small" color="error" label="异常" />
}

function interfaceChip(status: SourceInterfaceStatus) {
  if (status === 'ok') return <Chip size="small" color="success" variant="outlined" label="接口正常" />
  if (status === 'syncing') return <Chip size="small" color="info" variant="outlined" label="请求中" />
  if (status === 'idle') return <Chip size="small" color="warning" variant="outlined" label="尚未成功" />
  if (status === 'disabled') return <Chip size="small" variant="outlined" label="彩种停用" />
  if (status === 'missing') return <Chip size="small" color="error" variant="outlined" label="缺少彩种配置" />
  return <Chip size="small" color="error" variant="outlined" label="接口异常" />
}

function schedulerChip(status: SourceSchedulerStatus, mode: string) {
  if (status === 'running') return <Chip size="small" color="info" variant="outlined" label="正在执行" />
  if (status === 'retrying') return <Chip size="small" color="warning" variant="outlined" label="正在重试" />
  if (status === 'scheduled') return <Chip size="small" color="success" variant="outlined" label={mode === 'draw-window' ? '开奖窗口' : mode === 'retry' ? '等待重试' : '已调度'} />
  if (status === 'standby') return <Chip size="small" variant="outlined" label="备用实例" />
  if (status === 'stopped') return <Chip size="small" color="error" variant="outlined" label="调度未启动" />
  if (status === 'missing') return <Chip size="small" color="error" variant="outlined" label="未接入调度" />
  return <Chip size="small" color="error" variant="outlined" label="调度异常" />
}

export function NetworkPage() {
  const [feed, setFeedStatus] = useState<FeedStatus | null>(null)
  const [games, setGames] = useState<AdminGame[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [testing, setTesting] = useState('')
  const [error, setError] = useState('')
  const { showMessage } = useFeedback()

  const load = useCallback(async () => {
    setRefreshing(true)
    try {
      const [status, sourceGames] = await Promise.all([adminApi.feedStatus(), adminApi.games()])
      setFeedStatus(status)
      setGames(Array.isArray(sourceGames) ? sourceGames : [])
      setError('')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取接口健康状态失败')
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }, [])

  useEffect(() => {
    const initial = window.setTimeout(() => void load(), 0)
    const poll = window.setInterval(() => void load(), 20_000)
    return () => { window.clearTimeout(initial); window.clearInterval(poll) }
  }, [load])

  const testAll = async () => {
    setTesting('all')
    setError('')
    try {
      const result = await adminApi.syncOfficialSources()
      showMessage(result.failed > 0 ? `整体测试完成，${result.failed} 个彩种失败` : '全部数据源测试通过', result.failed > 0 ? 'warning' : 'success')
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '整体接口测试失败')
    } finally {
      setTesting('')
    }
  }

  const testOne = async (line: InterfaceHealthLine) => {
    if (!line.group) return
    setTesting(line.id)
    setError('')
    try {
      const result = await adminApi.testOfficialSource(line.group)
      showMessage(result.failed > 0 ? `${line.name} 测试完成，${result.failed} 个彩种失败` : `${line.name} 测试通过`, result.failed > 0 ? 'warning' : 'success')
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : `${line.name} 测试失败`)
    } finally {
      setTesting('')
    }
  }

  const lines = useMemo(() => feed ? buildInterfaceHealthLines(feed, games) : [], [feed, games])
  const health = useMemo(() => summarizeInterfaceHealthLines(lines), [lines])
  const healthSeverity = !feed?.running ? 'warning' : health.error > 0 ? 'error' : health.checking + health.pending > 0 ? 'info' : 'success'
  const pendingText = health.checking + health.pending > 0 ? `，${health.checking + health.pending} 条检测中或待首次成功` : ''
  const disabledText = health.disabled > 0 ? ` · ${health.disabled} 条未启用` : ''

  return (
    <Box p={{ xs: 2, lg: 2.5 }}>
      <PageHeader
        eyebrow="系统管理 / 数据接入"
        title="接口测试"
        description=""
      />

      {error && <Alert severity="error" sx={{ mt: 2 }}>{error}</Alert>}
      <Alert severity={healthSeverity} sx={{ mt: 2.5 }} icon={<PublicRounded />}>
        {feed?.running
          ? `开奖调度运行中 · 已启用 ${health.enabled} 条：${health.healthy} 条正常，${health.error} 条异常${pendingText}${disabledText} · 服务器时间 ${formatTime(feed.server_time)}`
          : '开奖调度服务未运行；接口状态仍按数据库中的最近一次同步结果展示。'}
      </Alert>

      <Card sx={{ mt: 1.5 }}>
        {refreshing && <LinearProgress />}
        <CardContent>
          <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" alignItems={{ xs: 'stretch', sm: 'center' }} gap={1.5}>
            <Box>
              <Typography fontWeight={850}>数据源健康检查</Typography>
              <Typography variant="caption" color="text.secondary">每 20 秒自动刷新 · 当前 {health.enabled} 条启用线路</Typography>
            </Box>
            <Button
              variant="contained"
              startIcon={<CloudSyncRounded />}
              disabled={Boolean(testing) || loading}
              onClick={() => void testAll()}
            >
              {testing === 'all' ? '整体测试中…' : '测试全部线路'}
            </Button>
          </Stack>
        </CardContent>
      </Card>

      {loading && !feed && <Box mt={3} display="grid" sx={{ placeItems: 'center' }}><CircularProgress size={28} /></Box>}
      {!loading && feed && lines.length === 0 && <Alert severity="info" sx={{ mt: 1.5 }}>当前没有配置开奖数据源线路。</Alert>}

      {lines.length > 0 && (
        <Card sx={{ mt: 1.5 }}>
          <TableContainer>
            <Table size="small" sx={{ minWidth: 1180 }}>
              <TableHead>
                <TableRow>
                  <TableCell>数据源 / 彩种线路</TableCell>
                  <TableCell>类型</TableCell>
                  <TableCell>状态</TableCell>
                  <TableCell>最后成功</TableCell>
                  <TableCell>最后错误</TableCell>
                  <TableCell align="center">连续失败</TableCell>
                  <TableCell>接口状态</TableCell>
                  <TableCell>调度状态</TableCell>
                  <TableCell align="right">测试</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {lines.map(line => (
                  <TableRow hover key={line.id}>
                    <TableCell sx={{ maxWidth: 300 }}>
                      <Typography fontSize={12} fontWeight={800}>{line.name}</Typography>
                      <Typography fontSize={10} color="text.secondary" sx={{ mt: .3 }}>
                        {line.sourceNames.join(' / ') || '未登记数据源'} · {line.enabledCount}/{line.games.length} 个彩种启用
                      </Typography>
                      <Typography fontSize={9.5} color="text.secondary" noWrap title={line.gameNames.join('、')}>
                        {line.gameNames.join('、') || '缺少彩种映射'}
                      </Typography>
                      {line.sourceURLs.length > 0 && (
                        <Typography fontSize={9.5} color="primary.main" noWrap title={line.sourceURLs.join('\n')}>
                          {line.sourceURLs.map(sourceHost).join(' / ')}
                        </Typography>
                      )}
                    </TableCell>
                    <TableCell><Typography fontSize={11}>{typeLabel(line)}</Typography></TableCell>
                    <TableCell>{overallChip(line.overallStatus)}</TableCell>
                    <TableCell>
                      <Typography fontSize={11}>{formatTime(line.lastSuccessAt)}</Typography>
                      <Typography fontSize={9.5} color="text.secondary">期号 {line.latestIssue || '—'}</Typography>
                    </TableCell>
                    <TableCell sx={{ maxWidth: 270 }}>
                      <Tooltip title={line.lastError || '暂无错误'} placement="top" arrow>
                        <Typography fontSize={10.5} color={line.lastError ? 'error.main' : 'text.secondary'} noWrap>
                          {line.lastError || '—'}
                        </Typography>
                      </Tooltip>
                    </TableCell>
                    <TableCell align="center">
                      <Typography fontWeight={800} color={line.consecutiveErrors > 0 ? 'error.main' : 'text.primary'}>{line.consecutiveErrors}</Typography>
                    </TableCell>
                    <TableCell>{interfaceChip(line.interfaceStatus)}</TableCell>
                    <TableCell>{schedulerChip(line.schedulerStatus, line.mode)}</TableCell>
                    <TableCell align="right">
                      <Tooltip title={line.group ? `测试 ${line.name}` : '该数据源尚未接入可测试调度'}>
                        <span>
                          <Button
                            size="small"
                            variant="outlined"
                            startIcon={<PlayArrowRounded />}
                            disabled={Boolean(testing) || !line.group}
                            onClick={() => void testOne(line)}
                          >
                            {testing === line.id ? '测试中…' : '单项测试'}
                          </Button>
                        </span>
                      </Tooltip>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        </Card>
      )}
    </Box>
  )
}
