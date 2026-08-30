import { useEffect, useMemo, useRef, useState } from 'react'
import { planApi, type RacingPlanDetail, type RacingPlanSelection } from '../api/plans'
import { createRefreshLoop } from '../utils/refreshLoop'
import { DEFAULT_RACING_PLAN, racingPlanKey, sameRacingPlan } from '../utils/racingPlans'
import { WS_EVENT, type WsEvent } from './useWebSocket'

type FeedResult<T> = { data: T; error?: string }
const message = (reason: unknown) => reason instanceof Error ? reason.message : '读取计划失败，正在重试'
const visible = () => document.visibilityState === 'visible'

/** GET stays read-only; a failed touch preserves the publications just read. */
async function visit<T>(data: T, enabled: boolean, signal: AbortSignal, touch: () => Promise<T>): Promise<FeedResult<T>> {
  signal.throwIfAborted()
  if (!enabled || !visible()) return { data }
  try {
    const generated = await touch()
    signal.throwIfAborted()
    return { data: generated }
  } catch (reason) {
    // Hidden/old-page loops are already disposed and ignore this fallback.
    // For a visible timeout, retain the successful GET snapshot as well.
    return { data, error: signal.aborted ? '更新计划超时，请稍后重试' : message(reason) }
  }
}

/** Disposing on hide aborts the flight and rejects even late transport results. */
function usePlanFeed<T>(scope: string, load: (signal: AbortSignal) => Promise<FeedResult<T>>, gameId?: string, suspended = false, revision = 0) {
  const [state, setState] = useState<{ scope: string; data: T | null; loading: boolean; error: string }>({ scope, data: null, loading: true, error: '' })
  const cancel = useRef<() => void>(() => {})
  useEffect(() => {
    setState(current => current.scope === scope ? current : { scope, data: null, loading: true, error: '' })
    let loop: ReturnType<typeof createRefreshLoop<FeedResult<T>>> | null = null
    const stop = () => { loop?.dispose(); loop = null }
    cancel.current = stop
    const onVisibility = () => {
      if (!visible() || suspended) { stop(); return }
      if (loop) return
      loop = createRefreshLoop({
        request: load,
        onData: ({ data, error }) => setState({ scope, data, loading: false, error: error ?? '' }),
        onError: reason => setState(current => ({ ...current, loading: false, error: message(reason) })),
        delay: (_, failures) => failures ? Math.min(30_000, 2000 * 2 ** Math.min(failures - 1, 4)) : 15_000,
      })
      loop.resume()
    }
    const refresh = () => loop?.refresh(true)
    const onEvent = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail
      if (!detail || !['draw_update', 'plan_update'].includes(detail.type)) return
      const eventGame = detail.game_id ?? detail.data?.game_id
      if (!gameId || !eventGame || eventGame === gameId) loop?.refresh()
    }
    onVisibility()
    window.addEventListener(WS_EVENT, onEvent)
    window.addEventListener('online', refresh)
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      stop()
      window.removeEventListener(WS_EVENT, onEvent)
      window.removeEventListener('online', refresh)
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [scope, load, gameId, suspended, revision])
  return { ...(state.scope === scope ? state : { scope, data: null, loading: true, error: '' }), cancel: () => cancel.current() }
}

export function usePlanCatalog(room = '') {
  const load = useMemo(() => async (signal: AbortSignal) => ({ data: await planApi.catalog(signal) }), [])
  return usePlanFeed(`catalog:${room}`, load)
}

export function usePlanDetail(gameId: string, room = '') {
  const load = useMemo(() => async (signal: AbortSignal) => {
    if (!gameId) return { data: null }
    const data = await planApi.detail(gameId, signal)
    return visit(data, data.automation_enabled === true, signal, () => planApi.activate(gameId, signal))
  }, [gameId])
  return usePlanFeed(`detail:${room}:${gameId}`, load, gameId)
}

/** Confirmation changes this page's selection, never a room-wide preference.
 * The default and other streams generate only while the page is visible. */
