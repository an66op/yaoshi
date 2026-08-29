import {
  Alert, Box, Button, Card, CardContent, Chip, CircularProgress, Dialog, DialogActions, DialogContent,
  DialogTitle, Stack, Table, TableBody, TableCell, TableContainer, TableHead, TableRow, TextField, Typography,
} from '@mui/material'
import AccountBalanceWalletRounded from '@mui/icons-material/AccountBalanceWalletRounded'
import DownloadRounded from '@mui/icons-material/DownloadRounded'
import PaidRounded from '@mui/icons-material/PaidRounded'
import PendingActionsRounded from '@mui/icons-material/PendingActionsRounded'
import { useCallback, useEffect, useState } from 'react'
import { adminApi, agentApi, type ProfitShareStatement } from '../api'
import { useFeedback } from './feedback'
import { createCsv } from '../utils/csv'

const money = (value: number) => new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)
const dateTime = (value?: string) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—'
const localToday = () => new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date())

const statusMeta: Record<string, { label: string; color: 'default' | 'success' | 'warning' | 'info' }> = {
  pending: { label: '待分账', color: 'warning' },
  partial: { label: '部分入账', color: 'info' },
  credited: { label: '已入账', color: 'success' },
  no_share: { label: '无分成', color: 'default' },
}

export function ProfitSharePanel({ agent = false, initialDate = '' }: { agent?: boolean; initialDate?: string }) {
  const [date, setDate] = useState(initialDate || localToday())
  const [data, setData] = useState<ProfitShareStatement | null>(null)
  const [loading, setLoading] = useState(true)
  const [running, setRunning] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [error, setError] = useState('')
  const { showMessage } = useFeedback()

  const load = useCallback(async (notify = false) => {
    setLoading(true); setError('')
    try {
      const result = await (agent ? agentApi : adminApi).profitShares(date)
      setData({ ...result, items: Array.isArray(result?.items) ? result.items : [] })
      if (notify) showMessage('代理分账账单已刷新')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取代理分账账单失败')
    } finally { setLoading(false) }
  }, [agent, date, showMessage])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])

  const run = async () => {
    setConfirmOpen(false); setRunning(true); setError('')
    try {
      const result = await adminApi.runProfitShares(date)
      showMessage(result.credited > 0 ? `已向 ${result.credited_rooms} 个房间入账 ${money(result.credited)}` : '没有新的待分账金额')
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '执行代理分账失败')
    } finally { setRunning(false) }
  }

  const exportStatement = () => {
    const rows = (data?.items ?? []).map(row => [row.biz_date, row.room_code, row.agent_username, row.bet_count, row.turnover, row.payout, row.gross_profit, row.rebate, row.accrued_share, row.paid_share, row.pending_share, statusMeta[row.status]?.label ?? row.status, row.last_transaction_id ?? '', dateTime(row.last_paid_at)])
    const csv = createCsv([['日期', '房间', '代理', '注单数', '有效投注', '派彩', '经营毛利', '会员回水', '应计分成', '已入账', '待分账', '状态', '余额流水号', '最后入账时间'], ...rows])
    const href = URL.createObjectURL(new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8' }))
    const link = document.createElement('a'); link.href = href; link.download = `代理分账_${data?.biz_date || date}.csv`; link.click(); URL.revokeObjectURL(href)
  }

  const cards = [
    ['应计分成', data?.total_accrued_share ?? 0, '由已结算注单的冻结分成汇总', PaidRounded, 'primary.main'],
    ['已入代理余额', data?.total_paid_share ?? 0, '可追溯到真实余额流水', AccountBalanceWalletRounded, 'success.main'],
    ['仍待分账', data?.total_pending_share ?? 0, '重复执行只补入尚未支付的差额', PendingActionsRounded, 'warning.main'],
  ] as const

  return <Card sx={{ mt: 1.5 }}>
    <CardContent>
      <Stack direction={{ xs: 'column', md: 'row' }} alignItems={{ md: 'center' }} justifyContent="space-between" gap={1.25}>
        <Box><Typography fontWeight={900}>每日代理分账</Typography><Typography variant="caption" color="text.secondary">应计金额来自下注时冻结快照；已入账金额来自代理余额流水，迟到结算会在下次执行时补差额。</Typography></Box>
        <Stack direction="row" gap={.75} flexWrap="wrap">
          <TextField size="small" type="date" label="分账日期" value={date} onChange={event => setDate(event.target.value)} slotProps={{ inputLabel: { shrink: true } }} />
          <Button variant="outlined" startIcon={<DownloadRounded />} disabled={!data?.items.length} onClick={exportStatement}>导出</Button>
          {!agent && <Button variant="contained" color="success" disabled={running || loading || (data?.total_pending_share ?? 0) <= 0} startIcon={running ? <CircularProgress color="inherit" size={15} /> : <PaidRounded />} onClick={() => setConfirmOpen(true)}>{running ? '分账中…' : '执行待分账'}</Button>}
        </Stack>
      </Stack>
      {error && <Alert severity="error" sx={{ mt: 1.25 }} onClose={() => setError('')}>{error}</Alert>}
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(3,1fr)' }, gap: 1, mt: 1.5 }}>{cards.map(([label, value, hint, Icon, color]) => <Box key={label} sx={{ p: 1.35, border: 1, borderColor: 'divider', borderRadius: 2.5, bgcolor: 'background.default' }}><Stack direction="row" gap={1} alignItems="flex-start"><Box sx={{ width: 36, height: 36, display: 'grid', placeItems: 'center', borderRadius: 2, bgcolor: color, color: '#fff', flex: '0 0 auto' }}><Icon fontSize="small" /></Box><Box minWidth={0}><Typography variant="caption" color="text.secondary">{label}</Typography><Typography fontSize={20} fontWeight={900}>{money(value)}</Typography><Typography fontSize={10} color="text.secondary">{hint}</Typography></Box></Stack></Box>)}</Box>
      <TableContainer sx={{ mt: 1.5, maxHeight: 380 }}><Table size="small" stickyHeader sx={{ minWidth: 980 }}><TableHead><TableRow><TableCell>房间 / 代理</TableCell><TableCell align="right">注单 / 投注</TableCell><TableCell align="right">毛利 / 回水</TableCell><TableCell align="right">应计分成</TableCell><TableCell align="right">已入账</TableCell><TableCell align="right">待分账</TableCell><TableCell>状态 / 凭证</TableCell></TableRow></TableHead><TableBody>{data?.items.map(row => { const status = statusMeta[row.status] ?? { label: row.status, color: 'default' as const }; return <TableRow hover key={`${row.biz_date}:${row.agent_id}`}><TableCell><Typography fontWeight={850}>{row.room_code || row.room_scope}</Typography><Typography fontSize={10} color="text.secondary">@{row.agent_username} · 代理 ID {row.agent_id}</Typography></TableCell><TableCell align="right"><Typography fontWeight={750}>{row.bet_count} 注</Typography><Typography fontSize={10} color="text.secondary">{money(row.turnover)}</Typography></TableCell><TableCell align="right"><Typography color={row.gross_profit >= 0 ? 'success.main' : 'error.main'} fontWeight={800}>{money(row.gross_profit)}</Typography><Typography fontSize={10} color="text.secondary">回水 {money(row.rebate)}</Typography></TableCell><TableCell align="right" sx={{ fontWeight: 850 }}>{money(row.accrued_share)}</TableCell><TableCell align="right" sx={{ fontWeight: 850 }}>{money(row.paid_share)}</TableCell><TableCell align="right"><Typography fontWeight={900} color={row.pending_share > 0 ? 'warning.main' : 'text.secondary'}>{money(row.pending_share)}</Typography></TableCell><TableCell><Chip size="small" label={status.label} color={status.color} variant={row.status === 'credited' ? 'filled' : 'outlined'} /><Typography display="block" fontSize={9} color="text.secondary" mt={.4}>{row.last_transaction_id ? `流水 #${row.last_transaction_id}` : '尚无入账流水'} · {dateTime(row.last_paid_at)}</Typography></TableCell></TableRow> })}{!loading && !data?.items.length && <TableRow><TableCell colSpan={7} align="center" sx={{ py: 6, color: 'text.secondary' }}>该日没有可分账的代理房间注单</TableCell></TableRow>}</TableBody></Table></TableContainer>
      <Stack direction="row" justifyContent="space-between" mt={1.25} color="text.secondary"><Typography variant="caption">日期 {data?.biz_date || date} · {data?.agent_count ?? 0} 个房间</Typography><Typography variant="caption">总投注 {money(data?.total_turnover ?? 0)} · 总毛利 {money(data?.total_gross_profit ?? 0)}</Typography></Stack>
    </CardContent>
    {!agent && <Dialog open={confirmOpen} onClose={() => setConfirmOpen(false)} maxWidth="xs" fullWidth><DialogTitle>确认执行代理分账</DialogTitle><DialogContent><Alert severity="warning" sx={{ mb: 1.5 }}>将把尚未支付的分成计入对应代理余额，并生成不可重复的余额流水。</Alert><Typography>日期：{date}</Typography><Typography>待分账：<b>{money(data?.total_pending_share ?? 0)}</b></Typography><Typography variant="caption" color="text.secondary">已入账部分不会再次发放；后续出现迟到结算时，仅补发新增差额。</Typography></DialogContent><DialogActions><Button onClick={() => setConfirmOpen(false)}>取消</Button><Button variant="contained" color="success" onClick={() => void run()}>确认入账</Button></DialogActions></Dialog>}
  </Card>
}
