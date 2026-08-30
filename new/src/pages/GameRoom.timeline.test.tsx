import type { ComponentProps } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import type { DrawResult } from '../api/lottery'
import type { MemberNotification } from '../api/portal'
import { GameTimeline } from './GameRoom'

vi.mock('../api/client', () => ({ apiBase: 'http://localhost:8080/api', request: vi.fn(), publicRequest: vi.fn() }))

const draw: DrawResult = { id: 11, game_id: 'speed-racing', issue: '34136854', numbers: [6, 1, 2, 8, 9, 10, 4, 7, 5, 3], draw_at: '2026-08-30T06:46:00Z' }
const base: ComponentProps<typeof GameTimeline> = {
  gameId: 'speed-racing', gameTitle: '极速赛车', currentIssue: '34136855', accepting: true,
  messages: [], draws: [draw], notices: [], feed: [], tickets: [], nickname: '王者玩家',
}
const markup = (updates: Partial<ComponentProps<typeof GameTimeline>> = {}) => renderToStaticMarkup(<GameTimeline {...base} {...updates} />)

describe('clock-independent game timeline', () => {
  it('uses default React memo with every rendered business field still exposed as a prop', () => {
    // No custom comparator may accidentally ignore a new issue, name or state.
    expect((GameTimeline as unknown as { compare: unknown }).compare).toBeNull()
    expect(markup()).toContain('下一期已开始受理。')
    expect(markup({ accepting: false })).not.toContain('下一期已开始受理。')
    expect(markup({ currentIssue: draw.issue })).not.toContain('下一期已开始受理。')
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

  it('keeps settlement numbers and financial details, filtering by the current game', () => {
    const notice: MemberNotification = {
      id: 9, game_id: 'speed-racing', title: '结算完成', content: '', level: 'info', category: 'winning', link: '', read: false,
      issue: draw.issue, draw_numbers: draw.numbers, won_count: 1, stake_amount: 88, payout_amount: 871.2,
      bet_details: [{ play_name: '冠军', selection: '6', amount: 88, odds: 9.9, result: 'won', payout: 871.2 }],
      created_at: '2026-08-30T06:46:01Z',
    }
    const html = markup({ notices: [notice] })
    expect(html).toContain('【极速赛车 - 34136854】结算完成')
    expect(html).toContain('中奖 871.20')
    expect(html).toContain('投注：88.00 · 中奖：871.20')
    expect(markup({ notices: [{ ...notice, game_id: 'speed-fly' }] })).not.toContain('结算完成')
  })
})
