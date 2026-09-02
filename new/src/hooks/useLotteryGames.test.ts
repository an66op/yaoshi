import { describe, expect, it, vi } from 'vitest'
import { gameLogoPaths, lotteryGameLogo, mapLotteryGame } from './useLotteryGames'
import { SPEED_RACING_TRIO_SRC } from '../data/gameArtwork'
import type { LotteryGame } from '../api/lottery'

// Logo selection is independent of browser/API state.
vi.mock('../api/client', () => ({
  apiBase: 'http://localhost:8080/api',
  request: vi.fn(),
  publicRequest: vi.fn(),
}))

const logoAssets = import.meta.glob('../../public/images/game-logos/*', { query: '?url', import: 'default' })

describe('lotteryGameLogo', () => {
  it('keeps the original speed-racing logo separate from result-card artwork', () => {
    expect(lotteryGameLogo('speed-racing')).toBe('/images/game-logos/speed-racing.png')
    expect(lotteryGameLogo('speed-racing')).not.toBe(SPEED_RACING_TRIO_SRC)
  })

  it('uses existing dedicated logo assets for every supported game', () => {
    for (const [gameId, path] of Object.entries(gameLogoPaths)) {
      expect(path, gameId).toMatch(/^\/images\/game-logos\//)
      expect(logoAssets[`../../public${path}`], gameId).toBeTypeOf('function')
      expect(lotteryGameLogo(gameId)).toBe(path)
    }
  })

  it('leaves unknown games without a fabricated logo', () => {
    expect(lotteryGameLogo()).toBeUndefined()
    expect(lotteryGameLogo('unconfigured-game')).toBeUndefined()
  })
})

describe('mapLotteryGame timing', () => {
  const drawMs = Date.parse('2026-08-30T13:31:10+08:00')
  const remote: LotteryGame = {
    id: 'speed-fly', code: 'speed-fly', name: '极速飞艇', category: 'racing', lobby_category: '彩票', lobby_sort_order: 0,
    badge: '飞艇', badge_color: '#f43f94', enabled: true, issue: '54776108', current_issue: '54776109', latest_numbers: [2, 5, 1, 4, 6, 9, 3, 7, 8, 10],
    next_draw_at: new Date(drawMs).toISOString(), seal_at: new Date(drawMs - 20_000).toISOString(), draw_interval: 90, seal_seconds: 20,
    issue_status: 'accepting', source_healthy: true, source_kind: 'external', source_name: '168高频彩开奖', sync_status: 'ok',
  }

  it('shares stage-aware countdowns across game-room, lobby, and switcher consumers', () => {
    const accepting = mapLotteryGame(remote, drawMs - 50_000)
    expect(accepting.due).toBe('00:30')
    expect(accepting.timing).toMatchObject({ phase: 'accepting', accepting: true, sealSeconds: 20, intervalSeconds: 90 })
    const sealed = mapLotteryGame(remote, drawMs - 15_000)
    expect(sealed.due).toBe('00:15')
    expect(sealed.timing).toMatchObject({ phase: 'sealed', accepting: false, phaseLabel: '封盘倒计时' })
    expect(sealed.period).toBe('54776109')
    expect(sealed.latestIssue).toBe('54776108')
    expect(sealed.balls).toEqual(remote.latest_numbers)
    expect(sealed.logo).toBe('/images/game-logos/speed-fly.png')
  })

  it('retains explicit server state without using a positive countdown to reopen it', () => {
    const game = mapLotteryGame({ ...remote, issue_status: 'error' }, drawMs - 50_000)
    expect(game.issueStatus).toBe('error')
    expect(game.timing.accepting).toBe(false)
    expect(game.due).toBe('--:--')
  })

  it('does not label the open period as the latest published draw when no result exists', () => {
    const game = mapLotteryGame({ ...remote, issue: '', latest_numbers: [] }, drawMs - 50_000)
    expect(game.period).toBe('54776109')
    expect(game.latestIssue).toBe('—')
    expect(game.balls).toEqual([])
  })

  it('does not invent an open period from the latest published draw while waiting for the source', () => {
    const game = mapLotteryGame({ ...remote, current_issue: '', issue_status: 'awaiting_draw' }, drawMs)
    expect(game.period).toBe('—')
    expect(game.latestIssue).toBe('54776108')
    expect(game.timing.accepting).toBe(false)
  })

  it('shows missing draw timing explicitly and keeps the original racing logo', () => {
    const game = mapLotteryGame({ ...remote, id: 'speed-racing', next_draw_at: '0001-01-01T00:00:00Z', latest_numbers: [] }, drawMs)
    expect(game.due).toBe('--:--')
    expect(game.timing.accepting).toBe(false)
    expect(game.logo).toBe('/images/game-logos/speed-racing.png')
    expect(game.balls).toEqual([])
  })

  it('separates the visible drawing issue from the server-confirmed betting issue', () => {
    const payload: LotteryGame = { ...remote, issue_status: 'awaiting_draw', betting_window: {
      issue: '54776110', issue_status: 'accepting', accept_at: new Date(drawMs).toISOString(),
      next_draw_at: new Date(drawMs + 90_000).toISOString(), seal_at: new Date(drawMs + 70_000).toISOString(),
      draw_interval: 90, seal_seconds: 20,
    } }
    expect(mapLotteryGame(payload, drawMs + 1000)).toMatchObject({
      period: '54776109', latestIssue: '54776108', timing: { phase: 'awaiting_draw', accepting: false },
      betting: { issue: '54776110', timing: { accepting: true, due: '01:09' } },
    })
    expect(mapLotteryGame(payload, drawMs + 70_000).betting?.timing.accepting).toBe(false)
    expect(mapLotteryGame({ ...payload, source_healthy: false }, drawMs + 1000).betting).toBeUndefined()
    expect(mapLotteryGame({ ...payload, betting_window: null }, drawMs + 1000).betting).toBeUndefined()
  })

  it('maps explicit rule readiness and keeps unconfigured results readable without an open betting window', () => {
    const supported = mapLotteryGame({ ...remote, rules_ready: true, rule_version: 'racing-v1' }, drawMs - 50_000)
    expect(supported).toMatchObject({ rulesReady: true, ruleVersion: 'racing-v1', timing: { accepting: true } })
    const denied = mapLotteryGame({ ...remote, rules_ready: false, rules_message: '规则审核中' }, drawMs - 50_000)
    expect(denied).toMatchObject({ rulesReady: false, rulesMessage: '规则审核中', timing: { accepting: false, phaseLabel: '玩法待配置' } })
    expect(denied.balls).toEqual(remote.latest_numbers)
    const unversionedPC28 = mapLotteryGame({ ...remote, id: 'pc-canada', name: 'PC加拿大', category: 'PC28', rules_ready: true }, drawMs - 50_000)
    expect(unversionedPC28).toMatchObject({ rulesReady: false, timing: { accepting: false } })
    expect(unversionedPC28.betting).toBeUndefined()
    const versionedPC28 = mapLotteryGame({ ...remote, id: 'pc-canada', name: 'PC加拿大', category: 'PC28', rules_ready: true, rule_version: 'pc28-v1', latest_numbers: [9, 1, 9] }, drawMs - 50_000)
    expect(versionedPC28).toMatchObject({ rulesReady: true, ruleVersion: 'pc28-v1', balls: [9, 1, 9], timing: { accepting: true } })
    const unversionedBingoA = mapLotteryGame({ ...remote, id: 'bingo-racing-a', name: '宾果赛车(A)', rules_ready: true }, drawMs - 50_000)
    expect(unversionedBingoA).toMatchObject({ rulesReady: false, timing: { accepting: false } })
    const versionedBingoA = mapLotteryGame({ ...remote, id: 'bingo-racing-a', name: '宾果赛车(A)', rules_ready: true, rule_version: 'racing-v2' }, drawMs - 50_000)
    expect(versionedBingoA).toMatchObject({ rulesReady: true, ruleVersion: 'racing-v2', timing: { accepting: true } })
    const bingoSSC1 = mapLotteryGame({ ...remote, id: 'bingo-ssc-1', name: '宾果时时彩(一)', category: 'ssc', rules_ready: true, rule_version: 'digits5-v3', latest_numbers: [2, 8, 5, 7, 9] }, drawMs - 50_000)
    expect(bingoSSC1).toMatchObject({ rulesReady: true, ruleVersion: 'digits5-v3', balls: [2, 8, 5, 7, 9], timing: { accepting: true } })
  })
})
