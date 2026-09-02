import { Avatar, Button, Dialog, Switch, Table, TableBody, TableCell, TableHead, TableRow, TextField } from '@mui/material'
import { isValidElement, type DependencyList, type ReactElement, type ReactNode, type SetStateAction } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AdminChatConversation, AdminChatMessage, UserTradingConfig, WorkspaceMember } from '../api'
import { UserPresenceChip } from '../components/UserPresenceChip'
import { AgentWorkspacePage } from './AgentWorkspacePage'

// Run the page's hooks and handlers directly, without a browser or live services.
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
    const previous = this.slots[index]
    if (!previous || !sameDeps(previous.deps, deps)) this.slots[index] = { value: factory(), deps }
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

const runtime = vi.hoisted(() => {
  const api = () => ({
    dashboard: vi.fn(), roomDashboard: vi.fn(), settings: vi.fn(), games: vi.fn(), users: vi.fn(),
    setUserStatus: vi.fn(), adjustUserBalance: vi.fn(), userTrading: vi.fn(), updateUserTrading: vi.fn(), bets: vi.fn(),
    chatConversations: vi.fn(), chatMessages: vi.fn(), markChatRead: vi.fn(),
  })
  return { hooks: null as PageHarness | null, agent: api(), tenant: api(), showMessage: vi.fn() }
})
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial?: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useMemo: <T,>(factory: () => T, deps: DependencyList) => runtime.hooks!.useMemo(factory, deps),
  useCallback: <T,>(callback: T, deps: DependencyList) => runtime.hooks!.useMemo(() => callback, deps),
  useEffect: (effect: () => void | (() => void), deps?: DependencyList) => runtime.hooks!.useEffect(effect, deps),
}))
vi.mock('../api', () => ({ agentApi: runtime.agent, tenantApi: runtime.tenant }))
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

type Props = {
  children?: ReactNode; action?: ReactNode; actions?: ReactNode; control?: ReactNode
  open?: boolean; disabled?: boolean; checked?: boolean; online?: boolean; colSpan?: number
  label?: string; placeholder?: string; value?: unknown; title?: string
  onClick?: () => void | Promise<void>
  onChange?: (event: { target: { value: string } }, checked?: boolean) => void | Promise<void>
}
type Element = ReactElement<Props>
function elements(node: ReactNode): Element[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  if (!isValidElement<Props>(node)) return []
  return [node, ...elements(node.props.children), ...elements(node.props.action), ...elements(node.props.actions), ...elements(node.props.control)]
}
function text(node: ReactNode): string {
  if (Array.isArray(node)) return node.map(text).join('')
  if (isValidElement<Props>(node)) return text(node.props.children) + (node.props.label ?? '')
  return typeof node === 'string' || typeof node === 'number' ? String(node) : ''
}
const ofType = (node: ReactNode, type: unknown) => elements(node).filter(element => element.type === type)
const button = (node: ReactNode, label: string) => ofType(node, Button).find(element => text(element) === label)!
const field = (node: ReactNode, label: string) => ofType(node, TextField).find(element => element.props.label === label)!
const search = (node: ReactNode) => ofType(node, TextField).find(element => element.props.placeholder?.includes('搜索'))!
const dialog = (node: ReactNode, title: string) => ofType(node, Dialog).find(element => element.props.open && text(element).includes(title))!
const table = (node: ReactNode) => ofType(node, Table)[0]
const rows = (node: ReactNode) => ofType(ofType(table(node), TableBody)[0], TableRow)
const headers = (node: ReactNode) => ofType(ofType(table(node), TableHead)[0], TableCell).map(text)
const cells = (node: ReactNode) => ofType(node, TableCell)
const rowFor = (node: ReactNode, username: string) => rows(node).find(row => text(row).includes(`@${username}`))!
const cellFor = (root: ReactNode, row: ReactNode, header: string) => cells(row)[headers(root).indexOf(header)]
const roster = (items: WorkspaceMember[]) => ({ items, total: items.length })
const deferred = <T,>() => {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(finish => { resolve = finish })
  return { promise, resolve }
}

