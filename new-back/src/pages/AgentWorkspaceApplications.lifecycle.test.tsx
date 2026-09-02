import { Alert, Button, Dialog, Table, TableBody, TableCell, TableHead, TableRow, Tabs, TextField } from '@mui/material'
import { isValidElement, type DependencyList, type ReactElement, type ReactNode, type SetStateAction } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AdminApplication } from '../api'
import { AgentWorkspacePage } from './AgentWorkspacePage'

// Exercise the legacy workspace application UI and handlers without a browser.
type Slot = { value?: unknown; deps?: DependencyList; effect?: () => void | (() => void); cleanup?: void | (() => void) }
const sameDeps = (left?: DependencyList, right?: DependencyList) => Boolean(left && right && left.length === right.length && left.every((value, index) => Object.is(value, right[index])))
class PageHarness {
  private slots: Slot[] = []
  private cursor = 0
  private effects: number[] = []
  render<T>(factory: () => T): T { this.cursor = 0; return factory() }
  useState<T>(initial?: T | (() => T)): [T, (value: SetStateAction<T>) => void] {
    const slot = this.slots[this.cursor++] ??= { value: typeof initial === 'function' ? (initial as () => T)() : initial }
    return [slot.value as T, value => { slot.value = typeof value === 'function' ? (value as (previous: T) => T)(slot.value as T) : value }]
  }
  useRef<T>(initial: T) {
    const slot = this.slots[this.cursor++] ??= { value: { current: initial } }
    return slot.value as { current: T }
  }
  useMemo<T>(factory: () => T, deps: DependencyList): T {
    const index = this.cursor++
    const slot = this.slots[index]
    if (!slot || !sameDeps(slot.deps, deps)) this.slots[index] = { value: factory(), deps }
    return this.slots[index].value as T
  }
  useEffect(effect: () => void | (() => void), deps?: DependencyList) {
    const index = this.cursor++
    const previous = this.slots[index]
    if (!previous || !sameDeps(previous.deps, deps)) {
      previous?.cleanup?.()
      this.slots[index] = { effect, deps }
      this.effects.push(index)
    }
  }
  flushEffects() { for (const index of this.effects.splice(0)) this.slots[index].cleanup = this.slots[index].effect?.() }
  unmount() { for (const slot of this.slots) slot.cleanup?.() }
}

const runtime = vi.hoisted(() => ({
  hooks: null as PageHarness | null,
  agentApplications: vi.fn(), tenantApplications: vi.fn(), agentReview: vi.fn(), tenantReview: vi.fn(),
  dashboard: vi.fn(), settings: vi.fn(), showMessage: vi.fn(),
}))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial?: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useMemo: <T,>(factory: () => T, deps: DependencyList) => runtime.hooks!.useMemo(factory, deps),
  useCallback: <T,>(callback: T, deps: DependencyList) => runtime.hooks!.useMemo(() => callback, deps),
  useEffect: (effect: () => void | (() => void), deps?: DependencyList) => runtime.hooks!.useEffect(effect, deps),
}))
vi.mock('../api', () => ({
  agentApi: { dashboard: runtime.dashboard, settings: runtime.settings, applications: runtime.agentApplications, reviewApplication: runtime.agentReview },
  tenantApi: { roomDashboard: runtime.dashboard, settings: runtime.settings, applications: runtime.tenantApplications, reviewApplication: runtime.tenantReview },
}))
vi.mock('../components/feedback', () => ({ useFeedback: () => ({ showMessage: runtime.showMessage }) }))
vi.mock('../hooks/useManagementWebSocket', () => ({ MANAGEMENT_WS_EVENT: 'test-management-event', useManagementWebSocketConnected: () => false }))
vi.mock('../utils/requestId', () => ({ createRequestId: () => 'test-request-id' }))
vi.mock('../utils/chatNotifications', () => ({
  CHAT_OPEN_CONVERSATION_EVENT: 'test-chat-open', chatPageForTarget: vi.fn(), consumePendingChatConversation: vi.fn(),
  reportChatUnreadChanged: vi.fn(), setActiveChatConversation: vi.fn(),
}))
vi.mock('../components/OperatingReportPanel', () => ({ OperatingReportPanel: () => null }))
vi.mock('../components/OddsEditors', () => ({ GameOddsNavigation: () => null, OddsOverrideGrid: () => null }))
vi.mock('../components/RedPacketForm', () => ({ AdminRedPacketCard: () => null, RedPacketForm: () => null }))

