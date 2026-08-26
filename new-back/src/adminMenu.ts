export type AdminMenuItemConfig = {
  path: string
  label: string
  group: string
  order: number
  visible: boolean
}

export const ADMIN_MENU_GROUPS = ['运营总览', '组织与账号', '财务中心', '彩票运营', '内容与服务', '系统管理'] as const

export const DEFAULT_ADMIN_MENU: AdminMenuItemConfig[] = [
  { path: '/', label: '运营首页', group: '运营总览', order: 10, visible: true },
  { path: '/tenants', label: '租户管理', group: '组织与账号', order: 20, visible: true },
  { path: '/agents', label: '代理管理', group: '组织与账号', order: 30, visible: true },
  { path: '/users', label: '用户管理', group: '组织与账号', order: 40, visible: true },
  { path: '/applications', label: '申请管理', group: '财务中心', order: 50, visible: true },
  { path: '/wallet', label: '钱包与支付', group: '财务中心', order: 60, visible: true },
  { path: '/reports', label: '经营报表', group: '财务中心', order: 70, visible: true },
  { path: '/board-report', label: '打盘报表', group: '财务中心', order: 80, visible: true },
  { path: '/entertainment', label: '游戏与彩种', group: '彩票运营', order: 90, visible: true },
  { path: '/results', label: '开奖结果查询', group: '彩票运营', order: 100, visible: true },
  { path: '/lottery-network', label: '开奖线路', group: '彩票运营', order: 110, visible: true },
  { path: '/limits', label: '赔率与限额', group: '彩票运营', order: 120, visible: true },
  { path: '/bets', label: '注单管理', group: '彩票运营', order: 130, visible: true },
  { path: '/monitor', label: '现场监控', group: '彩票运营', order: 140, visible: true },
  { path: '/members', label: '会员管理', group: '内容与服务', order: 145, visible: true },
  { path: '/lottery-chat', label: '彩票室', group: '内容与服务', order: 150, visible: true },
  { path: '/chat', label: '客服与群聊', group: '内容与服务', order: 155, visible: true },
  { path: '/announcements', label: '公告', group: '内容与服务', order: 158, visible: true },
  { path: '/activities', label: '活动管理', group: '内容与服务', order: 160, visible: true },
  { path: '/special-numbers', label: '房间靓号', group: '内容与服务', order: 170, visible: true },
  { path: '/menu-management', label: '菜单管理', group: '系统管理', order: 180, visible: true },
  { path: '/system', label: '系统设置', group: '系统管理', order: 190, visible: true },
]

export function normalizeAdminMenu(value: unknown): AdminMenuItemConfig[] {
  const saved = Array.isArray(value) ? value : []
  const byPath = new Map<string, Partial<AdminMenuItemConfig>>()
  for (const item of saved) {
    if (!item || typeof item !== 'object') continue
    const candidate = item as Partial<AdminMenuItemConfig>
    if (typeof candidate.path === 'string') byPath.set(candidate.path, candidate)
  }
  return DEFAULT_ADMIN_MENU.map((fallback) => {
    const candidate = byPath.get(fallback.path)
    const savedLabel = typeof candidate?.label === 'string' ? candidate.label.trim() : ''
    const legacyChatLabel = fallback.path === '/chat' && ['客服与聊天室', '在线客服与群聊'].includes(savedLabel)
    const legacyLotteryLabel = fallback.path === '/lottery-chat' && ['彩票大厅', '彩票聊天室'].includes(savedLabel)
    const legacyApplicationLabel = fallback.path === '/applications' && ['上下分审核', '入房审核'].includes(savedLabel)
    const label = savedLabel && !legacyChatLabel && !legacyLotteryLabel && !legacyApplicationLabel ? savedLabel.slice(0, 18) : fallback.label
    const group = typeof candidate?.group === 'string' && candidate.group.trim() ? candidate.group.trim().slice(0, 18) : fallback.group
    const order = typeof candidate?.order === 'number' && Number.isFinite(candidate.order) ? candidate.order : fallback.order
    const visible = fallback.path === '/menu-management' ? true : typeof candidate?.visible === 'boolean' ? candidate.visible : fallback.visible
    return { path: fallback.path, label, group, order, visible }
  }).sort((a, b) => a.order - b.order)
}

export function resetAdminMenu(): AdminMenuItemConfig[] {
  return DEFAULT_ADMIN_MENU.map(item => ({ ...item }))
}
