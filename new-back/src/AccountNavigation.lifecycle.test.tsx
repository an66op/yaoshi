import { isValidElement, type DependencyList, type ReactElement, type ReactNode, type SetStateAction } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { UsersPage } from './pages/UsersPage'

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
  useRef<T>(initial: T) { return this.useState(() => ({ current: initial }))[0] }
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
  hooks: null as PageHarness | null,
  me: vi.fn(), refreshSession: vi.fn(), logout: vi.fn(), users: vi.fn(), userStats: vi.fn(), agents: vi.fn(),
  createUser: vi.fn(), updateUser: vi.fn(), setUserStatus: vi.fn(), resetUserPassword: vi.fn(), adjustUserBalance: vi.fn(),
  showMessage: vi.fn(),
}))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useMemo: <T,>(factory: () => T, deps: DependencyList) => runtime.hooks!.useMemo(factory, deps),
  useCallback: <T,>(callback: T, deps: DependencyList) => runtime.hooks!.useMemo(() => callback, deps),
  useEffect: (effect: () => void | (() => void), deps?: DependencyList) => runtime.hooks!.useEffect(effect, deps),
  // Keep route components opaque while still identifying which lazy page App selects.
  lazy: (factory: () => Promise<unknown>) => {
    const page = () => null
    page.displayName = factory.toString().match(/pages\/([A-Za-z]+)/)?.[1]
    return page
  },
}))
vi.mock('./api', () => ({ adminApi: runtime, AuthError: class AuthError extends Error {} }))
vi.mock('./auth', () => ({ ADMIN_AUTH_EVENT_KEY: 'test-auth-event', clearLegacyAdminSession: vi.fn(), setCurrentUser: vi.fn(), broadcastAdminLogout: vi.fn() }))
vi.mock('./components/AdminShell', () => ({ AdminShell: 'admin-shell' }))
vi.mock('./components/FeedbackProvider', () => ({ FeedbackProvider: 'feedback-provider' }))
vi.mock('./components/feedback', () => ({ useFeedback: () => ({ showMessage: runtime.showMessage }) }))
vi.mock('./hooks/useManagementWebSocket', () => ({ useManagementWebSocket: vi.fn() }))
vi.mock('./pages/LoginPage', () => ({ LoginPage: 'login-page' }))
vi.mock('./theme', () => ({ createAdminTheme: () => ({}) }))

