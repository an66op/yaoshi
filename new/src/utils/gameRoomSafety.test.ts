import { describe, expect, it } from 'vitest'
import type { GameOdds } from '../api/portal'
import {
  DEFAULT_ROOM_FEATURES,
  canSubmitPlayWithOddsResponse,
  oddsForPlayCode,
  oddsForSelection,
  oddsLabel,
  playOddsFromResponse,
  roomFeaturesFromSettings,
} from './gameRoomSafety'

describe('room feature safety', () => {
  it('keeps the established member UI visible for legacy room settings', () => {
    expect(DEFAULT_ROOM_FEATURES).toEqual({
      showTurnover: true,
      showProfit: true,
      showRebate: true,
      webKeyboard: true,
      showMipai: true,
      showOrders: true,
      showStreak: true,
      showPrediction: true,
    })
  })

  it('only hides fields explicitly disabled by the room settings response', () => {
    expect(roomFeaturesFromSettings({
      prediction_enabled: true,
      game: {
        show_member_turnover: true,
        web_keyboard_enabled: true,
        show_mipai_tool: false,
        show_prediction_tool: true,
      },
    })).toEqual({
      showTurnover: true,
      showProfit: true,
      showRebate: true,
      webKeyboard: true,
      showMipai: false,
      showOrders: true,
      showStreak: true,
      showPrediction: true,
    })

    expect(roomFeaturesFromSettings({
      prediction_enabled: false,
      game: { show_prediction_tool: true },
    }).showPrediction).toBe(false)
  })
})

describe('authoritative game odds', () => {
  const response = (items: GameOdds['items'], showOdds = true): GameOdds => ({
    game_id: 'speed-racing',
    game_name: '极速赛车',
    show_odds: showOdds,
    items,
  })

  it('uses exact valid values returned by the backend for each play type', () => {
    const odds = playOddsFromResponse(response([
      { play_code: 'two_sided', play_name: '两面', odds: 1.993, min_bet: 1, max_bet: 1000, max_user_period: 5000 },
      { play_code: 'ball_1_5', play_name: '号码', odds: 9.9, min_bet: 1, max_bet: 1000, max_user_period: 5000 },
      { play_code: 'dragon_tiger', play_name: '龙虎', odds: 1.88, min_bet: 1, max_bet: 1000, max_user_period: 5000 },
      { play_code: 'sum', play_name: '冠亚和', odds: 1.97, min_bet: 1, max_bet: 1000, max_user_period: 5000 },
    ]))

    expect(oddsForSelection('冠军大', odds)).toBe(1.993)
    expect(oddsForSelection('冠军龙', odds)).toBe(1.88)
    expect(oddsForSelection('冠亚和小', odds)).toBe(1.97)
    expect(oddsForSelection('1/12345', odds)).toBe(9.9)
    expect(oddsLabel(oddsForPlayCode('two_sided', odds))).toBe('1.993')
    expect(oddsLabel(oddsForPlayCode('ball_1_5', odds))).toBe('9.90')
  })

  it('never invents a fallback when odds are hidden, missing, or invalid', () => {
    expect(playOddsFromResponse(response([], true))).toEqual({})
    expect(playOddsFromResponse(response([
      { play_code: 'two_sided', play_name: '两面', odds: 0, min_bet: 1, max_bet: 1000, max_user_period: 5000 },
      { play_code: 'ball_1_5', play_name: '号码', odds: Number.NaN, min_bet: 1, max_bet: 1000, max_user_period: 5000 },
    ]))).toEqual({})
    const hiddenResponse = response([
      { play_code: 'two_sided', play_name: '两面', odds: 1.993, min_bet: 1, max_bet: 1000, max_user_period: 5000 },
    ], false)
    expect(playOddsFromResponse(hiddenResponse)).toEqual({})
    expect(oddsForSelection('冠军大', {})).toBeNull()
    expect(oddsLabel(null)).toBe('待配置')
  })

  it('allows supported plays when the server explicitly hides odds, but not when the response is missing', () => {
    const hiddenResponse = response([], false)
    expect(canSubmitPlayWithOddsResponse('two_sided', hiddenResponse)).toBe(true)
    expect(canSubmitPlayWithOddsResponse('ball_1_5', hiddenResponse)).toBe(true)
    expect(canSubmitPlayWithOddsResponse('unknown', hiddenResponse)).toBe(false)
    expect(canSubmitPlayWithOddsResponse('two_sided', null)).toBe(false)
    expect(canSubmitPlayWithOddsResponse('two_sided', response([], true))).toBe(false)
    expect(oddsLabel(null, 2, true)).toBe('已隐藏')
  })
})
