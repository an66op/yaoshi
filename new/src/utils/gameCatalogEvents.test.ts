import { describe, expect, it } from 'vitest'
import { shouldReloadGameCatalog } from './gameCatalogEvents'

describe('shouldReloadGameCatalog', () => {
  it('refreshes draw and platform-wide catalog events', () => {
    expect(shouldReloadGameCatalog({ type: 'draw_update', data: {} }, '88001')).toBe(true)
    expect(shouldReloadGameCatalog({ type: 'game_catalog_update', data: {} }, '88001')).toBe(true)
  })

  it('accepts only the current room catalog event', () => {
    expect(shouldReloadGameCatalog({ type: 'game_catalog_update', data: { room_code: '88001' } }, '88001')).toBe(true)
    expect(shouldReloadGameCatalog({ type: 'game_catalog_update', data: { room_code: '99001' } }, '88001')).toBe(false)
  })

  it('ignores unrelated websocket events', () => {
    expect(shouldReloadGameCatalog({ type: 'chat_message', data: { room_code: '88001' } }, '88001')).toBe(false)
  })
})
