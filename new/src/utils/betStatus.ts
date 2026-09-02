const betStatusLabels: Record<string, string> = {
  pending: '待开奖',
  won: '已中奖',
  lost: '未中奖',
  cancelled: '已撤销',
}

export function isSettlementPush(status: string, remark?: string | null): boolean {
  if (status === 'push') return true
  if (status !== 'cancelled') return false
  const detail = String(remark ?? '').trim()
  return detail.includes('和局') && (detail.includes('返还本金') || detail.includes('返本'))
}

export function betStatusText(status: string, remark?: string | null): string {
  return isSettlementPush(status, remark) ? '和局返本' : betStatusLabels[status] ?? status
}

export function betStatusTone(status: string, remark?: string | null): string {
  return isSettlementPush(status, remark) ? 'push' : status
}
