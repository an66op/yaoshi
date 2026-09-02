import { Card, Dialog, Drawer, TableBody, TableCell, TableHead, TableRow, Tabs } from '@mui/material'
import { isValidElement, type DependencyList, type ReactElement, type ReactNode, type SetStateAction } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApplicationsPage } from './ApplicationsPage'

type Slot = { value?: unknown; deps?: DependencyList; effect?: () => void | (() => void); cleanup?: void | (() => void) }
const sameDeps = (left?: DependencyList, right?: DependencyList) => Boolean(left && right && left.length === right.length && left.every((value, index) => Object.is(value, right[index])))
class PageHarness {
  private slots: Slot[] = []
  private cursor = 0
  private effects: number[] = []
  render<T>(factory: () => T): T { this.cursor = 0; return factory() }
  useState<T>(initial: T | (() => T)): [T, (value: SetStateAction<T>) => void] {
    const slot = this.slots[this.cursor++] ??= { value: typeof initial === 'function' ? (initial as () => T)() : initial }
    return [slot.value as T, value => { slot.value = typeof value === 'function' ? (value as (previous: T) => T)(slot.value as T) : value }]
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

const runtime = vi.hoisted(() => ({
  hooks: null as PageHarness | null, role: 'agent', showMessage: vi.fn(),
  applications: vi.fn(), applicationStats: vi.fn(), tenants: vi.fn(), agents: vi.fn(), users: vi.fn(), createApplication: vi.fn(),
  adminReview: vi.fn(), tenantReview: vi.fn(), agentReview: vi.fn(),
}))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useMemo: <T,>(factory: () => T, deps: DependencyList) => runtime.hooks!.useMemo(factory, deps),
  useCallback: <T,>(callback: T, deps: DependencyList) => runtime.hooks!.useMemo(() => callback, deps),
  useEffect: (effect: () => void | (() => void), deps?: DependencyList) => runtime.hooks!.useEffect(effect, deps),
}))
vi.mock('../api', () => ({
  adminApi: { ...runtime, reviewApplication: runtime.adminReview },
  tenantApi: { applications: runtime.applications, applicationStats: runtime.applicationStats, reviewApplication: runtime.tenantReview },
  agentApi: { applications: runtime.applications, applicationStats: runtime.applicationStats, reviewApplication: runtime.agentReview },
}))
vi.mock('../auth', () => ({ getStoredUser: () => ({ role: runtime.role }) }))
vi.mock('../components/feedback', () => ({ useFeedback: () => ({ showMessage: runtime.showMessage }) }))
vi.mock('../hooks/useManagementWebSocket', () => ({ MANAGEMENT_WS_EVENT: 'management-test-event' }))

type Category = 'join' | 'wallet' | 'entertainment'
type Props = {
  children?: ReactNode; actions?: ReactNode; label?: string; value?: unknown; open?: boolean; colSpan?: number
  onClick?: () => void; onChange?: (event: { target: { value: string } }, value?: string) => void
}
type Element = ReactElement<Props>
function elements(node: ReactNode): Element[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  if (!isValidElement<Props>(node)) return []
  return [node, ...elements(node.props.children), ...elements(node.props.actions)]
}
function text(node: ReactNode): string {
  if (Array.isArray(node)) return node.map(text).join('')
  if (isValidElement<Props>(node)) return text(node.props.children)
  return typeof node === 'string' || typeof node === 'number' ? String(node) : ''
}
const button = (node: ReactNode, label: string) => elements(node).find(element => element.props.onClick && text(element) === label)!
const field = (node: ReactNode, label: string) => elements(node).find(element => element.props.label === label)!
const labels = (node: ReactNode) => elements(node).flatMap(element => typeof element.props.label === 'string' ? [element.props.label] : [])
const tableHead = (root: ReactNode) => elements(root).find(element => element.type === TableHead)!
const tableBody = (root: ReactNode) => elements(root).find(element => element.type === TableBody)!
const cells = (root: ReactNode) => elements(root).filter(element => element.type === TableCell)
const drawer = (root: ReactNode) => elements(root).find(element => element.type === Drawer && element.props.open)!
const dialog = (root: ReactNode) => elements(root).find(element => element.type === Dialog && element.props.open)!
const amountLabels = ['当前余额', '审核前余额', '审核后余额', '支付方式', '申请金额', '到账金额']
const application = (request_type = 'join') => ({
  id: 41, user_id: 91, username: 'member-one', account_type: 'member', request_type, status: 'pending',
  target_room_code: '99002', room_code: '88001', room_name: '当前房间', odds_multiplier: 1.2,
  payment_type: 'bank', payment_account_label: '测试银行账户', requested_amount: 1234, received_amount: 1188,
  user_balance: 777, balance_before: 500, balance_after: 600, game_id: '',
  operator: '审核员', created_at: '2026-08-31T09:00:00+08:00', reviewed_at: '2026-08-31T10:00:00+08:00',
  remark: '测试申请', review_remark: '测试审核', request_id: 'application-test-41', chat_message_id: 81,
})
let category: Category
const render = () => {
  const root = runtime.hooks!.render(() => ApplicationsPage({ initialCategory: category }))
  runtime.hooks!.flushEffects()
  return root
}
const load = async () => { render(); await vi.advanceTimersByTimeAsync(0); return render() }
const settle = async () => { for (let index = 0; index < 12; index++) await Promise.resolve(); return render() }

