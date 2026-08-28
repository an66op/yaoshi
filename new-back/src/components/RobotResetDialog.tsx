import {
  Alert, Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, TextField, ToggleButton, ToggleButtonGroup, Typography,
} from '@mui/material'
import AutoAwesomeRounded from '@mui/icons-material/AutoAwesomeRounded'
import TuneRounded from '@mui/icons-material/TuneRounded'
import { useMemo, useState } from 'react'
import type { RobotResetInput } from '../api'
import { createRequestId } from '../utils/requestId'

type RobotResetDialogProps = {
  open: boolean
  robotCount: number
  scopeLabel?: string
  submitting: boolean
  onClose: () => void
  onSubmit: (payload: RobotResetInput) => Promise<void>
}

const maxBalance = 100_000_000

function amountValue(value: string) {
  if (!value.trim()) return null
  const amount = Number(value)
  return Number.isFinite(amount) ? amount : null
}

export function RobotResetDialog({ open, robotCount, scopeLabel = '当前工作区', submitting, onClose, onSubmit }: RobotResetDialogProps) {
  const [requestId] = useState(() => createRequestId())
  const [mode, setMode] = useState<'random' | 'custom'>('random')
  const [balanceMin, setBalanceMin] = useState('1000000')
  const [balanceMax, setBalanceMax] = useState('10000000')
  const [nicknamePrefix, setNicknamePrefix] = useState('幸运用户')
  const [balance, setBalance] = useState('10000000')

  const validation = useMemo(() => {
    if (robotCount < 1) return '当前工作区暂无机器人'
    if (mode === 'custom') {
      const prefix = nicknamePrefix.trim()
      const target = amountValue(balance)
      if (!prefix) return '请填写昵称前缀'
      if (Array.from(prefix).length > 44) return '昵称前缀最多 44 个字符'
      if (target === null || target < 0 || target > maxBalance) return '统一余额应在 0–100000000 之间'
      return ''
    }
    const minimum = amountValue(balanceMin)
    const maximum = amountValue(balanceMax)
    if (minimum === null || maximum === null || minimum < 0 || maximum < 0 || minimum > maxBalance || maximum > maxBalance) return '随机余额范围应在 0–100000000 之间'
    if (minimum > maximum) return '随机余额下限不能大于上限'
    return ''
  }, [balance, balanceMax, balanceMin, mode, nicknamePrefix, robotCount])

  const submit = async () => {
    if (validation || submitting) return
    if (mode === 'custom') {
      await onSubmit({ request_id: requestId, mode, nickname_prefix: nicknamePrefix.trim(), balance: Number(balance) })
      return
    }
    await onSubmit({ request_id: requestId, mode, balance_min: Number(balanceMin), balance_max: Number(balanceMax) })
  }

  return <Dialog open={open} onClose={() => !submitting && onClose()} fullWidth maxWidth="sm">
    <DialogTitle>一键重置机器人</DialogTitle>
    <DialogContent>
      <Stack gap={1.6} pt={.5}>
        <Alert severity="warning">
          将覆盖{scopeLabel}全部 {robotCount} 个机器人的显示昵称和账户余额；不会影响真实会员、其他房间或机器人下注配置。
        </Alert>
        <Box>
          <Typography fontSize={12} fontWeight={850} mb={.8}>重置方式</Typography>
          <ToggleButtonGroup exclusive fullWidth size="small" value={mode} onChange={(_, value: 'random' | 'custom' | null) => value && setMode(value)}>
            <ToggleButton value="random"><AutoAwesomeRounded fontSize="small" sx={{ mr: .7 }} />随机生成</ToggleButton>
            <ToggleButton value="custom"><TuneRounded fontSize="small" sx={{ mr: .7 }} />自定义</ToggleButton>
          </ToggleButtonGroup>
        </Box>
        {mode === 'random' ? <>
          <Alert severity="info" icon={<AutoAwesomeRounded />}>昵称从现有机器人昵称素材库随机抽取且本次不重复；余额在设定区间内随机到分。</Alert>
          <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.2}>
            <TextField fullWidth type="number" label="随机余额下限" value={balanceMin} onChange={event => setBalanceMin(event.target.value)} slotProps={{ htmlInput: { min: 0, max: maxBalance, step: .01 } }} />
            <TextField fullWidth type="number" label="随机余额上限" value={balanceMax} onChange={event => setBalanceMax(event.target.value)} slotProps={{ htmlInput: { min: 0, max: maxBalance, step: .01 } }} />
          </Stack>
        </> : <>
          <TextField autoFocus fullWidth label="昵称前缀" value={nicknamePrefix} onChange={event => setNicknamePrefix(event.target.value)} inputProps={{ maxLength: 44 }} helperText={`将依次生成 ${nicknamePrefix.trim() || '昵称'}01、${nicknamePrefix.trim() || '昵称'}02…`} />
          <TextField fullWidth type="number" label="统一目标余额" value={balance} onChange={event => setBalance(event.target.value)} slotProps={{ htmlInput: { min: 0, max: maxBalance, step: .01 } }} helperText="这是重置后的余额，不是增加或扣减金额" />
        </>}
        {validation && <Typography color="error.main" fontSize={11}>{validation}</Typography>}
        <Typography color="text.secondary" fontSize={10.5}>每次确认使用唯一请求编号；网络重试不会重复执行。</Typography>
      </Stack>
    </DialogContent>
    <DialogActions>
      <Button disabled={submitting} onClick={onClose}>取消</Button>
      <Button color="warning" variant="contained" disabled={submitting || Boolean(validation)} onClick={() => void submit()}>{submitting ? '重置中…' : `确认重置 ${robotCount} 个`}</Button>
    </DialogActions>
  </Dialog>
}
