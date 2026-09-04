export type AdminMenuItemConfig = {
  path: string
  label: string
  group: string
  order: number
  visible: boolean
}

export const ADMIN_MENU_GROUPS = ['运营总览', '组织与账号', '申请与财务', '彩票运营', '内容与服务', '系统管理'] as const

export const DEFAULT_ADMIN_MENU: AdminMenuItemConfig[] = [
  { path: '/', label: '运营首页', group: '运营总览', order: 10, visible: true },
  { path: '/tenants', label: '租户管理', group: '组织与账号', order: 20, visible: true },
  { path: '/agents', label: '代理管理', group: '组织与账号', order: 30, visible: true },
  { path: '/applications', label: '申请管理', group: '申请与财务', order: 50, visible: true },
  { path: '/wallet', label: '钱包与收款', group: '申请与财务', order: 60, visible: true },
  { path: '/reports', label: '报表中心', group: '申请与财务', order: 70, visible: true },
  { path: '/board-report', label: '打盘报表', group: '申请与财务', order: 80, visible: false },
  { path: '/entertainment', label: '游戏列表', group: '彩票运营', order: 90, visible: true },
  { path: '/results', label: '开奖管理', group: '彩票运营', order: 100, visible: true },
  { path: '/lottery-network', label: '开奖线路', group: '彩票运营', order: 110, visible: false },
  { path: '/limits', label: '赔率与回水', group: '彩票运营', order: 120, visible: true },
  { path: '/bets', label: '注单管理', group: '彩票运营', order: 130, visible: true },
  { path: '/monitor', label: '现场监控', group: '彩票运营', order: 140, visible: false },
	{ path: '/members', label: '会员管理', group: '组织与账号', order: 145, visible: true },
	{ path: '/fly-orders', label: '飞单管理', group: '内容与服务', order: 146, visible: true },
	{ path: '/robots', label: '机器人管理', group: '内容与服务', order: 147, visible: true },
  { path: '/lottery-chat', label: '彩票室', group: '内容与服务', order: 150, visible: true },
  { path: '/chat', label: '客服与群聊', group: '内容与服务', order: 155, visible: true },
  { path: '/announcements', label: '公告与活动', group: '内容与服务', order: 158, visible: true },
  { path: '/plans', label: '计划管理', group: '内容与服务', order: 159, visible: true },
  { path: '/activities', label: '活动管理', group: '内容与服务', order: 160, visible: false },
  { path: '/special-numbers', label: '房间靓号', group: '内容与服务', order: 170, visible: false },
  { path: '/menu-management', label: '菜单管理', group: '系统管理', order: 180, visible: true },
  { path: '/logs', label: '日志', group: '系统管理', order: 190, visible: true },
  { path: '/data-maintenance', label: '数据维护', group: '系统管理', order: 195, visible: true },
  { path: '/game-guide', label: '游戏说明', group: '系统管理', order: 196, visible: true },
  { path: '/interface-test', label: '接口测试', group: '系统管理', order: 197, visible: true },
  { path: '/system', label: '系统设置', group: '系统管理', order: 200, visible: true },
]

export const DEFAULT_TENANT_MENU: AdminMenuItemConfig[] = [
  { path: '/', label: '首页', group: '租户工作台', order: 10, visible: true },
  { path: '/agents', label: '代理管理', group: '组织管理', order: 20, visible: true },
  { path: '/members', label: '会员管理', group: '直属房间', order: 30, visible: true },
  { path: '/applications', label: '申请管理', group: '直属房间', order: 40, visible: true },
  { path: '/entertainment', label: '游戏列表', group: '直属房间', order: 45, visible: true },
  { path: '/bets', label: '注单管理', group: '直属房间', order: 50, visible: true },
	{ path: '/fly-orders', label: '飞单管理', group: '直属房间', order: 55, visible: true },
  { path: '/lottery-chat', label: '彩票室', group: '直属房间', order: 60, visible: true },
  { path: '/chat', label: '客服与群聊', group: '直属房间', order: 70, visible: true },
  { path: '/robots', label: '机器人管理', group: '直属房间', order: 80, visible: true },
  { path: '/announcements', label: '公告与活动', group: '直属房间', order: 90, visible: true },
  { path: '/limits', label: '赔率与回水', group: '直属房间', order: 100, visible: true },
  { path: '/wallet', label: '收款方式', group: '直属房间', order: 110, visible: true },
  { path: '/reports', label: '报表中心', group: '直属房间', order: 120, visible: true },
  { path: '/system', label: '房间设置', group: '直属房间', order: 130, visible: true },
]

export const DEFAULT_AGENT_MENU: AdminMenuItemConfig[] = [
  { path: '/', label: '首页', group: '房间工作台', order: 10, visible: true },
  { path: '/members', label: '会员管理', group: '房间业务', order: 20, visible: true },
  { path: '/applications', label: '申请管理', group: '房间业务', order: 30, visible: true },
  { path: '/entertainment', label: '游戏列表', group: '房间业务', order: 35, visible: true },
  { path: '/bets', label: '注单管理', group: '房间业务', order: 40, visible: true },
	{ path: '/fly-orders', label: '飞单管理', group: '房间业务', order: 45, visible: true },
  { path: '/lottery-chat', label: '彩票室', group: '房间运营', order: 50, visible: true },
  { path: '/chat', label: '客服与群聊', group: '房间运营', order: 60, visible: true },
  { path: '/robots', label: '机器人管理', group: '房间运营', order: 70, visible: true },
  { path: '/announcements', label: '公告与活动', group: '房间运营', order: 80, visible: true },
  { path: '/limits', label: '赔率与回水', group: '房间运营', order: 90, visible: true },
  { path: '/wallet', label: '收款方式', group: '房间运营', order: 100, visible: true },
  { path: '/reports', label: '报表中心', group: '数据与设置', order: 110, visible: true },
  { path: '/system', label: '房间设置', group: '数据与设置', order: 120, visible: true },
]

