import type { Game } from '../types'

/** Use a server-confirmed separate window; never guess the next issue. */
export function roomBettingTarget(game: Game) {
  return game.betting ?? { issue: game.period, timing: game.timing }
}
