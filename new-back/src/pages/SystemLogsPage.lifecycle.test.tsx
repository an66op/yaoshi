import { Button, Tabs, TextField } from '@mui/material'
import { isValidElement, type DependencyList, type ReactElement, type ReactNode, type SetStateAction } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AuditLog, SystemLogItem } from '../api'
import { SystemLogsPage } from './SystemLogsPage'

type Slot = { value?: unknown; deps?: DependencyList; effect?: () => void | (() => void); cleanup?: void | (() => void) }
const sameDeps = (a?: DependencyList, b?: DependencyList) => Boolean(a && b && a.length === b.length && a.every((item, index) => Object.is(item, b[index])))
class Harness {
  private slots: Slot[] = []
  private cursor = 0
  private effects: number[] = []
  render<T>(factory: () => T) { this.cursor = 0; return factory() }
  useState<T>(initial: T | (() => T)): [T, (next: SetStateAction<T>) => void] {
    const slot = this.slots[this.cursor++] ??= { value: typeof initial === 'function' ? (initial as () => T)() : initial }
    return [slot.value as T, next => { slot.value = typeof next === 'function' ? (next as (current: T) => T)(slot.value as T) : next }]
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
      previous?.cleanup?.(); this.slots[index] = { deps, effect }; this.effects.push(index)
    }
  }
  flushEffects() { for (const index of this.effects.splice(0)) this.slots[index].cleanup = this.slots[index].effect?.() }
  unmount() { for (const slot of this.slots) slot.cleanup?.() }
}

const runtime = vi.hoisted(() => ({ hooks: null as Harness | null, systemLogs: vi.fn(), auditLogs: vi.fn(), games: vi.fn() }))
vi.mock('react', async original => ({
  ...await original<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useMemo: <T,>(factory: () => T, deps: DependencyList) => runtime.hooks!.useMemo(factory, deps),
  useCallback: <T,>(callback: T, deps: DependencyList) => runtime.hooks!.useMemo(() => callback, deps),
  useEffect: (effect: () => void | (() => void), deps?: DependencyList) => runtime.hooks!.useEffect(effect, deps),
}))
vi.mock('../api', () => ({ adminApi: { systemLogs: runtime.systemLogs, auditLogs: runtime.auditLogs, games: runtime.games } }))

type Props = {
  children?: ReactNode; label?: ReactNode; value?: unknown; 'data-testid'?: string; ref?: { current: Element | null } | ((element: Element | null) => void)
  onClick?: () => void; onChange?: (event: { target: { value: string } } | null, value: string) => void
}
type ElementNode = ReactElement<Props>
const elements = (node: ReactNode): ElementNode[] => Array.isArray(node) ? node.flatMap(elements) : isValidElement<Props>(node) ? [node, ...elements(node.props.children)] : []
const text = (node: ReactNode): string => Array.isArray(node) ? node.map(text).join('') : isValidElement<Props>(node) ? text(node.props.children) + text(node.props.label) : typeof node === 'string' || typeof node === 'number' ? String(node) : ''
const sourceLog = (id: number): SystemLogItem => ({ id, category: 'source', event_type: id === 60 ? 'sync_error' : 'sync_recovered', level: id === 60 ? 'error' : 'info', status: id === 60 ? 'error' : 'ok', source_group: 'pc28-163', game_id: 'pc-canada', job_id: '', message: id === 60 ? '母源过期' : '母源已恢复', imported: 1, latest_issue: '3477941', consecutive_errors: id === 60 ? 3 : 0, created_at: '2026-09-04T12:00:00Z' })
const auditLog = (id: number): AuditLog => ({ id, actor_id: 1, actor_name: '系统管理员', actor_role: 'admin', method: 'PUT', path: '/api/admin/system', status_code: 200, request_id: `request-${id}`, ip: '127.0.0.1', created_at: '2026-09-04T12:00:00Z' })