const current: WorkspaceMember = {
  id: 41, public_id: 100041, username: 'current-member', nickname: '当前会员', avatar: '/avatars/current.png',
  public_title: '公开称号', badge: '公开徽章', role: 'member', in_current_room: true, can_manage: true,
  balance: 913.27, status: 1, online: true, phone: '13800138000', remark: '仅本房间可见备注',
  risk_level: 'watch', login_count: 37, last_login_at: '2026-08-31T08:10:00Z', created_at: '2026-08-01T08:00:00Z',
}
const moved = (member: WorkspaceMember = current): WorkspaceMember => ({
  id: member.id, public_id: member.public_id, username: member.username, nickname: member.nickname,
  avatar: member.avatar, public_title: member.public_title, badge: member.badge, role: 'member',
  in_current_room: false, can_manage: false, balance: null, status: null, online: null,
})
const historical: WorkspaceMember = moved({ ...current, id: 42, public_id: 100042, username: 'historical-member', nickname: '历史会员' })
const trading: UserTradingConfig = {
  user_id: current.id, workspace_id: 8, username: current.username, odds_multiplier: 1,
  fly: { mode: 'inherit', rate: 0 }, rebate: { mode: 'inherit', rate: 0, effective: 0, source: 'room' },
  external_follow: { status: 'not_connected', capability: 'configuration_only', target_platform: '', target_account: '', endpoint_label: '', single_limit: 0, daily_limit: 0, remark: '' },
  game_id: 'room-game', game_name: '房间彩种', room_fly_rate: 0, room_rebate_rate: 0, odds: [],
}
const conversation: AdminChatConversation = {
  scope: 'workspace', room_scope: 'room-8', game_id: 'room-game', room_type: 'service', title: '客服会话', subtitle: '',
  latest_text: '历史聊天消息', latest_is_staff: false, message_count: 1, unread_count: 0, group_chat_enabled: true, enabled: true,
}
const message: AdminChatMessage = {
  id: 77, user_id: current.id, username: current.username, nickname: current.nickname,
  scope: conversation.scope, room_scope: conversation.room_scope, game_id: conversation.game_id, room_type: conversation.room_type,
  content: '历史聊天消息', message_type: 'text', is_staff: false, created_at: '2026-08-31T08:00:00Z',
}
const expectNoWrites = () => {
  for (const api of [runtime.agent, runtime.tenant]) {
    expect(api.setUserStatus).not.toHaveBeenCalled()
    expect(api.adjustUserBalance).not.toHaveBeenCalled()
    expect(api.updateUserTrading).not.toHaveBeenCalled()
  }
}
const expectNoManagement = (row: ReactNode) => {
  expect(ofType(row, Switch)).toHaveLength(0)
  expect(button(row, '赔率设置')).toBeUndefined()
  expect(button(row, '调整余额')).toBeUndefined()
}
const expectNoPrivateProfile = (profile: ReactNode) => {
  expect(text(profile)).not.toMatch(/可用积分|总注单|待结算|登录次数|风险等级|最近登录|注册时间|联系电话|913\.27|13800138000|仅本房间可见备注/)
}

