import { Alert, Button, Checkbox, Chip, Dialog, DialogTitle, TableBody, TableRow, TextField } from '@mui/material'
import { isValidElement, type DependencyList, type ReactElement, type ReactNode, type SetStateAction } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AdminGame, GameOddsLimits, PlayCatalogItem, PlayLimitItem, UpdateOddsLimitsInput } from '../api'
import { GameOddsNavigation, PlatformOddsGrid } from '../components/OddsEditors'
import { LimitsPage } from './LimitsPage'
import { oddsDraftItems } from '../oddsEditing'

// Drive the page's real hooks and event handlers in Node; no browser or live API.
type Slot = { value?: unknown; deps?: DependencyList; effect?: () => void | (() => void); cleanup?: void | (() => void) }
const sameDeps = (left?: DependencyList, right?: DependencyList) => Boolean(left && right && left.length === right.length && left.every((value, index) => Object.is(value, right[index])))
class PageHarness {
  private slots: Slot[] = []
  private cursor = 0
  private effects = new Set<number>()
  render<T>(factory: () => T): T { this.cursor = 0; return factory() }
  useState<T>(initial: T | (() => T)): [T, (value: SetStateAction<T>) => void] {
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
  games: vi.fn(), oddsLimits: vi.fn(), playCatalog: vi.fn(), updateOddsLimits: vi.fn(),
  resetOddsLimits: vi.fn(), showMessage: vi.fn(), confirm: vi.fn(),
  listeners: new Map<string, (event: Event) => void>(),
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
  games: runtime.games, oddsLimits: runtime.oddsLimits, playCatalog: runtime.playCatalog,
  updateOddsLimits: runtime.updateOddsLimits, resetOddsLimits: runtime.resetOddsLimits,
} }))
vi.mock('../components/feedback', () => ({ useFeedback: () => ({ showMessage: runtime.showMessage }) }))

