import { Button, Dialog, Paper, Switch, TableBody, TableRow, Tabs, TextField } from '@mui/material'
import { isValidElement, type DependencyList, type ReactElement, type ReactNode, type SetStateAction } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { CleanupRunView, RetentionPolicyView } from '../api'
import { DataMaintenancePage } from './DataMaintenancePage'

type Slot = { value?: unknown; deps?: DependencyList; effect?: () => void | (() => void); cleanup?: void | (() => void) }
const sameDeps = (a?: DependencyList, b?: DependencyList) => Boolean(a && b && a.length === b.length && a.every((value, index) => Object.is(value, b[index])))
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
    if (!this.slots[index] || !sameDeps(this.slots[index].deps, deps)) this.slots[index] = { value: factory(), deps }
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
  showMessage: vi.fn(), dataMaintenanceSummary: vi.fn(), tenants: vi.fn(), agents: vi.fn(), retentionPolicies: vi.fn(), updateRetentionPolicy: vi.fn(),
  dataCleanupRuns: vi.fn(), previewDataCleanup: vi.fn(), executeDataCleanup: vi.fn(), dataCleanupRun: vi.fn(), restoreSoftDeleted: vi.fn(),
}))
vi.mock('react', async original => ({ ...await original<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useCallback: <T,>(callback: T, deps: DependencyList) => runtime.hooks!.useMemo(() => callback, deps),
  useEffect: (effect: () => void | (() => void), deps?: DependencyList) => runtime.hooks!.useEffect(effect, deps),
}))
vi.mock('../api', () => ({ adminApi: runtime }))
vi.mock('../components/feedback', () => ({ useFeedback: () => ({ showMessage: runtime.showMessage }) }))
vi.mock('../utils/requestId', () => ({ createRequestId: () => 'test-maintenance-request' }))

type Props = {
  children?: ReactNode; label?: ReactNode; helperText?: ReactNode; value?: unknown; open?: boolean; checked?: boolean; disabled?: boolean;
  onClick?: () => void; onChange?: (event: { target: { value: string; checked?: boolean } }, value?: string) => void;
  slotProps?: { htmlInput?: Record<string, unknown> }; items?: unknown[]; title?: string;
}
type Element = ReactElement<Props>
function elements(node: ReactNode): Element[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  if (!isValidElement<Props>(node)) return []
  return [node, ...(node.type === Dialog && !node.props.open ? [] : elements(node.props.children))]
}
function text(node: ReactNode): string {
  if (Array.isArray(node)) return node.map(text).join('')
  if (isValidElement<Props>(node)) return node.type === Dialog && !node.props.open ? '' : text(node.props.children)
  return typeof node === 'string' || typeof node === 'number' ? String(node) : ''
}
const button = (root: ReactNode, label: string) => elements(root).find(node => node.type === Button && text(node) === label)!
const field = (root: ReactNode, label: string) => elements(root).find(node => node.type === TextField && node.props.label === label)!
const dialog = (root: ReactNode) => elements(root).find(node => node.type === Dialog && node.props.open)
const policyRow = (root: ReactNode, label: string) => elements(root).find(node => node.type === TableRow && text(node).startsWith(label))!
const change = (node: Element, value: string) => node.props.onChange!({ target: { value } })
const render = () => { const root = runtime.hooks!.render(() => DataMaintenancePage()); runtime.hooks!.flushEffects(); return root }
const settle = async () => { for (let i = 0; i < 12; i++) await Promise.resolve(); return render() }
const load = async () => { render(); await vi.advanceTimersByTimeAsync(0); return render() }
const tab = (value: string) => { elements(render()).find(node => node.type === Tabs && ['policies', 'cleanup', 'runs'].includes(String(node.props.value)))!.props.onChange!({ target: { value: '' } }, value); return render() }
const policy = (changes: Partial<RetentionPolicyView> = {}): RetentionPolicyView => ({
  id: 1, workspace_id: 0, data_class: 'game_chat_messages', enabled: false, retention_days: 7, purge_after_days: 0,
  action: 'soft_delete', updated_by_id: 0, updated_by_name: '', created_at: null, updated_at: null, inherited: false, description: '游戏房聊天保留策略', ...changes,
})
const previewItem = { data_class: 'game_chat_messages' as const, action: 'soft_delete' as const, description: '游戏房聊天', enabled: true, retention_days: 7, cutoff_at: null, eligible_count: 20, planned_count: 10, protected_from_deletion: 3 }
const run = (changes: Partial<CleanupRunView> = {}): CleanupRunView => ({
  id: 1, request_id: 'game-chat-soft-test', workspace_id: 1, all_workspaces: false, delete_mode: 'soft', actor_id: 1, actor_name: '管理员',
  status: 'completed', batch_limit: 1000, preview: [previewItem], result: [{ data_class: 'game_chat_messages', action: 'soft_delete', affected_count: 10 }],
  soft_restore_result: [], financial_restore_result: [], content_purge_count: 0, created_at: null, ...changes,
})