const normalizeMenu = (fallbacks: AdminMenuItemConfig[], value: unknown) => {
  const saved = Array.isArray(value) ? value : []
  const byPath = new Map<string, Partial<AdminMenuItemConfig>>()
  for (const item of saved) {
    if (!item || typeof item !== 'object') continue
    const candidate = item as Partial<AdminMenuItemConfig>
    if (typeof candidate.path === 'string') byPath.set(candidate.path, candidate)
  }
  return fallbacks.map((fallback) => {
    const candidate = byPath.get(fallback.path)
    const requiredMemberEntry = fallback.path === '/members'
    const legacyTenantAgentLabel = fallback.path === '/agents' && candidate?.label === '代理账号管理'
    const label = fallback.path === '/'
      ? '首页'
      : requiredMemberEntry
        ? fallback.label
      : legacyTenantAgentLabel
        ? fallback.label
      : typeof candidate?.label === 'string' && candidate.label.trim() ? candidate.label.trim().slice(0, 18) : fallback.label
    const group = requiredMemberEntry ? fallback.group : typeof candidate?.group === 'string' && candidate.group.trim() ? candidate.group.trim().slice(0, 18) : fallback.group
    const order = requiredMemberEntry ? fallback.order : typeof candidate?.order === 'number' && Number.isFinite(candidate.order) ? candidate.order : fallback.order
    const visible = requiredMemberEntry ? true : typeof candidate?.visible === 'boolean' ? candidate.visible : fallback.visible
    return { path: fallback.path, label, group, order, visible }
  }).sort((a, b) => a.order - b.order)
}

export const normalizeRoleMenu = (role: 'tenant' | 'agent', value: unknown) => normalizeMenu(role === 'tenant' ? DEFAULT_TENANT_MENU : DEFAULT_AGENT_MENU, value)

export function normalizeAdminMenu(value: unknown): AdminMenuItemConfig[] {
  const saved = Array.isArray(value) ? value : []
  const byPath = new Map<string, Partial<AdminMenuItemConfig>>()
  for (const item of saved) {
    if (!item || typeof item !== 'object') continue
    const candidate = item as Partial<AdminMenuItemConfig>
    if (typeof candidate.path === 'string') byPath.set(candidate.path, candidate)
  }
  // `/audit` was the old generic report entry. Keep its saved placement when
  // upgrading menus, while the required canonical destination is now `/logs`.
  if (!byPath.has('/logs') && byPath.has('/audit')) byPath.set('/logs', byPath.get('/audit')!)
  return DEFAULT_ADMIN_MENU.map((fallback) => {
    const candidate = byPath.get(fallback.path)
    const requiredMemberEntry = fallback.path === '/members'
    const savedLabel = typeof candidate?.label === 'string' ? candidate.label.trim() : ''
    const legacyChatLabel = fallback.path === '/chat' && ['客服与聊天室', '在线客服与群聊'].includes(savedLabel)
    const legacyLotteryLabel = fallback.path === '/lottery-chat' && ['彩票大厅', '彩票聊天室'].includes(savedLabel)
    const legacyApplicationLabel = fallback.path === '/applications' && ['上下分审核', '入房审核'].includes(savedLabel)
    const legacyResultsLabel = fallback.path === '/results' && savedLabel === '开奖结果查询'
    const legacyWalletLabel = fallback.path === '/wallet' && savedLabel === '钱包与支付'
    const legacyReportLabel = fallback.path === '/reports' && savedLabel === '经营报表'
    const legacyEntertainmentLabel = fallback.path === '/entertainment' && savedLabel === '游戏与彩种'
    const legacyAuditLabel = fallback.path === '/logs' && savedLabel === '操作审计'
    const retiredPath = ['/board-report', '/lottery-network', '/monitor', '/activities', '/special-numbers'].includes(fallback.path)
    const label = savedLabel && !requiredMemberEntry && !legacyChatLabel && !legacyLotteryLabel && !legacyApplicationLabel && !legacyResultsLabel && !legacyWalletLabel && !legacyReportLabel && !legacyEntertainmentLabel && !legacyAuditLabel && !retiredPath
      ? savedLabel.slice(0, 18)
      : fallback.label
    const group = requiredMemberEntry
      ? fallback.group
      : typeof candidate?.group === 'string' && candidate.group.trim() ? candidate.group.trim().slice(0, 18) : fallback.group
    const order = requiredMemberEntry
      ? fallback.order
      : typeof candidate?.order === 'number' && Number.isFinite(candidate.order) ? candidate.order : fallback.order
    const visible = fallback.path === '/menu-management' || fallback.path === '/logs' || requiredMemberEntry
      ? true
      : retiredPath
        ? false
        : typeof candidate?.visible === 'boolean' ? candidate.visible : fallback.visible
    return { path: fallback.path, label, group, order, visible }
  }).sort((a, b) => a.order - b.order)
}

export const resetRoleMenu = (role: 'tenant' | 'agent') => (role === 'tenant' ? DEFAULT_TENANT_MENU : DEFAULT_AGENT_MENU).map(item => ({ ...item }))

export function resetAdminMenu(): AdminMenuItemConfig[] {
  return DEFAULT_ADMIN_MENU.map(item => ({ ...item }))
}
