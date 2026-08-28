import { describe, expect, it } from 'vitest'
import { roomLogoPresets } from './roomLogoPresets'

describe('roomLogoPresets', () => {
  it('把会员端现有的王者 Logo 放在第一项', () => {
    expect(roomLogoPresets[0]).toEqual({
      id: 'wangzhe-classic',
      label: '王者经典',
      path: '/images/wangzhe-header-logo.png',
    })
  })

  it('提供四个带有唯一标识、名称和本地路径的预设', () => {
    expect(roomLogoPresets).toHaveLength(4)
    expect(roomLogoPresets.map(item => item.path)).toEqual([
      '/images/wangzhe-header-logo.png',
      '/images/room-logos/crown-crystal.webp',
      '/images/room-logos/crown-shield.webp',
      '/images/room-logos/crown-laurel.webp',
    ])
    expect(new Set(roomLogoPresets.map(item => item.id)).size).toBe(roomLogoPresets.length)
    expect(new Set(roomLogoPresets.map(item => item.path)).size).toBe(roomLogoPresets.length)
    roomLogoPresets.forEach(item => {
      expect(item.label.trim()).not.toBe('')
      expect(item.path).toMatch(/^\/images\/[a-z0-9/-]+\.(?:png|webp)$/)
    })
  })
})
