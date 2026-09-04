import { Alert, Box, Button, Card, Chip, CircularProgress, Stack, Table, TableBody, TableCell, TableContainer, TableHead, TableRow, Typography } from '@mui/material'
import HistoryRounded from '@mui/icons-material/HistoryRounded'
import { useCallback, useEffect, useRef, useState } from 'react'
import { adminApi, type SGSSCBackfillStatus } from '../api'
import { formatBeijingDateTime } from '../utils/drawTiming'

const queueLabels: Record<string, string> = { pending: '等待补采', running: '执行中', retry: '等待补采重试', settlement_retry: '等待结算重试', completed: '已完成', blocked: '需人工核对' }
const attemptLabels: Record<string, string> = { running: '执行中', recovered: '已恢复', source_error: '来源失败', conflict: '校对冲突', settlement_error: '结算失败', interrupted: '执行中断', blocked: '需人工核对' }
const reasonLabels: Record<string, string> = { pending_bet: '当前来源版本待结注单', recorded_issue: '已记录期缺少可信结果', draw_gap: '可信历史之间的缺期' }
const summaryLabels = [
  ['pending_issues', '等待处理'], ['running_issues', '执行中'], ['retry_issues', '等待重试'],
  ['blocked_issues', '需人工核对'], ['completed_issues', '已完成'], ['untracked_pending_issues', '待登记注单期'],
] as const

function checkedStatus(value: SGSSCBackfillStatus): SGSSCBackfillStatus {
  if (!value || value.game_id !== 'sg-ssc' || !Array.isArray(value.gaps) || !Array.isArray(value.records)
    || summaryLabels.some(([key]) => !Number.isSafeInteger(value.summary?.[key]) || value.summary[key] < 0)) {
    throw new Error('SG补采状态返回不完整，请重试')
  }
  return value
}

