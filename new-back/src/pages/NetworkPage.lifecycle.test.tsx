import { Alert, Button, Chip, Collapse, Pagination, Tabs, TextField } from '@mui/material'
import { isValidElement, type ComponentProps, type DependencyList, type ReactElement, type ReactNode, type SetStateAction } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { FeedStatus } from '../api'
import type { SourceDiagnosticSource, SourceDiagnostics, SourceProbeResult } from '../sourceDiagnostics'
import { NetworkPage, SourceDiagnosticCard } from './NetworkPage'

// Exercise the real page's hooks and callbacks without mounting MUI or requiring a browser.
type Slot = { value?: unknown; deps?: DependencyList; effect?: () => void | (() => void); cleanup?: void | (() => void) }
const sameDeps = (a?: DependencyList, b?: DependencyList) => Boolean(a && b && a.length === b.length && a.every((item, index) => Object.is(item, b[index])))
class Harness {
  private slots: Slot[] = []
  private cursor = 0
  private effects: number[] = []
  private mounted = true
  writesAfterUnmount = 0
  render<T>(factory: () => T) { this.cursor = 0; return factory() }
  useState<T>(initial: T | (() => T)): [T, (next: SetStateAction<T>) => void] {
    const slot = this.slots[this.cursor++] ??= { value: typeof initial === 'function' ? (initial as () => T)() : initial }
    return [slot.value as T, next => {
      if (!this.mounted) this.writesAfterUnmount++
      slot.value = typeof next === 'function' ? (next as (current: T) => T)(slot.value as T) : next
    }]
  }
  useRef<T>(initial: T) { const slot = this.slots[this.cursor++] ??= { value: { current: initial } }; return slot.value as { current: T } }
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
      this.slots[index] = { deps, effect }
      this.effects.push(index)
    }
  }
  flushEffects() { for (const index of this.effects.splice(0)) this.slots[index].cleanup = this.slots[index].effect?.() }
  unmount() {
    this.mounted = false
    for (const slot of this.slots) { slot.cleanup?.(); slot.cleanup = undefined }
  }
}

const runtime = vi.hoisted(() => ({
  hooks: null as Harness | null,
  sourceDiagnostics: vi.fn(), probeSource: vi.fn(), feedStatus: vi.fn(), games: vi.fn(), syncOfficialSources: vi.fn(),
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
  sourceDiagnostics: runtime.sourceDiagnostics, probeSource: runtime.probeSource, feedStatus: runtime.feedStatus,
  games: runtime.games, syncOfficialSources: runtime.syncOfficialSources,
} }))

type Props = {
  children?: ReactNode; action?: ReactNode; label?: string; value?: unknown; disabled?: boolean; severity?: string; color?: string; in?: boolean; unmountOnExit?: boolean
  'aria-label'?: string; 'aria-expanded'?: boolean; 'data-testid'?: string
  ref?: { current: Element | null } | ((element: Element | null) => void)
  onClick?: () => void; onChange?: (event: unknown, value: string | number) => void
}
function elements(node: ReactNode): ReactElement<Props>[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  return isValidElement<Props>(node) ? [node, ...elements(node.props.children), ...elements(node.props.action)] : []
}
function text(node: ReactNode): string {
  if (Array.isArray(node)) return node.map(text).join('')
  if (isValidElement<Props>(node)) return text(node.props.children) + text(node.props.action) + (node.props.label || '')
  return typeof node === 'string' || typeof node === 'number' ? String(node) : ''
}
type SourceCardElement = ReactElement<ComponentProps<typeof SourceDiagnosticCard>>
const sources: SourceDiagnosticSource[] = [
  { key: '168:10037', name: '极速赛车现用源', provider: '168', groups: ['赛车'], candidate: false, relation: 'production', game_ids: ['speed-racing'], endpoint: 'https://current.example/draw', upstream_game_id: 10037, warning: '', warning_persistent: false },
  { key: '163:160', name: '极速赛车候选源', provider: '163', groups: ['官方彩', '赛车'], candidate: true, relation: 'unverified_candidate', game_ids: ['speed-racing'], endpoint: 'https://candidate.example/draw', upstream_game_id: 160, warning: '历史核查曾发现数据延迟', warning_persistent: false, warning_checked_at: '2026-09-03T06:00:00Z' },
]
const snapshot: SourceDiagnostics = {
  games: [{
    game_id: 'speed-racing', name: '极速赛车', enabled: true, category: 'racing', lobby_category: '彩票', rule_version: 'racing-v2',
    rules_message: '10个号码，按名次结算。', source_kind: 'external', source_name: '168开奖网', source_key: sources[0].key,
    source_163_status: 'candidate_unverified', source_163_message: '163候选尚未通过身份与日程验收',
    sync_status: 'ok', last_sync_at: '2026-09-04T06:00:00Z', last_sync_error: '', next_issue: '20260904002', next_draw_at: '2026-09-04T06:05:00Z',
  }],
  catalog: sources,
}
const result = (sourceKey = sources[0].key, override: Partial<SourceProbeResult> = {}): SourceProbeResult => ({
  source_key: sourceKey, status: 'success', checked_at: '2026-09-04T06:00:01Z', duration_ms: 123,
  http_status: 200, issue: '20260904001', draw_at: '2026-09-04T06:00:00Z', numbers: [3, 1, 4, 2, 5, 7, 8, 10, 6, 9], history_count: 5,
  message: '号码校验通过；本次仅读取，不导入。', ...override,
})
const feed: FeedStatus = {
  running: true, server_time: '2026-09-04T06:00:00Z', server_time_ms: Date.parse('2026-09-04T06:00:00Z'), timezone: 'Asia/Shanghai',
  jobs: [{ id: 'racing-feed', name: '现用赛车同步任务', group: 'racing', game_ids: ['speed-racing'], timezone: 'Asia/Shanghai', mode: 'normal', running: false,
    last_success_at: '2026-09-04T06:00:00Z', next_run_at: '2026-09-04T06:05:00Z', imported: 1, latest_issue: '20260904001', consecutive_errors: 0, last_error: '' }],
}
const deferred = <T,>() => {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((accept, fail) => { resolve = accept; reject = fail })
  return { promise, resolve, reject }
}

