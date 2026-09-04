import { Button, Chip, Dialog, Paper, Switch, TextField, Typography } from '@mui/material'
import { isValidElement, type DependencyList, type ReactElement, type ReactNode, type SetStateAction } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AdminGame, WorkspaceGame } from '../api'
import { GameOddsNavigation } from '../components/OddsEditors'
import { ManagementPage } from './ManagementPages'
import { WorkspaceGamesPage } from './WorkspaceGamesPage'

// Exercise the real component trees and handlers without a browser or live writes.
type Slot = { value?: unknown; deps?: DependencyList; effect?: () => void | (() => void); cleanup?: void | (() => void) }
const sameDeps = (left?: DependencyList, right?: DependencyList) => Boolean(left && right && left.length === right.length && left.every((value, index) => Object.is(value, right[index])))
class PageHarness {
  private slots: Slot[] = []
  private cursor = 0
  private effects = new Set<number>()
  render(factory: () => ReactNode) { this.cursor = 0; return factory() }
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
      this.slots[index] = { ...previous, effect, deps }
      this.effects.add(index)
    }
  }
  flushEffects() {
    for (const index of this.effects) {
      this.slots[index].cleanup?.()
      this.slots[index].cleanup = this.slots[index].effect?.()
    }
    this.effects.clear()
  }
  unmount() { this.effects.clear(); for (const slot of this.slots) slot.cleanup?.() }
}

const runtime = vi.hoisted(() => ({
  hooks: null as PageHarness | null,
  role: 'tenant' as 'tenant' | 'agent',
  adminItems: [] as AdminGame[],
  workspaceItems: [] as WorkspaceGame[],
  adminGames: vi.fn(), entertainment: vi.fn(), gameCategories: vi.fn(), updateGameStatus: vi.fn(), assignGameCategory: vi.fn(),
  tenantGames: vi.fn(), tenantStatus: vi.fn(), agentGames: vi.fn(), agentStatus: vi.fn(), showMessage: vi.fn(),
}))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useMemo: <T,>(factory: () => T, deps: DependencyList) => runtime.hooks!.useMemo(factory, deps),
  useCallback: <T,>(callback: T, deps: DependencyList) => runtime.hooks!.useMemo(() => callback, deps),
  useEffect: (effect: () => void | (() => void), deps?: DependencyList) => runtime.hooks!.useEffect(effect, deps),
}))
vi.mock('../api', () => ({
  adminApi: {
    games: runtime.adminGames, entertainment: runtime.entertainment, gameCategories: runtime.gameCategories,
    updateGameStatus: runtime.updateGameStatus, assignGameCategory: runtime.assignGameCategory,
  },
  tenantApi: { games: runtime.tenantGames, setGameStatus: runtime.tenantStatus },
  agentApi: { games: runtime.agentGames, setGameStatus: runtime.agentStatus },
  resolveApiAsset: (path: string) => path,
}))
vi.mock('../auth', () => ({ getStoredUser: () => ({ role: runtime.role }) }))
vi.mock('../components/feedback', () => ({ useFeedback: () => ({ showMessage: runtime.showMessage }) }))

