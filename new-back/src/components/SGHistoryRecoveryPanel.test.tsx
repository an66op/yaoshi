import { Alert, Button, Chip, Table } from '@mui/material'
import { isValidElement, type DependencyList, type ReactElement, type ReactNode, type SetStateAction } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { SGSSCBackfillAttempt, SGSSCBackfillStatus } from '../api'
import { SGHistoryRecoveryPanel } from './SGHistoryRecoveryPanel'

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
      previous?.cleanup?.()
      this.slots[index] = { deps, effect }
      this.effects.push(index)
    }
  }
  flushEffects() { for (const index of this.effects.splice(0)) this.slots[index].cleanup = this.slots[index].effect?.() }
  unmount() { for (const slot of this.slots) slot.cleanup?.() }
}
const runtime = vi.hoisted(() => ({ hooks: null as Harness | null, status: vi.fn(), queue: vi.fn() }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useCallback: <T,>(callback: T, deps: DependencyList) => runtime.hooks!.useMemo(() => callback, deps),
  useEffect: (effect: () => void | (() => void), deps?: DependencyList) => runtime.hooks!.useEffect(effect, deps),
}))
vi.mock('../api', () => ({ adminApi: { sgSSCBackfillStatus: runtime.status, queueSGSSCBackfill: runtime.queue } }))

type Props = { children?: ReactNode; label?: string; disabled?: boolean; severity?: string; 'aria-label'?: string; onClick?: () => void }
function elements(node: ReactNode): ReactElement<Props>[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  return isValidElement<Props>(node) ? [node, ...elements(node.props.children)] : []
}
function text(node: ReactNode): string {
  if (Array.isArray(node)) return node.map(text).join('')
  if (isValidElement<Props>(node)) return text(node.props.children) + (node.props.label || '')
  return typeof node === 'string' || typeof node === 'number' ? String(node) : ''
}
const receipt = (id: number, override: Partial<SGSSCBackfillAttempt> = {}): SGSSCBackfillAttempt => ({
  id, issue: `20260903${String(id).padStart(3, '0')}`, attempt: 2, status: 'source_error', trigger: 'admin', operator: 'admin-a', request_id: `audit-${id}`,
  started_at: '2026-09-03T06:00:00Z', finished_at: '2026-09-03T06:00:01Z', numbers: '', imported: false, settled_bets: 0,
  error: '双站同一期号码不一致', source_revision: 'sgssc-168-115-v1', conversion_revision: 'sgssc-direct-v1', ...override,
})
const snapshot: SGSSCBackfillStatus = {
  game_id: 'sg-ssc', enabled: true, source_bound: true, message: '历史补采不改变实时期号、封盘或来源健康。', max_age_days: 30, batch_limit: 48,
  summary: { pending_issues: 3, running_issues: 1, retry_issues: 2, blocked_issues: 4, completed_issues: 5, untracked_pending_issues: 6 },
  gaps: [{ issue: '20260903168', draw_at: '2026-09-03T06:00:00Z', status: 'settlement_retry', reason: 'pending_bet', attempts: 2,
    last_error: '结算等待重试', next_retry_at: '2026-09-03T06:01:00Z', created_at: '2026-09-03T06:00:00Z', updated_at: '2026-09-03T06:00:30Z' }],
  has_more_gaps: true, records: [receipt(168)], next_before_id: 168, has_more_records: true,
}
const deferred = <T,>() => {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((accept, fail) => { resolve = accept; reject = fail })
  return { promise, resolve, reject }
}