describe.each([
  { name: 'agent room', tenantDirect: false, api: runtime.agent, other: runtime.tenant },
  { name: 'tenant direct room', tenantDirect: true, api: runtime.tenant, other: runtime.agent },
])('$name member roster lifecycle', scenario => {
  let section: 'users' | 'chat'
  const render = () => {
    const root = runtime.hooks!.render(() => AgentWorkspacePage({ section, tenantDirect: scenario.tenantDirect }))
    runtime.hooks!.flushEffects()
    return root
  }
  const ready = async () => {
    let root = render()
    // Chat selects a conversation before scheduling its messages in a second effect.
    for (let index = 0; index < 4; index++) { await vi.advanceTimersByTimeAsync(0); root = render() }
    return root
  }
  const drain = async () => { for (let index = 0; index < 24; index++) await Promise.resolve() }
  const settle = async () => { await drain(); return render() }
  const openBalance = async (root: ReactNode) => {
    button(rowFor(root, current.username), '调整余额').props.onClick!()
    let editor = dialog(await settle(), '调整余额')
    expect(editor).toBeDefined()
    field(editor, '调整金额').props.onChange!({ target: { value: '25' } })
    field(editor, '原因').props.onChange!({ target: { value: '测试调整' } })
    editor = dialog(render(), '调整余额')
    return editor
  }
  const openTrading = async (root: ReactNode) => {
    button(rowFor(root, current.username), '赔率设置').props.onClick!()
    let editor = dialog(await settle(), '会员赔率 ·')
    expect(editor).toBeDefined()
    field(editor, '自定义倍率').props.onChange!({ target: { value: '1.1' } })
    editor = dialog(render(), '会员赔率 ·')
    return editor
  }
  const prepareAction = async (kind: 'status' | 'balance' | 'trading') => {
    const root = await ready()
    if (kind === 'status') {
      const toggle = ofType(rowFor(root, current.username), Switch)[0]
      return { invoke: () => toggle.props.onChange!({ target: { value: '' } }, false), mutation: scenario.api.setUserStatus }
    }
    const editor = kind === 'balance' ? await openBalance(root) : await openTrading(root)
    const action = button(editor, kind === 'balance' ? '确认' : '保存会员赔率')
    return { invoke: () => action.props.onClick!(), mutation: kind === 'balance' ? scenario.api.adjustUserBalance : scenario.api.updateUserTrading }
  }
  const openProfile = (root: ReactNode) => {
    const avatar = ofType(root, Avatar).find(element => element.props.title === '查看会员资料')!
    expect(avatar).toBeDefined()
    avatar.props.onClick!()
  }

  beforeEach(() => {
    section = 'users'
    runtime.hooks = new PageHarness()
    runtime.showMessage.mockReset()
    for (const api of [runtime.agent, runtime.tenant]) {
      for (const mock of Object.values(api)) mock.mockReset()
      const dashboard = { active_member_count: 1, member_count: 2, member_balance: 0, today_stake: 0, today_payout: 0, today_net: 0, pending_applications: 0, pending_bets: 0, room_code: '88008' }
      api.dashboard.mockResolvedValue(dashboard)
      api.roomDashboard.mockResolvedValue(dashboard)
      api.settings.mockResolvedValue({ room_name: '当前房间', room_logo: '', chat_nickname: '', room_notice: '', announcements: [] })
      api.games.mockResolvedValue([])
      api.users.mockResolvedValue(roster([current]))
      api.setUserStatus.mockResolvedValue(undefined)
      api.adjustUserBalance.mockResolvedValue(undefined)
      api.userTrading.mockResolvedValue(trading)
      api.updateUserTrading.mockResolvedValue({ ...trading, odds_multiplier: 1.1 })
      api.bets.mockImplementation((params?: { status?: string }) => Promise.resolve(params?.status === 'pending'
        ? { items: [], total: 2 }
        : { items: [{ amount: 37.25, payout: 51.75 }], total: 6 }))
      api.chatConversations.mockResolvedValue({ items: [conversation], total: 1 })
      api.chatMessages.mockResolvedValue({ items: [message], has_more: false })
      api.markChatRead.mockResolvedValue({})
    }
    vi.useFakeTimers()
    vi.stubGlobal('window', { setTimeout, clearTimeout, setInterval, clearInterval, addEventListener: vi.fn(), removeEventListener: vi.fn() })
    vi.stubGlobal('document', { visibilityState: 'visible', hasFocus: () => true, addEventListener: vi.fn(), removeEventListener: vi.fn() })
  })
  afterEach(() => { runtime.hooks!.unmount(); vi.clearAllTimers(); vi.useRealTimers(); vi.unstubAllGlobals() })

  it('keeps current and historical members in one aligned roster with explicit room status', async () => {
    scenario.api.users.mockResolvedValue(roster([current, historical]))
    const root = await ready()
    expect(scenario.api.users).toHaveBeenCalledWith({ query: '', page: 1, pageSize: 20 })
    expect(scenario.other.users).not.toHaveBeenCalled()
    expect(headers(root)).toContain('房间状态')
    expect(rows(root)).toHaveLength(2)
    for (const row of rows(root)) expect(cells(row)).toHaveLength(headers(root).length)
    const active = rowFor(root, current.username)
    expect(text(cellFor(root, active, '房间状态'))).toBe('在本房间')
    expect(text(cellFor(root, active, '余额'))).toContain('913.27')
    expect(ofType(active, UserPresenceChip)[0].props.online).toBe(true)
    expect(ofType(active, Switch)[0].props.checked).toBe(true)
    expect(button(active, '赔率设置')).toBeDefined()
    expect(button(active, '调整余额')).toBeDefined()
    const old = rowFor(root, historical.username)
    expect(text(old)).toContain(historical.nickname)
    expect(text(old)).toContain(String(historical.public_id))
    expect(text(cellFor(root, old, '房间状态'))).toBe('已切换')
    for (const label of ['余额', '在线状态', '账号状态']) expect(text(cellFor(root, old, label))).toBe('—')
    expect(ofType(old, UserPresenceChip)).toHaveLength(0)
    expectNoManagement(old)
  })

  it.each([
    { label: 'can_manage is false', in_current_room: true, can_manage: false },
    { label: 'in_current_room is false', in_current_room: false, can_manage: true },
    { label: 'can_manage is missing', in_current_room: true, can_manage: undefined },
    { label: 'in_current_room is missing', in_current_room: undefined, can_manage: true },
  ])('fails closed when $label, even if legacy private fields are populated', async flags => {
    scenario.api.users.mockResolvedValue(roster([{ ...current, ...flags } as WorkspaceMember]))
    expectNoManagement(rowFor(await ready(), current.username))
    expectNoWrites()
  })

  it('distinguishes a genuinely empty room history from an empty search', async () => {
    scenario.api.users.mockResolvedValue(roster([]))
    let root = await ready()
    expect(text(table(root))).toContain('还没有会员加入过本房间')
    expect(cells(rows(root)[0])[0].props.colSpan).toBe(headers(root).length)
    search(root).props.onChange!({ target: { value: 'no-such-member' } })
    root = await ready()
    expect(scenario.api.users).toHaveBeenLastCalledWith({ query: 'no-such-member', page: 1, pageSize: 20 })
    expect(text(table(root))).toContain('未找到匹配的房间会员')
    expect(text(table(root))).not.toContain('还没有会员加入过本房间')
    expect(cells(rows(root)[0])[0].props.colSpan).toBe(headers(root).length)
  })

  it('preserves current-member status, balance and odds operations after revalidation', async () => {
    let root = await ready()
    ofType(rowFor(root, current.username), Switch)[0].props.onChange!({ target: { value: '' } }, false)
    root = await settle()
    expect(scenario.api.setUserStatus).toHaveBeenCalledExactlyOnceWith(current.id, 0)
    const balance = await openBalance(root)
    button(balance, '确认').props.onClick!()
    root = await settle()
    expect(scenario.api.adjustUserBalance).toHaveBeenCalledExactlyOnceWith(current.id, 25, '测试调整')
    const odds = await openTrading(root)
    button(odds, '保存会员赔率').props.onClick!()
    await settle()
    expect(scenario.api.updateUserTrading).toHaveBeenCalledExactlyOnceWith(current.id, expect.objectContaining({ odds_multiplier: 1.1, game_id: trading.game_id }))
    expect(scenario.api.users).toHaveBeenCalledWith({ userId: current.id, page: 1, pageSize: 1 })
    expect(scenario.other.users).not.toHaveBeenCalled()
  })

  it.each(['status', 'balance', 'trading'] as const)('deduplicates same-frame %s submissions while membership validation is pending', async kind => {
    const action = await prepareAction(kind)
    const membership = deferred<ReturnType<typeof roster>>()
    const requestsBefore = scenario.api.users.mock.calls.length
    scenario.api.users.mockReturnValueOnce(membership.promise)
    action.invoke()
    action.invoke()
    await drain()
    expect(scenario.api.users).toHaveBeenCalledTimes(requestsBefore + 1)
    expect(action.mutation).not.toHaveBeenCalled()
    membership.resolve(roster([current]))
    await settle()
    expect(action.mutation).toHaveBeenCalledTimes(1)
  })

  it.each(['status', 'balance', 'trading'] as const)('does not submit a deferred %s mutation after unmount', async kind => {
    const action = await prepareAction(kind)
    const membership = deferred<ReturnType<typeof roster>>()
    scenario.api.users.mockReturnValueOnce(membership.promise)
    action.invoke()
    runtime.hooks!.unmount()
    membership.resolve(roster([current]))
    await drain()
    expectNoWrites()
    expect(runtime.showMessage).not.toHaveBeenCalled()
  })

  describe.each([
    { name: 'has switched rooms', items: [moved()] },
    { name: 'is no longer in the response', items: [] },
  ])('when the previously current member $name', changed => {
    it('rejects a stale status callback before changing the account', async () => {
      const toggle = ofType(rowFor(await ready(), current.username), Switch)[0]
      scenario.api.users.mockResolvedValue(roster(changed.items))
      toggle.props.onChange!({ target: { value: '' } }, false)
      await settle()
      expect(scenario.api.users).toHaveBeenLastCalledWith({ userId: current.id, page: 1, pageSize: 1 })
      expectNoWrites()
    })

    it('closes a stale balance dialog without writing a financial mutation', async () => {
      const editor = await openBalance(await ready())
      scenario.api.users.mockResolvedValue(roster(changed.items))
      button(editor, '确认').props.onClick!()
      const root = await settle()
      expect(scenario.api.users).toHaveBeenLastCalledWith({ userId: current.id, page: 1, pageSize: 1 })
      expectNoWrites()
      expect(dialog(root, '调整余额')).toBeUndefined()
    })

    it('rejects a stale odds action before requesting private trading configuration', async () => {
      const action = button(rowFor(await ready(), current.username), '赔率设置')
      scenario.api.users.mockResolvedValue(roster(changed.items))
      action.props.onClick!()
      const root = await settle()
      expect(scenario.api.users).toHaveBeenLastCalledWith({ userId: current.id, page: 1, pageSize: 1 })
      expect(scenario.api.userTrading).not.toHaveBeenCalled()
      expect(scenario.other.userTrading).not.toHaveBeenCalled()
      expect(dialog(root, '会员赔率 ·')).toBeUndefined()
      expectNoWrites()
    })

    it('closes stale odds settings without saving trading configuration', async () => {
      const editor = await openTrading(await ready())
      scenario.api.users.mockResolvedValue(roster(changed.items))
      button(editor, '保存会员赔率').props.onClick!()
      const root = await settle()
      expect(scenario.api.users).toHaveBeenLastCalledWith({ userId: current.id, page: 1, pageSize: 1 })
      expectNoWrites()
      expect(dialog(root, '会员赔率 ·')).toBeUndefined()
    })
  })

  it.each(['balance', 'trading'] as const)('reconciles an open %s dialog when a refreshed roster reports the member moved', async kind => {
    let root = await ready()
    if (kind === 'balance') await openBalance(root)
    else await openTrading(root)
    root = render()
    scenario.api.users.mockResolvedValue(roster([moved()]))
    button(root, '查询').props.onClick!()
    root = await settle()
    expect(dialog(root, kind === 'balance' ? '调整余额' : '会员赔率 ·')).toBeUndefined()
    expect(text(cellFor(root, rowFor(root, current.username), '房间状态'))).toBe('已切换')
    expectNoWrites()
  })

  it('opens a historical chat author as a public-only profile without requesting bets', async () => {
    section = 'chat'
    // Deliberately preserve old private values: the room flags must govern rendering.
    scenario.api.users.mockResolvedValue(roster([{ ...current, in_current_room: false, can_manage: false }]))
    openProfile(await ready())
    const profile = dialog(await settle(), '会员资料')
    expect(profile).toBeDefined()
    expect(text(profile)).toContain(current.nickname)
    expect(text(profile)).toContain(String(current.public_id))
    expect(text(profile)).toContain('已切换')
    expectNoPrivateProfile(profile)
    expect(scenario.api.bets).not.toHaveBeenCalled()
    expect(scenario.other.bets).not.toHaveBeenCalled()
  })

  it('waits for current-room membership before fetching and showing private chat activity', async () => {
    section = 'chat'
    const membership = deferred<ReturnType<typeof roster>>()
    scenario.api.users.mockReturnValueOnce(membership.promise)
    openProfile(await ready())
    let root = await settle()
    expect(scenario.api.bets).not.toHaveBeenCalled()
    expectNoPrivateProfile(dialog(root, '会员资料'))
    membership.resolve(roster([current]))
    root = await settle()
    expect(scenario.api.bets).toHaveBeenCalledWith(expect.objectContaining({ userId: current.id, page: 1, pageSize: 100 }))
    expect(scenario.api.bets).toHaveBeenCalledWith(expect.objectContaining({ userId: current.id, status: 'pending' }))
    expect(scenario.api.users.mock.calls.length).toBeGreaterThanOrEqual(2)
    const profile = dialog(root, '会员资料')
    expect(text(profile)).toContain('可用积分913.27')
    expect(text(profile)).toContain('总注单6 笔')
    expect(text(profile)).toContain('待结算2 笔')
    expect(text(profile)).toContain('37.25')
    expect(text(profile)).toContain(current.phone)
  })

  it('looks up a chat author by exact member ID even when both message names are empty', async () => {
    section = 'chat'
    scenario.api.chatMessages.mockResolvedValue({ items: [{ ...message, username: '', nickname: '' }], has_more: false })
    openProfile(await ready())
    const profile = dialog(await settle(), '会员资料')
    expect(text(profile)).toContain(current.nickname)
    expect(scenario.api.users.mock.calls.length).toBeGreaterThanOrEqual(2)
    for (const [params] of scenario.api.users.mock.calls) expect(params).toEqual({ userId: message.user_id, page: 1, pageSize: 1 })
    expect(scenario.api.bets).toHaveBeenCalledWith(expect.objectContaining({ userId: message.user_id }))
  })

  it('discards private chat data when the member leaves while bets are loading', async () => {
    section = 'chat'
    const bets = deferred<{ items: Array<{ amount: number; payout: number }>; total: number }>()
    scenario.api.bets.mockReturnValue(bets.promise)
    openProfile(await ready())
    await settle()
    expect(scenario.api.bets).toHaveBeenCalledTimes(2)
    scenario.api.users.mockResolvedValue(roster([moved()]))
    bets.resolve({ items: [{ amount: 37.25, payout: 51.75 }], total: 6 })
    const profile = dialog(await settle(), '会员资料')
    expect(scenario.api.users.mock.calls.length).toBeGreaterThanOrEqual(2)
    expectNoPrivateProfile(profile)
    expect(text(profile)).not.toMatch(/37\.25|51\.75/)
    expectNoWrites()
  })

  it('does not start private chat requests after an unmounted membership lookup completes', async () => {
    section = 'chat'
    const membership = deferred<ReturnType<typeof roster>>()
    scenario.api.users.mockReturnValueOnce(membership.promise)
    openProfile(await ready())
    runtime.hooks!.unmount()
    membership.resolve(roster([current]))
    await drain()
    expect(scenario.api.bets).not.toHaveBeenCalled()
    expect(scenario.other.bets).not.toHaveBeenCalled()
    expectNoWrites()
  })
})