type Props = {
  children?: ReactNode; actions?: ReactNode; label?: ReactNode; title?: ReactNode; open?: boolean; checked?: boolean; disabled?: boolean
  value?: unknown; variant?: string; inputProps?: { 'aria-label'?: string }
  onClick?: () => void; onChange?: (event: { target: { checked: boolean; value: string } }) => void
}
type Element = ReactElement<Props>
function elements(node: ReactNode): Element[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  if (!isValidElement<Props>(node) || (node.type === Dialog && !node.props.open)) return []
  return [node, ...elements(node.props.children), ...elements(node.props.actions)]
}
function text(node: ReactNode): string {
  if (Array.isArray(node)) return node.map(text).join('')
  if (isValidElement<Props>(node)) return node.type === Dialog && !node.props.open ? '' : text(node.props.children)
  return typeof node === 'string' || typeof node === 'number' ? String(node) : ''
}
const ofType = (node: ReactNode, type: unknown) => elements(node).filter(element => element.type === type)
const button = (node: ReactNode, label: string) => ofType(node, Button).find(element => text(element) === label)!
const switchFor = (node: ReactNode, label: string) => ofType(node, Switch).find(element => element.props.inputProps?.['aria-label'] === label)!
const cardFor = (node: ReactNode, label: string) => ofType(node, Paper).findLast(element => Boolean(switchFor(element, label)))!
const captions = (node: ReactNode) => ofType(node, Typography).map(text)
const chips = (node: ReactNode) => ofType(node, Chip).map(element => element.props.label)
const displayText = (node: ReactNode) => [text(node), ...elements(node).flatMap(element => [text(element.props.label), text(element.props.title)])].join(' ')
const managementPage = () => (ManagementPage({ path: '/entertainment' }).type as () => ReactNode)()
const render = (factory: () => ReactNode) => {
  const root = runtime.hooks!.render(factory)
  runtime.hooks!.flushEffects()
  return root
}
const ready = async (factory: () => ReactNode) => {
  render(factory)
  await vi.runOnlyPendingTimersAsync()
  return render(factory)
}
const settle = async (factory: () => ReactNode) => {
  for (let index = 0; index < 12; index++) await Promise.resolve()
  return render(factory)
}
const fixture = (index: number, sourceKind: string): AdminGame => ({
  id: `internal-game-${index}`, code: `internal-code-${index}`, name: `测试彩种${index}`, category: 'racing-internal',
  lobby_category: '彩票', lobby_sort_order: index, badge: '', badge_color: '', enabled: true,
  issue: '12345', next_draw_at: '2026-09-04T05:00:00Z', turnover: 0, profit: 0,
  source_kind: sourceKind, source_name: 'raw-provider-10058', source_url: 'https://provider.example/raw-feed',
  sync_status: index === 1 ? 'error' : index === 2 ? 'ok' : 'idle',
  last_sync_at: index === 2 ? '2026-09-04T03:45:06Z' : null,
  last_sync_error: index === 1 ? 'raw-provider timeout internal-10058' : '', schedule_mode: 'official-feed',
})
function expectBusinessOnly(card: ReactNode, game: AdminGame) {
  expect(card).toBeTruthy()
  expect(captions(card)).toContain(game.name)
  expect(captions(card)).toContain(game.lobby_category?.trim() || '未分类')
  const shown = displayText(card)
  for (const forbidden of ['官方源', '外部源', '平台彩', '等待同步', '开奖源异常', game.id, game.code, game.category, game.source_name, game.source_url]) expect(shown).not.toContain(forbidden)
  if (game.last_sync_error) expect(shown).not.toContain(game.last_sync_error)
  if (game.last_sync_at) {
    expect(shown).not.toContain(game.last_sync_at)
    expect(shown).not.toContain(new Date(game.last_sync_at).toLocaleString('zh-CN', { hour12: false, month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }))
  }
}

