import { describe, expect, it } from 'vitest'
import { DEFAULT_AGENT_MENU, DEFAULT_TENANT_MENU, normalizeAdminMenu } from './adminMenu'

describe('interface test menu boundary', () => {
  it('exposes interface testing by default only on the platform menu', () => {
    const platform = normalizeAdminMenu([])
    expect(platform.find(item => item.path === '/interface-test')).toMatchObject({ label: '接口测试', visible: true })
    expect(DEFAULT_TENANT_MENU.some(item => item.path === '/interface-test')).toBe(false)
    expect(DEFAULT_AGENT_MENU.some(item => item.path === '/interface-test')).toBe(false)
  })

  it('keeps the retired lottery-network entry hidden even if legacy settings enabled it', () => {
    const platform = normalizeAdminMenu([{ path: '/lottery-network', visible: true }])
    expect(platform.find(item => item.path === '/lottery-network')?.visible).toBe(false)
    expect(platform.find(item => item.path === '/interface-test')?.visible).toBe(true)
  })
})
