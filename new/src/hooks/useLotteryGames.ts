import { useEffect, useMemo, useState } from 'react'
import { lotteryApi, type LotteryGame } from '../api/lottery'
import { games as fallbackGames } from '../data/games'
import { WS_EVENT, type WsEvent } from './useWebSocket'
import type { Game } from '../types'

const toClock = (seconds: number) => {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  if (h > 0) return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

const resolvedBadgeColor = (item: LotteryGame) => {
  const color = item.badge_color?.trim().toLowerCase()
  // 白色徽标会与大厅卡片、六合彩号码底色融在一起；香港六合彩固定采用其红色主题。
  if (!color || color === 'white' || color === '#fff' || color === '#ffffff') {
    return item.id === 'hong-kong-mark-six' ? '#d64155' : '#3b83ec'
  }
  return item.badge_color
}

const mapGame = (item: LotteryGame, nowMs: number): Game => {
  const nextMs = new Date(item.next_draw_at).getTime()
  const remaining = Math.max(0, Math.floor((nextMs - nowMs) / 1000))
  return {
    id: item.id,
    title: item.name,
    tag: item.badge || item.code.toUpperCase(),
    category: item.category,
    online: item.bettor_count != null ? String(item.bettor_count) : '—',
    period: item.current_issue || item.issue || '—',
    due: toClock(remaining),
    color: resolvedBadgeColor(item),
    balls: item.latest_numbers?.length ? item.latest_numbers : [0, 0, 0, 0, 0],
  }
}

const mapFallback = (game: Game, elapsed: number): Game => {
  const [minutes, seconds] = game.due.split(':').map(Number)
  const cycle = minutes * 60 + seconds
  const remaining = (cycle - elapsed % (cycle + 1) + cycle + 1) % (cycle + 1)
  return { ...game, due: toClock(remaining) }
}

/** 从后端拉取彩种列表；离线或失败时回退到本地演示数据。 */
export function useLotteryGames() {
  const [remote, setRemote] = useState<LotteryGame[] | null>(null)
  const [serverOffsetMs, setServerOffsetMs] = useState(0)
  const [tick, setTick] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      try {
        const [games, clock] = await Promise.all([lotteryApi.enabledGames(), lotteryApi.clock()])
        if (cancelled) return
        setRemote(games)
        setServerOffsetMs(clock.server_time_ms - Date.now())
        setError('')
      } catch (reason) {
        if (!cancelled) {
          setRemote(null)
          setError(reason instanceof Error ? reason.message : '无法连接后端')
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail
      if (detail?.type === 'draw_update') void load()
    }
    void load()
    window.addEventListener(WS_EVENT, onWs)
    const refresh = window.setInterval(() => void load(), 15_000)
    return () => {
      cancelled = true
      window.removeEventListener(WS_EVENT, onWs)
      window.clearInterval(refresh)
    }
  }, [])

  useEffect(() => {
    const timer = window.setInterval(() => setTick((value) => value + 1), 1_000)
    return () => window.clearInterval(timer)
  }, [])

  return useMemo(() => {
    const nowMs = Date.now() + serverOffsetMs
    // `remote !== null` means the request succeeded. Preserve an intentionally
    // empty enabled list instead of silently reviving local demo games.
    if (remote !== null) {
      return { games: remote.map((item) => mapGame(item, nowMs)), loading, error, live: true }
    }
    return {
      games: fallbackGames.map((game, index) => mapFallback(game, tick + index * 3)),
      loading,
      error,
      live: false,
    }
  }, [remote, serverOffsetMs, tick, loading, error])
}
