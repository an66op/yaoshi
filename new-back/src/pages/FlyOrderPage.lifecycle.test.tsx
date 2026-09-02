import { Button, Dialog, Select, Table, TableBody, TableCell, TableHead, TableRow, TextField } from '@mui/material'
import { isValidElement, type DependencyList, type ReactElement, type ReactNode, type SetStateAction } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AdminUser, UserTradingConfig, WorkspaceMember } from '../api'
import { FlyOrderPage } from './FlyOrderPage'

// Exercise membership and configuration lifecycles in Node, without live services.
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
  const api = () => ({ users: vi.fn(), userTrading: vi.fn(), updateUserTrading: vi.fn() })
  return { hooks: null as PageHarness | null, admin: api(), tenant: api(), agent: api(), showMessage: vi.fn() }
})
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial?: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useMemo: <T,>(factory: () => T, deps: DependencyList) => runtime.hooks!.useMemo(factory, deps),
  useCallback: <T,>(callback: T, deps: DependencyList) => runtime.hooks!.useMemo(() => callback, deps),
  useEffect: (effect: () => void | (() => void), deps?: DependencyList) => runtime.hooks!.useEffect(effect, deps),
}))
vi.mock('../api', () => ({ adminApi: runtime.admin, tenantApi: runtime.tenant, agentApi: runtime.agent }))
vi.mock('../components/feedback', () => ({ useFeedback: () => ({ showMessage: runtime.showMessage }) }))

type Props = {
  children?: ReactNode; action?: ReactNode; actions?: ReactNode; label?: string; value?: unknown
  open?: boolean; disabled?: boolean; colSpan?: number
  onClick?: () => void | Promise<void>
  onChange?: (event: { target: { value: string } }) => void
}
type Element = ReactElement<Props>
function elements(node: ReactNode): Element[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  if (!isValidElement<Props>(node)) return []
  return [node, ...elements(node.props.children), ...elements(node.props.action), ...elements(node.props.actions)]
}
function text(node: ReactNode): string {
  if (Array.isArray(node)) return node.map(text).join('')
  if (isValidElement<Props>(node)) return text(node.props.children) + (node.props.label ?? '')
  return typeof node === 'string' || typeof node === 'number' ? String(node) : ''
}
const ofType = (node: ReactNode, type: unknown) => elements(node).filter(element => element.type === type)
const button = (node: ReactNode, label: string) => ofType(node, Button).find(element => text(element) === label)!
const field = (node: ReactNode, label: string) => ofType(node, TextField).find(element => element.props.label === label)!
const dialog = (node: ReactNode) => ofType(node, Dialog).find(element => element.props.open)!
const table = (node: ReactNode) => ofType(node, Table)[0]
const rows = (node: ReactNode) => ofType(ofType(table(node), TableBody)[0], TableRow)
const headers = (node: ReactNode) => ofType(ofType(table(node), TableHead)[0], TableCell).map(text)
const cells = (node: ReactNode) => ofType(node, TableCell)
const rowFor = (node: ReactNode, username: string) => rows(node).find(row => text(row).includes(`${username} · #`))!
const cellFor = (root: ReactNode, row: ReactNode, header: string) => cells(row)[headers(root).indexOf(header)]
const list = (items: Array<WorkspaceMember | AdminUser>) => ({ items, total: items.length, page: 1, page_size: 20 })
const deferred = <T,>() => {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(finish => { resolve = finish })
  return { promise, resolve }
}

