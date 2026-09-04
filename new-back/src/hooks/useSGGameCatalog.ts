import { useCallback, useEffect, useRef, type Dispatch, type SetStateAction } from 'react'
import type { AdminGame } from '../api'

/** Keep only the selected SG chat live when its worker stops without a WS event.
 * Existing manual/event reads share the same in-flight request. */
export function useSGGameCatalog<T extends AdminGame>(
  request: () => Promise<T[]>,
  setGames: Dispatch<SetStateAction<T[]>>,
  selectedGameID: string | undefined,
  scope: string,
) {
  const context = useRef({ request, scope, selectedGameID, selection: {} })
  useEffect(() => {
    context.current = { request, scope, selectedGameID, selection: {} }
  }, [request, scope, selectedGameID])
  const publish = useRef(setGames)
  useEffect(() => { publish.current = setGames }, [setGames])
  const lifecycle = useRef({ active: false })
  const pending = useRef<{ request: () => Promise<T[]>; scope: string; lifetime: { active: boolean }; promise: Promise<void> } | null>(null)

  useEffect(() => {
    const current = { active: true }
    lifecycle.current = current
    return () => { current.active = false }
  }, [])

  const read = useCallback((poll: boolean): Promise<void> => {
    const current = context.current
    const lifetime = lifecycle.current
    if (!lifetime.active || (poll && current.selectedGameID !== 'sg-ssc')) return Promise.resolve()
    if (pending.current?.request === current.request && pending.current.scope === current.scope && pending.current.lifetime === lifetime) return pending.current.promise
    const valid = () => lifetime.active && context.current.request === current.request && context.current.scope === current.scope
      && (!poll || context.current.selection === current.selection)
    const promise = Promise.resolve().then(current.request).then(rows => {
      if (valid()) publish.current(Array.isArray(rows) ? rows : [])
    }).catch(reason => {
      if (!valid()) return
      if (context.current.selectedGameID === 'sg-ssc') {
        // Preserve confirmed history and rules; only live source availability
        // is unknown while this catalogue request failed.
        publish.current(rows => rows.map(game => game.id === 'sg-ssc' ? {
          ...game, current_issue: '', issue_status: 'error', source_healthy: false,
          next_draw_at: '', accept_at: undefined, seal_at: undefined,
          last_sync_error: reason instanceof Error ? reason.message : '开奖源状态暂时无法读取',
        } : game))
      }
      throw reason
    }).finally(() => {
      if (pending.current?.promise === promise) pending.current = null
    })
    pending.current = { request: current.request, scope: current.scope, lifetime, promise }
    return promise
  }, [])

  useEffect(() => {
    if (selectedGameID !== 'sg-ssc') return
    let active = true
    let timer = 0
    const tick = async () => {
      try { await read(true) } catch { /* source availability is already closed */ }
      if (active) timer = window.setTimeout(() => void tick(), 10_000)
    }
    timer = window.setTimeout(() => void tick(), 10_000)
    return () => { active = false; window.clearTimeout(timer) }
  }, [read, request, scope, selectedGameID])

  return useCallback(() => read(false), [read])
}