type Category = 'wallet' | 'join' | 'entertainment'
type ElementProps = {
  children?: ReactNode; action?: ReactNode; actions?: ReactNode; open?: boolean
  label?: string; helperText?: string; value?: string | number; severity?: string
  onClick?: () => void
  onChange?: ((event: { target: { value: string } }) => void) & ((event: null, next: Category | 'approved' | 'rejected') => void)
}
type Element = ReactElement<ElementProps>
function elements(node: ReactNode): Element[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  if (!isValidElement<ElementProps>(node)) return []
  return [node, ...elements(node.props.children), ...elements(node.props.action), ...elements(node.props.actions)]
}
function text(node: ReactNode): string {
  if (Array.isArray(node)) return node.map(text).join('')
  if (isValidElement<ElementProps>(node)) return text(node.props.children)
  return typeof node === 'string' || typeof node === 'number' ? String(node) : ''
}
const ofType = (node: ReactNode, type: unknown) => elements(node).filter(element => element.type === type)
const button = (node: ReactNode, label: string) => ofType(node, Button).find(element => text(element) === label)!
const field = (node: ReactNode, label: string) => ofType(node, TextField).find(element => element.props.label === label)!
const activeReview = (node: ReactNode) => ofType(node, Dialog).find(element => element.props.open)!
const list = (node: ReactNode) => ofType(node, Table)[0]
const row = (node: ReactNode) => ofType(ofType(list(node), TableBody)[0], TableRow)[0]
const categoryTabs = (node: ReactNode) => ofType(node, Tabs).find(element => ['wallet', 'join', 'entertainment'].includes(String(element.props.value)))!
const detailCopy = (node: ReactNode) => text(node) + elements(node).map(element => `${element.props.label ?? ''}${element.props.helperText ?? ''}`).join('')
const noMoney = (node: ReactNode) => {
  expect(detailCopy(node)).not.toMatch(/金额|到账|出款|支付|收款|余额|资金流水|¥|923\.45|812\.34|701\.23|银行账户/)
}

const application: AdminApplication = {
  id: 123, workspace_id: 45, user_id: 67, username: 'waiting-member', account_type: 'member', request_type: 'join',
  target_room_code: '556677', payment_type: 'bank', payment_account_label: '银行账户', requested_amount: 923.45,
  received_amount: 812.34, user_balance: 701.23, balance_before: 701.23, balance_after: 1513.57,
  remark: '希望加入当前房间', status: 'pending', operator: '', review_remark: '', odds_multiplier: 1.1,
  reviewed_at: null, created_at: '2026-08-31T08:00:00Z', updated_at: '2026-08-31T08:00:00Z',
}