beforeEach(() => {
  runtime.hooks = new PageHarness()
  runtime.role = 'tenant'
  for (const value of Object.values(runtime)) if (vi.isMockFunction(value)) value.mockReset()
  runtime.adminItems = ['external', 'official', 'simulated'].map((kind, index) => fixture(index + 1, kind))
  runtime.workspaceItems = runtime.adminItems.map(game => ({ ...game, platform_enabled: true, room_enabled: true }))
  runtime.adminGames.mockImplementation(async () => runtime.adminItems)
  runtime.entertainment.mockResolvedValue([])
  runtime.gameCategories.mockResolvedValue([{ id: 1, name: '彩票', sort_order: 1, game_count: 3, enabled_game_count: 3 }])
  runtime.updateGameStatus.mockImplementation(async (id: string, enabled: boolean) => {
    runtime.adminItems = runtime.adminItems.map(game => game.id === id ? { ...game, enabled } : game)
    return runtime.adminItems.find(game => game.id === id)!
  })
  runtime.assignGameCategory.mockResolvedValue({})
  for (const games of [runtime.tenantGames, runtime.agentGames]) games.mockImplementation(async () => runtime.workspaceItems)
  for (const status of [runtime.tenantStatus, runtime.agentStatus]) status.mockImplementation(async (id: string, enabled: boolean) => {
    runtime.workspaceItems = runtime.workspaceItems.map(game => game.id === id ? { ...game, enabled, room_enabled: enabled } : game)
    return runtime.workspaceItems.find(game => game.id === id)!
  })
  vi.useFakeTimers()
  vi.stubGlobal('window', { setTimeout, clearTimeout })
})
afterEach(() => {
  runtime.hooks!.unmount()
  vi.clearAllTimers()
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('business-facing game lists', () => {
  it.each(['tenant', 'agent'] as const)('%s cards hide source diagnostics and IDs while preserving room controls', async role => {
    runtime.role = role
    const root = await ready(WorkspaceGamesPage)
    for (const game of runtime.workspaceItems) {
      const card = cardFor(root, `${game.name}房间开关`)
      expectBusinessOnly(card, game)
      expect(chips(card)).toContain('房间已开放')
      expect(switchFor(card, `${game.name}房间开关`).props).toMatchObject({ checked: true, disabled: false })
    }
    const first = runtime.workspaceItems[0]
    switchFor(root, `${first.name}房间开关`).props.onChange!({ target: { checked: false, value: '' } })
    const changed = await settle(WorkspaceGamesPage)
    const expectedApi = role === 'tenant' ? runtime.tenantStatus : runtime.agentStatus
    const otherApi = role === 'tenant' ? runtime.agentStatus : runtime.tenantStatus
    expect(expectedApi).toHaveBeenCalledExactlyOnceWith(first.id, false)
    expect(otherApi).not.toHaveBeenCalled()
    expect(chips(cardFor(changed, `${first.name}房间开关`))).toContain('房间已关闭')
    expect(switchFor(changed, `${first.name}房间开关`).props.checked).toBe(false)
  })

  it('retains platform closure, unclassified and room-closed availability instead of replacing them with source health', async () => {
    runtime.workspaceItems = runtime.workspaceItems.map((game, index) => ({
      ...game, room_enabled: false, enabled: false, platform_enabled: index !== 0, lobby_category: index === 1 ? '' : game.lobby_category,
    }))
    const root = await ready(WorkspaceGamesPage)
    const expected = ['平台已关闭', '未上架 · 待平台分类', '房间已关闭']
    runtime.workspaceItems.forEach((game, index) => {
      const card = cardFor(root, `${game.name}房间开关`)
      expectBusinessOnly(card, game)
      expect(chips(card)).toContain(expected[index])
      expect(switchFor(card, `${game.name}房间开关`).props.disabled).toBe(index !== 2)
    })
  })

  it('platform game cards show business status/category/order without successful, failed or idle sync details', async () => {
    const root = await ready(managementPage)
    for (const game of runtime.adminItems) {
      const card = cardFor(root, `${game.name}状态`)
      expectBusinessOnly(card, game)
      expect(captions(card)).toContain('已开放')
      expect(captions(card)).toContain(`排序 ${game.lobby_sort_order}`)
      expect(button(card, '归类')).toBeTruthy()
      expect(switchFor(card, `${game.name}状态`).props).toMatchObject({ checked: true, disabled: false })
    }
  })

  it('platform enablement still targets the same game and updates its business status', async () => {
    let root: ReactNode = await ready(managementPage)
    button(root, '全部 3').props.onClick!()
    root = render(managementPage)
    const first = runtime.adminItems[0]
    switchFor(root, `${first.name}状态`).props.onChange!({ target: { checked: false, value: '' } })
    root = await settle(managementPage)
    expect(runtime.updateGameStatus).toHaveBeenCalledExactlyOnceWith(first.id, false)
    const card = cardFor(root, `${first.name}状态`)
    expectBusinessOnly(card, first)
    expect(captions(card)).toContain('已停用')
    expect(chips(card)).toContain('停用')
    expect(switchFor(card, `${first.name}状态`).props.checked).toBe(false)
  })

  it('classification remains editable and saves the internal ID without displaying it on the card', async () => {
    let root: ReactNode = await ready(managementPage)
    const first = runtime.adminItems[0]
    button(cardFor(root, `${first.name}状态`), '归类').props.onClick!()
    root = render(managementPage)
    expect(ofType(root, Dialog).some(dialog => text(dialog).includes(first.name))).toBe(true)
    const order = ofType(root, TextField).find(element => element.props.label === '分类内排序')!
    order.props.onChange!({ target: { checked: false, value: '8' } })
    root = render(managementPage)
    button(root, '保存归类').props.onClick!()
    await settle(managementPage)
    expect(runtime.assignGameCategory).toHaveBeenCalledExactlyOnceWith(first.id, { category: '彩票', sort_order: 8 })
  })

  it('odds game picker uses the business category for all source types and preserves game selection', () => {
    const stopped = { ...fixture(4, 'external'), name: '已停用测试', enabled: false }
    const onSelect = vi.fn()
    const root = render(() => GameOddsNavigation({ games: [...runtime.adminItems, stopped], gameId: runtime.adminItems[0].id, onSelect }))
    for (const game of runtime.adminItems) {
      const choice = ofType(root, Button).find(element => captions(element).includes(game.name))!
      expectBusinessOnly(choice, game)
      choice.props.onClick!()
    }
    expect(onSelect.mock.calls).toEqual(runtime.adminItems.map(game => [game.id]))
    expect(text(root)).not.toContain(stopped.name)
    expect(displayText(root)).not.toMatch(/官方源|外部源|平台彩/)
  })

  it('odds category switching stays parent-controlled and retains unclassified fallback', () => {
    const unknown = { ...fixture(4, 'official'), name: '待归类测试', lobby_category: ' ' }
    const onSelect = vi.fn()
    const props = { games: [...runtime.adminItems, unknown], gameId: runtime.adminItems[0].id, onSelect }
    let root = render(() => GameOddsNavigation(props))
    button(root, '未分类').props.onClick!()
    expect(onSelect).toHaveBeenCalledExactlyOnceWith(unknown.id)
    root = render(() => GameOddsNavigation(props))
    expect(captions(root)).not.toContain(unknown.name)
    root = render(() => GameOddsNavigation({ ...props, gameId: unknown.id }))
    const choice = ofType(root, Button).find(element => captions(element).includes(unknown.name))!
    expectBusinessOnly(choice, unknown)
    expect(choice.props.variant).toBe('outlined')
    expect(captions(root)).not.toContain(runtime.adminItems[0].name)
  })
})
