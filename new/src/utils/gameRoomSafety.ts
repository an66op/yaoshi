import type { GameOdds, RoomSettings } from '../api/portal'

export type RoomFeatureSettings = {
  showTurnover: boolean
  showProfit: boolean
  showRebate: boolean
  webKeyboard: boolean
  showMipai: boolean
  showOrders: boolean
  showStreak: boolean
  showPrediction: boolean
}

export type PlayOdds = Partial<Record<'two_sided' | 'ball_1_5' | 'dragon_tiger' | 'sum', number>>
type SupportedPlayCode = keyof PlayOdds

/**
 * These are presentation preferences, not authorization boundaries. Legacy
 * rooms do not contain the newer keys, so the established member UI remains
 * visible unless the room owner explicitly turns an item off.
 */
export const DEFAULT_ROOM_FEATURES: Readonly<RoomFeatureSettings> = Object.freeze({
  showTurnover: true,
  showProfit: true,
  showRebate: true,
  webKeyboard: true,
  showMipai: true,
  showOrders: true,
  showStreak: true,
  showPrediction: true,
})

export function roomFeaturesFromSettings(settings: Pick<RoomSettings, 'prediction_enabled' | 'game'>): RoomFeatureSettings {
  const game = settings.game && typeof settings.game === 'object' ? settings.game : {}
  return {
    showTurnover: game.show_member_turnover !== false,
    showProfit: game.show_member_profit !== false,
    showRebate: game.show_member_rebate !== false,
    webKeyboard: game.web_keyboard_enabled !== false,
    showMipai: game.show_mipai_tool !== false,
    showOrders: game.show_orders_tool !== false,
    showStreak: game.show_streak_tool !== false,
    showPrediction: settings.prediction_enabled !== false && game.show_prediction_tool !== false,
  }
}

function isValidOdds(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value > 1
}

function isSupportedPlayCode(value: string): value is SupportedPlayCode {
  return value === 'two_sided' || value === 'ball_1_5' || value === 'dragon_tiger' || value === 'sum'
}

/** Only odds explicitly returned by the member odds endpoint are accepted. */
export function playOddsFromResponse(response: GameOdds | null | undefined): PlayOdds {
  if (!response?.show_odds || !Array.isArray(response.items)) return {}
  const resolved: PlayOdds = {}
  for (const item of response.items) {
    if (!isValidOdds(item?.odds)) continue
    if (item.play_code === 'two_sided' || item.play_code === 'ball_1_5' || item.play_code === 'dragon_tiger' || item.play_code === 'sum') {
      resolved[item.play_code] = item.odds
    }
  }
  return resolved
}

export function playCodeForSelection(play: string): keyof PlayOdds | null {
  const normalized = play.replace(/\s+/g, '')
  if (!normalized) return null
  if (/冠亚(?:和)?/.test(normalized)) return 'sum'
  if (/[龙虎]/.test(normalized)) return 'dragon_tiger'
  if (/[大小单双]/.test(normalized)) return 'two_sided'
  if (/^(?:(?:10|[1-9])\/)?\d+$/.test(normalized)) return 'ball_1_5'
  return null
}

export function oddsForSelection(play: string, odds: PlayOdds): number | null {
  const code = playCodeForSelection(play)
  return code ? oddsForPlayCode(code, odds) : null
}

export function oddsForPlayCode(playCode: string, odds: PlayOdds): number | null {
  if (!isSupportedPlayCode(playCode)) return null
  const value = odds[playCode]
  return isValidOdds(value) ? value : null
}

/**
 * `show_odds=false` is an explicit room policy, not a missing configuration.
 * The server still resolves and validates the effective odds, while the
 * browser deliberately receives no numeric value. A missing response remains
 * fail-closed.
 */
export function canSubmitPlayWithOddsResponse(playCode: string, response: GameOdds | null | undefined): boolean {
  if (!response || !isSupportedPlayCode(playCode)) return false
  if (response.show_odds === false) return true
  return oddsForPlayCode(playCode, playOddsFromResponse(response)) !== null
}

export function oddsLabel(value: number | null | undefined, digits = 3, hidden = false) {
  if (hidden) return '已隐藏'
  if (!isValidOdds(value)) return '待配置'
  const precision = Math.max(2, Math.min(4, digits))
  const [whole, decimal = ''] = value.toFixed(precision).split('.')
  return `${whole}.${decimal.replace(/0+$/, '').padEnd(2, '0')}`
}
