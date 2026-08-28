export type RoomHistoryItem = {
  code: string
  name: string
  logo?: string
  status: 'current' | 'available' | 'pending' | 'disabled'
  lastUsedAt?: number
}

export function buildRoomEntries(
  current: Pick<RoomHistoryItem, 'code' | 'name' | 'logo'>,
  history: RoomHistoryItem[],
  limit = 8,
) {
  const byCode = new Map<string, RoomHistoryItem>()
  for (const item of history) {
    if (!/^\d{5,12}$/.test(item.code) || byCode.has(item.code)) continue
    byCode.set(item.code, item)
  }
  // A verified session may still belong to a legacy four-digit room. Keep
  // that current room visible even though new manual entries require 5 digits.
  if (/^\d{1,12}$/.test(current.code)) {
    byCode.set(current.code, {
      code: current.code,
      name: current.name || current.code,
      logo: current.logo,
      status: 'current',
      lastUsedAt: Number.MAX_SAFE_INTEGER,
    })
  }
  return [...byCode.values()]
    .sort((left, right) => {
      if (left.status === 'current' || right.status === 'current') {
        return left.status === 'current' ? -1 : 1
      }
      return (right.lastUsedAt ?? 0) - (left.lastUsedAt ?? 0)
    })
    .slice(0, Math.max(1, limit))
}
