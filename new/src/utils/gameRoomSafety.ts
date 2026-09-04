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

export type BasePlayCode = 'two_sided' | 'ball_1_5' | 'dragon_tiger' | 'dragon_tiger_tie' | 'sum' | 'leopard' | 'straight' | 'pair' | 'half_straight' | 'mixed'
type BingoRacingSumSelectionKey = 'big' | 'small' | 'odd' | 'even' | '3' | '4' | '5' | '6' | '7' | '8' | '9' | '10' | '11' | '12' | '13' | '14' | '15' | '16' | '17' | '18' | '19'
export type BingoRacingSumPricingCode = `sum_${BingoRacingSumSelectionKey}`
export type OddsPlayCode = BasePlayCode | BingoRacingSumPricingCode
export type PlayOdds = Partial<Record<OddsPlayCode, number>>
type SupportedPlayCode = BasePlayCode

const BINGO_RACING_GAME_IDS = new Set(['bingo-racing-a', 'bingo-racing-b'])

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
  return ['two_sided', 'ball_1_5', 'dragon_tiger', 'dragon_tiger_tie', 'sum', 'leopard', 'straight', 'pair', 'half_straight', 'mixed'].includes(value)
}

function isOddsPlayCode(value: string): value is OddsPlayCode {
  return isSupportedPlayCode(value) || /^sum_(?:big|small|odd|even|[3-9]|1[0-9])$/.test(value)
}

export function bingoRacingSumPricingCode(selection: string): BingoRacingSumPricingCode | null {
  const normalized = selection.trim().toLowerCase()
  const side = ({ '大': 'big', '小': 'small', '单': 'odd', '双': 'even', big: 'big', small: 'small', odd: 'odd', even: 'even' } as const)[normalized as '大' | '小' | '单' | '双' | 'big' | 'small' | 'odd' | 'even']
  if (side) return `sum_${side}`
  if (!/^\d+$/.test(normalized)) return null
  const value = Number(normalized)
  return value >= 3 && value <= 19 ? `sum_${value}` as BingoRacingSumPricingCode : null
}

export function pricingPlayCode(gameId: string, playCode: string, selection = ''): OddsPlayCode | null {
  if (!isSupportedPlayCode(playCode)) return null
  if (BINGO_RACING_GAME_IDS.has(gameId) && playCode === 'sum') return bingoRacingSumPricingCode(selection)
  return playCode
}

/** Only odds explicitly returned by the member odds endpoint are accepted. */
export function playOddsFromResponse(response: GameOdds | null | undefined): PlayOdds {
  if (!response?.show_odds || !Array.isArray(response.items)) return {}
  const resolved: PlayOdds = {}
  for (const item of response.items) {
    if (!isValidOdds(item?.odds)) continue
    if (isOddsPlayCode(item.play_code)) {
      resolved[item.play_code] = item.odds
    }
  }
  return resolved
}

export function playCodeForSelection(play: string): keyof PlayOdds | null {
  const normalized = play.replace(/\s+/g, '')
  if (!normalized) return null
  if (/冠亚(?:和)?|^和(?:\/|[大小单双0-9])/.test(normalized)) return 'sum'
  if (/龙虎和|(?:[1-5]\/|球)和(?:\/|$)/.test(normalized)) return 'dragon_tiger_tie'
  if (/[龙虎]/.test(normalized)) return 'dragon_tiger'
  if (/[大小单双]/.test(normalized)) return 'two_sided'
  if (/^(?:(?:10|[1-9])\/)?\d+$/.test(normalized)) return 'ball_1_5'
  return null
}

export function oddsForSelection(play: string, odds: PlayOdds, gameId = ''): number | null {
  const code = playCodeForSelection(play)
  if (!code) return null
  const normalized = play.replace(/\s+/g, '')
  const sumSelection = code === 'sum' ? normalized.match(/(?:冠亚(?:和)?|^和)\/?(大|小|单|双|[3-9]|1[0-9])/)?.[1] ?? '' : ''
  return oddsForPlaySelection(gameId, code, sumSelection, odds)
}

export function oddsForPlayCode(playCode: string, odds: PlayOdds): number | null {
  if (!isOddsPlayCode(playCode)) return null
  const value = odds[playCode]
  return isValidOdds(value) ? value : null
}

export function oddsForPlaySelection(gameId: string, playCode: string, selection: string, odds: PlayOdds): number | null {
  const pricingCode = pricingPlayCode(gameId, playCode, selection)
  return pricingCode ? oddsForPlayCode(pricingCode, odds) : null
}

/**
 * `show_odds=false` is an explicit room policy, not a missing configuration.
 * The server still resolves and validates the effective odds, while the
 * browser deliberately receives no numeric value. A missing response remains
 * fail-closed.
 */
export function canSubmitPlayWithOddsResponse(playCode: string, response: GameOdds | null | undefined, selection = ''): boolean {
  if (!response || response.rules_ready === false) return false
  const pricingCode = pricingPlayCode(response.game_id, playCode, selection)
  if (!pricingCode) return false
  // Hiding the numeric value is only a presentation policy. The endpoint still
  // returns one zero-valued item for every configured market, so its item list
  // remains the authoritative availability contract. Never let a hidden room
  // resurrect a missing selection (notably one Bingo Racing crown-sum price).
  if (response.show_odds === false) return response.items.some(item => item.play_code === pricingCode)
  return oddsForPlayCode(pricingCode, playOddsFromResponse(response)) !== null
}

export function oddsLabel(value: number | null | undefined, digits = 3, hidden = false) {
  if (hidden) return '已隐藏'
  if (!isValidOdds(value)) return '待配置'
  const precision = Math.max(2, Math.min(4, digits))
  const [whole, decimal = ''] = value.toFixed(precision).split('.')
  return `${whole}.${decimal.replace(/0+$/, '').padEnd(2, '0')}`
}