describe('network source diagnostics lifecycle', () => {
  let documentEvents: EventTarget & { visibilityState: string }
  let committedRef: Props['ref']
  const sentinelElement = { id: 'mock-load-more-sentinel' } as unknown as Element
  let observers: ObserverMock[]
  class ObserverMock {
    readonly callback: IntersectionObserverCallback
    readonly options?: IntersectionObserverInit
    observe = vi.fn()
    disconnect = vi.fn()
    constructor(callback: IntersectionObserverCallback, options?: IntersectionObserverInit) {
      this.callback = callback
      this.options = options
      observers.push(this)
    }
    emit(isIntersecting = true) {
      this.callback([{ isIntersecting, target: sentinelElement } as IntersectionObserverEntry], this as unknown as IntersectionObserver)
    }
  }
  const commitRef = (ref: Props['ref'], value: Element | null) => {
    if (typeof ref === 'function') ref(value)
    else if (ref) ref.current = value
  }
  const render = () => {
    const root = runtime.hooks!.render(NetworkPage)
    const ref = elements(root).find(node => node.props['data-testid'] === 'source-diagnostics-load-more')?.props.ref
    if (committedRef !== ref) commitRef(committedRef, null)
    commitRef(ref, sentinelElement)
    committedRef = ref
    runtime.hooks!.flushEffects()
    return root
  }
  const ready = async () => { render(); await vi.advanceTimersByTimeAsync(0); return render() }
  const button = (label: string, root: ReactNode = render()) => elements(root).find(node => node.type === Button && text(node) === label)!
  const sourceCards = (root: ReactNode = render()) => elements(root).filter(node => node.type === SourceDiagnosticCard) as unknown as SourceCardElement[]
  const sourceCard = (key = sources[0].key) => sourceCards().find(node => node.props.source.key === key)!
  const sourceView = (key = sources[0].key) => SourceDiagnosticCard(sourceCard(key).props)
  const collapse = (key = sources[0].key) => elements(sourceView(key)).find(node => node.type === Collapse)!
  const selectTab = (tab: 'games' | 'catalog' | 'jobs') => {
    elements(render()).find(node => node.type === Tabs)!.props.onChange!(null, tab)
    return render()
  }
  const selectField = (label: string, value: string) => {
    elements(render()).find(node => node.type === TextField && node.props.label === label)!.props.onChange!({ target: { value } }, '')
    return render()
  }
  const lastObserver = () => observers.at(-1)!
  const displayedCount = (shown: number, total: number) => expect(text(render()).replace(/\s+/g, '')).toContain(`已显示${shown}/${total}项`)
  const directory = (count: number): SourceDiagnosticSource[] => Array.from({ length: count }, (_, index) => ({
    ...sources[1], key: `163:${index + 1}`, name: `目录来源 ${index + 1}`, upstream_game_id: index + 1, game_ids: [],
  }))
  const details = (key = sources[0].key) => {
    const child = collapse(key).props.children as ReactElement<{ result: SourceProbeResult }>
    return (child.type as (props: { result: SourceProbeResult }) => ReactElement)(child.props)
  }
  const probe = (key = sources[0].key) => {
    const card = sourceCard(key)
    card.props.onTest(card.props.source)
  }

  beforeEach(() => {
    vi.useFakeTimers({ now: Date.parse('2026-09-04T06:00:00Z') })
    runtime.hooks = new Harness()
    runtime.sourceDiagnostics.mockReset().mockResolvedValue(snapshot)
    runtime.probeSource.mockReset().mockImplementation((key: string) => Promise.resolve(result(key)))
    runtime.feedStatus.mockReset().mockResolvedValue(feed)
    runtime.games.mockReset()
    runtime.syncOfficialSources.mockReset()
    observers = []
    committedRef = undefined
    documentEvents = Object.assign(new EventTarget(), { visibilityState: 'visible' })
    vi.stubGlobal('document', documentEvents)
    vi.stubGlobal('IntersectionObserver', ObserverMock)
    vi.stubGlobal('window', { setTimeout, clearTimeout, setInterval, clearInterval, IntersectionObserver: ObserverMock })
  })
  afterEach(() => {
    runtime.hooks?.unmount()
    vi.clearAllTimers(); vi.unstubAllGlobals(); vi.useRealTimers()
    // This page must never use the games endpoint or the write/import synchronization endpoint.
    expect(runtime.games).not.toHaveBeenCalled()
    expect(runtime.syncOfficialSources).not.toHaveBeenCalled()
  })

  it('initializes from local diagnostics only and never automatically probes or starts synchronization', async () => {
    const root = await ready()
    expect(runtime.sourceDiagnostics).toHaveBeenCalledTimes(1)
    expect(runtime.sourceDiagnostics.mock.calls[0][0]).toBeInstanceOf(AbortSignal)
    expect(runtime.probeSource).not.toHaveBeenCalled()
    expect(runtime.feedStatus).not.toHaveBeenCalled()
    expect(text(root)).toContain('163当前88项目录快照')
    expect(text(root)).toContain('当前彩种 1来源目录 2同步异常 0需核查来源 1163目录未找到同款 0163候选待验 1163有ID但过期/空 0本页会话已测 0')
    expect(sourceCards(root)).toHaveLength(2)
    expect(sourceCards(root).map(card => card.props.current)).toEqual([true, false])
    for (const card of sourceCards(root)) {
      expect(card.props.result).toBeUndefined()
      expect(card.props.expanded).toBe('')
    }
    render(); render()
    expect(runtime.sourceDiagnostics).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(30_000)
    expect(runtime.sourceDiagnostics).toHaveBeenCalledTimes(2)
    expect(runtime.probeSource).not.toHaveBeenCalled()
    expect(runtime.feedStatus).not.toHaveBeenCalled()
  })

  it('keeps failed initial configuration unknown instead of claiming healthy zeroes', async () => {
    runtime.sourceDiagnostics.mockRejectedValueOnce(new Error('配置服务不可达'))
    const root = await ready()
    expect(text(root)).toContain('配置服务不可达；来源状态未知，请重试。')
    expect(text(root)).toContain('当前彩种 —来源目录 —同步异常 未知需核查来源 未知')
    expect(sourceCards(root)).toHaveLength(0)
    expect(button('刷新配置').props.disabled).toBe(false)
    expect(runtime.probeSource).not.toHaveBeenCalled()
    expect(runtime.feedStatus).not.toHaveBeenCalled()
  })

  it('starts with 12 catalog entries and appends 12 on intersection without fetching upstreams', async () => {
    const catalog = directory(25)
    runtime.sourceDiagnostics.mockResolvedValue({ ...snapshot, catalog })
    await ready()
    expect(sourceCards(selectTab('catalog'))).toHaveLength(12)
    expect(sourceCards().map(card => card.props.source.key)).toEqual(catalog.slice(0, 12).map(source => source.key))
    expect(elements(render()).some(node => node.type === Pagination)).toBe(false)
    displayedCount(12, 25)
    const first = lastObserver()
    expect(first.options?.rootMargin).toBe('240px 0px')
    expect(first.observe).toHaveBeenCalledWith(sentinelElement)
    first.emit(false)
    expect(sourceCards()).toHaveLength(12)
    first.emit()
    first.emit()
    expect(sourceCards().map(card => card.props.source.key)).toEqual(catalog.slice(0, 24).map(source => source.key))
    displayedCount(24, 25)
    expect(first.disconnect).toHaveBeenCalled()
    lastObserver().emit()
    expect(sourceCards().map(card => card.props.source.key)).toEqual(catalog.map(source => source.key))
    displayedCount(25, 25)
    expect(button('加载更多')).toBeUndefined()
    expect(text(render())).toContain('已全部加载')
    expect(runtime.sourceDiagnostics).toHaveBeenCalledTimes(1)
    expect(runtime.probeSource).not.toHaveBeenCalled()
    expect(runtime.feedStatus).not.toHaveBeenCalled()
  })

  it('keeps a load-more button as a fallback when IntersectionObserver is unavailable', async () => {
    vi.stubGlobal('IntersectionObserver', undefined)
    vi.stubGlobal('window', { setTimeout, clearTimeout, setInterval, clearInterval })
    runtime.sourceDiagnostics.mockResolvedValue({ ...snapshot, catalog: directory(25) })
    await ready(); selectTab('catalog')
    expect(sourceCards()).toHaveLength(12)
    button('加载更多').props.onClick!()
    expect(sourceCards()).toHaveLength(24)
    button('加载更多').props.onClick!()
    expect(sourceCards()).toHaveLength(25)
    displayedCount(25, 25)
    expect(observers).toHaveLength(0)
    expect(runtime.sourceDiagnostics).toHaveBeenCalledTimes(1)
    expect(runtime.probeSource).not.toHaveBeenCalled()
  })

  it.each([
    ['搜索彩种、玩法或来源ID', '目录来源'], ['提供方', '163'], ['检查状态', 'abnormal'],
  ])('resets to 12 when changing %s and ignores the previous observer callback', async (label, value) => {
    runtime.sourceDiagnostics.mockResolvedValue({ ...snapshot, catalog: directory(37) })
    await ready(); selectTab('catalog')
    lastObserver().emit()
    expect(sourceCards()).toHaveLength(24)
    const old = lastObserver()
    selectField(label, value)
    expect(sourceCards()).toHaveLength(12)
    expect(old.disconnect).toHaveBeenCalledTimes(1)
    old.emit()
    expect(sourceCards()).toHaveLength(12)
    lastObserver().emit()
    expect(sourceCards()).toHaveLength(24)
    expect(runtime.sourceDiagnostics).toHaveBeenCalledTimes(1)
    expect(runtime.probeSource).not.toHaveBeenCalled()
  })

  it('resets tab lists to 12 and rejects old observer callbacks after tab cleanup or unmount', async () => {
    const catalog = directory(25).map((source, index) => ({ ...source, game_ids: [`game-${index}`] }))
    const games = catalog.map((source, index) => ({ ...snapshot.games[0], game_id: `game-${index}`, name: `彩种 ${index}`, source_key: source.key }))
    runtime.sourceDiagnostics.mockResolvedValue({ games, catalog })
    await ready()
    expect(sourceCards()).toHaveLength(12)
    lastObserver().emit()
    expect(sourceCards()).toHaveLength(24)
    const gameObserver = lastObserver()
    selectTab('catalog')
    expect(sourceCards()).toHaveLength(12)
    gameObserver.emit()
    expect(sourceCards()).toHaveLength(12)
    lastObserver().emit()
    expect(sourceCards()).toHaveLength(24)
    const catalogObserver = lastObserver()
    selectTab('games')
    expect(sourceCards()).toHaveLength(12)
    catalogObserver.emit()
    expect(sourceCards()).toHaveLength(12)
    const finalObserver = lastObserver()
    runtime.hooks!.unmount()
    expect(finalObserver.disconnect).toHaveBeenCalledTimes(1)
    finalObserver.emit()
    expect(runtime.hooks!.writesAfterUnmount).toBe(0)
    expect(runtime.probeSource).not.toHaveBeenCalled()
  })

  it('preserves the revealed count and expanded probe result across refresh and manual tests', async () => {
    const catalog = directory(37)
    runtime.sourceDiagnostics.mockResolvedValue({ ...snapshot, catalog })
    await ready(); selectTab('catalog')
    lastObserver().emit()
    expect(sourceCards()).toHaveLength(24)
    probe(catalog[20].key)
    await vi.advanceTimersByTimeAsync(0)
    expect(sourceCards()).toHaveLength(24)
    expect(collapse(catalog[20].key).props.in).toBe(true)
    runtime.sourceDiagnostics.mockResolvedValue({ ...snapshot, catalog: directory(38) })
    button('刷新配置').props.onClick!()
    await vi.advanceTimersByTimeAsync(0)
    expect(sourceCards()).toHaveLength(24)
    displayedCount(24, 38)
    expect(collapse(catalog[20].key).props.in).toBe(true)
    expect(sourceCard(catalog[20].key).props.result).toEqual(result(catalog[20].key))
    await vi.advanceTimersByTimeAsync(30_000)
    expect(sourceCards()).toHaveLength(24)
    expect(runtime.probeSource).toHaveBeenCalledTimes(1)
    expect(runtime.sourceDiagnostics).toHaveBeenCalledTimes(3)
  })

  it('tests only the clicked card, forwards cancellation, and expands the returned raw result', async () => {
    await ready()
    const slow = deferred<SourceProbeResult>()
    runtime.probeSource.mockReturnValueOnce(slow.promise)
    button('点击测试', sourceView()).props.onClick!()
    expect(runtime.probeSource).toHaveBeenCalledTimes(1)
    expect(runtime.probeSource.mock.calls[0][0]).toBe(sources[0].key)
    expect(runtime.probeSource.mock.calls[0][1]).toBeInstanceOf(AbortSignal)
    expect(runtime.probeSource.mock.calls[0][1].aborted).toBe(false)
    expect(sourceCard().props.testing).toBe(sources[0].key)
    expect(button('测试中', sourceView()).props.disabled).toBe(true)
    expect(button('点击测试', sourceView(sources[1].key)).props.disabled).toBe(true)
    slow.resolve(result())
    await vi.advanceTimersByTimeAsync(0)
    expect(sourceCard().props.result).toEqual(result())
    expect(sourceCard().props.testing).toBe('')
    expect(collapse().props.in).toBe(true)
    expect(collapse().props.unmountOnExit).toBe(true)
    expect(text(sourceView())).toContain('获取成功')
    expect(text(details())).toContain('耗时 123 ms · HTTP 200 · 历史返回 5 期')
    expect(text(details())).toContain('最新期号：20260904001')
    const raw = elements(details()).find(node => node.props['aria-label'] === '测试返回的原始号码')!
    expect(elements(raw.props.children).map(node => node.props.children)).toEqual(result().numbers)
    expect(text(render())).toContain('本页会话已测 1')
    expect(sourceCard(sources[1].key).props.result).toBeUndefined()
    button('收起结果', sourceView()).props.onClick!()
    expect(collapse().props.in).toBe(false)
    button('查看结果', sourceView()).props.onClick!()
    expect(collapse().props.in).toBe(true)
    expect(runtime.probeSource).toHaveBeenCalledTimes(1)
  })

  it('synchronously rejects duplicate clicks and other card tests before a re-render', async () => {
    await ready()
    const slow = deferred<SourceProbeResult>()
    runtime.probeSource.mockReturnValueOnce(slow.promise)
    const first = sourceCard()
    const second = sourceCard(sources[1].key)
    first.props.onTest(first.props.source)
    first.props.onTest(first.props.source)
    second.props.onTest(second.props.source)
    expect(runtime.probeSource).toHaveBeenCalledTimes(1)
    slow.resolve(result())
    await vi.advanceTimersByTimeAsync(0)
    probe(sources[1].key)
    await vi.advanceTimersByTimeAsync(0)
    expect(runtime.probeSource).toHaveBeenCalledTimes(2)
    expect(text(render())).toContain('本页会话已测 2')
    expect(collapse().props.in).toBe(false)
    expect(collapse(sources[1].key).props.in).toBe(true)
  })

  it.each([
    ['empty', '返回空数据', { numbers: [], history_count: 0, issue: null, draw_at: null, message: '上游没有返回开奖记录' }],
    ['stale', '开奖数据过期', { draw_at: '2026-07-01T06:00:00Z', message: '最近开奖已经超过该来源允许的周期' }],
    ['error', '请求或数据异常', { http_status: 403, numbers: [], history_count: 0, message: '上游拒绝当前读取请求' }],
  ] as const)('preserves an explicit %s outcome instead of treating HTTP completion as success', async (status, label, override) => {
    await ready()
    const response = result(sources[1].key, { status, ...override, numbers: 'numbers' in override ? [...override.numbers] : result().numbers })
    runtime.probeSource.mockResolvedValueOnce(response)
    probe(sources[1].key)
    await vi.advanceTimersByTimeAsync(0)
    expect(sourceCard(sources[1].key).props.result).toEqual(response)
    expect(collapse(sources[1].key).props.in).toBe(true)
    expect(text(sourceView(sources[1].key))).toContain(label)
    expect(text(sourceView(sources[1].key))).not.toContain('获取成功')
    expect(text(details(sources[1].key))).toContain(override.message)
    if (status === 'empty') expect(text(details(sources[1].key))).toContain('未获取到号码')
    await vi.advanceTimersByTimeAsync(60_000)
    expect(runtime.probeSource).toHaveBeenCalledTimes(1)
  })

  it('does not present a successful connection as clearing a persistent identity warning', () => {
    const risky = { ...sources[1], relation: 'different_product' as const, warning_persistent: true }
    const view = SourceDiagnosticCard({
      source: risky, result: result(risky.key), testing: '', expanded: '', onTest: vi.fn(), onExpand: vi.fn(),
    })
    const chip = elements(view).find(node => node.type === Chip && node.props.label === '接口可达 · 风险仍在')!
    expect(chip.props.color).toBe('warning')
    expect(text(view)).toContain(risky.warning)
    expect(text(view)).not.toContain('获取成功')
  })

  it('reports rejected requests with no fabricated issue, numbers, or HTTP status', async () => {
    await ready()
    runtime.probeSource.mockRejectedValueOnce(new Error('请求超时，请重试'))
    probe()
    await vi.advanceTimersByTimeAsync(0)
    expect(sourceCard().props.result).toMatchObject({
      source_key: sources[0].key, status: 'error', message: '请求超时，请重试', http_status: null, issue: null, draw_at: null, numbers: [], history_count: 0,
    })
    expect(collapse().props.in).toBe(true)
    expect(text(details())).toContain('HTTP —')
    expect(text(details())).toContain('未获取到号码')
    expect(sourceCard().props.testing).toBe('')
  })

  it('rejects a mismatched source response without attaching it to either source as success', async () => {
    await ready()
    runtime.probeSource.mockResolvedValueOnce(result(sources[1].key))
    probe()
    await vi.advanceTimersByTimeAsync(0)
    expect(sourceCard().props.result).toMatchObject({ source_key: sources[0].key, status: 'error', message: '测试返回的来源与请求不一致', numbers: [] })
    expect(sourceCard(sources[1].key).props.result).toBeUndefined()
  })

  it.each(['resolve', 'reject'] as const)('ignores a canceled probe that later %ss while allowing a replacement test', async outcome => {
    await ready()
    const old = deferred<SourceProbeResult>()
    const replacement = deferred<SourceProbeResult>()
    runtime.probeSource.mockReturnValueOnce(old.promise).mockReturnValueOnce(replacement.promise)
    probe()
    const oldSignal: AbortSignal = runtime.probeSource.mock.calls[0][1]
    button('取消测试').props.onClick!()
    expect(oldSignal.aborted).toBe(true)
    expect(sourceCard().props.testing).toBe('')
    probe(sources[1].key)
    expect(runtime.probeSource).toHaveBeenCalledTimes(2)
    if (outcome === 'resolve') old.resolve(result())
    else old.reject(new Error('已取消的请求迟到失败'))
    await vi.advanceTimersByTimeAsync(0)
    expect(sourceCard().props.result).toBeUndefined()
    expect(sourceCard().props.testing).toBe(sources[1].key)
    expect(sourceCard().props.expanded).toBe('')
    expect(text(render())).not.toContain('已取消的请求迟到失败')
    probe()
    expect(runtime.probeSource).toHaveBeenCalledTimes(2)
    replacement.resolve(result(sources[1].key))
    await vi.advanceTimersByTimeAsync(0)
    expect(sourceCard(sources[1].key).props.result).toEqual(result(sources[1].key))
    expect(sourceCard().props.testing).toBe('')
    expect(text(render())).toContain('本页会话已测 1')
  })

  it('retains the previous result if a retest is canceled, including after a late response', async () => {
    await ready(); probe(); await vi.advanceTimersByTimeAsync(0)
    const slow = deferred<SourceProbeResult>()
    runtime.probeSource.mockReturnValueOnce(slow.promise)
    probe()
    button('取消测试').props.onClick!()
    slow.resolve(result(sources[0].key, { status: 'stale', issue: 'wrong-late-issue' }))
    await vi.advanceTimersByTimeAsync(0)
    expect(sourceCard().props.result).toEqual(result())
    expect(collapse().props.in).toBe(true)
    expect(text(sourceView())).not.toContain('wrong-late-issue')
  })

  it.each(['resolve', 'reject'] as const)('aborts configuration and probe requests on unmount and ignores late %s', async outcome => {
    await ready()
    const config = deferred<SourceDiagnostics>()
    const test = deferred<SourceProbeResult>()
    runtime.sourceDiagnostics.mockReturnValueOnce(config.promise)
    runtime.probeSource.mockReturnValueOnce(test.promise)
    button('刷新配置').props.onClick!()
    probe()
    const configSignal: AbortSignal = runtime.sourceDiagnostics.mock.calls[1][0]
    const testSignal: AbortSignal = runtime.probeSource.mock.calls[0][1]
    runtime.hooks!.unmount()
    expect(configSignal.aborted).toBe(true)
    expect(testSignal.aborted).toBe(true)
    if (outcome === 'resolve') { config.resolve({ games: [], catalog: [] }); test.resolve(result()) }
    else { config.reject(new Error('late configuration error')); test.reject(new Error('late probe error')) }
    await vi.advanceTimersByTimeAsync(90_000)
    expect(runtime.hooks!.writesAfterUnmount).toBe(0)
    expect(runtime.sourceDiagnostics).toHaveBeenCalledTimes(2)
    expect(runtime.probeSource).toHaveBeenCalledTimes(1)
    expect(runtime.feedStatus).not.toHaveBeenCalled()
  })

  it('clears scheduled initial reads if unmounted before the first timer fires', async () => {
    render()
    runtime.hooks!.unmount()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(runtime.sourceDiagnostics).not.toHaveBeenCalled()
    expect(runtime.feedStatus).not.toHaveBeenCalled()
    expect(runtime.probeSource).not.toHaveBeenCalled()
    expect(runtime.hooks!.writesAfterUnmount).toBe(0)
  })

  it('loads synchronization jobs only after selecting that tab and stops polling on departure', async () => {
    await ready()
    selectTab('catalog')
    await vi.advanceTimersByTimeAsync(30_000)
    expect(runtime.feedStatus).not.toHaveBeenCalled()
    selectTab('jobs')
    await vi.advanceTimersByTimeAsync(0)
    expect(runtime.feedStatus).toHaveBeenCalledTimes(1)
    const signal: AbortSignal = runtime.feedStatus.mock.calls[0][0]
    expect(signal).toBeInstanceOf(AbortSignal)
    expect(text(render())).toContain('调度服务运行中')
    expect(text(render())).toContain('现用赛车同步任务')
    expect(text(render())).toContain('等待调度')
    expect(sourceCards()).toHaveLength(0)
    selectTab('games')
    expect(signal.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(60_000)
    expect(runtime.feedStatus).toHaveBeenCalledTimes(1)
    expect(runtime.probeSource).not.toHaveBeenCalled()
  })

  it('does not let a previous jobs-tab request overwrite a newer tab visit', async () => {
    await ready()
    const old = deferred<FeedStatus>()
    runtime.feedStatus.mockReturnValueOnce(old.promise)
    selectTab('jobs'); await vi.advanceTimersByTimeAsync(0)
    const oldSignal: AbortSignal = runtime.feedStatus.mock.calls[0][0]
    selectTab('games')
    expect(oldSignal.aborted).toBe(true)
    selectTab('jobs'); await vi.advanceTimersByTimeAsync(0)
    old.resolve({ ...feed, running: false, jobs: [{ ...feed.jobs[0], name: '旧请求不应显示', last_error: '旧错误' }] })
    await vi.advanceTimersByTimeAsync(0)
    expect(text(render())).toContain('调度服务运行中')
    expect(text(render())).not.toContain('旧请求不应显示')
    expect(text(render())).not.toContain('旧错误')
    expect(runtime.feedStatus).toHaveBeenCalledTimes(2)
  })

  it('aborts jobs on unmount and suppresses their late failure and future polls', async () => {
    await ready()
    const slow = deferred<FeedStatus>()
    runtime.feedStatus.mockReturnValueOnce(slow.promise)
    selectTab('jobs'); await vi.advanceTimersByTimeAsync(0)
    const signal: AbortSignal = runtime.feedStatus.mock.calls[0][0]
    runtime.hooks!.unmount()
    expect(signal.aborted).toBe(true)
    slow.reject(new Error('late jobs failure'))
    await vi.advanceTimersByTimeAsync(60_000)
    expect(runtime.hooks!.writesAfterUnmount).toBe(0)
    expect(runtime.feedStatus).toHaveBeenCalledTimes(1)
    expect(runtime.sourceDiagnostics).toHaveBeenCalledTimes(1)
  })

  it('preserves loaded configuration and probe results after a failed refresh, marking them stale until recovery', async () => {
    await ready(); probe(); await vi.advanceTimersByTimeAsync(0)
    runtime.sourceDiagnostics.mockRejectedValueOnce(new Error('刷新连接失败'))
    button('刷新配置').props.onClick!()
    await vi.advanceTimersByTimeAsync(0)
    const notice = elements(render()).find(node => node.type === Alert && text(node).includes('刷新连接失败'))!
    expect(notice.props.severity).toBe('error')
    expect(text(notice)).toContain('以下为上次读取的配置，非实时状态。')
    expect(text(render())).toContain('当前彩种 1来源目录 2')
    expect(sourceCards()).toHaveLength(2)
    expect(sourceCard().props.result).toEqual(result())
    expect(button('刷新配置').props.disabled).toBe(false)
    await vi.advanceTimersByTimeAsync(30_000)
    expect(text(render())).not.toContain('刷新连接失败')
    expect(sourceCard().props.result).toEqual(result())
    expect(runtime.probeSource).toHaveBeenCalledTimes(1)
  })

  it('keeps the last jobs status visible but marks a failed status refresh stale', async () => {
    await ready()
    selectTab('jobs'); await vi.advanceTimersByTimeAsync(0)
    runtime.feedStatus.mockRejectedValueOnce(new Error('任务状态不可达'))
    await vi.advanceTimersByTimeAsync(30_000)
    expect(text(render())).toContain('任务状态不可达；调度状态可能不是最新。')
    expect(text(render())).toContain('现用赛车同步任务')
    await vi.advanceTimersByTimeAsync(30_000)
    expect(text(render())).not.toContain('任务状态不可达')
    expect(text(render())).toContain('现用赛车同步任务')
  })

  it('renders one upstream as production for derivatives but only a cross-check for official Taiwan Bingo', async () => {
    const official: SourceDiagnosticSource = {
      ...sources[0], key: 'official:official-tw-bingo', name: '台湾彩券宾果', provider: '台湾彩券',
      game_ids: ['official-tw-bingo'], upstream_game_id: undefined,
    }
    const mother: SourceDiagnosticSource = {
      ...sources[1], key: '163:135', name: '163台湾宾果', relation: 'production', warning: '对原始彩券仅作交叉核查', warning_persistent: true,
      game_ids: ['bingo-ssc-2', 'official-tw-bingo'], game_relations: { 'official-tw-bingo': 'cross_check_only' }, upstream_game_id: 135,
    }
    runtime.sourceDiagnostics.mockResolvedValue({
      games: [{ ...snapshot.games[0], game_id: 'official-tw-bingo', name: '台湾宾果', source_key: official.key, source_name: '台湾彩券', source_163_status: 'candidate_unverified', source_163_message: '163仅作交叉核查' }],
      catalog: [official, mother],
    })
    await ready()
    expect(sourceCard(official.key).props.current).toBe(true)
    expect(sourceCard(mother.key).props.relationOverride).toBe('cross_check_only')
    expect(text(sourceView(mother.key))).toContain('仅交叉核查')
    expect(text(sourceView(mother.key))).not.toContain('生产母源组件')
  })

  it('does not count the three verified ID57 Canada products as fixed-directory misses', async () => {
    const canada: SourceDiagnosticSource = {
      ...sources[1], key: '163:57', name: '加拿大28', relation: 'production', candidate: false, warning: '', warning_persistent: false,
      game_ids: ['pc-canada', 'canada-28', 'canada-20'], upstream_game_id: 57,
    }
    const canadaGames = ['pc-canada', 'canada-28', 'canada-20'].map((gameID, index) => ({
      ...snapshot.games[0], game_id: gameID, name: `加拿大玩法${index + 1}`, source_key: null,
      source_163_status: 'verified_candidate' as const, source_163_message: '163 ID57同款候选已核',
    }))
    const fixedDirectoryMiss = {
      ...snapshot.games[0], game_id: 'official-kl8', name: '福彩快乐8', source_key: null,
      source_163_status: 'not_found' as const, source_163_message: '163固定目录未找到原始20球同款',
    }
    runtime.sourceDiagnostics.mockResolvedValue({ games: [...canadaGames, fixedDirectoryMiss], catalog: [canada] })
    const root = await ready()
    expect(text(root)).toContain('163目录未找到同款 1')
    expect(text(root)).toContain('163候选待验 0')
    expect(text(root)).toContain('163有ID但过期/空 0')
  })

  it('skips hidden configuration polls and shares one request between visible polling and manual refresh', async () => {
    await ready()
    documentEvents.visibilityState = 'hidden'
    await vi.advanceTimersByTimeAsync(90_000)
    expect(runtime.sourceDiagnostics).toHaveBeenCalledTimes(1)
    documentEvents.visibilityState = 'visible'
    const slow = deferred<SourceDiagnostics>()
    runtime.sourceDiagnostics.mockReturnValueOnce(slow.promise)
    const click = button('刷新配置').props.onClick!
    click(); click()
    expect(button('刷新配置').props.disabled).toBe(true)
    await vi.advanceTimersByTimeAsync(90_000)
    expect(runtime.sourceDiagnostics).toHaveBeenCalledTimes(2)
    slow.resolve(snapshot)
    await vi.advanceTimersByTimeAsync(0)
    expect(button('刷新配置').props.disabled).toBe(false)
    await vi.advanceTimersByTimeAsync(30_000)
    expect(runtime.sourceDiagnostics).toHaveBeenCalledTimes(3)
    expect(runtime.probeSource).not.toHaveBeenCalled()
    expect(runtime.feedStatus).not.toHaveBeenCalled()
  })

  it('skips hidden jobs reads and never overlaps slow jobs polls', async () => {
    await ready()
    documentEvents.visibilityState = 'hidden'
    selectTab('jobs'); await vi.advanceTimersByTimeAsync(60_000)
    expect(runtime.feedStatus).not.toHaveBeenCalled()
    const slow = deferred<FeedStatus>()
    runtime.feedStatus.mockReturnValueOnce(slow.promise)
    documentEvents.visibilityState = 'visible'
    await vi.advanceTimersByTimeAsync(30_000)
    expect(runtime.feedStatus).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(90_000)
    expect(runtime.feedStatus).toHaveBeenCalledTimes(1)
    slow.resolve(feed); await vi.advanceTimersByTimeAsync(0)
    documentEvents.visibilityState = 'hidden'
    await vi.advanceTimersByTimeAsync(60_000)
    expect(runtime.feedStatus).toHaveBeenCalledTimes(1)
    documentEvents.visibilityState = 'visible'
    await vi.advanceTimersByTimeAsync(30_000)
    expect(runtime.feedStatus).toHaveBeenCalledTimes(2)
    expect(runtime.probeSource).not.toHaveBeenCalled()
  })

  it('updates abnormal and untested filters from manual results without changing the binding', async () => {
    await ready()
    const warningChip = elements(render()).find(node => node.type === Chip && node.props.label === '需核查来源 1')!
    warningChip.props.onClick!()
    expect(sourceCards().map(card => card.props.source.key)).toEqual([sources[1].key])
    probe(sources[1].key); await vi.advanceTimersByTimeAsync(0)
    expect(sourceCards()).toHaveLength(0)
    expect(text(render())).toContain('没有符合筛选条件的记录。')
    const checkState = elements(render()).find(node => node.type === TextField && node.props.label === '检查状态')!
    checkState.props.onChange!({ target: { value: 'untested' } }, '')
    expect(sourceCards().map(card => card.props.source.key)).toEqual([sources[0].key])
    checkState.props.onChange!({ target: { value: 'all' } }, '')
    selectTab('games')
    expect(sourceCard().props.current).toBe(true)
    expect(sourceCard(sources[1].key).props.current).toBe(false)
    expect(sourceCard(sources[1].key).props.source.warning).toBe(sources[1].warning)
    expect(runtime.sourceDiagnostics).toHaveBeenCalledTimes(1)
    expect(runtime.probeSource).toHaveBeenCalledTimes(1)
  })
})
