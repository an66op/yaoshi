import type { WsEvent } from '../hooks/useWebSocket'

/**
 * Game catalog events are server-filtered by authenticated workspace first.
 * The room-code comparison is a second guard for a frame already queued while
 * a member switches rooms. Platform events deliberately omit room_code.
 */
export function shouldReloadGameCatalog(event: WsEvent, currentRoomCode: string) {
  if (event.type === 'draw_update') return true
  if (event.type !== 'game_catalog_update') return false

  const eventRoomCode = typeof event.data?.room_code === 'string'
    ? event.data.room_code.trim()
    : ''
  return eventRoomCode === '' || eventRoomCode === currentRoomCode.trim()
}
