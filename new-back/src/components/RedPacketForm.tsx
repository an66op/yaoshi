import CardGiftcardRounded from '@mui/icons-material/CardGiftcardRounded'
import { Box, Stack, TextField, Typography } from '@mui/material'
import type { ReactNode } from 'react'

export type RedPacketCover = 'classic' | 'celebration' | 'lucky'

const redPacketCovers: Array<{ value: RedPacketCover; label: string; note: string; gradient: string }> = [
  { value: 'classic', label: '经典红包', note: '暖橙金', gradient: 'linear-gradient(145deg, #ff9f42, #e95732)' },
  { value: 'celebration', label: '喜庆红包', note: '中国红', gradient: 'linear-gradient(145deg, #ff5b63, #bb1836)' },
  { value: 'lucky', label: '好运红包', note: '青金色', gradient: 'linear-gradient(145deg, #23b9ae, #087f86)' },
]

const coverGradient: Record<RedPacketCover, string> = Object.fromEntries(
  redPacketCovers.map(item => [item.value, item.gradient]),
) as Record<RedPacketCover, string>

function normalizedCover(value?: string): RedPacketCover {
  return value === 'celebration' || value === 'lucky' ? value : 'classic'
}

export function AdminRedPacketCard({
  count,
  total,
  greeting,
  cover,
  minTurnover = 0,
  status = 'active',
  claimedCount = 0,
  refunded = 0,
  closeReason,
  time,
  action,
  preview = false,
}: {
  count: number
  total: number
  greeting?: string
  cover?: string
  minTurnover?: number
  status?: string
  claimedCount?: number
  refunded?: number
  closeReason?: string
  time?: string
  action?: ReactNode
  preview?: boolean
}) {
  const tone = normalizedCover(cover)
  const amount = Number.isFinite(total) ? Math.max(0, total) : 0
  const statusLabel = status === 'empty' ? '已领完' : status === 'expired' ? '已过期' : status === 'closed' ? '已关闭' : ''
  return <Box
    aria-label={`${greeting || '恭喜发财'}，${count || 1} 个红包，总金额 ${amount.toFixed(2)}`}
    sx={{
      width: 'min(100%, 330px)',
      overflow: 'hidden',
      borderRadius: 1.35,
      background: coverGradient[tone],
      color: '#fff',
      boxShadow: preview ? '0 13px 30px rgba(126,39,37,.24)' : '0 8px 20px rgba(92,38,32,.2)',
      border: '1px solid rgba(255,255,255,.28)',
    }}
  >
    <Stack direction="row" alignItems="center" gap={1.15} px={1.45} py={1.35}>
      <Box sx={{
        display: 'grid', placeItems: 'center', flex: '0 0 auto', width: 43, height: 43,
        borderRadius: '50%', bgcolor: 'rgba(255,236,187,.94)', color: tone === 'lucky' ? '#08747f' : '#be3d34',
        boxShadow: 'inset 0 0 0 1px rgba(255,255,255,.65), 0 4px 10px rgba(83,29,22,.2)',
      }}><CardGiftcardRounded sx={{ fontSize: 24 }} /></Box>
      <Box minWidth={0} flex={1}>
        <Typography fontSize={14} fontWeight={850} noWrap>{greeting?.trim() || '恭喜发财'}</Typography>
        <Typography fontSize={10.5} sx={{ opacity: .88 }}>{statusLabel || `${Math.max(1, count || 1)} 个红包 · 点击领取`}</Typography>
      </Box>
    </Stack>
    <Stack direction="row" alignItems="center" gap={.75} px={1.45} py={.78} sx={{ bgcolor: 'rgba(76,22,21,.16)', borderTop: '1px solid rgba(255,255,255,.2)' }}>
      <Typography fontSize={9.5} fontWeight={750} sx={{ opacity: .9 }}>{statusLabel && closeReason ? closeReason : '王者奖励'}</Typography>
      {minTurnover > 0 && <Typography fontSize={9.2} sx={{ opacity: .9 }}>流水满 ¥ {minTurnover.toFixed(2)}</Typography>}
      <Typography ml="auto" fontSize={9.5} sx={{ opacity: .82 }}>{Math.max(0, claimedCount)}/{Math.max(1, count || 1)} 已领取 · ¥ {amount.toFixed(2)}</Typography>
      {refunded > 0 && <Typography fontSize={9.2} sx={{ opacity: .86 }}>退回 ¥ {refunded.toFixed(2)}</Typography>}
      {time && <Typography fontSize={9} sx={{ opacity: .68 }}>{time}</Typography>}
      {action}
    </Stack>
  </Box>
}

