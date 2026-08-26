import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Stack,
  Typography,
} from '@mui/material'
import PublicRounded from '@mui/icons-material/PublicRounded'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import CloudSyncRounded from '@mui/icons-material/CloudSyncRounded'
import { useCallback, useEffect, useState } from 'react'
import { adminApi, type FeedJobStatus, type FeedStatus } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'

function formatTime(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime()) || date.getTime() <= 0) return '—'
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(date)
}

function jobTone(job: FeedJobStatus): 'success' | 'warning' | 'error' | 'default' {
  if (job.last_error || job.consecutive_errors > 0) return 'error'
  if (job.mode === 'draw-window') return 'warning'
  if (job.running) return 'success'
  return 'default'
}

function jobLabel(job: FeedJobStatus) {
  if (job.last_error || job.consecutive_errors > 0) return '异常'
  if (job.mode === 'draw-window') return '开奖窗口'
  if (job.running) return '运行中'
  return '待机'
}

function latencyText(job: FeedJobStatus, serverMs: number) {
  if (!job.last_success_at) return '暂无成功同步'
  const lag = Math.max(0, serverMs - new Date(job.last_success_at).getTime())
  if (lag < 1000) return `${lag} ms`
  if (lag < 60_000) return `${Math.round(lag / 1000)} s`
  return `${Math.round(lag / 60_000)} 分钟前`
}

export function NetworkPage() {
  const [feed, setFeedStatus] = useState<FeedStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [syncing, setSyncing] = useState(false)
  const [error, setError] = useState('')
  const [selected, setSelected] = useState('')
  const { showMessage } = useFeedback()

  const load = useCallback(async (notify = false) => {
    setLoading(true)
    setError('')
    try {
      const status = await adminApi.feedStatus()
      setFeedStatus(status)
      setSelected(current => current || status.jobs[0]?.id || '')
      if (notify) showMessage('开奖线路状态已刷新')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取开奖线路失败')
    } finally {
      setLoading(false)
    }
  }, [showMessage])

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0)
    const poll = window.setInterval(() => void load(), 20_000)
    return () => { window.clearTimeout(timer); window.clearInterval(poll) }
  }, [load])

  const sync = async () => {
    setSyncing(true)
    setError('')
    try {
      const result = await adminApi.syncOfficialSources()
      showMessage(result.failed > 0 ? `同步完成，失败 ${result.failed} 个源` : '官方开奖源同步完成', result.failed > 0 ? 'warning' : 'success')
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '同步官方源失败')
    } finally {
      setSyncing(false)
    }
  }

  const jobs = feed?.jobs ?? []
  const active = jobs.find(job => job.id === selected) ?? jobs[0]
  const healthy = jobs.filter(job => !job.last_error && job.consecutive_errors === 0).length

  return (
    <Box p={{ xs: 2, lg: 2.5 }}>
      <PageHeader
        eyebrow="扩展服务 / 开奖"
        title="开奖线路"
        description="展示官方开奖调度任务健康度，并支持手动触发同步。"
        actions={
          <>
            <Button variant="outlined" startIcon={<RefreshRounded />} disabled={loading || syncing} onClick={() => void load(true)}>刷新</Button>
            <Button variant="contained" startIcon={<CloudSyncRounded />} disabled={loading || syncing} onClick={() => void sync()}>{syncing ? '同步中…' : '同步官方源'}</Button>
          </>
        }
      />
      {error && <Alert severity="error" sx={{ mt: 2 }}>{error}</Alert>}
      <Alert severity={feed?.running ? 'success' : 'warning'} sx={{ mt: 2.5 }} icon={<PublicRounded />}>
        {feed?.running
          ? `调度器运行中 · ${healthy}/${jobs.length} 条线路正常 · 服务器时间 ${formatTime(feed.server_time)}`
          : '开奖调度服务未运行，请检查服务状态。'}
      </Alert>
      {loading && !feed && <Box mt={2}><CircularProgress size={22} /></Box>}
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,1fr)', xl: 'repeat(4,1fr)' }, gap: 1.5, mt: 1.5 }}>
        {jobs.map(job => {
          const best = active?.id === job.id
          const tone = jobTone(job)
          return (
            <Card
              key={job.id}
              sx={best ? { color: '#fff', background: 'linear-gradient(145deg,#147fa9,#28b7aa)', cursor: 'pointer' } : { cursor: 'pointer' }}
              onClick={() => setSelected(job.id)}
            >
              <CardContent>
                <Stack direction="row" justifyContent="space-between" alignItems="center">
                  <Chip size="small" color={best ? undefined : tone} label={jobLabel(job)} sx={best ? { color: '#fff', bgcolor: 'rgba(255,255,255,.18)' } : {}} />
                  <Typography fontSize={11} color={best ? 'inherit' : 'text.secondary'}>{job.group}</Typography>
                </Stack>
                <Typography variant="h6" mt={3}>{job.name}</Typography>
                <Typography fontSize={22} fontWeight={850} color={best ? 'inherit' : tone === 'error' ? 'error.main' : 'success.main'}>
                  {latencyText(job, feed?.server_time_ms ?? 0)}
                </Typography>
                <Typography fontSize={11} mt={1} color={best ? 'rgba(255,255,255,.85)' : 'text.secondary'}>
                  最近成功 {formatTime(job.last_success_at)} · 期号 {job.latest_issue || '—'}
                </Typography>
              </CardContent>
            </Card>
          )
        })}
      </Box>
      {active && (
        <Card sx={{ mt: 1.5 }}>
          <CardContent>
            <Typography fontWeight={800}>{active.name}</Typography>
            <Typography variant="caption" color="text.secondary">时区 {active.timezone} · 模式 {active.mode} · 连续错误 {active.consecutive_errors}</Typography>
            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,1fr)', lg: 'repeat(4,1fr)' }, gap: 1.5, mt: 2 }}>
              {[
                ['下次执行', formatTime(active.next_run_at)],
                ['最近开始', formatTime(active.last_started_at)],
                ['最近完成', formatTime(active.last_finished_at)],
                ['累计导入', String(active.imported)],
              ].map(([label, value]) => (
                <Box key={label} sx={{ p: 1.2, borderRadius: 2, bgcolor: 'action.hover' }}>
                  <Typography variant="caption" color="text.secondary">{label}</Typography>
                  <Typography fontWeight={750} mt={.4}>{value}</Typography>
                </Box>
              ))}
            </Box>
            {active.last_error && <Alert severity="error" sx={{ mt: 2 }}>{active.last_error}</Alert>}
            <Typography variant="body2" color="text.secondary" mt={2}>覆盖游戏：{active.game_ids.join('、') || '—'}</Typography>
          </CardContent>
        </Card>
      )}
    </Box>
  )
}
