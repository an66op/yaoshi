import { Avatar, Box, Paper, Stack, Typography } from '@mui/material'
import type { PlanAutomationConfig } from '../api'

export function PlanAutomationExperts({ masters }: Pick<PlanAutomationConfig, 'masters'>) {
  return <Box>
    <Typography fontWeight={800} fontSize={13} mb={1}>专家模板 · {masters.length} 位</Typography>
    <Box display="grid" gridTemplateColumns={{ xs: '1fr', sm: 'repeat(3,minmax(0,1fr))' }} gap={1}>
      {masters.map(master => <Paper key={master.key} variant="outlined" sx={{ p: 1.2 }}>
        <Stack direction="row" alignItems="center" gap={1}><Avatar sx={{ width: 32, height: 32, bgcolor: master.color, fontSize: 15 }}>{master.name.slice(0, 1)}</Avatar><Box minWidth={0}><Typography fontWeight={800} fontSize={13}>{master.name}</Typography><Typography color="text.secondary" fontSize={11}>{master.title}</Typography></Box></Stack>
        <Typography fontSize={11} color="text.secondary" mt={1}>系统固定模板 · 自动生成</Typography>
      </Paper>)}
    </Box>
  </Box>
}
