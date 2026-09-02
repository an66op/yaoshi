import type { ComponentProps } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import type { DrawResult } from '../api/lottery'
import { BetKeyboard, GameTimeline } from './GameRoom'
import { roomBettingTarget } from '../utils/gameRoomBetting'
import type { Game } from '../types'
import { resolveLotteryTiming } from '../utils/lotteryTiming'

vi.mock('../api/client', () => ({ apiBase: 'http://localhost:8080/api', request: vi.fn(), publicRequest: vi.fn() }))

const draw: DrawResult = { id: 11, game_id: 'speed-racing', issue: '34136854', numbers: [6, 1, 2, 8, 9, 10, 4, 7, 5, 3], draw_at: '2026-08-30T06:46:00Z' }
const base: ComponentProps<typeof GameTimeline> = {
  gameId: 'speed-racing', gameTitle: '极速赛车', currentIssue: '34136855',
  messages: [], draws: [draw], feed: [], tickets: [], nickname: '王者玩家',
}
const markup = (updates: Partial<ComponentProps<typeof GameTimeline>> = {}) => renderToStaticMarkup(<GameTimeline {...base} {...updates} />)

describe('clock-independent game timeline', () => {
  it('uses default React memo with every rendered business field still exposed as a prop', () => {
    // No custom comparator may accidentally ignore a new issue, name or state.
    expect((GameTimeline as unknown as { compare: unknown }).compare).toBeNull()
    expect(markup()).not.toContain('下一期已开始受理。')
    expect(markup({ currentIssue: draw.issue })).toBe(markup())
    expect(markup({ currentIssue: '34136860' })).toBe(markup())
  })

  it('updates game names in the announcement and both accessible image targets', () => {
    const html = markup({ gameTitle: '更名后的赛车' })
    expect(html).toContain('【更名后的赛车 - 34136854】已开奖')
    expect(html).toContain('aria-label="预览更名后的赛车第34136854期开奖号码图片"')
    expect(html).toContain('aria-label="预览更名后的赛车最近开奖记录图片"')
  })

  it('retains short chat, accepted receipts, and feed labels when the period changes', () => {
    const html = markup({
      currentIssue: '34136856', nickname: '新的昵称',
      messages: [{ id: 5, user_id: 8, nickname: '新的昵称', room_type: 'group', room_scope: 'agent:2', game_id: 'speed-racing', content: '3', message_type: 'text', mine: true, created_at: '2026-08-30T06:45:00Z' }],
      tickets: [{ gameId: 'speed-racing', content: '4/88', lines: ['冠军[4/88.00]'], total: 88, balance: 1000, issue: '34136854', acceptedAt: '2026-08-30T06:45:10Z' }],
      feed: [{ nickname: '跟注成员', detail: '冠军[6/10.00]', amount: 10, created_at: '2026-08-30T06:45:20Z' }],
    })
    expect(html).toContain('<span class="game-chat-content">3</span>')
    expect(html).toContain('@新的昵称')
    expect(html).toContain('【极速赛车 - 34136854】下单成功')
    expect(html).toContain('【极速赛车 · 第 34136856 期】')
    expect(html).not.toContain('我的本期注单')
    expect(html).not.toContain('赔率')
  })

  it('keeps public settlement results without another personal settlement card', () => {
    const html = markup({ messages: [{ id: 9, user_id: 0, nickname: '开奖助手', room_type: 'group', room_scope: 'agent:2', game_id: 'speed-racing', content: '【极速赛车 - 34136854】\n结算内容如下：\n[王者玩家]\n得分：+231.30\n冠军 [5/9.00=-9.00]\n冠军 [6/27.00=+240.30]', message_type: 'settlement', mine: false, created_at: '2026-08-30T06:46:01Z' }] })
    expect(html).toContain('结算内容如下：')
    expect(html).toContain('得分：+231.30')
    expect(html).not.toContain('结算完成')
    expect(html).not.toContain('personal-settlement')
  })

  it('renders separate chronological draw announcements while nobody has bet', () => {
    const next = { ...draw, id: 12, issue: '34136855', draw_at: '2026-08-30T06:47:15Z' }
    const html = markup({ draws: [next, draw], messages: [], tickets: [], feed: [] })
    expect(html.match(/】已开奖/g)).toHaveLength(2)
    expect(html.indexOf('34136854】已开奖')).toBeLessThan(html.indexOf('34136855】已开奖'))
    expect(html.match(/grid-template-columns:repeat\(10, minmax\(0, 1fr\)\)/g)).toHaveLength(2)
  })

  it('renders Mark Six announcements with fixed wave colours, a separated special ball and no racing canvas', () => {
    const markSix: DrawResult = { id: 19, game_id: 'bingo-mark-six', issue: '115049455', numbers: [5, 9, 40, 47, 29, 2, 18], draw_at: '2026-09-01T06:45:00Z' }
    const html = markup({ gameId: markSix.game_id, gameTitle: '宾果六合彩', draws: [markSix], drawHistory: [markSix] })
    expect(html).toContain('lottery-ball mark-six-ball wave-green')
    expect(html).toContain('lottery-ball mark-six-ball wave-blue')
    expect(html).toContain('lottery-ball mark-six-ball wave-red mark-six-special-ball')
    expect(html).toContain('特码：18 红波 小双')
    expect(html).not.toContain('龙虎：')
    expect(html).not.toContain('draw-result-card')
    expect(html).not.toContain('aria-label="预览宾果六合彩')
  })

  it('keeps all shortcuts compact without a description output row', () => {
    const noop = () => undefined
    const html = renderToStaticMarkup(<BetKeyboard mode="quick" odds={{}} oddsHidden oddsResponseReady selectedCount={0} showModes={false} onShortcut={noop} onBackspace={noop} onClear={noop} onConfirm={noop} onModeChange={noop} onSelectNumber={noop} onSelectOption={noop} />)
    for (const label of ['梭哈', '取消', '上分', '查', '下分', '重复']) expect(html).toContain(`>${label}</button>`)
    expect(html).not.toContain('keyboard-shortcut-notice')
    expect(html).not.toContain('<output')
  })

  it('adds the chat tie key only to exact contracts that support a tie selection', () => {
    const noop = () => undefined
    const keyboard = (gameId: string, ruleVersion?: string) => renderToStaticMarkup(<BetKeyboard gameId={gameId} ruleVersion={ruleVersion} mode="quick" odds={{}} oddsHidden oddsResponseReady selectedCount={0} showModes={false} onShortcut={noop} onBackspace={noop} onClear={noop} onConfirm={noop} onModeChange={noop} onSelectNumber={noop} onSelectOption={noop} />)
    for (const gameId of ['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1']) {
      expect(keyboard(gameId, 'digits5-v3')).toMatch(/<button[^>]*>和<\/button>/)
      expect(keyboard(gameId, 'digits5-v3')).not.toMatch(/<button[^>]*>总和<\/button>/)
    }
    expect(keyboard('speed-ssc')).not.toMatch(/<button[^>]*>和<\/button>/)
    expect(keyboard('speed-ssc', 'digits5-v2')).not.toMatch(/<button[^>]*>和<\/button>/)
    expect(keyboard('sg-ssc')).not.toMatch(/<button[^>]*>和<\/button>/)
    for (const gameId of ['bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4']) expect(keyboard(gameId, 'digits5-v3')).not.toMatch(/<button[^>]*>和<\/button>/)
    expect(keyboard('pc-canada', 'pc28-v1')).toMatch(/<button[^>]*>和<\/button>/)
    expect(keyboard('canada-28', 'pc28-v2')).toMatch(/<button[^>]*>和<\/button>/)
    expect(keyboard('canada-20', 'pc28-v3')).toMatch(/<button[^>]*>和<\/button>/)
    expect(keyboard('canada-28', 'pc28-v1')).not.toMatch(/<button[^>]*>和<\/button>/)
  })
})