export function useRacingPlanStream(room = '') {
  const [chosen, setChosen] = useState({ room, selection: DEFAULT_RACING_PLAN })
  const selection = chosen.room === room ? chosen.selection : DEFAULT_RACING_PLAN
  const scope = `racing:${room}:${racingPlanKey(selection)}`
  const [activation, setActivation] = useState<{ room: string; loading: boolean; error: string }>({ room, loading: false, error: '' })
  const [revision, setRevision] = useState(0)
  const [confirmed, setConfirmed] = useState<{ scope: string; data: RacingPlanDetail } | null>(null)
  const pending = useRef<{ sequence: number; controller: AbortController | null; timeout: ReturnType<typeof setTimeout> | null }>({ sequence: 0, controller: null, timeout: null })
  const load = useMemo(() => async (signal: AbortSignal) => {
    const validate = (data: RacingPlanDetail) => {
      if (data.game_id !== 'speed-racing' || !sameRacingPlan(data.selection, selection)) throw new Error('计划数据与当前选择不一致，请重试')
      return data
    }
    const data = validate(await planApi.racingDetail(selection, signal))
    return visit(data, data.automation_enabled === true && data.stream.allowed, signal,
      async () => validate(await planApi.activateRacing(selection, signal)))
  }, [selection])
  const feed = usePlanFeed(scope, load, 'speed-racing', activation.room === room && activation.loading, revision)

  useEffect(() => {
    const cancelPending = () => {
      pending.current.sequence += 1
      pending.current.controller?.abort()
      if (pending.current.timeout !== null) clearTimeout(pending.current.timeout)
      pending.current.controller = null
      pending.current.timeout = null
    }
    const onVisibility = () => {
      if (!visible() && pending.current.controller) {
        cancelPending()
        setActivation({ room, loading: false, error: '' })
        setRevision(value => value + 1)
      }
    }
    document.addEventListener('visibilitychange', onVisibility)
    return () => { cancelPending(); document.removeEventListener('visibilitychange', onVisibility) }
  }, [room])

  const activate = async (next: RacingPlanSelection) => {
    if (!visible()) return false
    feed.cancel()
    const sequence = ++pending.current.sequence
    pending.current.controller?.abort()
    if (pending.current.timeout !== null) clearTimeout(pending.current.timeout)
    const controller = new AbortController()
    pending.current.controller = controller
    setActivation({ room, loading: true, error: '' })
    const timeout = setTimeout(() => {
      controller.abort()
      if (sequence === pending.current.sequence) {
        pending.current.sequence += 1
        pending.current.controller = null
        pending.current.timeout = null
        setActivation({ room, loading: false, error: '切换计划超时，请稍后重试' })
        setRevision(value => value + 1)
      }
    }, 15_000)
    pending.current.timeout = timeout
    try {
      // Closed automation still permits browsing actual saved publications.
      const data = feed.data?.automation_enabled === false
        ? await planApi.racingDetail(next, controller.signal)
        : await planApi.activateRacing(next, controller.signal)
      if (sequence !== pending.current.sequence) return false
      controller.signal.throwIfAborted()
      if (data.game_id !== 'speed-racing' || !sameRacingPlan(data.selection, next)) throw new Error('计划数据与当前选择不一致，请重试')
      setConfirmed({ scope: `racing:${room}:${racingPlanKey(next)}`, data })
      setChosen({ room, selection: { ...next } })
      setActivation({ room, loading: false, error: '' })
      return true
    } catch (reason) {
      if (sequence === pending.current.sequence) setActivation({ room, loading: false, error: message(reason) })
      return false
    } finally {
      clearTimeout(timeout)
      if (sequence === pending.current.sequence) {
        pending.current.controller = null
        pending.current.timeout = null
        setRevision(value => value + 1)
      }
    }
  }

  const data = feed.data ?? (confirmed?.scope === scope ? confirmed.data : null)
  return {
    ...feed, data, loading: feed.loading && !data, selection, activate,
    activating: activation.room === room && activation.loading,
    activationError: activation.room === room ? activation.error : '',
    clearActivationError: () => setActivation(current => ({ ...current, error: '' })),
  }
}
