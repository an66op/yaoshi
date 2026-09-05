import type { RoomSettings } from '../api/portal'

/** The lobby owns optional login popups; the room header remains the durable
 * place where every enabled tenant announcement can be read afterwards. */
export function selectedRoomAnnouncement(settings: RoomSettings) {
  const current = [...(settings.announcements ?? [])]
    .filter((item) => item.enabled && item.content.trim())
    .sort((left, right) => left.sort_order - right.sort_order)
  if (current.length === 1) return current[0].content.trim()
  if (current.length > 1) {
    return current.map((item) => {
      const title = item.title.trim()
      return title ? `${title}：${item.content.trim()}` : item.content.trim()
    }).join('\n')
  }
  return settings.room_notice?.trim() || ''
}
