import { describe, expect, it } from 'vitest'
import { DEFAULT_ADMIN_MENU, DEFAULT_AGENT_MENU, DEFAULT_TENANT_MENU, normalizeAdminMenu, normalizeRoleMenu, resetAdminMenu, resetRoleMenu } from './adminMenu'

describe('retired mixed-account management', () => {
  const legacy = [{ path: '/users', label: '用户管理', group: '组织与账号', order: 1, visible: true }]

  it('removes the entry rather than merely hiding it in platform defaults, saved menus and resets', () => {
    for (const menu of [DEFAULT_ADMIN_MENU, normalizeAdminMenu(legacy), resetAdminMenu()]) {
      expect(menu.some(item => item.path === '/users')).toBe(false)
      expect(menu.find(item => item.path === '/members')).toMatchObject({ label: '会员管理', visible: true })
      expect(menu.find(item => item.path === '/tenants')).toMatchObject({ label: '租户管理', visible: true })
      expect(menu.find(item => item.path === '/agents')).toMatchObject({ label: '代理管理', visible: true })
    }
  })

  it.each(['tenant', 'agent'] as const)('does not restore a legacy user-management entry in %s templates', role => {
    for (const menu of [normalizeRoleMenu(role, legacy), resetRoleMenu(role)]) {
      expect(menu.some(item => item.path === '/users')).toBe(false)
      expect(menu.find(item => item.path === '/members')).toMatchObject({ label: '会员管理', visible: true })
    }
  })
})

describe('interface test menu boundary', () => {
  it('adds plan management to the platform menu, including saved menus predating it', () => {
    const platform = normalizeAdminMenu([{ path: '/announcements', visible: true }])
    expect(platform.find(item => item.path === '/plans')).toMatchObject({ label: '计划管理', group: '内容与服务', visible: true })
  })

  it('adds the admin-only game documentation page to legacy platform menus', () => {
    const platform = normalizeAdminMenu([{ path: '/system', visible: true }])
    expect(platform.find(item => item.path === '/game-guide')).toMatchObject({
      label: '游戏说明',
      group: '系统管理',
      order: 196,
      visible: true,
    })
    for (const role of ['tenant', 'agent'] as const) {
      expect(normalizeRoleMenu(role, [{ path: '/game-guide', label: '伪造入口', visible: true }]).some(item => item.path === '/game-guide')).toBe(false)
    }
  })

  it.each(['tenant', 'agent'] as const)('does not grant %s plan automation through a forged menu entry', role => {
    const menu = normalizeRoleMenu(role, [{ path: '/plans', label: '计划管理', visible: true }])
    expect(menu.some(item => item.path === '/plans')).toBe(false)
    expect(menu.find(item => item.path === '/announcements')?.visible).toBe(true)
		if (role === 'agent') expect(menu.find(item => item.path === '/announcements')?.label).toBe('活动管理')
  })

  it('exposes interface testing by default only on the platform menu', () => {
    const platform = normalizeAdminMenu([])
    expect(platform.find(item => item.path === '/interface-test')).toMatchObject({ label: '接口测试', visible: true })
    expect(DEFAULT_TENANT_MENU.some(item => item.path === '/interface-test')).toBe(false)
    expect(DEFAULT_AGENT_MENU.some(item => item.path === '/interface-test')).toBe(false)
  })

  it('upgrades the old audit entry to the dedicated system log page', () => {
    expect(normalizeAdminMenu([]).find(item => item.path === '/logs')).toMatchObject({ label: '日志', group: '系统管理', visible: true })
    expect(normalizeAdminMenu([{ path: '/audit', label: '操作审计', order: 188, visible: false }]).find(item => item.path === '/logs')).toMatchObject({ label: '日志', order: 188, visible: true })
    expect(normalizeAdminMenu([]).some(item => item.path === '/audit')).toBe(false)
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
