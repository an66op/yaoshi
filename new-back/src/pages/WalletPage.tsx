import {
  Alert,
  Box,
  Button,
  Card,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  InputAdornment,
  MenuItem,
  Paper,
  Stack,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material'
import AddRounded from '@mui/icons-material/AddRounded'
import DeleteRounded from '@mui/icons-material/DeleteRounded'
import EditRounded from '@mui/icons-material/EditRounded'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import InboxRounded from '@mui/icons-material/InboxRounded'
import { useCallback, useEffect, useState } from 'react'
import { adminApi, type PaymentChannel, type PaymentChannelPayload } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'

const creditLabels: Record<string, string> = {
  manual: '人工处理',
  bank: '银行卡',
  alipay: '支付宝',
  wechat: '微信',
  usdt: 'USDT',
}

const emptyForm = (): PaymentChannelPayload => ({
  provider: '',
  name: '',
  merchant_no: '',
  credit_type: 'alipay',
  fee_rate: 0,
  min_amount: 1,
  max_amount: 20000,
  status: 'enabled',
  remark: '',
  sort_order: 0,
})

const money = (value: number) => new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)

export function WalletPage() {
  const [items, setItems] = useState<PaymentChannel[]>([])
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('all')
  const [applied, setApplied] = useState({ query: '', status: 'all' })
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<PaymentChannel | null>(null)
  const [form, setForm] = useState<PaymentChannelPayload>(emptyForm)
  const { showMessage } = useFeedback()

  const load = useCallback(async (notify = false) => {
    setLoading(true)
    setError('')
    try {
      const result = await adminApi.walletChannels(applied)
      setItems(result)
      if (notify) showMessage('钱包配置已刷新')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取钱包配置失败')
    } finally {
      setLoading(false)
    }
  }, [applied, showMessage])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])

  const openCreate = () => {
    setEditing(null)
    setForm(emptyForm())
    setDialogOpen(true)
  }

  const openEdit = (item: PaymentChannel) => {
    setEditing(item)
    setForm({
      provider: item.provider,
      name: item.name,
      merchant_no: item.merchant_no,
      credit_type: item.credit_type,
      fee_rate: item.fee_rate,
      min_amount: item.min_amount,
      max_amount: item.max_amount,
      status: item.status,
      remark: item.remark,
      sort_order: item.sort_order,
    })
    setDialogOpen(true)
  }

  const submit = async () => {
    setSaving(true)
    setError('')
    try {
      if (editing) {
        await adminApi.updateWalletChannel(editing.id, form)
        showMessage('收款方式已更新')
      } else {
        await adminApi.createWalletChannel(form)
        showMessage('收款方式已创建')
      }
      setDialogOpen(false)
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存收款方式失败')
    } finally {
      setSaving(false)
    }
  }

  const toggleStatus = async (item: PaymentChannel) => {
    const next = item.status === 'enabled' ? 'disabled' : 'enabled'
    try {
      await adminApi.setWalletChannelStatus(item.id, next)
      showMessage(next === 'enabled' ? '收款方式已启用' : '收款方式已停用')
      await load()
    } catch (reason) {
      showMessage(reason instanceof Error ? reason.message : '更新状态失败', 'error')
    }
  }

  const remove = async (item: PaymentChannel) => {
    if (!window.confirm(`确认删除收款方式「${item.name}」？`)) return
    try {
      await adminApi.deleteWalletChannel(item.id)
      showMessage('收款方式已删除')
      await load()
    } catch (reason) {
      showMessage(reason instanceof Error ? reason.message : '删除失败', 'error')
    }
  }

  return (
    <Box p={{ xs: 2, lg: 2.5 }}>
      <PageHeader
        eyebrow="业务管理 / 钱包"
        title="钱包配置"
        description="管理收款方式、出款渠道和充值规则。"
        actions={
          <>
            <Button variant="contained" startIcon={<AddRounded />} onClick={openCreate}>新增收款方式</Button>
            <Button variant="outlined" startIcon={<RefreshRounded />} onClick={() => void load(true)} disabled={loading}>刷新</Button>
          </>
        }
      />
      {error && <Alert severity="error" sx={{ mt: 2 }}>{error}</Alert>}
      <Stack gap={1.5} mt={2.5}>
        <Paper variant="outlined" sx={{ p: 1.5 }}>
          <Stack direction={{ xs: 'column', sm: 'row' }} gap={1} flexWrap="wrap">
            <TextField
              placeholder="搜索支付名称、商户号或备注"
              value={query}
              onChange={event => setQuery(event.target.value)}
              sx={{ minWidth: { sm: 220 }, flex: { xs: 1, lg: 0 } }}
              slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> } }}
            />
            <TextField select value={status} onChange={event => setStatus(event.target.value)} sx={{ minWidth: 145 }}>
              <MenuItem value="all">全部状态</MenuItem>
              <MenuItem value="enabled">已启用</MenuItem>
              <MenuItem value="disabled">已停用</MenuItem>
            </TextField>
            <Button variant="contained" onClick={() => setApplied({ query: query.trim(), status })}>查询</Button>
            <Button variant="outlined" onClick={() => { setQuery(''); setStatus('all'); setApplied({ query: '', status: 'all' }); showMessage('筛选条件已重置', 'info') }}>重置</Button>
          </Stack>
        </Paper>
        <Card>
          {loading && <Box px={2} py={1}><CircularProgress size={18} /></Box>}
          <TableContainer>
            <Table size="small" sx={{ minWidth: 980 }}>
              <TableHead>
                <TableRow>
                  {['序号', '支付接口', '支付名称', '商户号', '充值种类', '手续费率', '最小金额', '最大金额', '状态', '操作'].map(column => <TableCell key={column}>{column}</TableCell>)}
                </TableRow>
              </TableHead>
              <TableBody>
                {items.map((item, index) => (
                  <TableRow hover key={item.id}>
                    <TableCell>{index + 1}</TableCell>
                    <TableCell>{item.provider}</TableCell>
                    <TableCell>
                      <Typography fontSize={13} fontWeight={700}>{item.name}</Typography>
                      {item.remark && <Typography fontSize={11} color="text.secondary">{item.remark}</Typography>}
                    </TableCell>
                    <TableCell>{item.merchant_no || '—'}</TableCell>
                    <TableCell><Chip size="small" label={creditLabels[item.credit_type] ?? item.credit_type} /></TableCell>
                    <TableCell>{item.fee_rate.toFixed(2)}%</TableCell>
                    <TableCell>{money(item.min_amount)}</TableCell>
                    <TableCell>{money(item.max_amount)}</TableCell>
                    <TableCell>
                      <Stack direction="row" alignItems="center" gap={1}>
                        <Switch size="small" checked={item.status === 'enabled'} onChange={() => void toggleStatus(item)} />
                        <Chip size="small" color={item.status === 'enabled' ? 'success' : 'default'} label={item.status === 'enabled' ? '启用' : '停用'} />
                      </Stack>
                    </TableCell>
                    <TableCell align="right">
                      <Tooltip title="编辑"><IconButton size="small" onClick={() => openEdit(item)}><EditRounded fontSize="small" /></IconButton></Tooltip>
                      <Tooltip title="删除"><IconButton size="small" color="error" onClick={() => void remove(item)}><DeleteRounded fontSize="small" /></IconButton></Tooltip>
                    </TableCell>
                  </TableRow>
                ))}
                {!loading && !items.length && (
                  <TableRow>
                    <TableCell colSpan={10}>
                      <Stack minHeight={260} alignItems="center" justifyContent="center" color="text.secondary">
                        <Box sx={{ width: 58, height: 58, display: 'grid', placeItems: 'center', borderRadius: 3, bgcolor: 'action.hover', color: 'primary.main' }}><InboxRounded /></Box>
                        <Typography mt={1.5} fontSize={13} fontWeight={700}>暂无收款方式</Typography>
                        <Typography variant="caption">点击右上角新增收款方式开始配置</Typography>
                      </Stack>
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>
        </Card>
      </Stack>

      <Dialog open={dialogOpen} onClose={() => !saving && setDialogOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>{editing ? '编辑收款方式' : '新增收款方式'}</DialogTitle>
        <DialogContent>
          <Stack gap={2} pt={1}>
            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,1fr)' }, gap: 2 }}>
              <TextField label="支付接口" required value={form.provider} onChange={event => setForm(current => ({ ...current, provider: event.target.value }))} />
              <TextField label="支付名称" required value={form.name} onChange={event => setForm(current => ({ ...current, name: event.target.value }))} />
              <TextField label="商户号" value={form.merchant_no} onChange={event => setForm(current => ({ ...current, merchant_no: event.target.value }))} />
              <TextField select label="充值种类" value={form.credit_type} onChange={event => setForm(current => ({ ...current, credit_type: event.target.value }))}>
                {Object.entries(creditLabels).map(([value, label]) => <MenuItem key={value} value={value}>{label}</MenuItem>)}
              </TextField>
              <TextField type="number" label="手续费率 (%)" value={form.fee_rate} onChange={event => setForm(current => ({ ...current, fee_rate: Number(event.target.value) }))} />
              <TextField select label="状态" value={form.status} onChange={event => setForm(current => ({ ...current, status: event.target.value as PaymentChannel['status'] }))}>
                <MenuItem value="enabled">启用</MenuItem>
                <MenuItem value="disabled">停用</MenuItem>
              </TextField>
              <TextField type="number" label="最小金额" value={form.min_amount} onChange={event => setForm(current => ({ ...current, min_amount: Number(event.target.value) }))} />
              <TextField type="number" label="最大金额" value={form.max_amount} onChange={event => setForm(current => ({ ...current, max_amount: Number(event.target.value) }))} />
            </Box>
            <TextField label="备注" multiline minRows={3} value={form.remark} onChange={event => setForm(current => ({ ...current, remark: event.target.value }))} />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDialogOpen(false)} disabled={saving}>取消</Button>
          <Button variant="contained" onClick={() => void submit()} disabled={saving || !form.provider.trim() || !form.name.trim()}>{saving ? '保存中…' : '确认'}</Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}
