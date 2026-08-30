import { describe, expect, it } from 'vitest'
import { workspaceGameAvailability } from './workspaceGameAvailability'

describe('workspace game availability', () => {
  it.each([
    [false, false, '', 'platform_closed', false, false],
    [false, true, '', 'platform_closed', false, false],
    [false, false, '平台分类', 'platform_closed', false, false],
    [false, true, '平台分类', 'platform_closed', false, false],
    [true, false, '', 'uncategorized', false, false],
    [true, true, '', 'uncategorized', false, false],
    [true, false, '平台分类', 'room_closed', false, true],
    [true, true, '平台分类', 'available', true, true],
  ] as const)('platform=%s room=%s category=%s => %s', (platform, room, category, status, available, canEnable) => {
    expect(workspaceGameAvailability({ platform_enabled: platform, room_enabled: room, lobby_category: category, enabled: available }))
      .toMatchObject({ status, available, canEnable })
  })

  it('treats blank categories as unlisted even when both switches and a stale enabled flag are on', () => {
    const game = { platform_enabled: true, room_enabled: true, lobby_category: ' \n\t ', enabled: true }
    expect(workspaceGameAvailability(game)).toMatchObject({
      status: 'uncategorized', available: false, canEnable: false, color: 'warning', label: '未上架 · 待平台分类',
    })
  })

  it('accepts categories supplied by the platform without a client-side catalog', () => {
    expect(workspaceGameAvailability({ platform_enabled: true, room_enabled: true, lobby_category: '平台新增分类', enabled: true }))
      .toMatchObject({ status: 'available', available: true, color: 'success' })
  })

  it('counts only classified games with both switches and server availability enabled', () => {
    const games = [
      { platform_enabled: true, room_enabled: true, lobby_category: '', enabled: true },
      { platform_enabled: true, room_enabled: false, lobby_category: '已分类', enabled: false },
      { platform_enabled: false, room_enabled: true, lobby_category: '已分类', enabled: false },
      { platform_enabled: true, room_enabled: true, lobby_category: '已分类', enabled: false },
      { platform_enabled: true, room_enabled: true, lobby_category: '已分类', enabled: true },
    ]
    expect(games.filter(game => workspaceGameAvailability(game).available)).toHaveLength(1)
  })

  it('respects server denial even when both switches are on and the game is classified', () => {
    const game = { platform_enabled: true, room_enabled: true, lobby_category: '平台分类', enabled: false }
    expect(workspaceGameAvailability(game)).toMatchObject({
      status: 'unavailable', available: false, canEnable: false, color: 'warning', label: '当前不可用',
    })
    // No workspace-status field is returned: do not invent a specific reason.
    expect(workspaceGameAvailability(game).label).not.toContain('停用')
    expect(game.room_enabled).toBe(true)
  })

  it('requires a full server response rather than inferring availability from a saved room switch', () => {
    const game = { platform_enabled: true, room_enabled: false, lobby_category: '平台分类', enabled: false }
    const switchOnly = { ...game, room_enabled: true }
    expect(workspaceGameAvailability(switchOnly).available).toBe(false)
    expect(workspaceGameAvailability({ ...switchOnly, enabled: true }).available).toBe(true)
  })

  it('lets an explicitly closed room game be enabled even though its effective status is currently false', () => {
    expect(workspaceGameAvailability({ platform_enabled: true, room_enabled: false, lobby_category: '平台分类', enabled: false }))
      .toMatchObject({ status: 'room_closed', available: false, canEnable: true })
  })
})
