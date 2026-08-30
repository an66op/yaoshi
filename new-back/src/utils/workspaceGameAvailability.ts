import type { WorkspaceGame } from '../api'

type GameAvailabilityInput = Pick<WorkspaceGame, 'platform_enabled' | 'room_enabled' | 'lobby_category' | 'enabled'>

export type WorkspaceGameAvailability = {
  status: 'platform_closed' | 'uncategorized' | 'room_closed' | 'unavailable' | 'available'
  available: boolean
  canEnable: boolean
  label: string
  detail: string
  color: 'default' | 'warning' | 'success'
}

/** Explain known blockers, but only the server can confirm effective availability. */
export function workspaceGameAvailability(game: GameAvailabilityInput): WorkspaceGameAvailability {
  if (!game.platform_enabled) return {
    status: 'platform_closed', available: false, canEnable: false,
    label: '平台已关闭', detail: '等待平台开放后才能开启；已打开的房间开关仍可关闭。', color: 'default',
  }
  if (!game.lobby_category?.trim()) return {
    status: 'uncategorized', available: false, canEnable: false,
    label: '未上架 · 待平台分类', detail: '平台尚未设置大厅分类，会员端不会显示；请等待平台分类后再开启。', color: 'warning',
  }
  if (!game.room_enabled) return {
    status: 'room_closed', available: false, canEnable: true,
    label: '房间已关闭', detail: '分类沿用平台，可按需开启；会员端是否显示以服务端可用状态为准。', color: 'default',
  }
  if (!game.enabled) return {
    status: 'unavailable', available: false, canEnable: false,
    label: '当前不可用', detail: '房间开关已开启，但服务端当前未开放此彩种；请检查房间状态或联系平台。', color: 'warning',
  }
  return {
    status: 'available', available: true, canEnable: true,
    label: '房间已开放', detail: '平台已开放且已分类，服务端已确认本房彩种可用。', color: 'success',
  }
}
