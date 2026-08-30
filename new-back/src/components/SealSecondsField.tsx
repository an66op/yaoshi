import { TextField } from '@mui/material'
import { SEAL_SECONDS_ERROR, isValidSealSeconds } from '../utils/sealSeconds'

type SealSecondsFieldProps = {
  scope: 'platform' | 'room'
  value: number | undefined
  onChange: (value: number | undefined) => void
  disabled?: boolean
}

/** Defaults are copied at room creation, not a read-only global override. */
export function SealSecondsField({ scope, value, onChange, disabled = false }: SealSecondsFieldProps) {
  const valid = isValidSealSeconds(value)
  const description = scope === 'platform'
    ? '新租户/代理房间创建时复制为初始值；已有房间不随平台后续修改同步。'
    : '仅当前房间生效，可独立调整；减小提前量不会重新开放已封盘的当期。'

  return <TextField
    fullWidth
    type="number"
    name="seal_seconds"
    label={scope === 'platform' ? '默认封盘秒数' : '本房间封盘秒数'}
    value={value ?? ''}
    disabled={disabled}
    error={!valid}
    helperText={<>{description}<br />{valid ? '开奖前提前封盘的秒数，0～86400 的整数。' : SEAL_SECONDS_ERROR}</>}
    slotProps={{ htmlInput: { min: 0, max: 86400, step: 1, inputMode: 'numeric' } }}
    onChange={event => onChange(event.target.value.trim() === '' ? undefined : Number(event.target.value))}
  />
}
