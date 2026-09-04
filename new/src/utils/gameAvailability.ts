import type { Game } from '../types'
import { gameRulesReady } from './lotteryRules'

export type GameAvailability = {
  kind: 'results-only' | 'source-paused'
  label: string
  cardText: string
  roomMessage: string
  detailText: string
}

/**
 * Translate internal readiness into stable member-facing states. Callers may
 * pass an already reconciled readiness bit when catalog, odds and assistant
 * snapshots all have to agree.
 */
export function gameAvailability(
  game: Pick<Game, 'id' | 'sourceHealthy' | 'rulesReady' | 'ruleVersion'>,
  reconciledRulesReady = gameRulesReady(game),
): GameAvailability | null {
  if (!game.sourceHealthy) return {
    kind: 'source-paused',
    label: '开奖暂停',
    cardText: '开奖暂停 · 投注暂停',
    roomMessage: '开奖同步暂时暂停，当前可查看已公布结果和聊天，投注已暂停。',
    detailText: '开奖恢复并确认当前期号后再恢复受理，当前不会生成注单。',
  }
  if (!reconciledRulesReady) return {
    kind: 'results-only',
    label: '仅开奖',
    cardText: '仅开奖 · 投注未开放',
    roomMessage: '当前仅提供开奖查看和聊天，投注暂未开放。',
    detailText: '玩法、赔率、封盘与结算全部核验通过后才会开放提交，当前不会生成注单。',
  }
  return null
}
