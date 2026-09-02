import { useEffect, useState } from 'react'
import type { DrawResult } from '../api/lottery'
import { recentGameTimelineItems } from '../utils/gameTimelineBudget'

type TimelineWindow = {
  gameId: string
  ready: boolean
  startAt?: number
  anchorIssue?: string
  draws: DrawResult[]
}

/** Only mounting or switching games opens a new visit. The entry boundary
 * stays fixed while its bounded display cache follows newly confirmed draws. */
export function useGameTimelineWindow(gameId: string, incoming: DrawResult[], loading: boolean, error = ''): TimelineWindow {
  const [window, setWindow] = useState<TimelineWindow>(() => ({ gameId, ready: false, draws: [] }))
  useEffect(() => {
    setWindow(previous => {
      const current: TimelineWindow = previous.gameId === gameId ? previous : { gameId, ready: false, draws: [] }
      const rows = incoming.filter(draw => draw.game_id === gameId && draw.numbers.length && Number.isFinite(Date.parse(draw.draw_at)))
        .sort((a, b) => Date.parse(b.draw_at) - Date.parse(a.draw_at) || b.issue.localeCompare(a.issue, 'en', { numeric: true }))
      if (!current.ready) {
        if (loading || error || (incoming.length > 0 && !rows.length)) return current
        const latest = rows[0]
        return { gameId, ready: true, startAt: latest ? Date.parse(latest.draw_at) : undefined, anchorIssue: latest?.issue, draws: latest ? [latest] : [] }
      }
      const byIssue = new Map(current.draws.map(draw => [draw.issue, draw]))
      for (const draw of rows) {
        if (current.startAt !== undefined && Date.parse(draw.draw_at) < current.startAt && draw.issue !== current.anchorIssue) continue
        // If a newly provisioned game had no results on entry, begin with its
        // first confirmed draw instead of injecting an entire recovery history.
        if (current.startAt === undefined && !byIssue.has(draw.issue) && draw !== rows[0]) continue
        byIssue.set(draw.issue, draw)
      }
      const draws = recentGameTimelineItems([...byIssue.values()].sort((a, b) => Date.parse(a.draw_at) - Date.parse(b.draw_at)))
      return draws.length === current.draws.length && draws.every((draw, index) => draw === current.draws[index]) ? current : { ...current, draws }
    })
  }, [gameId, incoming, loading, error])
  // A game switch must not expose even one render of the previous room.
  return window.gameId === gameId ? window : { gameId, ready: false, draws: [] }
}
