import FiberManualRecordRounded from '@mui/icons-material/FiberManualRecordRounded'
import { Chip } from '@mui/material'

export function UserPresenceChip({ online = false }: { online?: boolean }) {
  return <Chip
    size="small"
    variant="outlined"
    color={online ? 'success' : 'default'}
    icon={<FiberManualRecordRounded />}
    label={online ? '在线' : '离线'}
    aria-label={online ? '当前在线' : '当前离线'}
    sx={{
      height: 23,
      '& .MuiChip-icon': { ml: .65, fontSize: 10 },
      '& .MuiChip-label': { px: .75, fontSize: 10, fontWeight: 800 },
    }}
  />
}