beforeEach(() => {
  runtime.hooks = new PageHarness()
  runtime.role = 'agent'
  category = 'join'
  for (const value of Object.values(runtime)) if (vi.isMockFunction(value)) value.mockReset()
  runtime.applications.mockResolvedValue({ items: [], total: 0 })
  runtime.applicationStats.mockResolvedValue({ pending: 3, approved_today: 2, rejected_today: 1, today_amount: 54321.67 })
  runtime.tenants.mockResolvedValue({ items: [] })
  runtime.agents.mockResolvedValue({ items: [] })
  runtime.users.mockResolvedValue({ items: [{ id: 91, username: 'member-one', nickname: '会员', balance: 100 }] })
  runtime.createApplication.mockResolvedValue(application('credit'))
  for (const review of [runtime.adminReview, runtime.tenantReview, runtime.agentReview]) review.mockResolvedValue({ ...application(), status: 'approved' })
  vi.useFakeTimers()
  vi.stubGlobal('window', { setTimeout, clearTimeout, addEventListener: vi.fn(), removeEventListener: vi.fn() })
})
afterEach(() => {
  runtime.hooks!.unmount()
  vi.clearAllTimers()
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe.each(['agent', 'tenant'])('%s application table', role => {
  it.each(['join', 'wallet', 'entertainment'] as const)('keeps %s headers, summary cards and empty-state width consistent', async selected => {
    runtime.role = role
    category = selected
    const root = await load()
    const headers = cells(tableHead(root)).map(text)
    const isJoin = selected === 'join'
    expect(headers).toHaveLength(isJoin ? 7 : 9)
    expect(headers).toContain(isJoin ? '目标房间' : selected === 'entertainment' ? '娱乐平台' : '支付方式')
    expect(headers.includes('申请金额')).toBe(!isJoin)
    expect(headers.includes('到账金额')).toBe(!isJoin)
    expect(text(root).includes('今日申请金额')).toBe(!isJoin)
    expect(text(root).includes('54,321.67')).toBe(!isJoin)
    const summaryCards = elements(root).filter(element => element.type === Card && !elements(element).some(child => child.type === TableHead))
    expect(summaryCards).toHaveLength(isJoin ? 3 : 4)
    expect(cells(tableBody(root))).toHaveLength(1)
    expect(cells(tableBody(root))[0].props.colSpan).toBe(headers.length)
    expect(runtime.applications).toHaveBeenCalledWith(expect.objectContaining({ type: selected }))
  })
})

it.each(['join', 'wallet', 'entertainment'] as const)('keeps loaded %s rows aligned with their headers', async selected => {
  category = selected
  runtime.applications.mockResolvedValue({ items: [{ ...application(selected === 'join' ? 'join' : 'credit'), game_id: selected === 'entertainment' ? '测试体育' : '' }], total: 1 })
  const root = await load()
  const rows = elements(tableBody(root)).filter(element => element.type === TableRow)
  expect(rows).toHaveLength(1)
  expect(cells(rows[0])).toHaveLength(cells(tableHead(root)).length)
  expect(text(rows[0]).includes('1,234.00')).toBe(selected !== 'join')
  expect(text(rows[0]).includes('1,188.00')).toBe(selected !== 'join')
})

it('updates the layout when switching from wallet to join and back to entertainment', async () => {
  category = 'wallet'
  let root = await load()
  const select = (value: Category) => {
    elements(root).find(element => element.type === Tabs && ['join', 'wallet', 'entertainment'].includes(String(element.props.value)))!.props.onChange!({ target: { value: '' } }, value)
  }
  select('join')
  root = render()
  await vi.advanceTimersByTimeAsync(0)
  root = render()
  expect(cells(tableHead(root))).toHaveLength(7)
  expect(text(root)).not.toContain('今日申请金额')
  expect(cells(tableBody(root))[0].props.colSpan).toBe(7)
  select('entertainment')
  root = render()
  await vi.advanceTimersByTimeAsync(0)
  root = render()
  expect(cells(tableHead(root))).toHaveLength(9)
  expect(text(root)).toContain('今日申请金额')
  expect(cells(tableBody(root))[0].props.colSpan).toBe(9)
})

describe('application detail fields depend on request type', () => {
  it.each(['join', 'wallet'] as const)('hides join financial data even when opened from the %s tab', async selected => {
    category = selected
    runtime.applications.mockResolvedValue({ items: [{ ...application(), game_id: '旧平台标记' }], total: 1 })
    button(await load(), '详情').props.onClick!()
    const detail = drawer(render())
    for (const label of amountLabels) expect(labels(detail)).not.toContain(label)
    expect(labels(detail)).not.toContain('娱乐平台')
    expect(field(detail, '目标房间').props.value).toContain('99002')
    expect(field(detail, '目标房间').props.value).not.toContain('88001')
    expect(field(detail, '会员赔率倍率').props.value).toBe('1.20×')
    for (const label of ['申请用户', '申请类型', '审核状态', '申请时间', '审核时间', '操作人']) expect(labels(detail)).toContain(label)
    expect(text(detail)).toContain('处理时间线')
    expect(text(detail)).toContain('测试申请')
    expect(text(detail)).toContain('测试审核')
  })

  it.each(['credit', 'debit'])('retains financial information for %s details', async type => {
    category = 'wallet'
    runtime.applications.mockResolvedValue({ items: [application(type)], total: 1 })
    button(await load(), '详情').props.onClick!()
    const detail = drawer(render())
    for (const label of amountLabels) expect(labels(detail)).toContain(label)
    expect(field(detail, '申请金额').props.value).toBe('1,234.00')
    expect(field(detail, '到账金额').props.value).toBe('1,188.00')
    expect(field(detail, '支付方式').props.value).toBe('测试银行账户')
    expect(field(detail, '当前余额').props.value).toBe('777.00')
    expect(field(detail, '审核前余额').props.value).toBe('500.00')
    expect(field(detail, '审核后余额').props.value).toBe('600.00')
  })

  it.each(['credit', 'debit'])('retains entertainment platform and funds for entertainment %s details', async type => {
    category = 'entertainment'
    runtime.applications.mockResolvedValue({ items: [{ ...application(type), game_id: '测试体育' }], total: 1 })
    button(await load(), '详情').props.onClick!()
    const detail = drawer(render())
    expect(field(detail, '娱乐平台').props.value).toBe('测试体育')
    for (const label of amountLabels) expect(labels(detail)).toContain(label)
  })
})

describe('existing application review behaviour', () => {
  it.each(['agent', 'tenant'])('%s can approve a join with a room multiplier and no money input', async role => {
    runtime.role = role
    runtime.applications.mockResolvedValue({ items: [application()], total: 1 })
    button(await load(), '审核').props.onClick!()
    let review = dialog(render())
    expect(labels(review)).not.toContain('实际到账金额')
    expect(labels(review)).not.toContain('实际出款金额')
    expect(text(review)).not.toContain('申请金额')
    expect(text(review)).not.toContain('变动前余额')
    expect(text(review)).not.toContain('预计变动后余额')
    expect(text(review)).toContain('目标房间 99002')
    field(review, '自定义倍率').props.onChange!({ target: { value: '0.9' } })
    review = dialog(render())
    button(review, '确认通过').props.onClick!()
    await settle()
    const api = role === 'agent' ? runtime.agentReview : runtime.tenantReview
    expect(api).toHaveBeenCalledExactlyOnceWith(41, { decision: 'approved', received_amount: 0, odds_multiplier: 0.9, remark: '' })
    expect(runtime.adminReview).not.toHaveBeenCalled()
    expect(runtime.showMessage).toHaveBeenCalledWith('入房申请已通过')
    expect(dialog(render())).toBeUndefined()
  })

  it.each(['credit', 'debit'])('preserves %s amount input and review payload', async type => {
    category = 'wallet'
    runtime.applications.mockResolvedValue({ items: [application(type)], total: 1 })
    button(await load(), '审核').props.onClick!()
    const review = dialog(render())
    expect(text(review)).toContain('变动前余额')
    expect(text(review)).toContain('预计变动后余额')
    field(review, type === 'credit' ? '实际到账金额' : '实际出款金额').props.onChange!({ target: { value: '1000' } })
    button(dialog(render()), '确认通过').props.onClick!()
    await settle()
    expect(runtime.agentReview).toHaveBeenCalledExactlyOnceWith(41, { decision: 'approved', received_amount: 1000, odds_multiplier: undefined, remark: '' })
  })
})