const current: WorkspaceMember = {
  id: 41, public_id: 100041, username: 'current-member', nickname: '当前会员', avatar: '/avatars/current.png', role: 'member',
  in_current_room: true, can_manage: true, balance: 100, online: true, status: 1, fly_mode: 'custom', fly_rate: 12.5,
}
// Leave stale private fly-order values populated to prove flags govern disclosure.
const moved = (member: WorkspaceMember = current): WorkspaceMember => ({ ...member, in_current_room: false, can_manage: false, fly_mode: 'custom', fly_rate: 99.99 })
const historical = moved({ ...current, id: 42, public_id: 100042, username: 'historical-member', nickname: '历史会员' })
const adminMember: AdminUser = {
  id: current.id, public_id: current.public_id, username: current.username, nickname: current.nickname, avatar: current.avatar,
  role: 'member', email: '', phone: '', remark: '', risk_level: 'normal', balance: 100, online: true, status: 1,
  fly_mode: 'custom', fly_rate: 12.5, room_code: '88008', last_login_at: null, login_count: 0,
  created_at: '2026-08-01T08:00:00Z', updated_at: '2026-08-01T08:00:00Z',
}
const config: UserTradingConfig = {
  user_id: current.id, workspace_id: 8, username: current.username,
  fly: { mode: 'custom', rate: 12.5 }, rebate: { mode: 'inherit', rate: 2, effective: 2, source: 'room' },
  external_follow: {
    status: 'not_connected', capability: 'configuration_only', target_platform: '测试目标平台', target_account: '账户别名',
    endpoint_label: '测试线路', single_limit: 50, daily_limit: 500, remark: '测试连接说明',
  },
  game_id: 'room-game', game_name: '房间彩种', room_fly_rate: 5, room_rebate_rate: 2, odds: [],
}
let role: 'admin' | 'tenant' | 'agent'
const render = () => {
  const root = runtime.hooks!.render(() => FlyOrderPage({ role }))
  runtime.hooks!.flushEffects()
  return root
}
const drain = async () => { for (let index = 0; index < 24; index++) await Promise.resolve() }
const settle = async () => { await drain(); return render() }
const ready = async () => { render(); return settle() }
const open = async (root: ReactNode) => {
  button(rowFor(root, current.username), '独立配置').props.onClick!()
  return dialog(await settle())
}
const edit = async (root: ReactNode) => {
  const editor = await open(root)
  expect(editor).toBeDefined()
  field(editor, '会员飞单比例 %').props.onChange!({ target: { value: '18.5' } })
  return dialog(render())
}
const expectNoMutation = () => {
  for (const api of [runtime.admin, runtime.tenant, runtime.agent]) expect(api.updateUserTrading).not.toHaveBeenCalled()
}
const expectValidation = (api: typeof runtime.agent) => expect(api.users).toHaveBeenLastCalledWith({ userId: current.id, page: 1, pageSize: 1 })

beforeEach(() => {
  role = 'agent'
  runtime.hooks = new PageHarness()
  runtime.showMessage.mockReset()
  for (const api of [runtime.admin, runtime.tenant, runtime.agent]) {
    for (const mock of Object.values(api)) mock.mockReset()
    api.users.mockResolvedValue(list([current]))
    api.userTrading.mockResolvedValue(config)
    api.updateUserTrading.mockResolvedValue({ ...config, fly: { mode: 'custom', rate: 18.5 } })
  }
  runtime.admin.users.mockResolvedValue(list([adminMember]))
})
afterEach(() => { runtime.hooks!.unmount() })