describe('server-confirmed room betting target', () => {
  const timing = resolveLotteryTiming({ issue_status: 'awaiting_draw', next_draw_at: '2026-08-30T06:46:00Z', seal_seconds: 30 }, Date.parse('2026-08-30T06:46:05Z'))
  const game = { id: 'speed-racing', rulesReady: true, period: '34136854', timing } as Game

  it('does not invent a next issue when the source has not confirmed one', () => {
    expect(roomBettingTarget(game)).toEqual({ issue: '34136854', timing })
    expect(roomBettingTarget(game).timing.accepting).toBe(false)
  })

  it('uses the confirmed next issue while preserving the previous draw header', () => {
    const nextTiming = { ...timing, accepting: true }
    const nextGame = { ...game, betting: { issue: '34136855', timing: nextTiming } }
    expect(roomBettingTarget(nextGame)).toEqual(nextGame.betting)
    expect(nextGame.period).toBe('34136854')
    expect(nextGame.timing.accepting).toBe(false)
  })

  it('cannot open a next betting window for unknown rules or an explicit server denial', () => {
    const next = { ...game, betting: { issue: '34136855', timing: { ...timing, accepting: true } } }
    for (const blocked of [{ ...next, id: 'canada-28' }, { ...next, rulesReady: false }]) {
      expect(roomBettingTarget(blocked)).toMatchObject({ issue: game.period, timing: { accepting: false, phaseLabel: '玩法待配置' } })
    }
  })
})