describe('SG history recovery panel lifecycle', () => {
  let documentEvents: EventTarget & { visibilityState: string }
  const render = () => { const root = runtime.hooks!.render(SGHistoryRecoveryPanel); runtime.hooks!.flushEffects(); return root }
  const button = (label: string) => elements(render()).find(node => node.type === Button && text(node) === label)!
  const table = (label: string) => elements(render()).find(node => node.type === Table && node.props['aria-label'] === label)!
  const ready = async () => { render(); await vi.advanceTimersByTimeAsync(0); return render() }

  beforeEach(() => {
    vi.useFakeTimers({ now: Date.parse('2026-09-03T06:00:00Z') })
    runtime.hooks = new Harness()
    runtime.status.mockReset().mockResolvedValue(snapshot)
    runtime.queue.mockReset().mockResolvedValue({ queued_issues: 3, message: '运行中和需人工核对的期不会重置。' })
    documentEvents = Object.assign(new EventTarget(), { visibilityState: 'visible' })
    vi.stubGlobal('document', documentEvents)
    vi.stubGlobal('window', { setTimeout, clearTimeout })
  })
  afterEach(() => { runtime.hooks?.unmount(); vi.clearAllTimers(); vi.unstubAllGlobals(); vi.useRealTimers() })

  it('renders exact queue counts, failures, audit identity and separate import/settlement results', async () => {
    runtime.status.mockResolvedValue({ ...snapshot, records: [receipt(168), receipt(167, { status: 'recovered', trigger: 'auto', numbers: '0,9,2,7,4', imported: true, settled_bets: 7, error: '' })] })
    const root = await ready()
    for (const label of ['等待处理 3', '执行中 1', '等待重试 2', '需人工核对 4', '已完成 5', '待登记注单期 6']) expect(text(root)).toContain(label)
    expect(text(table('SG缺期队列'))).toContain('等待结算重试当前来源版本待结注单')
    expect(text(root)).toContain('还有更多缺期未展示')
    for (const label of ['请求号 audit-168', 'admin-a', '双站同一期号码不一致', '本次未新增导入', '本次已导入', '本次结算 7 笔', 'sgssc-168-115-v1']) expect(text(root)).toContain(label)
    expect(runtime.status).toHaveBeenCalledWith(0, 20)
    expect(runtime.queue).not.toHaveBeenCalled()
  })

  it('labels draw gaps and never advertises automatic retry for a blocked queue item', async () => {
    runtime.status.mockResolvedValue({ ...snapshot, gaps: [{ ...snapshot.gaps[0], status: 'blocked', reason: 'draw_gap' }] })
    await ready()
    expect(text(table('SG缺期队列'))).toContain('可信历史之间的缺期')
    expect(text(table('SG缺期队列'))).toContain('需人工核对，不自动重试')
    expect(text(table('SG缺期队列'))).not.toContain('2026/09/03 14:01:00')
  })

  it('keeps failed initial reads unknown and blocks enqueue instead of showing healthy zeroes', async () => {
    runtime.status.mockRejectedValue(new Error('offline'))
    const root = await ready()
    expect(text(root)).toContain('当前状态未知，不能认定没有缺期')
    expect(elements(root).filter(node => node.type === Chip)).toHaveLength(0)
    expect(button('登记 SG 缺期补采').props.disabled).toBe(true)
    button('登记 SG 缺期补采').props.onClick!()
    expect(runtime.queue).not.toHaveBeenCalled()
  })

  it('marks a failed refresh stale and restores only after a successful visible poll', async () => {
    await ready()
    runtime.status.mockRejectedValueOnce(new Error('offline'))
    await vi.advanceTimersByTimeAsync(10_000)
    expect(text(render())).toContain('以下为上次读取结果，非实时状态')
    expect(text(render())).toContain('等待处理 3')
    expect(button('登记 SG 缺期补采').props.disabled).toBe(true)
    await vi.advanceTimersByTimeAsync(10_000)
    expect(text(render())).not.toContain('offline')
    expect(button('登记 SG 缺期补采').props.disabled).toBe(false)
  })

  it.each([{ enabled: false }, { source_bound: false }])('keeps history readable but forbids registration when %j', async override => {
    runtime.status.mockResolvedValue({ ...snapshot, ...override })
    await ready()
    expect(text(table('SG恢复记录'))).toContain('audit-168')
    expect(button('登记 SG 缺期补采').props.disabled).toBe(true)
    button('登记 SG 缺期补采').props.onClick!()
    expect(runtime.queue).not.toHaveBeenCalled()
  })

  it('treats POST as registration, synchronously blocks duplicate clicks, and refreshes after the receipt', async () => {
    await ready()
    const queued = deferred<{ queued_issues: number; message: string }>()
    runtime.queue.mockReturnValueOnce(queued.promise)
    const click = button('登记 SG 缺期补采').props.onClick!
    click(); click()
    expect(runtime.queue).toHaveBeenCalledTimes(1)
    expect(runtime.queue.mock.calls[0]).toEqual([])
    expect(button('登记中…').props.disabled).toBe(true)
    await vi.advanceTimersByTimeAsync(20_000)
    expect(runtime.status).toHaveBeenCalledTimes(1)
    queued.resolve({ queued_issues: 3, message: '运行中任务不会重置。' })
    await vi.advanceTimersByTimeAsync(0)
    const notice = elements(render()).find(node => node.type === Alert && text(node).includes('补采请求已登记'))!
    expect(notice.props.severity).toBe('info')
    expect(text(notice)).toContain('本次入队 3 期')
    expect(text(notice)).toContain('等待后台执行')
    expect(text(notice)).not.toContain('已恢复')
    expect(runtime.status).toHaveBeenCalledTimes(2)
  })

  it('shows uncertain POST errors without claiming failure or success of the worker', async () => {
    await ready()
    runtime.queue.mockRejectedValueOnce(new Error('request timeout'))
    button('登记 SG 缺期补采').props.onClick!()
    await vi.advanceTimersByTimeAsync(0)
    expect(text(render())).toContain('request timeout；可刷新记录确认是否已登记')
    expect(text(render())).not.toContain('补采请求已登记')
    expect(button('登记 SG 缺期补采').props.disabled).toBe(false)
  })

  it('pages durable records with the server cursor and polls the selected page without mixing pages', async () => {
    await ready()
    runtime.status.mockResolvedValue({ ...snapshot, records: [receipt(150)], next_before_id: undefined, has_more_records: false })
    button('更早记录').props.onClick!()
    await vi.advanceTimersByTimeAsync(0)
    expect(runtime.status).toHaveBeenLastCalledWith(168, 20)
    expect(text(table('SG恢复记录'))).toContain('audit-150')
    expect(text(table('SG恢复记录'))).not.toContain('audit-168')
    expect(button('更早记录').props.disabled).toBe(true)
    await vi.advanceTimersByTimeAsync(10_000)
    expect(runtime.status).toHaveBeenLastCalledWith(168, 20)
    runtime.status.mockResolvedValue(snapshot)
    button('上一页').props.onClick!()
    await vi.advanceTimersByTimeAsync(0)
    expect(runtime.status).toHaveBeenLastCalledWith(0, 20)
    expect(text(table('SG恢复记录'))).toContain('audit-168')
  })

  it('does not invent a cursor when the backend has_more response omits it', async () => {
    runtime.status.mockResolvedValue({ ...snapshot, next_before_id: undefined })
    await ready()
    expect(button('更早记录').props.disabled).toBe(true)
    button('更早记录').props.onClick!()
    expect(runtime.status).toHaveBeenCalledTimes(1)
  })

  it('pauses hidden polling and shares one in-flight read with manual refresh', async () => {
    await ready()
    const slow = deferred<SGSSCBackfillStatus>()
    runtime.status.mockReturnValueOnce(slow.promise)
    const refresh = button('刷新状态').props.onClick!
    refresh(); refresh()
    await vi.advanceTimersByTimeAsync(30_000)
    expect(runtime.status).toHaveBeenCalledTimes(2)
    slow.resolve(snapshot)
    await vi.advanceTimersByTimeAsync(0)
    documentEvents.visibilityState = 'hidden'
    documentEvents.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(30_000)
    expect(runtime.status).toHaveBeenCalledTimes(2)
    documentEvents.visibilityState = 'visible'
    documentEvents.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(0)
    expect(runtime.status).toHaveBeenCalledTimes(3)
  })

  it.each(['read', 'write'])('ignores late %s responses and starts no follow-up after SG is unmounted', async kind => {
    await ready()
    const slow = deferred<never>()
    if (kind === 'read') { runtime.status.mockReturnValueOnce(slow.promise); button('刷新状态').props.onClick!() }
    else { runtime.queue.mockReturnValueOnce(slow.promise); button('登记 SG 缺期补采').props.onClick!() }
    await vi.advanceTimersByTimeAsync(0)
    runtime.hooks!.unmount()
    const requests = runtime.status.mock.calls.length
    slow.reject(new Error('late failure'))
    await vi.advanceTimersByTimeAsync(30_000)
    expect(runtime.status).toHaveBeenCalledTimes(requests)
    expect(text(render())).not.toContain('late failure')
  })

  it('rejects a malformed status without treating missing counters as zero', async () => {
    runtime.status.mockResolvedValue({ ...snapshot, summary: {} })
    expect(text(await ready())).toContain('SG补采状态返回不完整')
    expect(elements(render()).filter(node => node.type === Chip)).toHaveLength(0)
  })
})
