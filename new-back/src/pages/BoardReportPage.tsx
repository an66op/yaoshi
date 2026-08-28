import { Box } from '@mui/material'
import { PageHeader } from '../components/PageHeader'
import { BoardReportPanel } from '../components/BoardReportPanel'

export function BoardReportPage() {
  return (
    <Box p={{ xs: 2, lg: 2.5 }}>
      <PageHeader
        eyebrow="数据中心 / 打盘"
        title="打盘报表"
        description=""
      />
      <Box mt={2.5}>
        <BoardReportPanel />
      </Box>
    </Box>
  )
}
