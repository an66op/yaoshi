import { Box, Button, Checkbox, FormControlLabel, TextField } from '@mui/material'
import { isValidElement, type ComponentProps, type DependencyList, type ReactElement, type ReactNode, type SetStateAction } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { PlayLimitItem } from '../api'
import { oddsDraftItems } from '../oddsEditing'
import { PlatformOddsGrid } from './OddsEditors'
import { PlatformOddsRow, PlatformOddsRowView } from './PlatformOddsRow'

type Slot = { value?: unknown; deps?: DependencyList; effect?: () => void | (() => void); cleanup?: void | (() => void) }
const sameDeps = (left?: DependencyList, right?: DependencyList) => Boolean(left && right && left.length === right.length && left.every((value, index) => Object.is(value, right[index])))
class GridHarness {
  private slots: Slot[] = []
  private cursor = 0
  private layoutEffects = new Set<number>()
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
  useEffect(effect: () => void | (() => void), deps: DependencyList | undefined, layout = false) {
    const index = this.cursor++
    const previous = this.slots[index]
    if (!previous || !sameDeps(previous.deps, deps)) {
      this.slots[index] = { ...previous, effect, deps }
      ;(layout ? this.layoutEffects : this.effects).add(index)
    }
  }
  flushEffects() {
    // Refs are committed before this method. Layout effects publish the latest
    // controlled props before the passive observer effect attaches.
    for (const queue of [this.layoutEffects, this.effects]) {
      for (const index of queue) {
        this.slots[index].cleanup?.()
        this.slots[index].cleanup = this.slots[index].effect?.()
      }
      queue.clear()
    }
  }
  unmount() { this.layoutEffects.clear(); this.effects.clear(); for (const slot of this.slots) slot.cleanup?.() }
}

const runtime = vi.hoisted(() => ({ hooks: null as GridHarness | null }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useMemo: <T,>(factory: () => T, deps: DependencyList) => runtime.hooks!.useMemo(factory, deps),
  useCallback: <T,>(callback: T, deps: DependencyList) => runtime.hooks!.useMemo(() => callback, deps),
  useEffect: (effect: () => void | (() => void), deps?: DependencyList) => runtime.hooks!.useEffect(effect, deps),
  useLayoutEffect: (effect: () => void | (() => void), deps?: DependencyList) => runtime.hooks!.useEffect(effect, deps, true),
}))

type GridProps = ComponentProps<typeof PlatformOddsGrid>
type RowProps = ComponentProps<typeof PlatformOddsRow>
type NodeProps = {
  children?: ReactNode; control?: ReactNode; label?: string; value?: unknown; disabled?: boolean; checked?: boolean
  ref?: { current: HTMLDivElement | null }; inputProps?: { 'aria-label'?: string }
  onClick?: () => void; onChange?: (event: { target: { value: string } }, checked?: boolean) => void
}
type Node = ReactElement<NodeProps>
function elements(node: ReactNode): Node[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  if (!isValidElement<NodeProps>(node)) return []
  return [node, ...elements(node.props.children), ...elements(node.props.control)]
}
function text(node: ReactNode): string {
  if (Array.isArray(node)) return node.map(text).join('')
  if (isValidElement<NodeProps>(node)) return text(node.props.children)
  return typeof node === 'string' || typeof node === 'number' ? String(node) : ''
}
const ofType = (node: ReactNode, type: unknown) => elements(node).filter(element => element.type === type)
const button = (node: ReactNode, label: string) => ofType(node, Button).find(element => text(element) === label)!
const field = (node: ReactNode, label: string) => ofType(node, TextField).find(element => element.props.label === label)!
const rows = (node: ReactNode) => ofType(node, PlatformOddsRow) as unknown as ReactElement<RowProps>[]
const input = (value: string) => ({ target: { value } })

class TestIntersectionObserver {
  static instances: TestIntersectionObserver[] = []
  private callback: IntersectionObserverCallback
  readonly options?: IntersectionObserverInit
  target: Element | null = null
  observe = vi.fn((target: Element) => { this.target = target })
  disconnect = vi.fn()
  constructor(callback: IntersectionObserverCallback, options?: IntersectionObserverInit) {
    this.callback = callback
    this.options = options
    TestIntersectionObserver.instances.push(this)
  }
  notify(isIntersecting = true) {
    this.callback([{ isIntersecting, target: this.target } as IntersectionObserverEntry], this as unknown as IntersectionObserver)
  }
}

const makeItems = (): PlayLimitItem[] => Array.from({ length: 77 }, (_, index) => ({
  play_code: 'play_' + String(index).padStart(3, '0'), play_name: '玩法' + String(index + 1).padStart(2, '0'),
  odds: 1.98, min_bet: 1, max_bet: 100, max_user_period: 1000, max_period_total: 5000, sort_order: index,
  configured: true, configuration_source: 'admin_save', configured_at: '2026-09-03T00:00:00Z', rule_version: 'mark6-v2',
}))

