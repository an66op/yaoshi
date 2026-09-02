import { Box, Chip, Paper, Stack, TextField, Typography } from '@mui/material'
import { isValidSealSeconds } from '../utils/sealSeconds'
import { alignedLotteryGameIds } from '../utils/gameTimingOverrides'

type GameItem = { id: string; name: string; enabled?: boolean; platform_enabled?: boolean }
export type GameTimingOverrides = Record<string, { seal_seconds?: number }>

/** Edits the room snapshot used by the backend; a blank field inherits the room default. */
export function GameSealSecondsOverrides({
  scope,
  games,
  defaultSeconds,
  value,
  disabled = false,
  onChange,
}: {
  scope: 'platform' | 'room'
  games: GameItem[]
  defaultSeconds: number | undefined
  value: GameTimingOverrides | undefined
  disabled?: boolean
  onChange: (value: GameTimingOverrides) => void
}) {
  const configured = value && typeof value === 'object' && !Array.isArray(value) ? value : {}
  const byId = new Map(games.map(game => [game.id, game]))
  const rows = alignedLotteryGameIds.map(id => byId.get(id)).filter((game): game is GameItem => Boolean(game))
  if (!rows.length) return null

  const change = (gameId: string, raw: string) => {
    const next = { ...configured }
    if (raw.trim() === '') {
      delete next[gameId]
    } else {
      next[gameId] = { seal_seconds: Number(raw) }
    }
    onChange(next)
  }

  return <Paper variant="outlined" sx={{ p: 1.5 }}>
    <Stack direction={{ xs: 'column', sm: 'row' }} gap={.8} justifyContent="space-between" mb={1.25}>
      <Box>
        <Typography fontWeight={850}>彩票彩种独立封盘</Typography>
        <Typography variant="caption" color="text.secondary">
          留空继承{scope === 'platform' ? '平台默认值；新房间创建时复制这份配置' : '本房间默认值'}。
        </Typography>
      </Box>
      <Chip size="small" variant="outlined" label={`${Object.keys(configured).filter(id => alignedLotteryGameIds.includes(id as typeof alignedLotteryGameIds[number])).length}/8 已单独设置`} sx={{ width: 'fit-content' }} />
    </Stack>
    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,minmax(0,1fr))' }, gap: 1 }}>
      {rows.map(game => {
        const seconds = configured[game.id]?.seal_seconds
        const valid = seconds === undefined || isValidSealSeconds(seconds)
        const enabled = game.platform_enabled ?? game.enabled
        return <TextField
          key={game.id}
          size="small"
          type="number"
          label={game.name}
          value={seconds ?? ''}
          disabled={disabled}
          error={!valid}
          helperText={!valid ? '请输入 0～86400 的整数' : `${enabled === false ? '当前未启用 · ' : ''}留空继承 ${defaultSeconds ?? 30} 秒`}
          slotProps={{ htmlInput: { min: 0, max: 86400, step: 1, inputMode: 'numeric' } }}
          onChange={event => change(game.id, event.target.value)}
        />
      })}
    </Box>
  </Paper>
}