describe.each([
  { name: 'agent room', tenantDirect: false, applications: runtime.agentApplications, review: runtime.agentReview },
  { name: 'tenant direct room', tenantDirect: true, applications: runtime.tenantApplications, review: runtime.tenantReview },
])('$name application displays', scenario => {
  let section: 'room-reviews' | 'applications'
  const render = () => {
    const root = runtime.hooks!.render(() => AgentWorkspacePage({ section, tenantDirect: scenario.tenantDirect }))
    runtime.hooks!.flushEffects()
    return root
  }
  const ready = async () => { render(); await vi.runOnlyPendingTimersAsync(); return render() }
  const settle = async () => { for (let index = 0; index < 16; index++) await Promise.resolve(); return render() }
  const openReview = (root: ReactNode) => { button(row(root), '审核').props.onClick!(); return activeReview(render()) }

  beforeEach(() => {
    section = 'room-reviews'
    runtime.hooks = new PageHarness()
    for (const value of Object.values(runtime)) if (vi.isMockFunction(value)) value.mockReset()
    runtime.dashboard.mockResolvedValue({ active_member_count: 1, member_count: 1, member_balance: 0, today_stake: 0, today_payout: 0, today_net: 0, pending_applications: 1, pending_bets: 0 })
    runtime.settings.mockResolvedValue({ room_name: '当前房间', room_logo: '', chat_nickname: '', room_notice: '', announcements: [] })
    scenario.applications.mockResolvedValue({ items: [application], total: 1 })
    scenario.review.mockResolvedValue(application)
    vi.useFakeTimers()
    vi.stubGlobal('window', { setTimeout, clearTimeout })
  })
  afterEach(() => { runtime.hooks!.unmount(); vi.clearAllTimers(); vi.useRealTimers(); vi.unstubAllGlobals() })

  it('omits all money and payment information from the join list and approval details', async () => {
    const root = await ready()
    expect(scenario.applications).toHaveBeenCalledWith({ query: '', type: 'join', page: 1, pageSize: 20 })
    expect(ofType(ofType(list(root), TableHead)[0], TableCell).map(text)).toEqual(['用户', '目标房间', '状态', '操作'])
    expect(ofType(row(root), TableCell)).toHaveLength(4)
    expect(text(row(root))).toContain('556677')
    noMoney(list(root))
    const review = openReview(root)
    expect(detailCopy(review)).toContain('审核入房申请')
    expect(detailCopy(review)).toContain('目标房间 556677')
    expect(field(review, '自定义倍率').props.value).toBe('1.1')
    expect(field(review, '审核备注')).toBeDefined()
    noMoney(review)
    button(review, '确认通过').props.onClick!()
    await settle()
    expect(scenario.review).toHaveBeenCalledExactlyOnceWith(application.id, { decision: 'approved', received_amount: 0, odds_multiplier: 1.1, remark: '' })
    expect(runtime.showMessage).toHaveBeenCalledWith('入房申请已通过')
  })

  it('keeps join rejection focused on room membership, without balance messaging', async () => {
    const review = openReview(await ready())
    ofType(review, Tabs)[0].props.onChange!(null, 'rejected')
    let rejected = activeReview(render())
    noMoney(rejected)
    expect(ofType(rejected, Alert).map(text).join('')).toBe('拒绝不会改变会员的房间归属。')
    expect(field(rejected, '拒绝原因')).toBeDefined()
    expect(field(rejected, '自定义倍率')).toBeUndefined()
    field(rejected, '拒绝原因').props.onChange!({ target: { value: '请补充入房资料' } })
    rejected = activeReview(render())
    button(rejected, '确认拒绝').props.onClick!()
    await settle()
    expect(scenario.review).toHaveBeenCalledExactlyOnceWith(application.id, { decision: 'rejected', received_amount: 0, odds_multiplier: undefined, remark: '请补充入房资料' })
    expect(runtime.showMessage).toHaveBeenCalledWith('入房申请已拒绝')
  })

  it('also hides money when switching from the applications route to the join tab', async () => {
    section = 'applications'
    scenario.applications.mockResolvedValueOnce({ items: [{ ...application, request_type: 'credit' }], total: 1 })
    const root = await ready()
    expect(categoryTabs(root).props.value).toBe('wallet')
    expect(text(list(root))).toContain('金额')
    categoryTabs(root).props.onChange!(null, 'join')
    const joined = await ready()
    expect(scenario.applications).toHaveBeenLastCalledWith({ query: '', type: 'join', page: 1, pageSize: 20 })
    noMoney(list(joined))
    noMoney(openReview(joined))
  })

  it.each([
    { category: 'wallet', type: 'credit', label: '实际到账金额', balance: '1,624.68' },
    { category: 'wallet', type: 'debit', label: '实际出款金额', balance: '-222.22' },
    { category: 'entertainment', type: 'credit', label: '实际到账金额', balance: '1,624.68' },
  ] as const)('preserves $category $type amount columns and balance review controls', async ({ category, type, label, balance }) => {
    section = 'applications'
    scenario.applications.mockResolvedValue({ items: [{ ...application, request_type: type, game_id: category === 'entertainment' ? 'platform-one' : '' }], total: 1 })
    let root = await ready()
    if (category === 'entertainment') { categoryTabs(root).props.onChange!(null, category); root = await ready() }
    expect(ofType(ofType(list(root), TableHead)[0], TableCell).map(text)).toEqual(['用户', category === 'entertainment' ? '娱乐平台' : '类型', '金额', '状态', '操作'])
    expect(ofType(row(root), TableCell)).toHaveLength(5)
    expect(text(row(root))).toContain('¥ 923.45')
    const review = openReview(root)
    expect(text(review)).toContain('申请金额 ¥ 923.45')
    expect(field(review, label).props.value).toBe('923.45')
    expect(text(review)).toContain('变动前余额¥ 701.23')
    expect(text(review)).toContain(`预计变动后余额¥ ${balance}`)
    expect(text(review)).toContain('通过后立即写入余额与资金流水')
    expect(field(review, '自定义倍率')).toBeUndefined()
  })
})