describe('platform odds progressive rendering', () => {
  let props: GridProps
  let baseline: PlayLimitItem[]
  let changed: ReturnType<typeof vi.fn<(items: PlayLimitItem[]) => void>>
  let lastRef: NodeProps['ref']
  const sentinel = { nodeName: 'DIV' } as HTMLDivElement
  const render = () => {
    const root = runtime.hooks!.render(() => PlatformOddsGrid(props))
    const ref = ofType(root, Box).find(node => node.props.ref)?.props.ref
    if (lastRef && lastRef !== ref) lastRef.current = null
    if (ref) ref.current = sentinel
    lastRef = ref
    runtime.hooks!.flushEffects()
    return root
  }
  const selectFiltered = (root: ReactNode, checked = true) => {
    const control = ofType(root, FormControlLabel).find(node => node.props.label === '选择当前筛选')!
    ofType(control.props.control, Checkbox)[0].props.onChange!(input(''), checked)
  }

  beforeEach(() => {
    runtime.hooks = new GridHarness()
    TestIntersectionObserver.instances = []
    vi.stubGlobal('IntersectionObserver', TestIntersectionObserver)
    baseline = makeItems()
    changed = vi.fn((items: PlayLimitItem[]) => { props = { ...props, items: oddsDraftItems(items, baseline) } })
    props = { items: baseline, catalog: Object.fromEntries(baseline.map((item, index) => [item.play_code, { category: index < 40 ? '普通玩法' : '扩展玩法', example: '示例' + index }])), onChange: changed }
    lastRef = undefined
  })
  afterEach(() => { runtime.hooks!.unmount(); vi.unstubAllGlobals() })

  it('mounts only 12 of 77 rows and consumes each observer once before appending the next batch', () => {
    let root = render()
    expect(rows(root)).toHaveLength(12)
    expect(rows(root).map(row => row.props.item)).toEqual(baseline.slice(0, 12))
    expect(text(root)).toContain('筛选结果 77 项 / 已显示 12 项')
    const first = TestIntersectionObserver.instances[0]
    expect(first.observe).toHaveBeenCalledExactlyOnceWith(sentinel)
    expect(first.options).toEqual({ rootMargin: '120px 0px' })
    first.notify(false)
    expect(rows(render())).toHaveLength(12)
    first.notify()
    first.notify()
    first.notify()
    root = render()
    expect(rows(root)).toHaveLength(24)
    expect(first.disconnect).toHaveBeenCalled()
    first.notify()
    expect(rows(render())).toHaveLength(24)

    for (const count of [36, 48, 60, 72, 77]) {
      TestIntersectionObserver.instances.at(-1)!.notify()
      root = render()
      expect(rows(root)).toHaveLength(count)
    }
    expect(rows(root).map(row => row.props.item.play_code)).toEqual(baseline.map(item => item.play_code))
    expect(new Set(rows(root).map(row => row.props.item.play_code)).size).toBe(77)
    expect(button(root, '继续加载玩法')).toBeUndefined()
    expect(changed).not.toHaveBeenCalled()
  })

  it('keeps the load-more button usable without IntersectionObserver and disconnects stale observers on filtering', () => {
    let root = render()
    const stale = TestIntersectionObserver.instances[0]
    field(root, '搜索玩法').props.onChange!(input('玩法77'))
    root = render()
    expect(rows(root)).toHaveLength(1)
    expect(rows(root)[0].props.item).toBe(baseline[76])
    expect(stale.disconnect).toHaveBeenCalled()
    stale.notify()
    expect(rows(render())).toHaveLength(1)

    vi.stubGlobal('IntersectionObserver', undefined)
    field(root, '搜索玩法').props.onChange!(input(''))
    root = render()
    expect(rows(root)).toHaveLength(12)
    for (const count of [24, 36, 48, 60, 72, 77]) {
      button(root, '继续加载玩法').props.onClick!()
      root = render()
      expect(rows(root)).toHaveLength(count)
    }
    expect(changed).not.toHaveBeenCalled()
  })

  it('searches the entire catalog and resets the visible batch after search or category changes', () => {
    let root = render()
    TestIntersectionObserver.instances.at(-1)!.notify()
    root = render()
    expect(rows(root)).toHaveLength(24)
    field(root, '玩法分类').props.onChange!(input('扩展玩法'))
    root = render()
    expect(rows(root)).toHaveLength(12)
    expect(rows(root)[0].props.item).toBe(baseline[40])
    expect(text(root)).toContain('筛选结果 37 项 / 已显示 12 项')
    TestIntersectionObserver.instances.at(-1)!.notify()
    root = render()
    expect(rows(root)).toHaveLength(24)
    field(root, '搜索玩法').props.onChange!(input('PLAY_076'))
    root = render()
    expect(rows(root)).toHaveLength(1)
    expect(rows(root)[0].props.item).toBe(baseline[76])
    field(root, '搜索玩法').props.onChange!(input(''))
    root = render()
    expect(rows(root)).toHaveLength(12)
    field(root, '玩法分类').props.onChange!(input('all'))
    root = render()
    expect(rows(root)).toHaveLength(12)
    expect(rows(root)[0].props.item).toBe(baseline[0])
  })

  it('selects and batch edits all 77 filtered records even though only 12 are mounted', () => {
    let root = render()
    selectFiltered(root)
    root = render()
    expect(text(root)).toContain('已选 77 项 / 筛选结果 77 项 / 已显示 12 项')
    expect(rows(root).every(row => row.props.selected)).toBe(true)
    field(root, '批量数值').props.onChange!(input('3.72'))
    root = render()
    button(root, '应用到所选').props.onClick!()
    expect(changed).toHaveBeenCalledTimes(1)
    expect(changed.mock.calls[0][0]).toHaveLength(77)
    expect(changed.mock.calls[0][0].every(item => item.odds === 3.72)).toBe(true)
    expect(props.items[76].odds).toBe(3.72)
    expect(rows(render())).toHaveLength(12)
  })

  it('retains the complete catalog when applying a batch to an off-screen filtered subset', () => {
    let root = render()
    field(root, '玩法分类').props.onChange!(input('扩展玩法'))
    root = render()
    selectFiltered(root)
    root = render()
    expect(text(root)).toContain('已选 37 项 / 筛选结果 37 项 / 已显示 12 项')
    field(root, '批量字段').props.onChange!(input('max_bet'))
    root = render()
    field(root, '批量数值').props.onChange!(input('125'))
    root = render()
    button(root, '应用到所选').props.onClick!()
    const fullDraft = changed.mock.calls[0][0]
    expect(fullDraft).toHaveLength(77)
    expect(fullDraft.slice(0, 40).every((item, index) => item === baseline[index])).toBe(true)
    expect(fullDraft.slice(40).every(item => item.max_bet === 125)).toBe(true)
    expect(fullDraft[76].max_bet).toBe(125)
  })

  it('preserves untouched memo-row props and lets old row callbacks use the latest committed draft and handler', () => {
    let root = render()
    const first = rows(root)[0].props
    const second = rows(root)[1].props
    field(PlatformOddsRowView(first), '平台赔率（0关闭）').props.onChange!(input('3.72'))
    root = render()
    const firstEdited = rows(root)[0].props
    const secondUnchanged = rows(root)[1].props
    expect(firstEdited.item).not.toBe(first.item)
    expect(firstEdited.item.odds).toBe(3.72)
    for (const key of Object.keys(second) as Array<keyof RowProps>) expect(secondUnchanged[key], key).toBe(second[key])
    expect(firstEdited.onChange).toBe(first.onChange)
    expect(firstEdited.onSelect).toBe(first.onSelect)

    const latestHandler = vi.fn((items: PlayLimitItem[]) => { props = { ...props, items: oddsDraftItems(items, baseline) } })
    props = { ...props, onChange: latestHandler }
    render()
    second.onChange(second.item.play_code, { odds: 4.5 })
    root = render()
    expect(changed).toHaveBeenCalledTimes(1)
    expect(latestHandler).toHaveBeenCalledTimes(1)
    expect(props.items).toHaveLength(77)
    expect(props.items[0].odds).toBe(3.72)
    expect(props.items[1].odds).toBe(4.5)
    expect(rows(root)[0].props.item).toBe(firstEdited.item)
    expect(rows(root)[0].props.onChange).toBe(first.onChange)
    second.onSelect(second.item.play_code, true)
    root = render()
    expect(rows(root)[1].props.selected).toBe(true)
    expect(rows(root)[0].props.selected).toBe(false)
  })

  it('prevents stale row callbacks and current batch controls from editing a disabled grid', () => {
    let root = render()
    const enabledRow = rows(root)[0].props
    selectFiltered(root)
    root = render()
    field(root, '批量数值').props.onChange!(input('3.72'))
    props = { ...props, disabled: true }
    root = render()
    const disabledRow = rows(root)[0].props
    expect(disabledRow.disabled).toBe(true)
    expect(ofType(PlatformOddsRowView(disabledRow), TextField).every(node => node.props.disabled)).toBe(true)
    expect(ofType(PlatformOddsRowView(disabledRow), Checkbox)[0].props.disabled).toBe(true)
    expect(field(root, '批量数值').props.disabled).toBe(true)
    expect(button(root, '应用到所选').props.disabled).toBe(true)
    enabledRow.onChange(enabledRow.item.play_code, { odds: 99 })
    enabledRow.onSelect(enabledRow.item.play_code, false)
    button(root, '应用到所选').props.onClick!()
    expect(changed).not.toHaveBeenCalled()
    expect(props.items).toBe(baseline)
    expect(rows(render())[0].props.selected).toBe(true)
  })
})
