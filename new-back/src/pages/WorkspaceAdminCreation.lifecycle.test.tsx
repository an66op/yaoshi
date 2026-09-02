import { isValidElement, type DependencyList, type ReactElement, type ReactNode, type SetStateAction } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { WorkspaceAdminAccountFields, WorkspaceAdminCreatedDialog } from '../components/WorkspaceAdminAccount'
import { AgentsPage } from './AgentsPage'
import { TenantsPage } from './TenantsPage'
import { TenantWorkspacePage } from './TenantWorkspacePage'

// Drive page hooks and their actual event handlers without mounting MUI or a browser.
// Child components stay opaque, as in LoginPage.lifecycle.test.tsx.
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
  useRef<T>(initial: T) {
    const slot = this.slots[this.cursor++] ??= { value: { current: initial } }
    return slot.value as { current: T }
  }
  useCallback<T>(callback: T, deps: DependencyList): T {
    const index = this.cursor++
    const slot = this.slots[index]
    if (!slot || !sameDeps(slot.deps, deps)) this.slots[index] = { value: callback, deps }
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
  showMessage: vi.fn(),
  adminAgents: vi.fn(), adminTenants: vi.fn(), tenantAgents: vi.fn(), tenantDashboard: vi.fn(),
  createAgent: vi.fn(), createTenant: vi.fn(), createTenantAgent: vi.fn(),
  updateAgent: vi.fn(), updateTenant: vi.fn(), updateTenantAgent: vi.fn(),
}))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useCallback: <T,>(callback: T, deps: DependencyList) => runtime.hooks!.useCallback(callback, deps),
  useEffect: (effect: () => void | (() => void), deps?: DependencyList) => runtime.hooks!.useEffect(effect, deps),
}))
vi.mock('../api', () => ({
  adminApi: { agents: runtime.adminAgents, tenants: runtime.adminTenants, createAgent: runtime.createAgent, createTenant: runtime.createTenant, updateAgent: runtime.updateAgent, updateTenant: runtime.updateTenant },
  tenantApi: { dashboard: runtime.tenantDashboard, agents: runtime.tenantAgents, createAgent: runtime.createTenantAgent, updateAgent: runtime.updateTenantAgent },
}))
vi.mock('../components/feedback', () => ({ useFeedback: () => ({ showMessage: runtime.showMessage }) }))

type ElementProps = {
  children?: ReactNode
  actions?: ReactNode
  action?: ReactNode
  label?: string
  'aria-label'?: string
  role?: string
  editing?: boolean
  open?: boolean
  disabled?: boolean
  password?: string
  username?: string
  severity?: string
  variant?: string
  account?: { role: string; username: string; roomCode: string; status: number } | null
  onClick?: () => void
  onClose?: () => void
  onChange?: (event: { target: { value: string } }) => void
  onUsernameChange?: (value: string) => void
  onPasswordChange?: (value: string) => void
}
type Element = ReactElement<ElementProps>
function elements(node: ReactNode): Element[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  if (!isValidElement<ElementProps>(node)) return []
  return [node, ...elements(node.props.children), ...elements(node.props.actions), ...elements(node.props.action)]
}
function text(node: ReactNode): string {
  if (Array.isArray(node)) return node.map(text).join('')
  if (isValidElement<ElementProps>(node)) return text(node.props.children)
  return typeof node === 'string' || typeof node === 'number' ? String(node) : ''
}
const accountFields = (root: ReactNode) => elements(root).find(element => element.type === WorkspaceAdminAccountFields)!
const summary = (root: ReactNode) => elements(root).find(element => element.type === WorkspaceAdminCreatedDialog)!
const createForm = (root: ReactNode) => elements(root).find(element => typeof element.props.open === 'boolean' && elements(element.props.children).some(child => child.type === WorkspaceAdminAccountFields))!
const submit = (root: ReactNode) => elements(createForm(root)).find(element => element.props.onClick && element.props.variant === 'contained')!
const cancel = (root: ReactNode) => elements(createForm(root)).find(element => element.props.onClick && text(element) === '取消')!
const errorText = (root: ReactNode) => elements(createForm(root)).filter(element => element.props.severity === 'error').map(text).join('')
const roomField = (root: ReactNode) => elements(createForm(root)).find(element => /^(公开)?房间号$/.test(element.props.label ?? ''))!

