import { useEffect, useState } from 'react'
import { lotteryApi, type DrawResult } from '../api/lottery'
import { WS_EVENT, type WsEvent, useWebSocketConnected } from './useWebSocket'

/** Repeated refreshes may return identical rows. Reuse only entirely unchanged
 * draw records, so corrected numbers/timestamps still repaint immediately. */
export function reuseUnchangedDraws(previous: DrawResult[], incoming: DrawResult[]) {
  const byID = new Map(previous.map(draw => [`${draw.game_id}:${draw.id}`, draw]))
  const next = incoming.map(draw => {
    const current = byID.get(`${draw.game_id}:${draw.id}`)
    return current && current.issue === draw.issue && current.draw_at === draw.draw_at
      && current.numbers.length === draw.numbers.length
      && current.numbers.every((number, index) => number === draw.numbers[index])
      ? current : draw
  })
  return next.length === previous.length && next.every((draw, index) => draw === previous[index]) ? previous : next
}

export function useGameDraws(gameId: string, limit = 12, recoveryMs = 10_000) {
  const realtimeConnected = useWebSocketConnected()
  const [draws, setDraws] = useState<DrawResult[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    // Draws belong to exactly one game. Clear only when that scope changes,
    // not when a recovery request for the same game temporarily fails.
    setDraws([])
    setLoading(true)
    setError('')
  }, [gameId])

  useEffect(() => {
    let cancelled = false
    let inFlight = false
    let queued = false
    let activeRequest: AbortController | null = null
    const load = async () => {
      if (cancelled) return
      // Coalesce bursts into one follow-up read. Parallel requests could let
      // an older snapshot arrive last and overwrite a newer draw list.
      if (inFlight) {
        queued = true
        return
      }
      inFlight = true
      const controller = new AbortController()
      activeRequest = controller
      // Keep the deadline until the entire body has been consumed, not only
      // until fetch receives response headers. A stalled body must release
      // the single-flight slot so later draw events can recover.
      const deadline = window.setTimeout(() => controller.abort(), 15_000)
      try {
        const rows = await lotteryApi.draws(gameId, limit, controller.signal)
        if (cancelled) return
        setDraws(current => reuseUnchangedDraws(current, rows))
        setError('')
      } catch (reason) {
        if (!cancelled) {
          // Preserve the last confirmed draw list while WebSocket/polling
          // recovers; the game-id effect already starts a new hook lifecycle.
          setError(controller.signal.aborted ? '读取开奖超时，请稍后重试' : reason instanceof Error ? reason.message : '读取开奖失败')
        }
      } finally {
        window.clearTimeout(deadline)
        if (activeRequest === controller) activeRequest = null
        if (!cancelled) setLoading(false)
        inFlight = false
        if (queued && !cancelled) {
          queued = false
          void load()
        }
      }
    }
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail
      if (detail?.type === 'draw_update' && (detail.game_id ?? detail.data.game_id) === gameId) void load()
    }
    void load()
    window.addEventListener(WS_EVENT, onWs)
    const timer = realtimeConnected ? 0 : window.setInterval(() => void load(), recoveryMs)
    return () => {
      cancelled = true
      activeRequest?.abort()
      window.removeEventListener(WS_EVENT, onWs)
      if (timer) window.clearInterval(timer)
    }
  }, [gameId, limit, realtimeConnected, recoveryMs])

  return { draws, loading, error }
}
