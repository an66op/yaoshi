import { Alert, Avatar, Button, Chip, Dialog, DialogTitle, Switch, TableBody, TableCell, TableHead, TablePagination, TableRow, TextField } from '@mui/material'
import { isValidElement, type DependencyList, type ReactElement, type ReactNode, type SetStateAction } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentItem } from '../api'
import { WorkspaceAdminAccountFields, WorkspaceAdminCreatedDialog } from '../components/WorkspaceAdminAccount'
import { TenantWorkspacePage } from './TenantWorkspacePage'

// Exercise real page handlers without a browser or rendering MUI internals.
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
  dashboard: vi.fn(), agents: vi.fn(), createAgent: vi.fn(), updateAgent: vi.fn(), resetAgentPassword: vi.fn(),
  showMessage: vi.fn(), prepareRoomLogo: vi.fn(),
}))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useCallback: <T,>(callback: T, deps: DependencyList) => runtime.hooks!.useCallback(callback, deps),
  useEffect: (effect: () => void | (() => void), deps?: DependencyList) => runtime.hooks!.useEffect(effect, deps),
}))
vi.mock('../api', () => ({ tenantApi: {
  dashboard: runtime.dashboard, agents: runtime.agents, createAgent: runtime.createAgent,
  updateAgent: runtime.updateAgent, resetAgentPassword: runtime.resetAgentPassword,
} }))
vi.mock('../components/feedback', () => ({ useFeedback: () => ({ showMessage: runtime.showMessage }) }))
vi.mock('../utils/roomLogo', () => ({ prepareRoomLogo: runtime.prepareRoomLogo }))

type Change = { target: { value: string; checked?: boolean; files?: File[] } }
type ElementProps = {
  children?: ReactNode; actions?: ReactNode; action?: ReactNode
  label?: string; placeholder?: string; helperText?: string; type?: string; value?: string | number; src?: string
  open?: boolean; disabled?: boolean; editing?: boolean; checked?: boolean
  password?: string; username?: string; severity?: string; variant?: string
  page?: number; rowsPerPage?: number; count?: number; rowsPerPageOptions?: number[]
  account?: { role: string; username: string; roomCode: string; status: number } | null
  onClick?: () => void; onClose?: () => void; onChange?: (event: Change) => void
  onPageChange?: (event: null, page: number) => void; onRowsPerPageChange?: (event: Change) => void
  onUsernameChange?: (value: string) => void; onPasswordChange?: (value: string) => void
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
const ofType = (node: ReactNode, type: unknown) => elements(node).filter(element => element.type === type)
const button = (node: ReactNode, label: string) => ofType(node, Button).find(element => text(element) === label)!
const field = (node: ReactNode, label: string) => ofType(node, TextField).find(element => element.props.label === label)!
const accountFields = (node: ReactNode) => ofType(node, WorkspaceAdminAccountFields)[0]
const editDialog = (node: ReactNode) => ofType(node, Dialog).find(element => Boolean(accountFields(element)))!
const passwordDialog = (node: ReactNode) => ofType(node, Dialog).find(element => Boolean(field(element, '新密码')))!
const body = (node: ReactNode) => ofType(node, TableBody)[0]
const rows = (node: ReactNode) => ofType(body(node), TableRow)
const pagination = (node: ReactNode) => ofType(node, TablePagination)[0]
const search = (node: ReactNode) => ofType(node, TextField).find(element => element.props.placeholder?.startsWith('搜索'))!
const change = (node: Element, value: string) => node.props.onChange!({ target: { value } })
const deferred = <T,>() => {
  let resolve!: (value: T) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail })
  return { promise, resolve, reject }
}
const agent: AgentItem = {
  id: 42, public_id: 600321, username: 'agent-river', nickname: '江河代理', email: 'river@example.test', phone: '13800000000',
  room_code: '778899', room_name: '江河客厅', room_logo: 'data:image/png;base64,existing-logo', workspace_id: 120,
  balance: 500, status: 1, member_count: 18, rebate_rate: 1.5, profit_share_rate: 12.5, remark: '重点联系',
  created_at: '2026-08-20 11:00:00', last_login_at: '2026-08-30 20:30:00', login_count: 7,
}

