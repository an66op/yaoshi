import { Box, Stack, Typography } from '@mui/material'
import type { ReactNode } from 'react'

export function PageHeader({ eyebrow, title, description, actions }: { eyebrow: string; title: string; description: string; actions?: ReactNode }) {
  return <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" alignItems={{ xs: 'stretch', sm: 'flex-start' }} gap={2}>
    <Box><Typography variant="overline" color="primary.main" fontWeight={800} letterSpacing={1.2}>{eyebrow}</Typography><Typography variant="h4" mt={-.4}>{title}</Typography><Typography variant="body2" color="text.secondary" mt={.5}>{description}</Typography></Box>
    {actions && <Stack direction="row" gap={1} flexWrap="wrap" sx={{ '& > *': { flexGrow: { xs: 1, sm: 0 } } }}>{actions}</Stack>}
  </Stack>
}