class ObserverMock {
  static instances: ObserverMock[] = []
  private readonly callback: IntersectionObserverCallback
  observe = vi.fn()
  disconnect = vi.fn()
  constructor(callback: IntersectionObserverCallback) { this.callback = callback; ObserverMock.instances.push(this) }
  emit() { this.callback([{ isIntersecting: true } as IntersectionObserverEntry], this as unknown as IntersectionObserver) }
}

describe('system logs page lifecycle', () => {
  const sentinel = {} as Element
  let committedRefs = new Map<string, Props['ref']>()
  const commit = (ref: Props['ref'], value: Element | null) => { if (typeof ref === 'function') ref(value); else if (ref) ref.current = value }
  const render = () => {
    const root = runtime.hooks!.render(SystemLogsPage)
    for (const id of ['source-log-load-more', 'audit-log-load-more']) {
      const ref = elements(root).find(node => node.props['data-testid'] === id)?.props.ref
      if (committedRefs.get(id) !== ref) commit(committedRefs.get(id), null)
      commit(ref, sentinel); committedRefs.set(id, ref)
    }
    runtime.hooks!.flushEffects()
    return root
  }
  const settle = async () => { for (let index = 0; index < 10; index++) await Promise.resolve(); return render() }
  const ready = async () => { render(); await vi.advanceTimersByTimeAsync(0); return settle() }

  beforeEach(() => {
    vi.useFakeTimers({ now: Date.parse('2026-09-04T12:00:00+08:00') })
    runtime.hooks = new Harness(); committedRefs = new Map(); ObserverMock.instances = []
    runtime.games.mockReset().mockResolvedValue([{ id: 'pc-canada', name: 'PC加拿大' }])
    runtime.systemLogs.mockReset().mockResolvedValueOnce({ items: [sourceLog(60), sourceLog(50)], has_more: true, next_before_id: 50 }).mockResolvedValueOnce({ items: [sourceLog(50), sourceLog(40)], has_more: false })
    runtime.auditLogs.mockReset().mockResolvedValueOnce({ items: [auditLog(30)], has_more: true, next_before_id: 30 }).mockResolvedValueOnce({ items: [auditLog(20)], has_more: false })
    vi.stubGlobal('IntersectionObserver', ObserverMock)
    vi.stubGlobal('window', { setTimeout, clearTimeout })
  })
  afterEach(() => { runtime.hooks!.unmount(); vi.clearAllTimers(); vi.useRealTimers(); vi.unstubAllGlobals() })

  it('loads source history by cursor, deduplicates overlap and sends server-side filters', async () => {
    let root = await ready()
    expect(runtime.systemLogs).toHaveBeenCalledTimes(1)
    expect(runtime.systemLogs.mock.calls[0][0]).toMatchObject({ limit: 50 })
    expect(text(root)).toContain('已加载 2 条')
    ObserverMock.instances.at(-1)!.emit()
    root = await settle()
    expect(runtime.systemLogs.mock.calls[1][0]).toMatchObject({ beforeId: 50, limit: 50 })
    expect(text(root)).toContain('已加载 3 条')

    const status = elements(root).find(node => node.type === TextField && node.props.label === '状态')!
    status.props.onChange!({ target: { value: 'error' } }, 'error'); root = render()
    elements(root).find(node => node.type === Button && text(node) === '查询')!.props.onClick!(); render()
    await vi.advanceTimersByTimeAsync(0); await settle()
    expect(runtime.systemLogs.mock.calls.at(-1)?.[0]).toMatchObject({ status: 'error', limit: 50 })
  })

  it('keeps operation history on a separate cursor-backed tab', async () => {
    let root = await ready()
    elements(root).find(node => node.type === Tabs)!.props.onChange!(null, 'operation'); root = render()
    expect(text(root)).toContain('操作日志')
    expect(text(root)).toContain('搜索只筛选当前已加载的 1 条记录')
    ObserverMock.instances.at(-1)!.emit(); root = await settle()
    expect(runtime.auditLogs).toHaveBeenLastCalledWith(30, 50)
    expect(text(root)).toContain('搜索只筛选当前已加载的 2 条记录')
  })
})