describe.each(['tenant', 'agent'] as const)('%s fly-order membership lifecycle', scope => {
  const api = runtime[scope]
  beforeEach(() => { role = scope })

  it('shows room status and redacts historical rows despite stale fly-order fields', async () => {
    api.users.mockResolvedValue(list([current, historical]))
    const root = await ready()
    expect(api.users).toHaveBeenCalledExactlyOnceWith({ query: '', status: 'all', page: 1, pageSize: 20 })
    expect(headers(root)).toContain('房间状态')
    for (const row of rows(root)) expect(cells(row)).toHaveLength(headers(root).length)
    const active = rowFor(root, current.username)
    expect(text(cellFor(root, active, '房间状态'))).toBe('在本房间')
    expect(text(cellFor(root, active, '站内飞单'))).toContain('12.5%')
    expect(button(active, '独立配置')).toBeDefined()
    const old = rowFor(root, historical.username)
    expect(text(old)).toContain(historical.nickname)
    expect(text(cellFor(root, old, '房间状态'))).toBe('已切换')
    for (const header of ['站内飞单', '外部连接', '操作']) expect(text(cellFor(root, old, header))).toBe('—')
    expect(text(old)).not.toContain('99.99')
    expect(button(old, '独立配置')).toBeUndefined()
    expect(text(root)).toContain('本页单独比例1')
  })

  it.each([
    { label: 'can_manage false', in_current_room: true, can_manage: false },
    { label: 'in_current_room false', in_current_room: false, can_manage: true },
    { label: 'can_manage missing', in_current_room: true, can_manage: undefined },
    { label: 'in_current_room missing', in_current_room: undefined, can_manage: true },
  ])('denies independent configuration with $label', async flags => {
    api.users.mockResolvedValue(list([{ ...current, ...flags } as WorkspaceMember]))
    const root = await ready()
    const row = rowFor(root, current.username)
    expect(button(row, '独立配置')).toBeUndefined()
    for (const header of ['站内飞单', '外部连接', '操作']) expect(text(cellFor(root, row, header))).toBe('—')
    expect(api.userTrading).not.toHaveBeenCalled()
    expectNoMutation()
  })

  it('preserves current-member configuration and revalidates before and after reading and saving', async () => {
    const editor = await edit(await ready())
    expect(api.userTrading).toHaveBeenCalledExactlyOnceWith(current.id)
    expect(api.users.mock.calls.length).toBeGreaterThanOrEqual(3)
    expectValidation(api)
    expect(field(editor, '目标平台').props.value).toBe(config.external_follow.target_platform)
    button(editor, '保存会员配置').props.onClick!()
    await settle()
    expect(api.updateUserTrading).toHaveBeenCalledExactlyOnceWith(current.id, {
      fly_mode: 'custom', fly_rate: 18.5,
      external_follow: {
        target_platform: '测试目标平台', target_account: '账户别名', endpoint_label: '测试线路',
        single_limit: 50, daily_limit: 500, remark: '测试连接说明',
      },
      rebate_mode: 'inherit', rebate_rate: 2, game_id: '', odds: [],
    })
    expect(api.users.mock.calls.length).toBeGreaterThanOrEqual(5)
    expectValidation(api)
    expect(runtime.admin.userTrading).not.toHaveBeenCalled()
    expect(runtime.admin.updateUserTrading).not.toHaveBeenCalled()
  })

  it('revalidates by exact member ID when the roster username and nickname are empty', async () => {
    api.users.mockResolvedValue(list([{ ...current, username: '', nickname: '' }]))
    const root = await ready()
    button(rowFor(root, ''), '独立配置').props.onClick!()
    const editor = dialog(await settle())
    expect(field(editor, '目标平台').props.value).toBe(config.external_follow.target_platform)
    expect(api.userTrading).toHaveBeenCalledExactlyOnceWith(current.id)
    expect(api.users.mock.calls.length).toBeGreaterThanOrEqual(3)
    for (const [params] of api.users.mock.calls.slice(1)) expect(params).toEqual({ userId: current.id, page: 1, pageSize: 1 })
    expectNoMutation()
  })

  describe.each([
    { name: 'moved', items: [moved()] },
    { name: 'missing', items: [] },
  ])('when the member is $name on revalidation', changed => {
    it('rejects a stale row action before requesting private trading configuration', async () => {
      const action = button(rowFor(await ready(), current.username), '独立配置')
      api.users.mockResolvedValue(list(changed.items))
      action.props.onClick!()
      const root = await settle()
      expectValidation(api)
      expect(api.userTrading).not.toHaveBeenCalled()
      expect(dialog(root)).toBeUndefined()
      expectNoMutation()
    })

    it('closes a stale editor without submitting configuration', async () => {
      const editor = await edit(await ready())
      api.users.mockResolvedValue(list(changed.items))
      button(editor, '保存会员配置').props.onClick!()
      const root = await settle()
      expectValidation(api)
      expectNoMutation()
      expect(dialog(root)).toBeUndefined()
    })
  })

  it('discards private configuration when membership changes during the trading request', async () => {
    const pending = deferred<UserTradingConfig>()
    api.userTrading.mockReturnValue(pending.promise)
    button(rowFor(await ready(), current.username), '独立配置').props.onClick!()
    let root = await settle()
    expect(api.userTrading).toHaveBeenCalledTimes(1)
    expect(field(dialog(root), '目标平台')).toBeUndefined()
    api.users.mockResolvedValue(list([moved()]))
    pending.resolve(config)
    root = await settle()
    expectValidation(api)
    expect(dialog(root)).toBeUndefined()
    expect(field(root, '目标平台')).toBeUndefined()
    expectNoMutation()
  })

  it('invalidates the open editor when a filter reload discovers historical membership', async () => {
    await edit(await ready())
    const root = render()
    api.users.mockResolvedValue(list([moved()]))
    ofType(root, Select).find(element => element.props.label === '会员状态')!.props.onChange!({ target: { value: 'active' } })
    render()
    const refreshed = await settle()
    expect(api.users).toHaveBeenLastCalledWith({ query: '', status: 'active', page: 1, pageSize: 20 })
    expect(dialog(refreshed)).toBeUndefined()
    expect(text(cellFor(refreshed, rowFor(refreshed, current.username), '房间状态'))).toBe('已切换')
    expectNoMutation()
  })

  it('does not redisplay saved private configuration after post-save membership denial', async () => {
    const saved = deferred<UserTradingConfig>()
    api.updateUserTrading.mockReturnValue(saved.promise)
    const editor = await edit(await ready())
    button(editor, '保存会员配置').props.onClick!()
    await drain()
    expect(api.updateUserTrading).toHaveBeenCalledTimes(1)
    api.users.mockResolvedValue(list([moved()]))
    saved.resolve({ ...config, fly: { mode: 'custom', rate: 18.5 } })
    const root = await settle()
    expectValidation(api)
    expect(dialog(root)).toBeUndefined()
    expect(field(root, '目标平台')).toBeUndefined()
    expect(runtime.showMessage).not.toHaveBeenCalledWith(expect.anything(), 'success')
  })

  it('deduplicates same-frame saves while membership validation is pending', async () => {
    const editor = await edit(await ready())
    const permission = deferred<ReturnType<typeof list>>()
    const requestsBefore = api.users.mock.calls.length
    api.users.mockReturnValueOnce(permission.promise)
    const save = button(editor, '保存会员配置')
    save.props.onClick!()
    save.props.onClick!()
    await drain()
    expect(api.users).toHaveBeenCalledTimes(requestsBefore + 1)
    expectNoMutation()
    permission.resolve(list([current]))
    await settle()
    expect(api.updateUserTrading).toHaveBeenCalledTimes(1)
  })

  it('does not request private configuration after a pending opening is unmounted', async () => {
    const root = await ready()
    const permission = deferred<ReturnType<typeof list>>()
    api.users.mockReturnValueOnce(permission.promise)
    button(rowFor(root, current.username), '独立配置').props.onClick!()
    runtime.hooks!.unmount()
    permission.resolve(list([current]))
    await drain()
    expect(api.userTrading).not.toHaveBeenCalled()
    expectNoMutation()
    expect(runtime.showMessage).not.toHaveBeenCalled()
  })

  it('does not submit configuration after a pending save revalidation is unmounted', async () => {
    const editor = await edit(await ready())
    const permission = deferred<ReturnType<typeof list>>()
    api.users.mockReturnValueOnce(permission.promise)
    button(editor, '保存会员配置').props.onClick!()
    runtime.hooks!.unmount()
    permission.resolve(list([current]))
    await drain()
    expectNoMutation()
    expect(runtime.showMessage).not.toHaveBeenCalled()
  })
})

