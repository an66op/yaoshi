import type { Game } from '../types'
import { gameRulesReady, rulesBlockedTiming } from './lotteryRules'

/** Use a server-confirmed separate window; never guess the next issue. */
export function roomBettingTarget(game: Game) {
  if (!gameRulesReady(game)) return { issue: game.period, timing: rulesBlockedTiming(game.timing) }
  return game.betting ?? { issue: game.period, timing: game.timing }
}
