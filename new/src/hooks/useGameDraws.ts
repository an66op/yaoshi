import { useEffect, useState } from 'react'
import { lotteryApi, type DrawResult } from '../api/lottery'
import { WS_EVENT, type WsEvent } from './useWebSocket'

export function useGameDraws(gameId: string, limit = 12, pollMs = 12_000) {
  const [draws, setDraws] = useState<DrawResult[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      try {
        const rows = await lotteryApi.draws(gameId, limit)
        if (cancelled) return
        setDraws(rows)
        setError('')
      } catch (reason) {
        if (!cancelled) {
          setDraws([])
          setError(reason instanceof Error ? reason.message : '读取开奖失败')
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail
      if (detail?.type === 'draw_update' && detail.data.game_id === gameId) void load()
    }
    void load()
    window.addEventListener(WS_EVENT, onWs)
    const timer = window.setInterval(() => void load(), pollMs)
    return () => {
      cancelled = true
      window.removeEventListener(WS_EVENT, onWs)
      window.clearInterval(timer)
    }
  }, [gameId, limit, pollMs])

  return { draws, loading, error }
}
