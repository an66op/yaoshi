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
})