const cases = [
  { name: 'platform creates agent', role: 'agent', factory: () => AgentsPage(), open: /^开通代理账号$/, api: runtime.createAgent, list: runtime.adminAgents, update: runtime.updateAgent },
  { name: 'platform creates tenant', role: 'tenant', factory: () => TenantsPage(), open: /^开通租户账号$/, api: runtime.createTenant, list: runtime.adminTenants, update: runtime.updateTenant },
  { name: 'tenant creates agent', role: 'agent', factory: () => TenantWorkspacePage({ section: 'agents' }), open: /^开通代理账号$/, api: runtime.createTenantAgent, list: runtime.tenantAgents, update: runtime.updateTenantAgent },
] as const

describe.each(cases)('$name', scenario => {
  const render = () => {
    const result = runtime.hooks!.render(scenario.factory)
    runtime.hooks!.flushEffects()
    return result
  }
  const open = async () => {
    render()
    await vi.runOnlyPendingTimersAsync()
    const ready = render()
    const button = elements(ready).find(element => element.props.onClick && scenario.open.test(text(element)))!
    expect(button, 'create action is available').toBeDefined()
    button.props.onClick!()
    return render()
  }
  const fill = (root: ReactNode, username = ' submitted-admin ', password = 'private-password-123') => {
    accountFields(root).props.onUsernameChange!(username)
    accountFields(root).props.onPasswordChange!(password)
    roomField(root).props.onChange!({ target: { value: '987654' } })
    elements(createForm(root)).find(element => element.props.label === '房间名称')?.props.onChange!({ target: { value: '测试房间' } })
    return render()
  }
  const settle = async () => {
    for (let index = 0; index < 12; index++) await Promise.resolve()
    return render()
  }

  beforeEach(() => {
    runtime.hooks = new PageHarness()
    for (const value of Object.values(runtime)) if (vi.isMockFunction(value)) value.mockReset()
    runtime.adminAgents.mockResolvedValue({ items: [], total: 0 })
    runtime.adminTenants.mockResolvedValue({ items: [], total: 0 })
    runtime.tenantAgents.mockResolvedValue({ items: [], total: 0 })
    runtime.tenantDashboard.mockResolvedValue({ agent_count: 0, active_agent_count: 0, member_count: 0 })
    vi.useFakeTimers()
    vi.stubGlobal('window', { location: { origin: 'http://127.0.0.1:5174' }, setTimeout, clearTimeout })
  })
  afterEach(() => {
    runtime.hooks!.unmount()
    vi.clearAllTimers()
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('requires administrator credentials and leaves a rejected account visible inside the form', async () => {
    const initial = await open()
    expect(accountFields(initial).props).toMatchObject({ role: scenario.role, username: '', password: '' })
    expect(summary(initial).props.account).toBeNull()
    const ready = fill(initial, 'ab')
    submit(ready).props.onClick!()
    const rejected = await settle()
    expect(scenario.api).not.toHaveBeenCalled()
    expect(createForm(rejected).props.open).toBe(true)
    expect(errorText(rejected)).toContain('3–50')
    expect(summary(rejected).props.account).toBeNull()
  })

  it('shows the API-returned account only after success and clears the initial password', async () => {
    const response = { username: 'canonical-api-account', room_code: '876543', status: 0, password: 'server-private-field', token: 'server-private-token' }
    scenario.api.mockResolvedValue(response)
    const ready = fill(await open())
    submit(ready).props.onClick!()
    const finished = await settle()
    expect(scenario.api).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ username: 'submitted-admin', password: 'private-password-123', room_code: '987654' }))
    expect(summary(finished).props.account).toEqual({ role: scenario.role, username: 'canonical-api-account', roomCode: '876543', status: 0 })
    expect(JSON.stringify(summary(finished).props.account)).not.toMatch(/password|token|private/)
    expect(createForm(finished).props.open).toBe(false)
    expect(accountFields(finished).props.password).toBe('')
    expect(runtime.showMessage).toHaveBeenCalledWith(expect.stringContaining('账号已创建'))
    summary(finished).props.onClose!()
    expect(summary(render()).props.account).toBeNull()
  })

  it('keeps server errors in the create form without a false success confirmation', async () => {
    scenario.api.mockRejectedValue(new Error('登录账号已被使用，请换一个账号'))
    const ready = fill(await open())
    submit(ready).props.onClick!()
    const rejected = await settle()
    expect(createForm(rejected).props.open).toBe(true)
    expect(errorText(rejected)).toContain('登录账号已被使用')
    expect(summary(rejected).props.account).toBeNull()
    expect(runtime.showMessage).not.toHaveBeenCalled()
    expect(accountFields(rejected).props).toMatchObject({ username: ' submitted-admin ', password: 'private-password-123', disabled: false })
    expect(submit(rejected).props.disabled).toBe(false)
  })

  it('prevents duplicate creation and closing the form while the request is pending', async () => {
    let resolve!: (value: unknown) => void
    scenario.api.mockImplementation(() => new Promise(done => { resolve = done }))
    const ready = fill(await open())
    const button = submit(ready)
    button.props.onClick!()
    button.props.onClick!()
    let pending = render()
    expect(scenario.api).toHaveBeenCalledTimes(1)
    expect(summary(pending).props.account).toBeNull()
    expect(accountFields(pending).props.disabled).toBe(true)
    expect(submit(pending).props.disabled).toBe(true)
    expect(cancel(pending).props.disabled).toBe(true)
    createForm(pending).props.onClose!()
    cancel(pending).props.onClick!()
    pending = render()
    expect(createForm(pending).props.open).toBe(true)
    expect(accountFields(pending).props.password).toBe('private-password-123')
    resolve({ username: 'new-admin', room_code: '987654', status: 1 })
    const finished = await settle()
    expect(createForm(finished).props.open).toBe(false)
    expect(accountFields(finished).props.password).toBe('')
    expect(summary(finished).props.account?.username).toBe('new-admin')
  })

  it('discards credentials on cancel and opens the next creation empty', async () => {
    const ready = fill(await open())
    cancel(ready).props.onClick!()
    const closed = render()
    expect(createForm(closed).props.open).toBe(false)
    expect(accountFields(closed).props.password).toBe('')
    expect(scenario.api).not.toHaveBeenCalled()
    expect(summary(closed).props.account).toBeNull()
    const reopened = await open()
    expect(accountFields(reopened).props).toMatchObject({ username: '', password: '' })
  })

  it('maintains existing login account details directly in the dedicated management page', async () => {
    const row = {
      id: 61, public_id: 300001, username: 'existing-owner', nickname: '已有账号',
      email: 'owner@example.test', phone: '13800000001', room_code: '987654', room_name: '关联房间', room_logo: '',
      workspace_id: 17, status: 1, remark: '', balance: 0, member_count: 0, agent_count: 0,
      rebate_rate: 0, profit_share_rate: 12, login_count: 3, last_login_at: '2026-08-31 12:30:00',
    }
    scenario.list.mockResolvedValue({ items: [row], total: 1 })
    scenario.update.mockResolvedValue({ ...row, email: 'edited@example.test' })
    render()
    await vi.runOnlyPendingTimersAsync()
    let root = render()
    expect(text(root)).toContain('@existing-owner')
    expect(text(root)).toContain('owner@example.test')
    expect(text(root)).toContain('13800000001')
    const iconOnlySettings = scenario.list === runtime.adminAgents
    const settingsLabel = `设置 @${row.username} 代理账号`
    const settings = elements(root).find(element => element.props.onClick && (iconOnlySettings
      ? element.props['aria-label'] === settingsLabel
      : /^(账号设置|编辑账号)$/.test(text(element))))!
    expect(settings).toBeDefined()
    if (iconOnlySettings) {
      expect(text(settings)).toBe('')
      expect(settings.props['aria-label']).toBe(settingsLabel)
    }
    settings.props.onClick!()
    root = render()
    expect(createForm(root).props.open).toBe(true)
    expect(accountFields(root).props).toMatchObject({ role: scenario.role, username: 'existing-owner', password: '', editing: true })
    expect(text(root)).not.toContain('用户管理')
    elements(createForm(root)).find(element => element.props.label === '邮箱')!.props.onChange!({ target: { value: 'edited@example.test' } })
    submit(render()).props.onClick!()
    root = await settle()
    expect(scenario.update).toHaveBeenCalledExactlyOnceWith(row.id, expect.objectContaining({ email: 'edited@example.test', room_code: row.room_code }))
    expect(scenario.api).not.toHaveBeenCalled()
    expect(createForm(root).props.open).toBe(false)
    expect(summary(root).props.account).toBeNull()
  })
})