/** Mounted only for SG; historical recovery never supplies a live betting period. */
export function SGHistoryRecoveryPanel() {
  const [data, setData] = useState<SGSSCBackfillStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [queuing, setQueuing] = useState(false)
  const [queueError, setQueueError] = useState('')
  const [notice, setNotice] = useState('')
  const [cursors, setCursors] = useState([0])
  const cursor = cursors[cursors.length - 1]
  const life = useRef({ active: false })
  const requestGeneration = useRef(0)
  const pendingRead = useRef<Promise<void> | null>(null)
  const writing = useRef(false)
  const currentCursor = useRef(0)
  const confirmed = useRef<SGSSCBackfillStatus | null>(null)
  const readFailed = useRef(false)

  const read = useCallback((): Promise<void> => {
    if (!life.current.active || writing.current) return Promise.resolve()
    if (pendingRead.current) return pendingRead.current
    const lifetime = life.current
    const generation = ++requestGeneration.current
    const before = currentCursor.current
    const valid = () => lifetime.active && generation === requestGeneration.current && before === currentCursor.current
    setLoading(true)
    const promise = Promise.resolve().then(async () => {
      if (!valid()) return
      const next = checkedStatus(await adminApi.sgSSCBackfillStatus(before, 20))
      if (!valid()) return
      confirmed.current = next
      readFailed.current = false
      setData(next)
      setError('')
    }).catch(reason => {
      if (!valid()) return
      readFailed.current = true
      setError(reason instanceof Error ? reason.message : '读取SG历史补采状态失败')
    }).finally(() => {
      if (pendingRead.current === promise) pendingRead.current = null
      if (valid()) setLoading(false)
    })
    pendingRead.current = promise
    return promise
  }, [])

  useEffect(() => {
    const lifetime = { active: true }
    life.current = lifetime
    pendingRead.current = null
    let timer = 0
    let ticking = false
    const tick = async () => {
      if (!lifetime.active || ticking) return
      ticking = true
      try { if (document.visibilityState !== 'hidden') await read() } finally {
        ticking = false
        if (lifetime.active && document.visibilityState !== 'hidden') {
          window.clearTimeout(timer)
          timer = window.setTimeout(() => void tick(), 10_000)
        }
      }
    }
    const visible = () => {
      window.clearTimeout(timer)
      if (document.visibilityState !== 'hidden') void tick()
    }
    timer = window.setTimeout(() => void tick(), 0)
    document.addEventListener('visibilitychange', visible)
    return () => {
      lifetime.active = false
      requestGeneration.current += 1
      window.clearTimeout(timer)
      document.removeEventListener('visibilitychange', visible)
    }
  }, [read])

  const navigate = (next: number[]) => {
    if (!life.current.active || pendingRead.current || writing.current || cursor !== currentCursor.current) return
    currentCursor.current = next[next.length - 1]
    requestGeneration.current += 1
    confirmed.current = null
    setCursors(next)
    setData(null)
    setError('')
    void read()
  }
  const enqueue = async () => {
    const snapshot = confirmed.current
    if (!life.current.active || writing.current || pendingRead.current || readFailed.current || !snapshot?.enabled || !snapshot.source_bound) return
    const lifetime = life.current
    writing.current = true
    setQueuing(true)
    setQueueError('')
    setNotice('')
    try {
      const result = await adminApi.queueSGSSCBackfill()
      if (!lifetime.active) return
      setNotice(`补采请求已登记（本次入队 ${result.queued_issues} 期），等待后台执行。${result.message}`)
    } catch (reason) {
      if (lifetime.active) setQueueError(reason instanceof Error ? reason.message : '登记SG补采失败')
    } finally {
      if (lifetime.active) {
        writing.current = false
        setQueuing(false)
        void read()
      }
    }
  }

  const nextCursor = data?.next_before_id
  const canReadOlder = data?.has_more_records && Number.isSafeInteger(nextCursor) && Number(nextCursor) > 0 && (cursor === 0 || Number(nextCursor) < cursor)
  const locked = loading || queuing
  return <Card variant="outlined" sx={{ mb: 2, p: { xs: 1.25, sm: 1.75 } }} aria-label="SG历史补采与恢复">
    <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ sm: 'center' }} justifyContent="space-between" gap={1}>
      <Box><Typography fontWeight={850}>SG 历史补采与恢复</Typography><Typography variant="caption" color="text.secondary">可见时每 10 秒刷新 · 补采不改变实时期号、封盘或来源健康</Typography></Box>
      <Stack direction="row" gap={1}>
        <Button size="small" disabled={locked} onClick={() => void read()}>刷新状态</Button>
        <Button size="small" variant="outlined" startIcon={queuing ? <CircularProgress size={14} /> : <HistoryRounded />} disabled={locked || Boolean(error) || !data?.enabled || !data.source_bound} onClick={() => void enqueue()}>{queuing ? '登记中…' : '登记 SG 缺期补采'}</Button>
      </Stack>
    </Stack>
    {loading && <Stack direction="row" alignItems="center" gap={1} mt={1}><CircularProgress size={14} /><Typography variant="caption">正在读取补采状态…</Typography></Stack>}
    {error && <Alert severity="error" sx={{ mt: 1 }}>{error}{data ? '；以下为上次读取结果，非实时状态。' : '；当前状态未知，不能认定没有缺期。'}</Alert>}
    {queueError && <Alert severity="error" sx={{ mt: 1 }}>{queueError}；可刷新记录确认是否已登记。</Alert>}
    {notice && <Alert severity="info" sx={{ mt: 1 }}>{notice}</Alert>}
    {data && <>
      <Alert severity={!data.enabled || !data.source_bound ? 'warning' : 'info'} sx={{ mt: 1 }}>{data.message} 最多回看 {data.max_age_days} 天，后台每分钟每批最多 {data.batch_limit} 期、最多 2 个开奖日期。已保存的可信结果仍可走安全幂等结算。</Alert>
      <Stack direction="row" gap={.75} flexWrap="wrap" useFlexGap mt={1.25} aria-label="SG缺期汇总">
        {summaryLabels.map(([key, label]) => <Chip key={key} size="small" variant="outlined" color={key === 'blocked_issues' && data.summary[key] > 0 ? 'warning' : 'default'} label={`${label} ${data.summary[key]}`} />)}
      </Stack>
      <Typography fontWeight={750} fontSize={13} mt={1.5} mb={.5}>缺期队列</Typography>
      <TableContainer sx={{ maxHeight: 280 }}><Table size="small" aria-label="SG缺期队列" sx={{ minWidth: 730 }}><TableHead><TableRow><TableCell>期号 / 开奖时间</TableCell><TableCell>状态 / 登记原因</TableCell><TableCell>尝试次数</TableCell><TableCell>失败原因 / 重试时间</TableCell></TableRow></TableHead><TableBody>
        {data.gaps.map(item => <TableRow key={item.issue}><TableCell>{item.issue}<Typography variant="caption" display="block" color="text.secondary">{formatBeijingDateTime(item.draw_at)}</Typography></TableCell><TableCell>{queueLabels[item.status] || item.status}<Typography variant="caption" display="block" color="text.secondary">{reasonLabels[item.reason] || item.reason || '—'}</Typography></TableCell><TableCell>{item.attempts}</TableCell><TableCell sx={{ maxWidth: 340, overflowWrap: 'anywhere' }}>{item.last_error || '—'}<Typography variant="caption" display="block" color="text.secondary">{item.status === 'blocked' ? '需人工核对，不自动重试' : `重试 ${formatBeijingDateTime(item.next_retry_at)}`} · 更新 {formatBeijingDateTime(item.updated_at)}</Typography></TableCell></TableRow>)}
        {!data.gaps.length && <TableRow><TableCell colSpan={4} sx={{ color: 'text.secondary' }}>当前展示队列无待处理项；尚未登记的注单期数请查看上方汇总。</TableCell></TableRow>}
      </TableBody></Table></TableContainer>
      {data.has_more_gaps && <Typography variant="caption" color="warning.main">仅显示前 {data.gaps.length} 个队列项，还有更多缺期未展示。</Typography>}
      <Stack direction="row" justifyContent="space-between" alignItems="center" mt={1.5} mb={.5}><Typography fontWeight={750} fontSize={13}>恢复记录</Typography><Typography variant="caption" color="text.secondary">{cursor === 0 ? '最新记录' : `历史第 ${cursors.length} 页`} · 北京时间</Typography></Stack>
      <TableContainer sx={{ maxHeight: 380 }}><Table size="small" aria-label="SG恢复记录" sx={{ minWidth: 960 }}><TableHead><TableRow><TableCell>期号 / 请求号</TableCell><TableCell>状态 / 执行时间</TableCell><TableCell>触发 / 操作者</TableCell><TableCell>结果 / 结算</TableCell><TableCell>失败原因 / 来源版本</TableCell></TableRow></TableHead><TableBody>
        {data.records.map(item => <TableRow key={item.id}><TableCell>{item.issue}<Typography variant="caption" display="block" sx={{ overflowWrap: 'anywhere', maxWidth: 240 }}>请求号 {item.request_id || '—'}</Typography><Typography variant="caption" color="text.secondary">记录 #{item.id} · 第 {item.attempt} 次尝试</Typography></TableCell><TableCell><Chip size="small" variant="outlined" color={item.status === 'recovered' ? 'success' : item.status === 'running' ? 'info' : 'warning'} label={attemptLabels[item.status] || item.status} /><Typography variant="caption" display="block">开始 {formatBeijingDateTime(item.started_at)}</Typography><Typography variant="caption" display="block">结束 {formatBeijingDateTime(item.finished_at)}</Typography></TableCell><TableCell>{item.trigger === 'admin' ? '手动登记' : item.trigger === 'auto' ? '自动登记' : item.trigger}<Typography variant="caption" display="block">{item.operator || '—'}</Typography></TableCell><TableCell>{item.numbers || '暂无核验号码'}<Typography variant="caption" display="block">{item.imported ? '本次已导入' : '本次未新增导入'} · 本次结算 {item.settled_bets} 笔</Typography></TableCell><TableCell sx={{ maxWidth: 340, overflowWrap: 'anywhere' }}>{item.error || '—'}<Typography variant="caption" display="block" color="text.secondary">{item.source_revision || '—'} / {item.conversion_revision || '—'}</Typography></TableCell></TableRow>)}
        {!data.records.length && <TableRow><TableCell colSpan={5} sx={{ color: 'text.secondary' }}>暂无补采执行记录；登记请求不代表已执行或已恢复。</TableCell></TableRow>}
      </TableBody></Table></TableContainer>
      <Stack direction="row" justifyContent="flex-end" gap={1} mt={1}>
        <Button size="small" disabled={locked || cursor === 0} onClick={() => navigate([0])}>最新记录</Button>
        <Button size="small" disabled={locked || cursors.length <= 1} onClick={() => navigate(cursors.slice(0, -1))}>上一页</Button>
        <Button size="small" disabled={locked || Boolean(error) || !canReadOlder} onClick={() => { if (canReadOlder) navigate([...cursors, Number(nextCursor)]) }}>更早记录</Button>
      </Stack>
    </>}
  </Card>
}
