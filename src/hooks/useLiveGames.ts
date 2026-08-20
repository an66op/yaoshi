import { useEffect, useMemo, useState } from 'react'
import { games } from '../data/games'

const toSeconds = (value: string) => {
  const [minutes, seconds] = value.split(':').map(Number)
  return minutes * 60 + seconds
}

const toClock = (value: number) => `${String(Math.floor(value / 60)).padStart(2, '0')}:${String(value % 60).padStart(2, '0')}`

/** 仅用于前端演示：让大厅与彩种详情共享同一组实时倒计时和在线人数。 */
export function useLiveGames() {
  const [elapsed, setElapsed] = useState(0)

  useEffect(() => {
    const timer = window.setInterval(() => setElapsed((seconds) => seconds + 1), 1_000)
    return () => window.clearInterval(timer)
  }, [])

  return useMemo(() => games.map((game, index) => {
    const cycle = 300
    const remaining = (toSeconds(game.due) - elapsed % cycle + cycle) % cycle
    const online = Math.max(1, Number(game.online) + ((elapsed * (index + 2) + index * 7) % 17) - 8)
    return { ...game, due: toClock(remaining), online: String(online) }
  }), [elapsed])
}
