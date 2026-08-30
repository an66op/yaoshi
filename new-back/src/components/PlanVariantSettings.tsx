import { Alert, Box, Button, Checkbox, FormControlLabel, Paper, Stack, Typography } from '@mui/material'
import type { PlanPositionOption, PlanVariantOption } from '../api'

type Props = {
  positions: number[]
  planKeys: string[]
  availablePositions: PlanPositionOption[]
  options: PlanVariantOption[]
  disabled?: boolean
  maxActiveStreams: number
  onPositionsChange: (positions: number[]) => void
  onPlanKeysChange: (keys: string[]) => void
}

const groups: Array<{ kind: PlanVariantOption['kind']; label: string }> = [
  { kind: 'numbers', label: '号码计划' },
  { kind: 'size', label: '大小计划' },
  { kind: 'parity', label: '单双计划' },
  { kind: 'dragon_tiger', label: '龙虎计划' },
]

const toggle = <T,>(items: T[], value: T, checked: boolean): T[] => checked ? [...new Set([...items, value])] : items.filter(item => item !== value)

export function PlanVariantSettings({ positions, planKeys, availablePositions, options, disabled, maxActiveStreams, onPositionsChange, onPlanKeysChange }: Props) {
  const hasDefault = positions.includes(1) && planKeys.includes('four-period-five-codes')
  return <Paper variant="outlined" sx={{ p: 1.5, borderRadius: '10px' }}>
    <Stack direction="row" gap={1} alignItems="center" justifyContent="space-between" mb={1}>
      <Box><Typography fontWeight={850} fontSize={14}>极速赛车 · 可选计划</Typography><Typography color="text.secondary" fontSize={12}>会员只能切换到勾选的名次与计划。</Typography></Box>
      <Button size="small" disabled={disabled} onClick={() => { onPositionsChange(availablePositions.map(item => item.position)); onPlanKeysChange(options.map(item => item.key)) }}>全部开放</Button>
    </Stack>
    <Typography fontSize={12} fontWeight={800} mb={.5}>名次</Typography>
    <Box display="grid" gridTemplateColumns={{ xs: 'repeat(2,minmax(0,1fr))', sm: 'repeat(5,minmax(0,1fr))' }} mb={1.5}>
      {availablePositions.map(item => <FormControlLabel key={item.position} sx={{ m: 0, minWidth: 0 }} label={<Typography fontSize={12}>{item.label}</Typography>} control={<Checkbox size="small" disabled={disabled} checked={positions.includes(item.position)} onChange={event => onPositionsChange(toggle(positions, item.position, event.target.checked))} />} />)}
    </Box>
    <Box display="grid" gridTemplateColumns={{ xs: '1fr', md: 'repeat(2,minmax(0,1fr))' }} gap={1}>
      {groups.map(group => <Box key={group.kind} sx={{ p: 1, bgcolor: 'action.hover', borderRadius: '6px' }}>
        <Typography fontSize={12} fontWeight={800}>{group.label}</Typography>
        <Box display="grid" gridTemplateColumns="repeat(2,minmax(0,1fr))">
          {options.filter(item => item.kind === group.kind).map(item => <FormControlLabel key={item.key} sx={{ m: 0, minWidth: 0 }} label={<Typography fontSize={12}>{item.label}</Typography>} control={<Checkbox size="small" disabled={disabled} checked={planKeys.includes(item.key)} onChange={event => onPlanKeysChange(toggle(planKeys, item.key, event.target.checked))} />} />)}
        </Box>
      </Box>)}
    </Box>
    <Stack gap={.5} mt={1.2}>
      <Typography fontSize={12} color="text.secondary">“四期五码”表示同组 5 个号码跨 4 个实际开放期使用，再生成下一组；不预造未来期号，断档会重新起组。大小按 1–5 / 6–10 区分；龙虎比较所选名次与对位（第 1 对第 10、第 2 对第 9，以此类推）。</Typography>
      <Typography fontSize={12} color="text.secondary">按访问生成，最多同时访问 {maxActiveStreams} 个名次与计划组合，不预留默认名额，不为全部组合批量生成记录。其他彩种配置不受影响。</Typography>
      {!hasDefault && <Alert severity="info" sx={{ mt: .5 }}>默认的“冠军·四期五码”未开放，会员需先选择已开放的计划。已发布历史不会被改写。</Alert>}
    </Stack>
  </Paper>
}
