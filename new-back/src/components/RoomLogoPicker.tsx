import DeleteOutlineRounded from '@mui/icons-material/DeleteOutlineRounded'
import PhotoCameraRounded from '@mui/icons-material/PhotoCameraRounded'
import { Avatar, Box, Button, IconButton, Stack, Tooltip, Typography } from '@mui/material'
import type { ChangeEventHandler, ReactNode } from 'react'
import { roomLogoPresets } from '../roomLogoPresets'

type RoomLogoPickerProps = {
  value: string
  fallback: string
  heading?: ReactNode
  description?: ReactNode
  previewSize?: number
  fallbackBackground?: string
  onChange: (value: string) => void
  onUpload: ChangeEventHandler<HTMLInputElement>
}

export function RoomLogoPicker({
  value,
  fallback,
  heading,
  description,
  previewSize = 64,
  fallbackBackground = 'linear-gradient(135deg,#0891b2 0%,#2563eb 54%,#7c3aed 100%)',
  onChange,
  onUpload,
}: RoomLogoPickerProps) {
  return <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ sm: 'center' }} gap={1.5}>
    <Avatar
      aria-label="当前房间 Logo"
      src={value || undefined}
      variant="rounded"
      sx={{
        width: previewSize,
        height: previewSize,
        flexShrink: 0,
        color: '#fff',
        background: value ? 'transparent' : fallbackBackground,
        bgcolor: value ? 'action.hover' : undefined,
        border: 1,
        borderColor: 'divider',
        boxShadow: '0 4px 12px rgba(37,99,235,.18)',
        fontSize: Math.round(previewSize * .38),
        fontWeight: 950,
        '& .MuiAvatar-img': { objectFit: 'contain' },
      }}
    >
      {(fallback || '房').slice(0, 1)}
    </Avatar>

    <Box flex={1} minWidth={0}>
      {heading && <Typography fontWeight={900}>{heading}</Typography>}
      {description && <Typography fontSize={10.5} color="text.secondary">{description}</Typography>}
      <Stack direction="row" alignItems="center" gap={.7} flexWrap="wrap" useFlexGap mt={heading || description ? .85 : 0} role="group" aria-label="默认房间 Logo">
        {roomLogoPresets.map(preset => {
          const selected = value === preset.path
          return <Tooltip title={preset.label} key={preset.id}>
            <IconButton
              aria-label={`使用${preset.label} Logo`}
              aria-pressed={selected}
              onClick={() => onChange(preset.path)}
              sx={{
                width: 48,
                height: 48,
                p: .35,
                border: 2,
                borderColor: selected ? 'primary.main' : 'divider',
                borderRadius: 2,
                bgcolor: selected ? 'action.selected' : 'transparent',
                '&:hover': { bgcolor: 'action.hover', borderColor: selected ? 'primary.main' : 'text.disabled' },
                '&.Mui-focusVisible': { outline: '3px solid', outlineColor: 'primary.light', outlineOffset: 2 },
              }}
            >
              <Avatar alt="" src={preset.path} variant="rounded" sx={{ width: 38, height: 38, bgcolor: 'transparent', '& .MuiAvatar-img': { objectFit: 'contain' } }} />
            </IconButton>
          </Tooltip>
        })}
        <Button component="label" size="small" variant="outlined" startIcon={<PhotoCameraRounded />} sx={{ whiteSpace: 'nowrap' }}>
          {value ? '更换 Logo' : '选择 Logo'}
          <input hidden type="file" accept="image/png,image/jpeg,image/webp" onChange={onUpload} />
        </Button>
        {value && <Button size="small" color="error" startIcon={<DeleteOutlineRounded />} onClick={() => onChange('')}>移除</Button>}
      </Stack>
    </Box>
  </Stack>
}
