import { Alert, Box, Button, Drawer, IconButton, TableBody, TableRow, TextField, Tooltip } from '@mui/material'
import { isValidElement, type DependencyList, type ReactElement, type ReactNode, type SetStateAction } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AdminGame, AdminUser, UserTradingConfig } from '../api'
import { GameOddsNavigation, OddsOverrideGrid } from '../components/OddsEditors'
import { UsersPage } from './UsersPage'

// Exercise real page hooks and callbacks in Node, without a browser or live API.
type Slot = { value?: unknown; deps?: DependencyList; effect?: () => void | (() => void); cleanup?: void | (() => void) }
const sameDeps = (left?: DependencyList, right?: DependencyList) => Boolean(left && right && left.length === right.length && left.every((value, index) => Object.is(value, right[index])))
class PageHarness {
  private slots: Slot[] = []
  private cursor = 0
  private effects = new Set<number>()
  stateWrites = 0
  render<T>(factory: () => T): T { this.cursor = 0; return factory() }
  useState<T>(initial: T | (() => T)): [T, (value: SetStateAction<T>) => void] {
    const slot = this.slots[this.cursor++] ??= { value: typeof initial === 'function' ? (initial as () => T)() : initial }
    return [slot.value as T, value => { this.stateWrites += 1; slot.value = typeof value === 'function' ? (value as (previous: T) => T)(slot.value as T) : value }]
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
  users: vi.fn(), userStats: vi.fn(), dashboard: vi.fn(), userBalanceHistory: vi.fn(),
  userTrading: vi.fn(), updateUserTrading: vi.fn(), showMessage: vi.fn(),
}))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useMemo: <T,>(factory: () => T, deps: DependencyList) => runtime.hooks!.useMemo(factory, deps),
  useCallback: <T,>(callback: T, deps: DependencyList) => runtime.hooks!.useMemo(() => callback, deps),
  useEffect: (effect: () => void | (() => void), deps?: DependencyList) => runtime.hooks!.useEffect(effect, deps),
}))
vi.mock('../api', () => ({ adminApi: {
  users: runtime.users, userStats: runtime.userStats, dashboard: runtime.dashboard, userBalanceHistory: runtime.userBalanceHistory,
  userTrading: runtime.userTrading, updateUserTrading: runtime.updateUserTrading,
} }))
vi.mock('../components/feedback', () => ({ useFeedback: () => ({ showMessage: runtime.showMessage }) }))

type Props = {
  children?: ReactNode; actions?: ReactNode; title?: string; label?: string; severity?: string; component?: string
  open?: boolean; disabled?: boolean; value?: unknown; gameId?: string; items?: UserTradingConfig['odds']
  onClick?: () => void; onClose?: () => void; onSelect?: (id: string) => void
  onChange?: (value: { target: { value: string } } | UserTradingConfig['odds']) => void
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
const ofType = (node: ReactNode, type: unknown) => elements(node).filter(element => element.type === type)
const button = (node: ReactNode, label: string) => ofType(node, Button).find(element => text(element) === label)!
const field = (node: ReactNode, label: string) => ofType(node, TextField).find(element => element.props.label === label)!
const drawer = (node: ReactNode) => ofType(node, Drawer)[0]
const navigation = (node: ReactNode) => ofType(drawer(node), GameOddsNavigation)[0]
const grid = (node: ReactNode) => ofType(drawer(node), OddsOverrideGrid)[0]
const errors = (node: ReactNode) => ofType(node, Alert).filter(element => element.props.severity === 'error').map(text)
const detailsButton = (node: ReactNode, user: AdminUser) => {
  const row = ofType(ofType(node, TableBody)[0], TableRow).find(item => text(item).includes(user.nickname))!
  return ofType(ofType(row, Tooltip).find(item => item.props.title === '查看详情'), IconButton)[0]
}
const deferred = <T,>() => {
  let resolve!: (value: T) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail })
  return { promise, resolve, reject }
}