type Props = {
  children?: ReactNode; actions?: ReactNode; control?: ReactNode; label?: string; severity?: string; open?: boolean; disabled?: boolean
  value?: unknown; variant?: string; inputProps?: { 'aria-label'?: string }
  gameId?: string; games?: AdminGame[]; items?: PlayLimitItem[]; catalog?: Record<string, PlayCatalogItem>
  onClick?: () => void; onClose?: () => void; onSelect?: (id: string) => void; onChange?: (value: PlayLimitItem[] | { target: { value: string } }, checked?: boolean) => void
}
type Element = ReactElement<Props>
function elements(node: ReactNode): Element[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  if (!isValidElement<Props>(node)) return []
  return [node, ...elements(node.props.children), ...elements(node.props.actions), ...elements(node.props.control)]
}
function text(node: ReactNode): string {
  if (Array.isArray(node)) return node.map(text).join('')
  if (isValidElement<Props>(node)) return text(node.props.children)
  return typeof node === 'string' || typeof node === 'number' ? String(node) : ''
}
const ofType = (node: ReactNode, type: unknown) => elements(node).filter(element => element.type === type)
const button = (node: ReactNode, label: string) => ofType(node, Button).find(element => text(element) === label)!
const navigation = (node: ReactNode) => ofType(node, GameOddsNavigation)[0]
const renderNavigation = (node: ReactNode, harness: PageHarness) => {
  const pageHooks = runtime.hooks
  runtime.hooks = harness
  try {
    const props = navigation(node).props
    const result = harness.render(() => GameOddsNavigation({ games: props.games!, gameId: props.gameId!, onSelect: props.onSelect! }))
    harness.flushEffects()
    return result
  } finally {
    runtime.hooks = pageHooks
  }
}
const grid = (node: ReactNode) => ofType(node, PlatformOddsGrid)[0]
const chips = (node: ReactNode) => ofType(node, Chip).map(element => element.props.label)
const alerts = (node: ReactNode, severity: string) => ofType(node, Alert).filter(element => element.props.severity === severity).map(text)
const deferred = <T,>() => {
  let resolve!: (value: T) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail })
  return { promise, resolve, reject }
}
const game = (id: string, name: string, enabled = true): AdminGame => ({
  id, code: id.toUpperCase(), name, category: name, lobby_category: '彩票', lobby_sort_order: 1,
  badge: name, badge_color: 'blue', enabled, issue: '12345', next_draw_at: '2026-09-01T00:00:00Z',
  turnover: 0, profit: 0, source_kind: 'simulated', source_name: '平台', source_url: '',
  sync_status: 'ok', last_sync_at: null, last_sync_error: '', schedule_mode: 'interval',
})
const racing = game('speed-racing', '极速赛车')
const digits = game('speed-ssc', '极速时时彩')
const unknown = game('bingo-ssc-2', '宾果时时彩(2)')
const stopped = game('au-lucky-5', '澳洲幸运5', false)
const catalogItem = (play_code: string, play_name: string, category: string, description: string, example: string): PlayCatalogItem => ({
  play_code, play_name, category, description, example, sort_order: 0,
})
const racingCatalog = [
  catalogItem('ball_1_5', '指定名次号码', '号码', '第1–10名号码1–10', '1/0/20'),
  catalogItem('two_sided', '两面', '两面盘', '每个名次的大小单双', '6/大/20'),
  catalogItem('dragon_tiger', '龙虎', '龙虎', '第1–5名镜像比较', '1/龙/20'),
  catalogItem('sum', '冠亚和', '冠亚和', '前两名和值3–19', '冠亚/14/20'),
]
const digitsCatalog = [
  catalogItem('ball_1_5', '指定球位号码', '号码', '第1–5球号码0–9', '1/0/20'),
  catalogItem('two_sided', '两面', '两面盘', '每个球位的大小单双', '5/大/20'),
  catalogItem('dragon_tiger', '龙虎', '龙虎', '第1球与第5球比较', '1/龙/20'),
  catalogItem('dragon_tiger_tie', '龙虎和', '龙虎', '第1球与第5球相等', '1/和/20'),
  ...['豹子', '顺子', '对子', '半顺', '杂六'].map((name, index) => catalogItem(['leopard', 'straight', 'pair', 'half_straight', 'mixed'][index], `${name}`, '三球形态', '前三、中三、后三分别判定', `中三/${name}/20`)),
]
const itemsFor = (catalog: PlayCatalogItem[]): PlayLimitItem[] => catalog.map(item => ({
  play_code: item.play_code, play_name: item.play_name, odds: item.play_code === 'ball_1_5' ? 9.9 : 1.993, configured: true, configuration_source: 'admin_save', configured_at: null,
  min_bet: 1, max_bet: 1000, max_user_period: 5000, max_period_total: 20000, sort_order: item.sort_order,
}))
const limitsFor = (selected: AdminGame): GameOddsLimits => ({
  game_id: selected.id, game_name: selected.name,
  items: selected.id === unknown.id ? [] : itemsFor(selected.id === racing.id ? racingCatalog : digitsCatalog),
  rules_ready: selected.id !== unknown.id,
  rule_version: selected.id === unknown.id ? '' : selected.id === racing.id ? 'racing-v2' : 'digits5-v3',
  config_revision: `revision-${selected.id}-1`,
  rules_message: selected.id === unknown.id ? '该彩种尚未配置完整玩法，暂不受理投注' : '',
})
const allGames = [racing, digits, unknown, stopped]
const guardFor = (selected: AdminGame) => ({ expected_rule_version: limitsFor(selected).rule_version!, expected_revision: limitsFor(selected).config_revision! })
const inputFor = (selected: AdminGame, items: PlayLimitItem[]) => ({ ...guardFor(selected), items: oddsDraftItems(items, limitsFor(selected).items) })
const persisted = (items: PlayLimitItem[]) => items.map(item => ({ ...item, configured: item.odds > 1, configuration_source: item.odds > 1 ? 'admin_save' : 'unconfigured', configured_at: null }))
const render = () => {
  const root = runtime.hooks!.render(() => LimitsPage())
  runtime.hooks!.flushEffects()
  return root
}
const settle = async () => { for (let index = 0; index < 16; index++) await Promise.resolve(); return render() }
const ready = async () => {
  render()
  await vi.runOnlyPendingTimersAsync()
  render()
  await vi.runOnlyPendingTimersAsync()
  return render()
}
const select = async (root: ReactNode, id: string) => {
  navigation(root).props.onSelect!(id)
  render()
  await vi.runOnlyPendingTimersAsync()
  return render()
}

