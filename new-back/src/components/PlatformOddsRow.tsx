import { Box, Checkbox, Chip, Paper, Stack, TextField, Typography } from '@mui/material'
import { memo } from 'react'
import type { PlayLimitItem } from '../api'
import { oddsEditableFields } from '../oddsEditing'

type PlatformOddsRowProps = {
  item: PlayLimitItem
  category: string
  example?: string
  selected: boolean
  disabled: boolean
  onSelect: (code: string, selected: boolean) => void
  onChange: (code: string, patch: Partial<PlayLimitItem>) => void
}

export function PlatformOddsRowView({ item, category, example, selected, disabled, onSelect, onChange }: PlatformOddsRowProps) {
  const modified = item.configuration_source === 'pending_admin_save'
  const configured = item.configured === true && item.configuration_source === 'admin_save' && Number.isFinite(item.odds) && item.odds > 1
  const confirmationLabel = modified ? '待保存确认'
    : item.configuration_source === 'rule_version_mismatch' ? '规则已变更'
      : configured ? '后台已确认' : '未配置 / 停用'

  return <Paper variant="outlined" sx={{ p: .75, borderRadius: 1, borderColor: modified ? 'warning.main' : !configured ? 'error.light' : 'divider' }}>
    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr 1fr', lg: 'minmax(155px,1.2fr) repeat(5,minmax(92px,1fr))' }, gap: .65, alignItems: 'center' }}>
      <Box sx={{ gridColumn: { xs: '1 / -1', lg: 'auto' } }}>
        <Stack direction="row" gap={.4} alignItems="center" flexWrap="wrap">
          <Checkbox size="small" checked={selected} disabled={disabled} inputProps={{ 'aria-label': `选择${item.play_name}` }} onChange={(_, checked) => onSelect(item.play_code, checked)} sx={{ p: .25 }} />
          <Typography fontSize={12.5} fontWeight={900}>{item.play_name}</Typography>
          <Chip size="small" variant="outlined" label={category} sx={{ height: 19, fontSize: 8.5 }} />
          <Chip size="small" color={modified ? 'warning' : configured ? 'success' : 'error'} variant="outlined" label={confirmationLabel} sx={{ height: 19, fontSize: 8.5 }} />
        </Stack>
        <Typography fontSize={9} color="text.secondary">{item.play_code}{example ? ` · 例：${example}` : ''}</Typography>
      </Box>
      {oddsEditableFields.map(field => <TextField
        key={field.key}
        size="small"
        type="number"
        label={field.label}
        value={item[field.key]}
        disabled={disabled}
        onChange={event => onChange(item.play_code, { [field.key]: Number(event.target.value) })}
        inputProps={{ min: 0, step: field.step, 'aria-label': `${item.play_name}${field.label}` }}
        sx={{ '& .MuiOutlinedInput-root': { borderRadius: 1 } }}
      />)}
    </Box>
  </Paper>
}

// Items and callbacks keep their identities when a different row is edited.
export const PlatformOddsRow = memo(PlatformOddsRowView)