beforeEach(() => {
  runtime.hooks = new PageHarness()
  for (const value of Object.values(runtime)) if (vi.isMockFunction(value)) value.mockReset()
  runtime.dataMaintenanceSummary.mockResolvedValue({ soft_deleted_chat_count: 2, soft_deleted_robot_chat_count: 3, soft_deleted_game_chat_count: 5, soft_deleted_notification_count: 7 })
  runtime.tenants.mockResolvedValue({ items: [], total: 0 })
  runtime.agents.mockResolvedValue({ items: [], total: 0 })
  runtime.retentionPolicies.mockResolvedValue([policy(), policy({ id: 2, data_class: 'chat_messages', retention_days: 30 })])
  runtime.updateRetentionPolicy.mockResolvedValue(policy())
  runtime.dataCleanupRuns.mockResolvedValue({ items: [], has_more: false })
  runtime.previewDataCleanup.mockResolvedValue({ request_id: 'game-chat-preview', all_workspaces: true, delete_mode: 'soft', items: [previewItem] })
  runtime.dataCleanupRun.mockResolvedValue(run())
  runtime.restoreSoftDeleted.mockResolvedValue({})
  vi.useFakeTimers()
  vi.stubGlobal('window', { setTimeout, clearTimeout })
})
afterEach(() => { runtime.hooks!.unmount(); vi.clearAllTimers(); vi.useRealTimers(); vi.unstubAllGlobals() })

