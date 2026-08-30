import { MenuItem, Stack, TextField } from '@mui/material'
import { isSpeedRacingPlan, planRecommendationNumberError, SPEED_RACING_PLAN_RULE, type PlanRecommendationDraft } from '../utils/planRecommendation'

type NumberFieldValues = Pick<PlanRecommendationDraft, 'numbersText' | 'size' | 'parity'>

export function PlanRecommendationNumberFields({ gameId, value, onChange }: { gameId: string; value: NumberFieldValues; onChange: (patch: Partial<NumberFieldValues>) => void }) {
  const racing = isSpeedRacingPlan(gameId)
  const error = value.numbersText.trim() ? planRecommendationNumberError(gameId, value.numbersText) : ''
  return <>
    <TextField label="推荐号码" error={Boolean(error)} helperText={error || (racing ? SPEED_RACING_PLAN_RULE : '使用逗号分隔，例如 1,5,9')} value={value.numbersText} onChange={event => onChange({ numbersText: event.target.value })} />
    {!racing && <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.3}>
      <TextField select fullWidth label="大小" value={value.size} onChange={event => onChange({ size: event.target.value as NumberFieldValues['size'] })}><MenuItem value="">不推荐</MenuItem><MenuItem value="大">大</MenuItem><MenuItem value="小">小</MenuItem></TextField>
      <TextField select fullWidth label="单双" value={value.parity} onChange={event => onChange({ parity: event.target.value as NumberFieldValues['parity'] })}><MenuItem value="">不推荐</MenuItem><MenuItem value="单">单</MenuItem><MenuItem value="双">双</MenuItem></TextField>
    </Stack>}
  </>
}
