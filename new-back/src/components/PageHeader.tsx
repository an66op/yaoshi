import { Stack } from '@mui/material'
import type { ReactNode } from 'react'

type PageHeaderProps = {
  eyebrow: string
  title: string
  description: string
  actions?: ReactNode
}

export function PageHeader({ actions }: PageHeaderProps) {
  if (!actions) return null
  return <Stack direction="row" justifyContent="flex-end" alignItems="center" gap={1} flexWrap="wrap" sx={{ minHeight: 36, '& > *': { flexGrow: { xs: 1, sm: 0 } } }}>
    {actions}
  </Stack>
}