const member = (id: number): AdminUser => ({
  id, public_id: 10000 + id, username: `member-${id}`, nickname: `会员${id}`, email: '', phone: '',
  role: 'member', remark: '', risk_level: 'normal', balance: 100, status: 1, online: false,
  last_login_at: null, login_count: 0, created_at: '2026-09-01T00:00:00Z', updated_at: '2026-09-01T00:00:00Z',
})
const first = member(41)
const second = member(42)
const games = [
  { id: 'speed-racing', name: '极速赛车', lobby_category: '彩票', enabled: true },
  { id: 'speed-ssc', name: '极速时时彩', lobby_category: '168', enabled: true },
  { id: 'sg-ssc', name: 'SG时时彩', lobby_category: '168', enabled: true },
] as AdminGame[]
const config = (user = first, gameId = games[0].id, override = 8.5): UserTradingConfig => ({
  user_id: user.id, workspace_id: 8, username: user.username, odds_multiplier: 1,
  fly: { mode: 'inherit', rate: 0 }, rebate: { mode: 'inherit', rate: 0, effective: 0, source: 'room' },
  external_follow: { status: 'not_connected', capability: 'configuration_only', target_platform: '', target_account: '', endpoint_label: '', single_limit: 0, daily_limit: 0, remark: '' },
  game_id: gameId, game_name: games.find(item => item.id === gameId)!.name, room_fly_rate: 0, room_rebate_rate: 0,
  odds: [{ play_code: 'ball_1_5', play_name: '号码', base_odds: 9.9, room_odds: 9.8, override, effective: override, has_override: true }],
})
const render = () => {
  const root = runtime.hooks!.render(() => UsersPage())
  runtime.hooks!.flushEffects()
  return root
}
const drain = async () => { for (let index = 0; index < 16; index++) await Promise.resolve() }
const settle = async () => { await drain(); return render() }
const ready = async () => { render(); await vi.advanceTimersByTimeAsync(0); return render() }
const open = async (root: ReactNode, user = first) => { detailsButton(root, user).props.onClick!(); return settle() }
const select = async (root: ReactNode, gameId: string) => { navigation(root).props.onSelect!(gameId); return settle() }
const editMultiplier = (root: ReactNode, value = '1.1') => field(drawer(root), '倍率').props.onChange!({ target: { value } })
const saveButton = (root: ReactNode) => ofType(drawer(root), Button).find(item => ['已保存', '保存会员交易配置', '保存中…'].includes(text(item)))!

beforeEach(() => {
  runtime.hooks = new PageHarness()
  for (const value of Object.values(runtime)) if (vi.isMockFunction(value)) value.mockReset()
  runtime.users.mockResolvedValue({ items: [first, second], total: 2 })
  runtime.userStats.mockResolvedValue({ total: 2, active: 2, disabled: 0, new_today: 0 })
  runtime.dashboard.mockResolvedValue({ games })
  runtime.userBalanceHistory.mockResolvedValue([])
  runtime.userTrading.mockImplementation(async (id: number, gameId?: string) => config(id === first.id ? first : second, gameId))
  runtime.updateUserTrading.mockImplementation(async (id: number, input: { game_id: string; odds_multiplier: number }) => ({ ...config(id === first.id ? first : second, input.game_id), odds_multiplier: input.odds_multiplier }))
  vi.useFakeTimers()
  vi.stubGlobal('window', { setTimeout, clearTimeout, setInterval, clearInterval })
  vi.stubGlobal('document', { visibilityState: 'visible' })
})
afterEach(() => { runtime.hooks?.unmount(); vi.useRealTimers(); vi.unstubAllGlobals() })