it('preserves admin member configuration without requiring workspace-only membership flags', async () => {
  role = 'admin'
  let root = await ready()
  expect(runtime.admin.users).toHaveBeenCalledExactlyOnceWith({ query: '', status: 'all', role: 'member', kind: 'member', page: 1, pageSize: 20 })
  expect(headers(root)).toContain('所属房间')
  expect(headers(root)).not.toContain('房间状态')
  expect(text(cellFor(root, rowFor(root, current.username), '所属房间'))).toContain('88008')
  const editor = await edit(root)
  expect(field(editor, '目标平台').props.value).toBe(config.external_follow.target_platform)
  button(editor, '保存会员配置').props.onClick!()
  root = await settle()
  expect(runtime.admin.userTrading).toHaveBeenCalledExactlyOnceWith(current.id)
  expect(runtime.admin.updateUserTrading).toHaveBeenCalledExactlyOnceWith(current.id, expect.objectContaining({ fly_mode: 'custom', fly_rate: 18.5, game_id: '', odds: [] }))
  expect(runtime.admin.users).toHaveBeenCalledTimes(1)
  expect(runtime.agent.users).not.toHaveBeenCalled()
  expect(runtime.tenant.users).not.toHaveBeenCalled()
  expect(dialog(root)).toBeDefined()
  expect(button(dialog(root), '已保存')).toBeDefined()
})