type Props = {
  children?: ReactNode; actions?: ReactNode; title?: string; label?: string; placeholder?: string; value?: unknown
  open?: boolean; disabled?: boolean; select?: boolean; view?: string; section?: string; tenantDirect?: boolean; path?: string
  onClick?: () => void; onChange?: (event: { target: { value: string } }) => void
  onNavigate?: (path: string) => void
  onLogout?: () => Promise<void>
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
const namedPage = (node: ReactNode, name: string) => elements(node).find(element => typeof element.type === 'function' && (element.type as { displayName?: string }).displayName === name)
const shell = (node: ReactNode) => elements(node).find(element => element.type === 'admin-shell')!
const profile = (role: string) => ({ id: 1, username: `${role}-account`, role, status: 1 })
const member = {
  id: 21, public_id: 800021, username: 'member-one', nickname: '会员一', role: 'member', email: '', phone: '',
  risk_level: 'normal', remark: '', status: 1, online: true, balance: 100, login_count: 1,
}
const render = (factory: () => ReactNode) => {
  const result = runtime.hooks!.render(factory)
  runtime.hooks!.flushEffects()
  return result
}
const settle = async () => { for (let index = 0; index < 12; index++) await Promise.resolve() }
let location: { pathname: string }
let history: { replaceState: ReturnType<typeof vi.fn>; pushState: ReturnType<typeof vi.fn> }
let listeners: Map<string, (event?: Event) => void>

beforeEach(() => {
  runtime.hooks = new PageHarness()
  for (const value of Object.values(runtime)) if (vi.isMockFunction(value)) value.mockReset()
  runtime.me.mockResolvedValue(profile('admin'))
  runtime.refreshSession.mockResolvedValue({})
  runtime.logout.mockResolvedValue({})
  runtime.users.mockResolvedValue({ items: [member], total: 1 })
  runtime.userStats.mockResolvedValue({ total: 1, active: 1, disabled: 0, new_today: 1 })
  runtime.agents.mockResolvedValue({ items: [] })
  runtime.createUser.mockResolvedValue(member)
  runtime.updateUser.mockResolvedValue(member)
  runtime.adjustUserBalance.mockResolvedValue({ ...member, balance: 120 })
  location = { pathname: '/' }
  history = {
    replaceState: vi.fn((_data: unknown, _title: string, path: string) => { location.pathname = path }),
    pushState: vi.fn((_data: unknown, _title: string, path: string) => { location.pathname = path }),
  }
  listeners = new Map()
  vi.useFakeTimers()
  vi.stubGlobal('window', {
    location, history, localStorage: { getItem: () => null }, setTimeout, clearTimeout, setInterval, clearInterval,
    addEventListener: (name: string, listener: (event?: Event) => void) => listeners.set(name, listener),
    removeEventListener: (name: string) => listeners.delete(name),
    dispatchEvent: (event: Event) => { listeners.get(event.type)?.(event); return !event.defaultPrevented },
  })
  vi.stubGlobal('document', { visibilityState: 'visible' })
})
afterEach(() => {
  runtime.hooks!.unmount()
  vi.clearAllTimers()
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('retired account route lifecycle', () => {
  it.each(['admin', 'tenant', 'agent'])('opens the existing scoped member page for %s at the old URL', async role => {
    runtime.me.mockResolvedValue(profile(role))
    location.pathname = '/users'
    render(App)
    await settle()
    const root = render(App)
    expect(history.replaceState).toHaveBeenCalledWith({}, '', '/members')
    expect(shell(root).props.path).toBe('/members')
    const page = namedPage(root, role === 'admin' ? 'UsersPage' : 'AgentWorkspacePage')
    expect(page).toBeDefined()
    expect(page!.props.view).toBeUndefined()
    if (role !== 'admin') expect(page!.props.section).toBe('users')
    expect(page!.props.tenantDirect === true).toBe(role === 'tenant')
  })

  it('normalizes old navigation events and back/forward entries without rendering an account page', async () => {
    render(App)
    await settle()
    shell(render(App)).props.onNavigate!('/users')
    expect(history.pushState).toHaveBeenCalledWith({}, '', '/members')
    expect(namedPage(render(App), 'UsersPage')).toBeDefined()
    location.pathname = '/users/'
    listeners.get('popstate')!()
    expect(location.pathname).toBe('/members')
    expect(shell(render(App)).props.path).toBe('/members')
  })

  it('still requires management authentication for the old URL', async () => {
    runtime.me.mockRejectedValue(new Error('unauthenticated'))
    location.pathname = '/users'
    render(App)
    await settle()
    const root = render(App)
    expect(elements(root).some(element => element.type === 'login-page')).toBe(true)
    expect(shell(root)).toBeUndefined()
  })

  it('honors unsaved-page protection for sidebar navigation, browser history and logout', async () => {
    location.pathname = '/limits'
    render(App)
    await settle()
    let root = render(App)
    listeners.set('yaotu-before-navigate', event => event?.preventDefault())
    shell(root).props.onNavigate!('/members')
    expect(history.pushState).not.toHaveBeenCalled()
    await shell(root).props.onLogout!()
    expect(runtime.logout).not.toHaveBeenCalled()
    location.pathname = '/members'
    listeners.get('popstate')!()
    root = render(App)
    expect(location.pathname).toBe('/limits')
    expect(shell(root).props.path).toBe('/limits')
    listeners.delete('yaotu-before-navigate')
    shell(root).props.onNavigate!('/members')
    expect(shell(render(App)).props.path).toBe('/members')
  })
})

describe('platform member management boundary', () => {
  const load = async () => {
    render(UsersPage)
    await vi.advanceTimersByTimeAsync(0)
    return render(UsersPage)
  }
  const openForm = async () => {
    button(await load(), '新增会员').props.onClick!()
    await settle()
    return render(UsersPage)
  }
  const form = (root: ReactNode) => elements(root).find(element => element.props.open === true && button(element, '保存会员'))!

  it('only requests members and never displays mixed administrative accounts', async () => {
    runtime.users.mockResolvedValue({ items: [member, ...['admin', 'tenant', 'agent'].map(role => ({ ...member, id: role, username: `hidden-${role}`, nickname: `hidden-${role}`, role }))], total: 1 })
    const root = await load()
    expect(runtime.users).toHaveBeenCalledWith(expect.objectContaining({ kind: 'member', role: 'member' }))
    expect(runtime.userStats).toHaveBeenCalledWith('member')
    expect(text(root)).toContain('会员一')
    expect(text(root)).not.toMatch(/hidden-(admin|tenant|agent)|新增后台用户|用户管理/)
    expect(elements(root).some(element => element.props.title === '会员管理')).toBe(true)
  })

  it('has no alternative role selector and creates only a member', async () => {
    const root = await openForm()
    const current = form(root)
    expect(field(current, '账号角色').props).toMatchObject({ disabled: true, value: '普通会员' })
    expect(field(current, '账号角色').props.select).not.toBe(true)
    expect(elements(current).some(element => ['admin', 'tenant', 'agent'].includes(String(element.props.value)))).toBe(false)
    field(current, '登录帐号').props.onChange!({ target: { value: 'new-member' } })
    field(current, '初始密码').props.onChange!({ target: { value: 'MemberPass123' } })
    button(form(render(UsersPage)), '保存会员').props.onClick!()
    await settle()
    expect(runtime.createUser).toHaveBeenCalledWith(expect.objectContaining({ username: 'new-member', role: 'member' }))
    expect(runtime.showMessage).toHaveBeenCalledWith('会员创建成功')
  })

  it('keeps member scope after filtering and resetting filters', async () => {
    const root = await load()
    elements(root).find(element => element.props.placeholder?.startsWith('搜索会员'))!.props.onChange!({ target: { value: ' member ' } })
    button(render(UsersPage), '查询').props.onClick!()
    render(UsersPage)
    await vi.advanceTimersByTimeAsync(0)
    expect(runtime.users).toHaveBeenLastCalledWith(expect.objectContaining({ query: 'member', kind: 'member', role: 'member' }))
    button(render(UsersPage), '重置').props.onClick!()
    render(UsersPage)
    await vi.advanceTimersByTimeAsync(0)
    expect(runtime.users).toHaveBeenLastCalledWith(expect.objectContaining({ query: '', kind: 'member', role: 'member' }))
  })

  it('updates members without offering a change to a management role', async () => {
    const root = await load()
    const edit = elements(root).find(element => element.props.title === '编辑资料')!
    elements(edit).find(element => element.props.onClick)!.props.onClick!()
    const current = form(render(UsersPage))
    expect(field(current, '账号角色').props.value).toBe('普通会员')
    field(current, '昵称').props.onChange!({ target: { value: '会员新昵称' } })
    button(form(render(UsersPage)), '保存会员').props.onClick!()
    await settle()
    expect(runtime.updateUser).toHaveBeenCalledWith(member.id, expect.objectContaining({ nickname: '会员新昵称', role: 'member' }))
  })

  it('refreshes member statistics rather than account totals after a balance adjustment', async () => {
    const root = await load()
    const adjust = elements(root).find(element => element.props.title === '调整余额')!
    elements(adjust).find(element => element.props.onClick)!.props.onClick!()
    const current = render(UsersPage)
    field(current, '调整金额').props.onChange!({ target: { value: '20' } })
    field(current, '调整原因').props.onChange!({ target: { value: '会员测试调整' } })
    button(render(UsersPage), '确认调整').props.onClick!()
    await settle()
    expect(runtime.adjustUserBalance).toHaveBeenCalledWith(member.id, 20, '会员测试调整')
    expect(runtime.userStats).toHaveBeenLastCalledWith('member')
  })
})