export function RedPacketForm({
  count,
  total,
  greeting,
  cover,
  minTurnover,
  onCount,
  onTotal,
  onGreeting,
  onCover,
  onMinTurnover,
}: {
  count: string
  total: string
  greeting: string
  cover: RedPacketCover
  minTurnover: string
  onCount: (value: string) => void
  onTotal: (value: string) => void
  onGreeting: (value: string) => void
  onCover: (value: RedPacketCover) => void
  onMinTurnover: (value: string) => void
}) {
  const average = Number(count) > 0 && Number(total) > 0 ? Number(total) / Number(count) : 0
  return <Stack gap={1.8} pt={.5}>
    <Box>
      <Typography fontSize={11} fontWeight={850} color="text.secondary" mb={.8}>发送预览</Typography>
      <Box display="flex" justifyContent="center">
        <AdminRedPacketCard
          preview
          count={Number.isInteger(Number(count)) && Number(count) > 0 ? Number(count) : 1}
          total={Number(total) || 0}
          greeting={greeting}
          cover={cover}
          minTurnover={Number(minTurnover) || 0}
        />
      </Box>
    </Box>
    <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.25}>
      <TextField
        autoFocus
        fullWidth
        type="number"
        label="红包个数"
        value={count}
        onChange={event => onCount(event.target.value)}
        inputProps={{ min: 1, max: 500, step: 1 }}
      />
      <TextField
        fullWidth
        type="number"
        label="总金额"
        value={total}
        onChange={event => onTotal(event.target.value)}
        inputProps={{ min: .01, max: 1_000_000, step: .01 }}
      />
    </Stack>
      <Typography variant="caption" color="text.secondary" mt={-1.05}>
      {average > 0 ? `平均每个约 ${average.toFixed(2)}` : '每个红包至少 0.01'}
    </Typography>
    <TextField
      fullWidth
      label="红包标语"
      value={greeting}
      onChange={event => onGreeting(event.target.value)}
      inputProps={{ maxLength: 30 }}
      placeholder="恭喜发财"
    />
    <TextField
      fullWidth
      type="number"
      label="领取所需当日有效流水"
      value={minTurnover}
      onChange={event => onMinTurnover(event.target.value)}
      inputProps={{ min: 0, max: 1_000_000_000, step: .01 }}
      helperText="填 0 表示不限制；仅统计北京时间当天已结算的有效投注"
    />
    <Box>
      <Typography fontSize={12} fontWeight={850} mb={1}>红包封面</Typography>
      <Box display="grid" gridTemplateColumns="repeat(3, minmax(0, 1fr))" gap={1}>
        {redPacketCovers.map(item => <Box
          key={item.value}
          component="button"
          type="button"
          onClick={() => onCover(item.value)}
          aria-pressed={cover === item.value}
          sx={{
            minWidth: 0,
            p: .65,
            border: 2,
            borderColor: cover === item.value ? 'primary.main' : 'divider',
            borderRadius: 1.35,
            bgcolor: cover === item.value ? 'action.selected' : 'background.paper',
            color: 'text.primary',
            cursor: 'pointer',
            textAlign: 'left',
          }}
        >
          <Box sx={{ height: 48, borderRadius: 1.4, background: item.gradient, display: 'grid', placeItems: 'center', color: '#fff', boxShadow: 'inset 0 0 0 1px #ffffff2f' }}><CardGiftcardRounded sx={{ fontSize: 23 }} /></Box>
          <Typography noWrap fontSize={10.5} fontWeight={800} mt={.65}>{item.label}</Typography>
          <Typography noWrap fontSize={9} color="text.secondary">{item.note}</Typography>
        </Box>)}
      </Box>
    </Box>
  </Stack>
}
