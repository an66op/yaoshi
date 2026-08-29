import { describe, expect, it } from 'vitest'
import { DEFAULT_AGENT_MENU, DEFAULT_TENANT_MENU, normalizeAdminMenu, normalizeRoleMenu } from './adminMenu'

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

  it('exposes one scoped fly-order entry to every management role', () => {
    expect(normalizeAdminMenu([]).find(item => item.path === '/fly-orders')).toMatchObject({ label: '飞单管理', group: '内容与服务', visible: true })
    expect(DEFAULT_TENANT_MENU.find(item => item.path === '/fly-orders')).toMatchObject({ label: '飞单管理', visible: true })
    expect(DEFAULT_AGENT_MENU.find(item => item.path === '/fly-orders')).toMatchObject({ label: '飞单管理', visible: true })
  })

  it('restores the canonical platform member entry when legacy settings hid or moved it', () => {
    const platform = normalizeAdminMenu([{ path: '/members', label: '旧会员入口', group: '内容与服务', order: 999, visible: false }])
    expect(platform.find(item => item.path === '/members')).toMatchObject({
      label: '会员管理',
      group: '组织与账号',
      order: 145,
      visible: true,
    })
  })

  it.each(['tenant', 'agent'] as const)('restores the canonical %s member entry from legacy settings', role => {
    const menu = normalizeRoleMenu(role, [{ path: '/members', label: '旧入口', group: '其他', order: 999, visible: false }])
    const fallback = (role === 'tenant' ? DEFAULT_TENANT_MENU : DEFAULT_AGENT_MENU).find(item => item.path === '/members')
    expect(menu.find(item => item.path === '/members')).toMatchObject({
      label: fallback?.label,
      group: fallback?.group,
      order: fallback?.order,
      visible: true,
    })
  })
})