describe('game-room chat maintenance policy', () => {
  it('includes game-room recycling in the summary and explains protected evidence and bounded scheduling', async () => {
    const root = await load()
    const card = elements(root).find(node => node.type === Paper && text(node).startsWith('回收站内容'))!
    expect(text(card)).toContain('回收站内容17')
    expect(text(card)).toContain('游戏房 5')
    for (const label of ['默认保留 7 天', '策略默认停用', '每小时第 10 分钟', '每批 1,000 条', '最多 5 批', '最多运行 2 分钟', '下注命令与回执', '不是清空整张聊天表']) expect(text(root)).toContain(label)
    expect(field(root, '回收站保留天数').props.value).toBe(0)
    expect(field(root, '回收站保留天数').props.slotProps?.htmlInput).toMatchObject({ min: 0, max: 3650, step: 1 })
    expect(elements(policyRow(root, '普通聊天（非游戏房）')).filter(node => node.type === TextField)).toHaveLength(1)
  })

  it('treats missing new fields from a legacy server as zero without enabling deletion', async () => {
    runtime.retentionPolicies.mockResolvedValue([policy({ purge_after_days: undefined })])
    runtime.dataMaintenanceSummary.mockResolvedValue({ soft_deleted_chat_count: 2, soft_deleted_robot_chat_count: 3, soft_deleted_notification_count: 7 })
    const root = await load()
    expect(field(root, '回收站保留天数').props.value).toBe(0)
    const card = elements(root).find(node => node.type === Paper && text(node).startsWith('回收站内容'))!
    expect(text(card)).toContain('回收站内容12')
    expect(text(card)).toContain('游戏房 0')
    expect(runtime.updateRetentionPolicy).not.toHaveBeenCalled()
  })

  it('enables recoverable game-chat cleanup with purge zero without a permanent-delete confirmation', async () => {
    const root = await load()
    elements(policyRow(root, '游戏房聊天')).find(node => node.type === Switch)!.props.onChange!({ target: { value: '', checked: true } })
    button(render(), '保存策略').props.onClick!()
    await settle()
    expect(runtime.updateRetentionPolicy).toHaveBeenCalledWith('game_chat_messages', { workspace_id: 0, enabled: true, retention_days: 7, purge_after_days: 0 })
    expect(dialog(render())).toBeUndefined()
    expect(runtime.executeDataCleanup).not.toHaveBeenCalled()
  })

  it('requires the exact typed phrase before enabling automatic permanent deletion and resets it after cancelling', async () => {
    runtime.retentionPolicies.mockResolvedValue([policy({ enabled: true })])
    await load()
    change(field(render(), '回收站保留天数'), '14')
    button(render(), '保存策略').props.onClick!()
    const confirmation = dialog(render())!
    expect(text(confirmation)).toContain('不可恢复')
    expect(text(confirmation)).toContain('影响继承该策略的房间')
    expect(text(confirmation)).toContain('回收站保留 14 天')
    expect(runtime.updateRetentionPolicy).not.toHaveBeenCalled()
    change(field(confirmation, '输入“永久删除”确认自动清理'), '执行')
    expect(button(dialog(render()), '确认启用自动永久清理').props.disabled).toBe(true)
    button(dialog(render()), '确认启用自动永久清理').props.onClick!()
    expect(runtime.updateRetentionPolicy).not.toHaveBeenCalled()
    change(field(dialog(render()), '输入“永久删除”确认自动清理'), '永久删除')
    button(dialog(render()), '取消').props.onClick!()
    expect(dialog(render())).toBeUndefined()
    button(render(), '保存策略').props.onClick!()
    expect(field(dialog(render()), '输入“永久删除”确认自动清理').props.value).toBe('')
    expect(runtime.updateRetentionPolicy).not.toHaveBeenCalled()
  })

  it('persists the confirmed policy directly once rather than reopening its confirmation', async () => {
    runtime.retentionPolicies.mockResolvedValue([policy({ enabled: true })])
    await load()
    change(field(render(), '回收站保留天数'), '7')
    button(render(), '保存策略').props.onClick!()
    change(field(dialog(render()), '输入“永久删除”确认自动清理'), '永久删除')
    expect(button(dialog(render()), '确认启用自动永久清理').props.disabled).toBe(false)
    button(dialog(render()), '确认启用自动永久清理').props.onClick!()
    await settle()
    expect(runtime.updateRetentionPolicy).toHaveBeenCalledExactlyOnceWith('game_chat_messages', { workspace_id: 0, enabled: true, retention_days: 7, purge_after_days: 7 })
    expect(dialog(render())).toBeUndefined()
    expect(runtime.executeDataCleanup).not.toHaveBeenCalled()
  })

  it('keeps an unsuccessful save in the confirmation dialog for a deliberate retry', async () => {
    runtime.retentionPolicies.mockResolvedValue([policy({ enabled: true })])
    runtime.updateRetentionPolicy.mockRejectedValueOnce(new Error('保存失败'))
    await load()
    change(field(render(), '回收站保留天数'), '7')
    button(render(), '保存策略').props.onClick!()
    change(field(dialog(render()), '输入“永久删除”确认自动清理'), '永久删除')
    button(dialog(render()), '确认启用自动永久清理').props.onClick!()
    await settle()
    expect(text(dialog(render()))).toContain('保存失败')
    button(dialog(render()), '确认启用自动永久清理').props.onClick!()
    await settle()
    expect(runtime.updateRetentionPolicy).toHaveBeenCalledTimes(2)
    expect(dialog(render())).toBeUndefined()
  })

  it.each([['14', 14], ['-1', 0], ['4000', 3650], ['7.6', 8]])('normalizes disabled-policy purge input %s to %i without enabling it', async (input, expected) => {
    await load()
    change(field(render(), '回收站保留天数'), input)
    button(render(), '保存策略').props.onClick!()
    await settle()
    expect(runtime.updateRetentionPolicy).toHaveBeenCalledWith('game_chat_messages', { workspace_id: 0, enabled: false, retention_days: 7, purge_after_days: expected })
    expect(dialog(render())).toBeUndefined()
  })

  it('never sends the new purge field when saving other retention classes', async () => {
    await load()
    const row = policyRow(render(), '普通聊天（非游戏房）')
    change(elements(row).find(node => node.type === TextField)!, '40')
    button(render(), '保存策略').props.onClick!()
    await settle()
    expect(runtime.updateRetentionPolicy).toHaveBeenCalledExactlyOnceWith('chat_messages', { workspace_id: 0, enabled: false, retention_days: 40 })
  })
})

