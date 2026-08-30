import type { LotteryGame } from '../api/lottery'
import { resolveLotteryTiming } from './lotteryTiming'

/** Deadlines request a fresh server decision; they never invent/reopen a period.
 * Focus the fast recovery on the room being viewed, not every stale daily game. */
export function gameCatalogRefreshDelay(games: LotteryGame[] | undefined, nowMs: number | null, connected: boolean, activeGameId: string | null, failures = 0) {
  if (failures > 0) return Math.min(30_000, 2000 * 2 ** Math.min(failures - 1, 4))
  let delay = connected ? 30_000 : 10_000
  if (!games || nowMs === null) return 5000
  const candidates = activeGameId ? games.filter(game => game.id === activeGameId) : games
  for (const game of candidates) {
    if (game.enabled === false) continue
    const timing = resolveLotteryTiming(game, nowMs)
    const deadline = timing.phase === 'accepting' ? timing.sealAtMs
      : timing.phase === 'sealed' ? timing.drawAtMs
        : timing.phase === 'pending' ? timing.acceptAtMs : null
    if (deadline !== null && deadline > nowMs) {
      delay = Math.min(delay, Math.max(250, deadline - nowMs + 150))
    } else if (timing.phase === 'pending') {
      const nearby = deadline !== null && nowMs - deadline <= 120_000
      delay = Math.min(delay, nearby ? 2000 : activeGameId ? 10_000 : delay)
    } else if (['awaiting_draw', 'settling', 'settled'].includes(timing.phase) && game.source_healthy === true) {
      const elapsed = timing.drawAtMs === null ? Infinity : nowMs - timing.drawAtMs
      // Fast near the expected draw; a long outage must not poll at 2s forever.
      const nearby = elapsed >= -1500 && elapsed <= Math.max(120_000, Math.min(300_000, (timing.intervalSeconds ?? 75) * 2000))
      delay = Math.min(delay, nearby ? 2000 : activeGameId ? 10_000 : delay)
    } else if (activeGameId && (timing.phase === 'unavailable' || timing.phase === 'error')) {
      delay = Math.min(delay, 10_000)
    }
  }
  return delay
}
