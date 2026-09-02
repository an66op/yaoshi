// Matches the first preset in management's room Logo picker.
export const DEFAULT_ROOM_LOGO = '/images/wangzhe-header-logo.png'

const roomLogo = (value?: string) => value?.trim() || DEFAULT_ROOM_LOGO

export type RoomHistoryItem = {
  code: string
  name: string
  logo?: string
  status: 'current' | 'available' | 'pending' | 'disabled'
  lastUsedAt?: number
}

type RoomDisplayItem = RoomHistoryItem & { logo: string }

export function buildRecentRoomEntries(history: RoomHistoryItem[], limit = 8) {
  const byCode = new Map<string, RoomDisplayItem>()
  for (const item of history) {
    if (!/^\d{5,12}$/.test(item.code) || byCode.has(item.code)) continue
    byCode.set(item.code, { ...item, logo: roomLogo(item.logo) })
  }
  return [...byCode.values()]
    .sort((left, right) => {
      const leftCurrent = left.status === 'current'
      const rightCurrent = right.status === 'current'
      if (leftCurrent !== rightCurrent) return leftCurrent ? -1 : 1
      return (right.lastUsedAt ?? 0) - (left.lastUsedAt ?? 0)
    })
    .slice(0, Math.max(0, limit))
}

export function buildRoomEntries(
  current: Pick<RoomHistoryItem, 'code' | 'name' | 'logo'>,
  history: RoomHistoryItem[],
  limit = 8,
) {
  const byCode = new Map<string, RoomHistoryItem>()
  for (const item of buildRecentRoomEntries(history, Number.MAX_SAFE_INTEGER)) byCode.set(item.code, item)
  // A verified session may still belong to a legacy four-digit room. Keep
  // that current room visible even though new manual entries require 5 digits.
  if (/^\d{1,12}$/.test(current.code)) {
    byCode.set(current.code, {
      code: current.code,
      name: current.name || current.code,
      logo: roomLogo(current.logo === undefined ? byCode.get(current.code)?.logo : current.logo),
      status: 'current',
      lastUsedAt: Number.MAX_SAFE_INTEGER,
    })
  }
  return [...byCode.values()]
    .sort((left, right) => {
      const leftCurrent = left.status === 'current'
      const rightCurrent = right.status === 'current'
      if (leftCurrent !== rightCurrent) return leftCurrent ? -1 : 1
      return (right.lastUsedAt ?? 0) - (left.lastUsedAt ?? 0)
    })
    .slice(0, Math.max(1, limit))
}