describe('game-room cleanup preview and recovery', () => {
  it.each([['日常内容清理', 'soft'], ['永久清理回收站', 'hard']])('includes game-room chat in the %s preset without executing it', async (label, mode) => {
    await load()
    tab('cleanup')
    button(render(), label).props.onClick!()
    change(field(render(), '维护范围'), 'all')
    button(render(), '生成预览').props.onClick!()
    await settle()
    expect(runtime.previewDataCleanup).toHaveBeenCalledWith(expect.objectContaining({ all_workspaces: true, delete_mode: mode,
      data_classes: ['chat_messages', 'robot_chat_messages', 'game_chat_messages', 'notifications'] }))
    expect(runtime.executeDataCleanup).not.toHaveBeenCalled()
    const table = elements(render()).find(node => typeof node.type === 'function' && node.type.name === 'ItemTable')!
    expect(table.props.items).toEqual([previewItem])
    const tableBody = (table.type as (props: Props) => ReactNode)(table.props)
    expect(text(tableBody)).toContain('游戏房聊天')
  })

  it('exposes recovery for completed soft-deleted game-room chat and only restores on explicit confirmation', async () => {
    runtime.dataCleanupRuns.mockResolvedValue({ items: [run()], has_more: false })
    await load()
    tab('runs')
    button(render(), '详情').props.onClick!()
    await settle()
    button(dialog(render()), '恢复软删除').props.onClick!()
    expect(runtime.restoreSoftDeleted).not.toHaveBeenCalled()
    const restoreDialog = elements(render()).find(node => node.type === Dialog && node.props.open && text(node).startsWith('确认恢复数据'))!
    expect(text(restoreDialog)).toContain('游戏房聊天')
    button(restoreDialog, '确认恢复').props.onClick!()
    await settle()
    expect(runtime.restoreSoftDeleted).toHaveBeenCalledExactlyOnceWith('game-chat-soft-test')
  })

  it.each([{ delete_mode: 'hard' as const }, { content_purge_count: 1 }, { content_purged_at: '2026-08-31T09:00:00Z' }])('does not offer a misleading restore for permanently purged game-room chat (%j)', async changes => {
    runtime.dataCleanupRuns.mockResolvedValue({ items: [run(changes)], has_more: false })
    runtime.dataCleanupRun.mockResolvedValue(run(changes))
    await load()
    tab('runs')
    expect(elements(render()).some(node => node.type === TableBody)).toBe(true)
    button(render(), '详情').props.onClick!()
    await settle()
    expect(button(dialog(render()), '恢复软删除')).toBeUndefined()
  })
})