describe('tenant agent account management', () => {
  const render = (section: 'agents' | 'dashboard' = 'agents') => {
    const root = runtime.hooks!.render(() => TenantWorkspacePage({ section }))
    runtime.hooks!.flushEffects()
    return root
  }
  const settle = async () => {
    for (let index = 0; index < 12; index++) await Promise.resolve()
    return render()
  }
  const ready = async (section: 'agents' | 'dashboard' = 'agents') => {
    render(section)
    await vi.runOnlyPendingTimersAsync()
    return render(section)
  }
  const openEdit = async () => { button(rows(await ready())[0], '编辑账号').props.onClick!(); return render() }
  const openReset = async () => { button(rows(await ready())[0], '重置密码').props.onClick!(); return render() }

  beforeEach(() => {
    runtime.hooks = new PageHarness()
    for (const value of Object.values(runtime)) if (vi.isMockFunction(value)) value.mockReset()
    runtime.dashboard.mockResolvedValue({ agent_count: 61, active_agent_count: 52, member_count: 180 })
    runtime.agents.mockResolvedValue({ items: [agent], total: 61 })
    runtime.updateAgent.mockResolvedValue(agent)
    runtime.resetAgentPassword.mockResolvedValue({ id: agent.id })
    vi.useFakeTimers()
    vi.stubGlobal('window', { location: { origin: 'http://127.0.0.1:5174' }, setTimeout, clearTimeout })
  })
  afterEach(() => {
    runtime.hooks!.unmount()
    vi.clearAllTimers(); vi.useRealTimers(); vi.unstubAllGlobals()
  })

  it('puts account identity, contact details, status, login and share before secondary room information', async () => {
    const root = await ready()
    expect(ofType(ofType(root, TableHead)[0], TableCell).map(text)).toEqual(['代理账号', '联系方式', '账号状态', '上次登录', '分成率', '关联房间', '账号操作'])
    const cells = ofType(rows(root)[0], TableCell)
    expect(text(cells[0])).toBe('江河代理@agent-river公开 ID 600321')
    expect(text(cells[1])).toContain('13800000000river@example.test')
    expect(ofType(cells[2], Chip)[0].props.label).toBe('正常')
    expect(text(cells[3])).toBe('2026-08-30 20:30:00累计登录 7 次')
    expect(text(cells[4])).toBe('12.5%')
    expect(text(cells[5])).toContain('江河客厅房间号 77889918 位会员 · 返水 1.5%')
    expect(ofType(cells[5], Avatar)[0].props.src).toBe(agent.room_logo)
    expect(text(cells[6])).toBe('编辑账号重置密码')
    expect(button(root, '开通代理账号')).toBeDefined()
    expect(text(root)).not.toMatch(/开通代理房间及管理员账号|房间管理员/)
  })

  it('shows explicit missing-account-data fallbacks without exposing the internal user ID', async () => {
    runtime.agents.mockResolvedValue({ items: [{ ...agent, nickname: '', phone: '', email: '', public_id: 0, status: 0, last_login_at: '', login_count: 0 }], total: 1 })
    const cells = ofType(rows(await ready())[0], TableCell)
    expect(text(cells[0])).toBe('agent-river@agent-river公开 ID 未分配')
    expect(text(cells[1])).toBe('未填写手机号未填写邮箱')
    expect(ofType(cells[2], Chip)[0].props.label).toBe('停用')
    expect(text(cells[3])).toBe('尚未登录累计登录 0 次')
  })

  it('keeps every account, share and room configuration in the account-focused edit form', async () => {
    const root = await openEdit()
    const dialog = editDialog(root)
    expect(text(ofType(dialog, DialogTitle)[0])).toBe('编辑代理账号 · @agent-river')
    expect(accountFields(dialog).props).toMatchObject({ editing: true, username: agent.username, password: '' })
    expect(text(dialog).indexOf('账号资料')).toBeLessThan(text(dialog).indexOf('关联房间资料'))
    for (const [label, value] of Object.entries({ '代理昵称': agent.nickname, '手机号': agent.phone, '邮箱': agent.email, '备注': agent.remark, '代理分成 %': agent.profit_share_rate, '房间号': agent.room_code, '房间名称': agent.room_name, '房间返水 %': agent.rebate_rate })) {
      expect(field(dialog, label).props.value).toBe(value)
    }
    expect(ofType(dialog, Switch)[0].props.checked).toBe(true)
    expect(ofType(dialog, Avatar)[0].props.src).toBe(agent.room_logo)
    expect(field(dialog, '代理分成 %').props.helperText).toBe('逐注正毛利 × 比例，亏损注不抵扣；手动结算')
    change(field(dialog, '代理昵称'), '新昵称')
    change(field(dialog, '手机号'), '13900000000')
    change(field(dialog, '邮箱'), 'updated@example.test')
    change(field(dialog, '代理分成 %'), '15')
    ofType(dialog, Switch)[0].props.onChange!({ target: { value: '', checked: false } })
    button(editDialog(render()), '保存修改').props.onClick!()
    const finished = await settle()
    expect(runtime.updateAgent).toHaveBeenCalledExactlyOnceWith(agent.id, {
      username: agent.username, password: '', nickname: '新昵称', phone: '13900000000', email: 'updated@example.test',
      remark: agent.remark, profit_share_rate: 15, rebate_rate: agent.rebate_rate, status: 0,
      room_code: agent.room_code, room_name: agent.room_name, room_logo: agent.room_logo,
    })
    expect(runtime.createAgent).not.toHaveBeenCalled()
    expect(editDialog(finished).props.open).toBe(false)
    expect(runtime.showMessage).toHaveBeenCalledWith('代理账号资料已保存')
  })

  it('preserves room code validation and keeps an invalid edit visible', async () => {
    const root = await openEdit()
    change(field(root, '房间号'), 'abc12-34')
    const changed = render()
    expect(field(changed, '房间号').props.value).toBe('1234')
    button(editDialog(changed), '保存修改').props.onClick!()
    const rejected = await settle()
    expect(runtime.updateAgent).not.toHaveBeenCalled()
    expect(editDialog(rejected).props.open).toBe(true)
    expect(ofType(editDialog(rejected), Alert).map(text).join('')).toContain('5–12 位数字')
  })

  it('retains logo replacement and removal alongside the room settings', async () => {
    const root = await openEdit()
    runtime.prepareRoomLogo.mockResolvedValue('data:image/png;base64,new-logo')
    const file = { name: 'room.png' } as File
    const input = elements(editDialog(root)).find(element => element.type === 'input' && element.props.type === 'file')!
    const event = { target: { files: [file], value: 'room.png' } }
    input.props.onChange!(event)
    let changed = await settle()
    expect(runtime.prepareRoomLogo).toHaveBeenCalledExactlyOnceWith(file)
    expect(event.target.value).toBe('')
    expect(ofType(editDialog(changed), Avatar)[0].props.src).toBe('data:image/png;base64,new-logo')
    button(editDialog(changed), '移除').props.onClick!()
    changed = render()
    expect(ofType(editDialog(changed), Avatar)[0].props.src).toBeUndefined()
    expect(button(editDialog(changed), '选择 Logo')).toBeDefined()
    button(editDialog(changed), '保存修改').props.onClick!()
    await settle()
    expect(runtime.updateAgent).toHaveBeenCalledWith(agent.id, expect.objectContaining({ room_logo: '', rebate_rate: 1.5, profit_share_rate: 12.5 }))
  })

  it('keeps API errors and every edited value in the form, with no false success', async () => {
    const root = await openEdit()
    runtime.updateAgent.mockRejectedValue(new Error('账号资料保存失败，请重试'))
    change(field(root, '代理昵称'), '保留输入')
    button(editDialog(render()), '保存修改').props.onClick!()
    const failed = await settle()
    expect(editDialog(failed).props.open).toBe(true)
    expect(field(failed, '代理昵称').props.value).toBe('保留输入')
    expect(ofType(editDialog(failed), Alert).map(text).join('')).toContain('账号资料保存失败，请重试')
    expect(runtime.showMessage).not.toHaveBeenCalled()
  })

  it('shows the exact password target and locks duplicate reset and closing while pending', async () => {
    const pending = deferred<{ id: number }>()
    runtime.resetAgentPassword.mockReturnValue(pending.promise)
    const root = await openReset()
    expect(text(passwordDialog(root))).toContain('江河代理@agent-river公开 ID 600321')
    change(field(passwordDialog(root), '新密码'), 'new-password-123')
    const action = button(passwordDialog(render()), '确认重置')
    action.props.onClick!(); action.props.onClick!()
    let waiting = render()
    expect(runtime.resetAgentPassword).toHaveBeenCalledExactlyOnceWith(agent.id, 'new-password-123')
    expect(button(passwordDialog(waiting), '重置中…').props.disabled).toBe(true)
    expect(field(passwordDialog(waiting), '新密码').props.disabled).toBe(true)
    expect(button(passwordDialog(waiting), '取消').props.disabled).toBe(true)
    passwordDialog(waiting).props.onClose!()
    button(passwordDialog(waiting), '取消').props.onClick!()
    waiting = render()
    expect(passwordDialog(waiting).props.open).toBe(true)
    pending.resolve({ id: agent.id })
    const finished = await settle()
    expect(passwordDialog(finished).props.open).toBe(false)
    expect(field(passwordDialog(finished), '新密码').props.value).toBe('')
    expect(runtime.showMessage).toHaveBeenCalledWith('代理账号 @agent-river 的密码已重置')
  })

  it('keeps a failed password reset target visible and allows a deliberate retry', async () => {
    runtime.resetAgentPassword.mockRejectedValueOnce(new Error('密码服务暂不可用'))
    const root = await openReset()
    change(field(passwordDialog(root), '新密码'), 'new-password-123')
    button(passwordDialog(render()), '确认重置').props.onClick!()
    const failed = await settle()
    expect(passwordDialog(failed).props.open).toBe(true)
    expect(text(passwordDialog(failed))).toContain('@agent-river')
    expect(ofType(passwordDialog(failed), Alert).map(text).join('')).toContain('密码服务暂不可用')
    expect(field(passwordDialog(failed), '新密码').props.value).toBe('new-password-123')
    expect(button(passwordDialog(failed), '确认重置').props.disabled).toBe(false)
    expect(runtime.showMessage).not.toHaveBeenCalled()
    button(passwordDialog(failed), '确认重置').props.onClick!()
    expect(passwordDialog(await settle()).props.open).toBe(false)
    expect(runtime.resetAgentPassword).toHaveBeenCalledTimes(2)
  })

  it('discards a cancelled password and rejects too few characters or too many UTF-8 bytes', async () => {
    const root = await openReset()
    for (const invalid of ['short', '🙂'.repeat(4), '密'.repeat(25)]) {
      change(field(passwordDialog(root), '新密码'), invalid)
      const changed = render()
      expect(button(passwordDialog(changed), '确认重置').props.disabled).toBe(true)
      button(passwordDialog(changed), '确认重置').props.onClick!()
      expect(runtime.resetAgentPassword).not.toHaveBeenCalled()
    }
    change(field(passwordDialog(render()), '新密码'), '密'.repeat(8))
    expect(button(passwordDialog(render()), '确认重置').props.disabled).toBe(false)
    button(passwordDialog(render()), '取消').props.onClick!()
    expect(field(passwordDialog(render()), '新密码').props.value).toBe('')
    button(rows(render())[0], '重置密码').props.onClick!()
    const reopened = render()
    expect(field(passwordDialog(reopened), '新密码').props.value).toBe('')
    expect(ofType(passwordDialog(reopened), Alert)).toHaveLength(0)
  })

  it('uses supported page sizes, reaches later accounts, and resets page on a search or size change', async () => {
    const root = await ready()
    expect(runtime.agents).toHaveBeenLastCalledWith({ query: '', page: 1, pageSize: 20 })
    expect(pagination(root).props).toMatchObject({ page: 0, count: 61, rowsPerPage: 20, rowsPerPageOptions: [20, 50, 100] })
    pagination(root).props.onPageChange!(null, 2)
    await ready()
    expect(runtime.agents).toHaveBeenLastCalledWith({ query: '', page: 3, pageSize: 20 })
    change(search(render()), 'river')
    await ready()
    expect(runtime.agents).toHaveBeenLastCalledWith({ query: 'river', page: 1, pageSize: 20 })
    pagination(render()).props.onPageChange!(null, 1)
    await ready()
    pagination(render()).props.onRowsPerPageChange!({ target: { value: '50' } })
    await ready()
    expect(runtime.agents).toHaveBeenLastCalledWith({ query: 'river', page: 1, pageSize: 50 })
    expect(pagination(render()).props.page).toBe(0)
  })

  it('does not let a stale list response overwrite a newer search', async () => {
    const old = deferred<{ items: AgentItem[]; total: number }>()
    runtime.agents.mockReturnValueOnce(old.promise)
    render()
    await vi.runOnlyPendingTimersAsync()
    change(search(render()), 'new-agent')
    const newest = { ...agent, username: 'new-agent', nickname: '新的代理' }
    runtime.agents.mockResolvedValue({ items: [newest], total: 1 })
    await ready()
    expect(text(body(render()))).toContain('@new-agent')
    old.resolve({ items: [agent], total: 61 })
    const final = await settle()
    expect(text(body(final))).toContain('@new-agent')
    expect(text(body(final))).not.toContain('@agent-river')
    expect(pagination(final).props.count).toBe(1)
  })

  it('distinguishes an empty list from a search with no matches', async () => {
    runtime.agents.mockResolvedValue({ items: [], total: 0 })
    expect(text(body(await ready()))).toContain('还没有代理账号，点击“开通代理账号”开始。')
    change(search(render()), 'missing')
    expect(text(body(await ready()))).toBe('没有找到匹配的代理账号')
  })

  it('describes dashboard entries as subordinate agents instead of the tenants own rooms', async () => {
    const root = await ready('dashboard')
    expect(text(root)).toContain('代理账号总数61正常代理账号52')
    expect(text(root)).toContain('下级代理')
    expect(text(root)).not.toContain('我的房间')
    expect(ofType(root, Chip)[0].props.label).toBe('江河代理 · @agent-river')
    expect(text(root)).toContain('全部 61 个账号可在“代理管理”查看')
  })

  it('keeps creation account-centric and returns only sanitized confirmation data', async () => {
    const root = await ready()
    button(root, '开通代理账号').props.onClick!()
    const dialog = editDialog(render())
    expect(text(ofType(dialog, DialogTitle)[0])).toBe('开通代理账号')
    accountFields(dialog).props.onUsernameChange!(' new-agent ')
    accountFields(dialog).props.onPasswordChange!('private-password')
    change(field(dialog, '房间号'), '556677')
    runtime.createAgent.mockResolvedValue({ username: 'api-agent', room_code: '556677', status: 1, password: 'server-private' })
    button(editDialog(render()), '开通代理账号').props.onClick!()
    const finished = await settle()
    expect(runtime.createAgent).toHaveBeenCalledWith(expect.objectContaining({ username: 'new-agent', password: 'private-password', room_code: '556677' }))
    expect(ofType(finished, WorkspaceAdminCreatedDialog)[0].props.account).toEqual({ role: 'agent', username: 'api-agent', roomCode: '556677', status: 1 })
    expect(accountFields(finished).props.password).toBe('')
    expect(runtime.showMessage).toHaveBeenCalledWith('代理账号已创建')
  })
})