describe('member trading editor identity and lifecycle', () => {
  it('keeps navigation and odds on the accepted game when another game fails, blocking pending edits and saves', async () => {
    let root = await open(await ready())
    const oldEditor = root
    const pending = deferred<UserTradingConfig>()
    runtime.userTrading.mockReturnValueOnce(pending.promise)
    navigation(root).props.onSelect!(games[1].id)
    editMultiplier(oldEditor)
    grid(oldEditor).props.onChange!([])
    saveButton(oldEditor).props.onClick!()
    root = render()
    expect(navigation(root).props.gameId).toBe(games[0].id)
    expect(grid(root).props.items).toEqual(config().odds)
    expect(field(drawer(root), '倍率').props.disabled).toBe(true)
    expect(ofType(drawer(root), Box).find(item => item.props.component === 'fieldset')!.props.disabled).toBe(true)
    expect(saveButton(root).props.disabled).toBe(true)
    expect(runtime.updateUserTrading).not.toHaveBeenCalled()
    pending.reject(new Error('新彩种读取失败'))
    root = await settle()
    expect(navigation(root).props.gameId).toBe(games[0].id)
    expect(grid(root).props.items).toEqual(config().odds)
    expect(errors(root)).toContain('新彩种读取失败')
    expect(field(drawer(root), '倍率').props.disabled).toBe(false)
    editMultiplier(root)
    saveButton(render()).props.onClick!()
    await settle()
    expect(runtime.updateUserTrading).toHaveBeenCalledExactlyOnceWith(first.id, expect.objectContaining({ game_id: games[0].id, odds_multiplier: 1.1 }))
  })

  it.each(['success', 'failure'] as const)('ignores an older game load %s after the newer game has been accepted', async outcome => {
    let root = await open(await ready())
    const older = deferred<UserTradingConfig>()
    const newer = deferred<UserTradingConfig>()
    runtime.userTrading.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise)
    navigation(root).props.onSelect!(games[1].id)
    root = render()
    navigation(root).props.onSelect!(games[2].id)
    newer.resolve(config(first, games[2].id, 7.7))
    root = await settle()
    expect(navigation(root).props.gameId).toBe(games[2].id)
    editMultiplier(root, '1.2')
    if (outcome === 'success') older.resolve(config(first, games[1].id, 6.6))
    else older.reject(new Error('旧彩种失败'))
    root = await settle()
    expect(navigation(root).props.gameId).toBe(games[2].id)
    expect(grid(root).props.items).toEqual(config(first, games[2].id, 7.7).odds)
    expect(field(drawer(root), '倍率').props.value).toBe(1.2)
    expect(errors(root)).toEqual([])
    expect(saveButton(root).props.disabled).toBe(false)
  })

  it('does not let an older load release the latest pending read lock', async () => {
    let root = await open(await ready())
    const older = deferred<UserTradingConfig>()
    const newer = deferred<UserTradingConfig>()
    runtime.userTrading.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise)
    navigation(root).props.onSelect!(games[1].id)
    root = render()
    navigation(root).props.onSelect!(games[2].id)
    older.resolve(config(first, games[1].id))
    root = await settle()
    expect(navigation(root).props.gameId).toBe(games[0].id)
    expect(field(drawer(root), '倍率').props.disabled).toBe(true)
    editMultiplier(root)
    expect(field(drawer(render()), '倍率').props.value).toBe(1)
    newer.resolve(config(first, games[2].id))
    expect(navigation(await settle()).props.gameId).toBe(games[2].id)
  })

  it.each([first, second])('discards an initial load after closing and reopening member $id', async reopened => {
    const pending = deferred<UserTradingConfig>()
    runtime.userTrading.mockReturnValueOnce(pending.promise)
    let root = await open(await ready())
    expect(grid(root)).toBeUndefined()
    drawer(root).props.onClose!()
    root = await open(render(), reopened)
    editMultiplier(root, '1.2')
    pending.resolve(config(first, games[1].id, 3.3))
    root = await settle()
    expect(text(drawer(root))).toContain(`@${reopened.username}`)
    expect(navigation(root).props.gameId).toBe(games[0].id)
    expect(grid(root).props.items).toEqual(config(reopened).odds)
    expect(field(drawer(root), '倍率').props.value).toBe(1.2)
    saveButton(root).props.onClick!()
    await settle()
    expect(runtime.updateUserTrading).toHaveBeenCalledExactlyOnceWith(reopened.id, expect.objectContaining({ game_id: games[0].id, odds_multiplier: 1.2 }))
  })

  it('guards same-frame edits, duplicate saves, pending mutations and stale member actions', async () => {
    const pending = deferred<UserTradingConfig>()
    runtime.updateUserTrading.mockReturnValueOnce(pending.promise)
    let root = await open(await ready())
    const oldEditor = root
    editMultiplier(root, '1.2')
    navigation(root).props.onSelect!(games[1].id)
    expect(runtime.userTrading).toHaveBeenCalledTimes(1)
    saveButton(root).props.onClick!()
    saveButton(root).props.onClick!()
    editMultiplier(root, '1.3')
    button(root, '0.80 倍').props.onClick!()
    grid(root).props.onChange!([])
    navigation(root).props.onSelect!(games[1].id)
    detailsButton(root, second).props.onClick!()
    root = render()
    expect(text(drawer(root))).toContain(`@${first.username}`)
    expect(field(drawer(root), '倍率').props.value).toBe(1.2)
    expect(grid(root).props.items).toEqual(config().odds)
    expect(runtime.userTrading).toHaveBeenCalledTimes(1)
    expect(runtime.updateUserTrading).toHaveBeenCalledExactlyOnceWith(first.id, expect.objectContaining({ odds_multiplier: 1.2, game_id: games[0].id }))
    pending.resolve({ ...config(), odds_multiplier: 1.2 })
    root = await settle()
    editMultiplier(oldEditor, '1.4')
    expect(field(drawer(render()), '倍率').props.value).toBe(1.2)
    expect(saveButton(root).props.disabled).toBe(true)
    expect(runtime.showMessage).toHaveBeenCalledWith('飞单、返水与单独赔率已保存')
  })

  it.each(['success', 'failure'] as const)('ignores a closed member save %s without unlocking or overwriting a new member save', async outcome => {
    const oldWrite = deferred<UserTradingConfig>()
    const newWrite = deferred<UserTradingConfig>()
    runtime.updateUserTrading.mockReturnValueOnce(oldWrite.promise).mockReturnValueOnce(newWrite.promise)
    let root = await open(await ready())
    editMultiplier(root, '1.1')
    saveButton(render()).props.onClick!()
    root = render()
    drawer(root).props.onClose!()
    root = await open(render(), second)
    editMultiplier(root, '1.2')
    saveButton(render()).props.onClick!()
    if (outcome === 'success') oldWrite.resolve({ ...config(), fly: { mode: 'custom', rate: 99 } })
    else oldWrite.reject(new Error('旧会员保存失败'))
    root = await settle()
    expect(text(drawer(root))).toContain(`@${second.username}`)
    expect(field(drawer(root), '倍率').props.value).toBe(1.2)
    expect(field(drawer(root), '飞单模式').props.value).toBe('inherit')
    expect(text(saveButton(root))).toBe('保存中…')
    expect(errors(root)).toEqual([])
    expect(runtime.showMessage).not.toHaveBeenCalled()
    saveButton(root).props.onClick!()
    expect(runtime.updateUserTrading).toHaveBeenCalledTimes(2)
    newWrite.resolve({ ...config(second), odds_multiplier: 1.2 })
    root = await settle()
    expect(saveButton(root).props.disabled).toBe(true)
    expect(runtime.showMessage).toHaveBeenCalledExactlyOnceWith('飞单、返水与单独赔率已保存')
  })

  it('rejects stale callbacks after a game round trip and after switching member sessions', async () => {
    let root = await open(await ready())
    const original = root
    root = await select(root, games[1].id)
    root = await select(root, games[0].id)
    grid(original).props.onChange!([])
    editMultiplier(original, '1.4')
    expect(grid(render()).props.items).toEqual(config().odds)
    expect(field(drawer(render()), '倍率').props.value).toBe(1)
    root = await open(root, second)
    editMultiplier(root, '1.2')
    grid(original).props.onChange!([])
    navigation(original).props.onSelect!(games[1].id)
    saveButton(original).props.onClick!()
    drawer(original).props.onClose!()
    root = render()
    expect(text(drawer(root))).toContain(`@${second.username}`)
    expect(grid(root).props.items).toEqual(config(second).odds)
    expect(field(drawer(root), '倍率').props.value).toBe(1.2)
    expect(runtime.updateUserTrading).not.toHaveBeenCalled()
  })

  it('finishes failed initial loading and prevents an old retry from replacing a reopened draft', async () => {
    runtime.userTrading.mockRejectedValueOnce(new Error('初次读取失败'))
    let root = await open(await ready())
    expect(text(drawer(root))).not.toContain('加载交易配置中')
    expect(grid(root)).toBeUndefined()
    const retry = button(root, '重新读取交易配置')
    expect(retry).toBeDefined()
    drawer(root).props.onClose!()
    root = await open(render())
    editMultiplier(root, '1.2')
    retry.props.onClick!()
    root = await settle()
    expect(runtime.userTrading).toHaveBeenCalledTimes(2)
    expect(field(drawer(root), '倍率').props.value).toBe(1.2)
    expect(saveButton(root).props.disabled).toBe(false)
  })

  it.each(['user', 'game'] as const)('rejects mismatched %s identity in load and save responses', async mismatch => {
    let root = await open(await ready())
    runtime.userTrading.mockResolvedValueOnce(mismatch === 'user' ? config(second, games[1].id) : config(first, games[2].id))
    root = await select(root, games[1].id)
    expect(navigation(root).props.gameId).toBe(games[0].id)
    expect(errors(root).join('')).toContain('会员或彩种不一致')
    editMultiplier(root, '1.2')
    runtime.updateUserTrading.mockResolvedValueOnce(mismatch === 'user' ? config(second) : config(first, games[1].id))
    saveButton(render()).props.onClick!()
    root = await settle()
    expect(navigation(root).props.gameId).toBe(games[0].id)
    expect(field(drawer(root), '倍率').props.value).toBe(1.2)
    expect(saveButton(root).props.disabled).toBe(false)
    expect(runtime.showMessage).not.toHaveBeenCalled()
    expect(errors(root).join('')).toContain('会员或彩种不一致')
  })

  it.each(['read-success', 'read-failure', 'save-success', 'save-failure'] as const)('discards %s after unmount without state updates or feedback', async operation => {
    let root = await open(await ready())
    const pending = deferred<UserTradingConfig>()
    if (operation.startsWith('read')) {
      runtime.userTrading.mockReturnValueOnce(pending.promise)
      navigation(root).props.onSelect!(games[1].id)
    } else {
      runtime.updateUserTrading.mockReturnValueOnce(pending.promise)
      editMultiplier(root)
      saveButton(render()).props.onClick!()
    }
    root = render()
    runtime.hooks!.unmount()
    const writes = runtime.hooks!.stateWrites
    editMultiplier(root)
    navigation(root).props.onSelect!(games[2].id)
    saveButton(root).props.onClick!()
    if (operation.endsWith('success')) pending.resolve(config(first, operation.startsWith('read') ? games[1].id : games[0].id))
    else pending.reject(new Error('卸载后失败'))
    await drain()
    expect(runtime.hooks!.stateWrites).toBe(writes)
    expect(runtime.showMessage).not.toHaveBeenCalled()
  })
})