beforeEach(() => {
  runtime.hooks = new PageHarness()
  for (const value of Object.values(runtime)) if (vi.isMockFunction(value)) value.mockReset()
  runtime.games.mockResolvedValue(allGames)
  runtime.oddsLimits.mockImplementation(async (id: string) => limitsFor(allGames.find(item => item.id === id)!))
  runtime.playCatalog.mockImplementation(async (id: string) => id === unknown.id ? [] : id === racing.id ? racingCatalog : digitsCatalog)
  runtime.updateOddsLimits.mockImplementation(async (id: string, input: UpdateOddsLimitsInput) => ({ ...limitsFor(allGames.find(item => item.id === id)!), config_revision: `revision-${id}-2`, items: persisted(input.items) }))
  runtime.resetOddsLimits.mockImplementation(async (id: string) => ({ ...limitsFor(allGames.find(item => item.id === id)!), config_revision: `revision-${id}-2`, items: persisted(limitsFor(allGames.find(item => item.id === id)!).items.map(item => ({ ...item, odds: 0 }))) }))
  runtime.confirm.mockReturnValue(true)
  vi.useFakeTimers()
  runtime.listeners.clear()
  vi.stubGlobal('window', { setTimeout, clearTimeout, confirm: runtime.confirm,
    addEventListener: (type: string, listener: (event: Event) => void) => runtime.listeners.set(type, listener),
    removeEventListener: (type: string) => runtime.listeners.delete(type),
  })
})
afterEach(() => {
  runtime.hooks!.unmount()
  vi.clearAllTimers()
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('per-game platform odds lifecycle', () => {
  it('distinguishes an audited backend save from a legacy price awaiting confirmation', () => {
    const catalog = Object.fromEntries([
      catalogItem('sum_big', '冠亚和大', '冠亚和', '冠亚和大', '冠亚/大/20'),
      catalogItem('sum_3', '冠亚和3', '冠亚和', '冠亚和3', '冠亚/3/20'),
    ].map(item => [item.play_code, item]))
    const root = PlatformOddsGrid({
      items: [
        { ...itemsFor([catalog.sum_big])[0], configured: true, configuration_source: 'admin_save', configured_at: '2026-09-02T00:00:00Z' },
        { ...itemsFor([catalog.sum_3])[0], odds: 0, configured: false, configuration_source: 'legacy_unconfirmed' },
      ],
      catalog,
      onChange: vi.fn(),
    })
    expect(text(root)).toContain('冠亚和')
    expect(chips(root)).toContain('后台已确认')
    expect(chips(root)).toContain('未配置 / 停用')
  })

  it('shows saved odds risk without overwriting values, and clears it after a safe save', async () => {
    const warning = { code: 'SHAPE_COVERAGE_RISK', play_codes: ['leopard', 'straight', 'pair', 'half_straight', 'mixed'], message: '前三形态赔率存在覆盖套利风险，新的形态投注将被拒绝' }
    runtime.oddsLimits.mockImplementation(async (id: string) => ({
      ...limitsFor(allGames.find(item => item.id === id)!), risk_warnings: id === digits.id ? [warning] : [],
    }))
    let root = await select(await ready(), digits.id)
    expect(alerts(root, 'warning').join('')).toContain('已保存配置 · 赔率风险')
    expect(alerts(root, 'warning').join('')).toContain(warning.message)
    expect(grid(root).props.items).toEqual(limitsFor(digits).items)
    expect(button(root, '保存设置').props.disabled).toBe(true)
    const edited = grid(root).props.items!.map(item => ({ ...item, odds: ({ pair: 2.8, half_straight: 2.4, mixed: 3 } as Record<string, number>)[item.play_code] ?? item.odds }))
    grid(root).props.onChange!(edited)
    root = render()
    expect(alerts(root, 'warning').join('')).toContain(warning.message)
    button(root, '保存设置').props.onClick!()
    root = await settle()
    expect(runtime.updateOddsLimits).toHaveBeenCalledExactlyOnceWith(digits.id, inputFor(digits, edited))
    expect(alerts(root, 'warning')).toEqual([])
    expect(grid(root).props.items).toEqual(edited)
  })

  it('does not carry a previous game risk into another game or hide a warning after reset', async () => {
    const warning = { code: 'SHAPE_COVERAGE_RISK', play_codes: ['leopard'], message: '当前保存的前三形态赔率存在风险' }
    runtime.resetOddsLimits.mockResolvedValue({ ...limitsFor(digits), risk_warnings: [warning] })
    let root = await select(await ready(), digits.id)
    button(root, '清空当前').props.onClick!()
    root = await settle()
    expect(alerts(root, 'warning').join('')).toContain(warning.message)
    root = await select(root, racing.id)
    expect(alerts(root, 'warning')).toEqual([])
  })

  it('loads racing odds and the matching four-play guide together', async () => {
    let root = await ready()
    expect(runtime.oddsLimits).toHaveBeenCalledExactlyOnceWith(racing.id)
    expect(runtime.playCatalog).toHaveBeenCalledExactlyOnceWith(racing.id)
    expect(Object.keys(grid(root).props.catalog!)).toEqual(['ball_1_5', 'two_sided', 'dragon_tiger', 'sum'])
    expect(grid(root).props.items).toEqual(limitsFor(racing).items)
    expect(chips(root)).toContain('运行中')
    button(root, '玩法说明').props.onClick!()
    root = render()
    const guide = ofType(root, Dialog)[0]
    expect(guide.props.open).toBe(true)
    expect(text(ofType(guide, DialogTitle)[0])).toBe('极速赛车 · 玩法说明')
    expect(ofType(ofType(guide, TableBody)[0], TableRow)).toHaveLength(4)
    expect(text(guide)).toContain('冠亚/14/20')
    expect(text(guide)).not.toMatch(/豹子|顺子|总和尾/)
  })

  it('switches the guide and grid to SSC independent tie and three shape windows, never retired sums', async () => {
    let root = await select(await ready(), digits.id)
    expect(runtime.oddsLimits).toHaveBeenLastCalledWith(digits.id)
    expect(runtime.playCatalog).toHaveBeenLastCalledWith(digits.id)
    expect(grid(root).props.catalog!.dragon_tiger_tie).toMatchObject({ play_name: '龙虎和', description: '第1球与第5球相等' })
    expect(grid(root).props.items).toHaveLength(9)
    button(root, '玩法说明').props.onClick!()
    root = render()
    const guide = ofType(root, Dialog)[0]
    expect(text(guide)).toContain('极速时时彩 · 玩法说明')
    expect(text(guide)).toContain('1/和/20')
    expect(text(guide)).toContain('前三、中三、后三分别判定')
    expect(text(guide)).not.toMatch(/冠亚|总和尾|默认赔率/)
  })

  it('keeps unknown games visible but paused, cannot save or reset, and explains retained configuration', async () => {
    const root = await select(await ready(), unknown.id)
    expect(navigation(root).props.games!.map(item => item.id)).toContain(unknown.id)
    expect(chips(root)).toContain('玩法待配置')
    expect(chips(root)).not.toContain('运行中')
    expect(grid(root)).toBeUndefined()
    expect(alerts(root, 'warning').join('')).toContain('现有赔率配置保留，确认专属规则后再开放')
    expect(text(root)).toContain('此彩种的专属玩法待核对，当前不提供投注赔率')
    expect(button(root, '玩法说明').props.disabled).toBe(true)
    for (const label of ['保存设置', '清空当前']) {
      expect(button(root, label).props.disabled).toBe(true)
      button(root, label).props.onClick!()
    }
    expect(runtime.updateOddsLimits).not.toHaveBeenCalled()
    expect(runtime.resetOddsLimits).not.toHaveBeenCalled()
    expect(runtime.confirm).not.toHaveBeenCalled()
  })

  it('does not relabel a modeled but disabled game as running or hide it from configuration navigation', async () => {
    const root = await select(await ready(), stopped.id)
    expect(navigation(root).props.gameId).toBe(stopped.id)
    expect(navigation(root).props.games!.find(item => item.id === stopped.id)).toBeDefined()
    expect(chips(root)).toContain('已停用')
    expect(chips(root)).not.toContain('运行中')
  })

  it('keeps any retained legacy rows read-only when the server explicitly marks rules unavailable', async () => {
    const retained = itemsFor(racingCatalog)
    runtime.oddsLimits.mockImplementation(async (id: string) => id === unknown.id
      ? { ...limitsFor(unknown), items: retained }
      : limitsFor(allGames.find(item => item.id === id)!))
    let root = await select(await ready(), unknown.id)
    expect(grid(root).props.items).toEqual(retained)
    grid(root).props.onChange!([])
    root = render()
    expect(grid(root).props.items).toEqual(retained)
    expect(chips(root)).not.toContain('运行中')
    expect(button(root, '保存设置').props.disabled).toBe(true)
    expect(button(root, '清空当前').props.disabled).toBe(true)
    expect(runtime.updateOddsLimits).not.toHaveBeenCalled()
    expect(runtime.resetOddsLimits).not.toHaveBeenCalled()
  })

  it('waits for both endpoints and ignores an old catalog that finishes after switching games', async () => {
    const oldCatalog = deferred<PlayCatalogItem[]>()
    runtime.playCatalog.mockImplementation((id: string) => id === racing.id ? oldCatalog.promise : Promise.resolve(digitsCatalog))
    let root = await ready()
    expect(grid(root)).toBeUndefined()
    expect(button(root, '保存设置').props.disabled).toBe(true)
    root = await select(root, digits.id)
    expect(grid(root).props.items).toEqual(limitsFor(digits).items)
    oldCatalog.resolve(racingCatalog)
    root = await settle()
    expect(navigation(root).props.gameId).toBe(digits.id)
    expect(grid(root).props.items).toEqual(limitsFor(digits).items)
    expect(grid(root).props.catalog!.dragon_tiger_tie.play_name).toBe('龙虎和')
    expect(alerts(root, 'error')).toEqual([])
  })

  it('ignores an old odds rejection after the new game has loaded', async () => {
    const oldOdds = deferred<GameOddsLimits>()
    runtime.oddsLimits.mockImplementation((id: string) => id === racing.id ? oldOdds.promise : Promise.resolve(limitsFor(digits)))
    let root = await select(await ready(), digits.id)
    oldOdds.reject(new Error('旧彩种请求失败'))
    root = await settle()
    expect(grid(root).props.items).toEqual(limitsFor(digits).items)
    expect(alerts(root, 'error')).toEqual([])
    expect(button(root, '保存设置').props.disabled).toBe(true)
  })

  it('does not let a late ready response reactivate a newly selected unknown game', async () => {
    const oldOdds = deferred<GameOddsLimits>()
    runtime.oddsLimits.mockImplementation((id: string) => id === racing.id ? oldOdds.promise : Promise.resolve(limitsFor(unknown)))
    let root = await select(await ready(), unknown.id)
    oldOdds.resolve(limitsFor(racing))
    root = await settle()
    expect(navigation(root).props.gameId).toBe(unknown.id)
    expect(grid(root)).toBeUndefined()
    expect(chips(root)).toContain('玩法待配置')
    expect(chips(root)).not.toContain('运行中')
    expect(button(root, '保存设置').props.disabled).toBe(true)
    expect(alerts(root, 'warning').join('')).toContain('现有赔率配置保留')
  })

  it('fails closed when the selected game catalog fails instead of showing mismatched odds', async () => {
    runtime.playCatalog.mockRejectedValue(new Error('玩法说明读取失败'))
    const root = await ready()
    expect(grid(root)).toBeUndefined()
    expect(alerts(root, 'error')).toEqual(['玩法说明读取失败'])
    expect(button(root, '保存设置').props.disabled).toBe(true)
    expect(button(root, '清空当前').props.disabled).toBe(true)
  })

  it('saves edited values to the selected game and freezes navigation and editing until completion', async () => {
    const pending = deferred<GameOddsLimits>()
    runtime.updateOddsLimits.mockReturnValue(pending.promise)
    let root = await ready()
    const edited = grid(root).props.items!.map(item => ({ ...item, odds: item.odds + 0.1 }))
    grid(root).props.onChange!(edited)
    root = render()
    button(root, '保存设置').props.onClick!()
    root = render()
    expect(button(root, '保存中…').props.disabled).toBe(true)
    expect(button(root, '清空当前').props.disabled).toBe(true)
    expect(button(root, '刷新配置').props.disabled).toBe(true)
    navigation(root).props.onSelect!(digits.id)
    grid(root).props.onChange!([])
    root = render()
    expect(navigation(root).props.gameId).toBe(racing.id)
    expect(grid(root).props.items).toEqual(oddsDraftItems(edited, limitsFor(racing).items))
    expect(runtime.updateOddsLimits).toHaveBeenCalledExactlyOnceWith(racing.id, inputFor(racing, edited))
    pending.resolve({ ...limitsFor(racing), items: edited })
    root = await settle()
    expect(button(root, '保存设置').props.disabled).toBe(true)
    expect(runtime.showMessage).toHaveBeenCalledWith('极速赛车 赔率限额已保存')
  })

  it('keeps the accepted category and visible game when cancelling a cross-category draft discard', async () => {
    runtime.games.mockResolvedValue([racing, { ...digits, lobby_category: '168' }])
    const navigationHooks = new PageHarness()
    let root = await ready()
    let gameNavigation = renderNavigation(root, navigationHooks)
    const edited = grid(root).props.items!.map(item => ({ ...item, max_bet: 1200 }))
    grid(root).props.onChange!(edited)
    runtime.confirm.mockReturnValue(false)
    button(gameNavigation, '168').props.onClick!()
    root = render()
    gameNavigation = renderNavigation(root, navigationHooks)
    expect(runtime.confirm).toHaveBeenCalledWith(expect.stringContaining('切换彩种将放弃'))
    expect(navigation(root).props.gameId).toBe(racing.id)
    expect(button(gameNavigation, '彩票').props.variant).toBe('contained')
    expect(button(gameNavigation, '168').props.variant).toBe('text')
    expect(text(gameNavigation)).toContain(racing.name)
    expect(text(gameNavigation)).not.toContain(digits.name)
    expect(grid(root).props.items).toEqual(oddsDraftItems(edited, limitsFor(racing).items))
    expect(runtime.oddsLimits).toHaveBeenCalledExactlyOnceWith(racing.id)

    runtime.confirm.mockReturnValue(true)
    button(gameNavigation, '168').props.onClick!()
    render()
    await vi.runOnlyPendingTimersAsync()
    root = render()
    gameNavigation = renderNavigation(root, navigationHooks)
    expect(navigation(root).props.gameId).toBe(digits.id)
    expect(button(gameNavigation, '168').props.variant).toBe('contained')
    expect(text(gameNavigation)).toContain(digits.name)
    expect(text(gameNavigation)).not.toContain(racing.name)
    expect(grid(root).props.items).toEqual(limitsFor(digits).items)
  })

  it('derives navigation immediately from externally accepted games and normalizes blank categories', () => {
    const onSelect = vi.fn()
    const uncategorized = { ...digits, lobby_category: '  ' }
    const disabled = { ...unknown, enabled: false, lobby_category: '宾果' }
    let props = { games: [racing, uncategorized, disabled], gameId: racing.id, onSelect }
    const renderGames = () => runtime.hooks!.render(() => GameOddsNavigation(props))
    let root = renderGames()
    button(root, '未分类').props.onClick!()
    expect(onSelect).toHaveBeenCalledExactlyOnceWith(digits.id)
    root = renderGames()
    expect(button(root, '彩票').props.variant).toBe('contained')
    props = { ...props, gameId: digits.id }
    root = renderGames()
    expect(button(root, '未分类').props.variant).toBe('contained')
    expect(text(root)).toContain(digits.name)
    expect(text(root)).not.toContain(racing.name)
    expect(button(root, '宾果')).toBeUndefined()
    props = { ...props, gameId: disabled.id }
    root = renderGames()
    expect(button(root, '彩票').props.variant).toBe('contained')
    props = { ...props, games: [] }
    expect(ofType(renderGames(), Button)).toHaveLength(0)
  })

  it.each(['save', 'reset'] as const)('keeps category and game aligned when a pending %s rejects cross-category navigation', async operation => {
    runtime.games.mockResolvedValue([racing, { ...digits, lobby_category: '168' }])
    const pending = deferred<GameOddsLimits>()
    const operationMock = operation === 'save' ? runtime.updateOddsLimits : runtime.resetOddsLimits
    operationMock.mockReturnValue(pending.promise)
    const navigationHooks = new PageHarness()
    let root = await ready()
    if (operation === 'save') {
      grid(root).props.onChange!(grid(root).props.items!.map(item => ({ ...item, max_bet: 1200 })))
      root = render()
    }
    const beforeWrite = renderNavigation(root, navigationHooks)
    button(root, operation === 'save' ? '保存设置' : '清空当前').props.onClick!()
    const confirmCount = runtime.confirm.mock.calls.length
    // Exercise the same-frame child callback before the parent rerenders busy state.
    button(beforeWrite, '168').props.onClick!()
    root = render()
    let gameNavigation = renderNavigation(root, navigationHooks)
    expect(navigation(root).props.gameId).toBe(racing.id)
    expect(button(gameNavigation, '彩票').props.variant).toBe('contained')
    expect(button(gameNavigation, '168').props.variant).toBe('text')
    expect(text(gameNavigation)).toContain(racing.name)
    expect(text(gameNavigation)).not.toContain(digits.name)
    expect(runtime.confirm).toHaveBeenCalledTimes(confirmCount)
    expect(runtime.oddsLimits).toHaveBeenCalledExactlyOnceWith(racing.id)
    expect(operationMock).toHaveBeenCalledTimes(1)

    pending.resolve(limitsFor(racing))
    root = await settle()
    gameNavigation = renderNavigation(root, navigationHooks)
    button(gameNavigation, '168').props.onClick!()
    render()
    await vi.runOnlyPendingTimersAsync()
    root = render()
    gameNavigation = renderNavigation(root, navigationHooks)
    expect(navigation(root).props.gameId).toBe(digits.id)
    expect(button(gameNavigation, '168').props.variant).toBe('contained')
    expect(text(gameNavigation)).toContain(digits.name)
    expect(text(gameNavigation)).not.toContain(racing.name)
  })

  it.each(['save', 'reset'] as const)('%s locks same-frame duplicate writes and stale game-selection callbacks', async operation => {
    const pending = deferred<GameOddsLimits>()
    const operationMock = operation === 'save' ? runtime.updateOddsLimits : runtime.resetOddsLimits
    operationMock.mockReturnValue(pending.promise)
    let root = await ready()
    const draft = limitsFor(racing).items.map(item => ({ ...item, max_bet: 2345 }))
    if (operation === 'save') {
      grid(root).props.onChange!(draft)
      root = render()
    }
    const label = operation === 'save' ? '保存设置' : '清空当前'
    button(root, label).props.onClick!()
    // Before a render, every closure still has saving=false and busy=null.
    navigation(root).props.onSelect!(digits.id)
    for (const staleLabel of ['保存设置', '清空当前', '刷新配置']) button(root, staleLabel).props.onClick!()
    grid(root).props.onChange!([])
    let current = render()
    expect(navigation(current).props.gameId).toBe(racing.id)
    expect(grid(current).props.items).toEqual(operation === 'save' ? oddsDraftItems(draft, limitsFor(racing).items) : limitsFor(racing).items)
    expect(operationMock).toHaveBeenCalledTimes(1)
    expect(runtime.updateOddsLimits.mock.calls.length + runtime.resetOddsLimits.mock.calls.length).toBe(1)
    pending.resolve(limitsFor(racing))
    current = await settle()
    expect(navigation(current).props.gameId).toBe(racing.id)
    expect(grid(current).props.items).toEqual(limitsFor(racing).items)
  })

  it('cancelling a reset neither mutates settings nor holds the write lock', async () => {
    runtime.confirm.mockReturnValue(false)
    let root = await ready()
    button(root, '清空当前').props.onClick!()
    expect(runtime.confirm).toHaveBeenCalledTimes(1)
    expect(runtime.resetOddsLimits).not.toHaveBeenCalled()
    root = await select(render(), digits.id)
    expect(navigation(root).props.gameId).toBe(digits.id)
    expect(button(root, '保存设置').props.disabled).toBe(true)
  })

  it.each(['save', 'reset'] as const)('releases the lock after failed %s, preserves edits and allows another game to load', async operation => {
    const operationMock = operation === 'save' ? runtime.updateOddsLimits : runtime.resetOddsLimits
    operationMock.mockRejectedValue(new Error('写入失败，请重试'))
    let root = await ready()
    const edited = grid(root).props.items!.map(item => ({ ...item, max_bet: 2345 }))
    grid(root).props.onChange!(edited)
    root = render()
    button(root, operation === 'save' ? '保存设置' : '清空当前').props.onClick!()
    root = await settle()
    expect(grid(root).props.items).toEqual(oddsDraftItems(edited, limitsFor(racing).items))
    expect(alerts(root, 'error')).toEqual(['写入失败，请重试'])
    expect(button(root, '保存设置').props.disabled).toBe(false)
    root = await select(root, digits.id)
    expect(navigation(root).props.gameId).toBe(digits.id)
    expect(grid(root).props.items).toEqual(limitsFor(digits).items)
    expect(alerts(root, 'error')).toEqual([])
  })

  it('marks numeric edits as pending, restores saved metadata on revert, and never saves an unchanged form', async () => {
    let root = await ready()
    const original = limitsFor(racing).items
    button(root, '保存设置').props.onClick!()
    expect(runtime.updateOddsLimits).not.toHaveBeenCalled()
    const edited = original.map((item, index) => index === 0 ? { ...item, odds: 8.5 } : item)
    grid(root).props.onChange!(edited)
    root = render()
    expect(chips(root)).toContain('1 项未保存')
    expect(grid(root).props.items![0]).toMatchObject({ odds: 8.5, configured: false, configuration_source: 'pending_admin_save' })
    expect(button(root, '保存设置').props.disabled).toBe(false)
    grid(root).props.onChange!(original)
    root = render()
    expect(grid(root).props.items).toEqual(original)
    expect(chips(root)).not.toContain('1 项未保存')
    expect(button(root, '保存设置').props.disabled).toBe(true)
  })

  it('sends the loaded rule version and revision for clear, leaves every price zero and has no default-price sync', async () => {
    let root = await ready()
    expect(button(root, '补全彩种')).toBeUndefined()
    expect(button(root, '恢复当前')).toBeUndefined()
    button(root, '清空当前').props.onClick!()
    root = await settle()
    expect(runtime.resetOddsLimits).toHaveBeenCalledExactlyOnceWith(racing.id, guardFor(racing))
    expect(runtime.confirm).toHaveBeenCalledWith(expect.stringContaining('全部平台赔率及房间、会员覆盖'))
    expect(grid(root).props.items!.every(item => item.odds === 0 && item.configured === false)).toBe(true)
    expect(chips(root)).toContain('赔率待配置')
    expect(chips(root)).not.toContain('运行中')
    expect(runtime.showMessage).toHaveBeenCalledWith('极速赛车 赔率已清空，全部玩法暂停受理')
  })

  it('fails closed if the server omits configuration revision information', async () => {
    runtime.oddsLimits.mockResolvedValue({ ...limitsFor(racing), config_revision: '' })
    let root = await ready()
    expect(grid(root).props.disabled).toBe(true)
    expect(button(root, '保存设置').props.disabled).toBe(true)
    expect(button(root, '清空当前').props.disabled).toBe(true)
    grid(root).props.onChange!([])
    button(root, '清空当前').props.onClick!()
    root = render()
    expect(grid(root).props.items).toEqual(limitsFor(racing).items)
    expect(alerts(root, 'warning').join('')).toContain('配置版本信息缺失')
    expect(runtime.updateOddsLimits).not.toHaveBeenCalled()
    expect(runtime.resetOddsLimits).not.toHaveBeenCalled()
  })

  it.each(['ODDS_CONFIGURATION_CONFLICT', 'RULE_VERSION_CONFLICT'])('preserves draft on %s and requires an explicit refresh before further writes', async code => {
    runtime.updateOddsLimits.mockRejectedValue(Object.assign(new Error('配置版本冲突，请刷新'), { status: 409, code }))
    let root = await ready()
    const edited = limitsFor(racing).items.map(item => ({ ...item, max_bet: 1200 }))
    grid(root).props.onChange!(edited)
    root = render()
    button(root, '保存设置').props.onClick!()
    root = await settle()
    expect(grid(root).props.items).toEqual(oddsDraftItems(edited, limitsFor(racing).items))
    expect(alerts(root, 'warning').join('')).toContain('你的草稿仍保留')
    expect(button(root, '保存设置').props.disabled).toBe(true)
    expect(button(root, '清空当前').props.disabled).toBe(true)
    button(root, '保存设置').props.onClick!()
    button(root, '清空当前').props.onClick!()
    expect(runtime.updateOddsLimits).toHaveBeenCalledTimes(1)
    expect(runtime.resetOddsLimits).not.toHaveBeenCalled()
    runtime.confirm.mockReturnValueOnce(false)
    button(root, '刷新配置').props.onClick!()
    expect(runtime.oddsLimits).toHaveBeenCalledTimes(1)
    const latest = { ...limitsFor(racing), config_revision: 'latest-server-revision' }
    runtime.oddsLimits.mockResolvedValueOnce(latest)
    button(root, '刷新配置').props.onClick!()
    root = await settle()
    expect(grid(root).props.items).toEqual(latest.items)
    expect(alerts(root, 'warning')).toEqual([])
    expect(chips(root)).not.toContain('4 项未保存')
    grid(root).props.onChange!(edited)
    root = render()
    runtime.updateOddsLimits.mockResolvedValueOnce({ ...latest, items: persisted(edited), config_revision: 'saved-after-refresh' })
    button(root, '保存设置').props.onClick!()
    root = await settle()
    expect(runtime.updateOddsLimits).toHaveBeenLastCalledWith(racing.id, { ...inputFor(racing, edited), expected_revision: 'latest-server-revision' })
    expect(grid(root).props.items).toEqual(persisted(edited))
  })

  it('protects same-frame edits from game changes, refresh, page navigation and window close', async () => {
    let root = await ready()
    const edited = limitsFor(racing).items.map(item => ({ ...item, max_bet: 1200 }))
    grid(root).props.onChange!(edited)
    runtime.confirm.mockReturnValue(false)
    navigation(root).props.onSelect!(digits.id)
    button(root, '刷新配置').props.onClick!()
    root = render()
    expect(navigation(root).props.gameId).toBe(racing.id)
    expect(runtime.oddsLimits).toHaveBeenCalledTimes(1)
    expect(grid(root).props.items).toEqual(oddsDraftItems(edited, limitsFor(racing).items))
    const leave = new Event('yaotu-before-navigate', { cancelable: true })
    runtime.listeners.get('yaotu-before-navigate')!(leave)
    expect(leave.defaultPrevented).toBe(true)
    const unload = new Event('beforeunload', { cancelable: true })
    runtime.listeners.get('beforeunload')!(unload)
    expect(unload.defaultPrevented).toBe(true)
    runtime.confirm.mockReturnValue(true)
    root = await select(root, digits.id)
    expect(navigation(root).props.gameId).toBe(digits.id)
    const cleanUnload = new Event('beforeunload', { cancelable: true })
    runtime.listeners.get('beforeunload')!(cleanUnload)
    expect(cleanUnload.defaultPrevented).toBe(false)
  })

  it('rejects invalid limit ordering before submitting and keeps the edited value visible', async () => {
    let root = await ready()
    const edited = limitsFor(racing).items.map((item, index) => index === 0 ? { ...item, max_bet: 6000 } : item)
    grid(root).props.onChange!(edited)
    root = render()
    button(root, '保存设置').props.onClick!()
    root = render()
    expect(runtime.updateOddsLimits).not.toHaveBeenCalled()
    expect(alerts(root, 'error').join('')).toContain('单注最高不能高于会员单期')
    expect(grid(root).props.items![0].max_bet).toBe(6000)
  })

  it('wires selected-row batch editing without writing to the API or changing unselected rows', () => {
    const onChange = vi.fn()
    const props = { items: limitsFor(racing).items, catalog: Object.fromEntries(racingCatalog.map(item => [item.play_code, item])), onChange }
    const renderGrid = () => runtime.hooks!.render(() => PlatformOddsGrid(props))
    let root = renderGrid()
    ofType(root, Checkbox).find(item => item.props.inputProps?.['aria-label'] === '选择指定名次号码')!.props.onChange!({ target: { value: '' } }, true)
    root = renderGrid()
    ofType(root, TextField).find(item => item.props.label === '批量数值')!.props.onChange!({ target: { value: '8.7654' } })
    root = renderGrid()
    button(root, '应用到所选').props.onClick!()
    expect(onChange).toHaveBeenCalledOnce()
    expect(onChange.mock.calls[0][0].map((item: PlayLimitItem) => item.odds)).toEqual([8.7654, 1.993, 1.993, 1.993])
    expect(runtime.updateOddsLimits).not.toHaveBeenCalled()
    expect(runtime.resetOddsLimits).not.toHaveBeenCalled()
  })
})
