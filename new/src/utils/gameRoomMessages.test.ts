import { describe, expect, it } from 'vitest'
import type { ChatMessage } from '../api/chat'
import type { DrawResult } from '../api/lottery'
import { buildGameTimelineEntries, compactAcceptedReceiptContent, formatGameMessageTime, isRoomCommandContent, ticketsForGame } from './gameRoomMessages'

const acceptedContent = '@王者玩家\n【极速飞艇 - 54776105】下单成功\n冠军[4/352.00] · 赔率 9.900\n\n使用：352.00\n剩余：10035483.00'

describe('game-room command routing', () => {
  it.each(['3', '6', '4444', '单', '冠军', '冠军4', '买123', '3,4,5', '4/88', '4444/88', '查', '取消', '重复', '大梭哈', '申请上分 200.50', '下分/10'])(
    'sends %s to the authoritative command parser', content => {
      expect(isRoomCommandContent(content)).toBe(true)
    },
  )

  it.each(['', ' ', '...', '.', ',', '，', '#', '好', '大家好', '今天3个人', '大伙一起聊', '上分', '上分100.123', '下分0'])(
    'does not treat ordinary/invalid application text %s as a bet', content => {
      expect(isRoomCommandContent(content)).toBe(false)
    },
  )
})

describe('compact accepted receipts', () => {
  it('removes only the displayed odds suffix, preserving all ticket and balance values', () => {
    expect(compactAcceptedReceiptContent(acceptedContent)).toBe('@王者玩家\n【极速飞艇 - 54776105】下单成功\n冠军[4/352.00]\n\n使用：352.00\n剩余：10035483.00')
    expect(acceptedContent).toContain('赔率 9.900')
  })

  it('works for future game names and multiple lines without changing their order', () => {
    const receipt = '【另一个彩种 - 123】下单成功\n第一名[8/20.00] · 赔率 9.8\n第二名[7/30.00] · 赔率 10\n使用：50.00'
    expect(compactAcceptedReceiptContent(receipt)).toBe('【另一个彩种 - 123】下单成功\n第一名[8/20.00]\n第二名[7/30.00]\n使用：50.00')
  })

  it.each(['大家讨论 · 赔率 9.900', '@会员\n解析失败，请使用玩法/金额', '@会员\n【极速飞艇 - 54776105】结算\n冠军[4/352.00] · 赔率 9.900'])(
    'never edits non-acceptance messages', content => expect(compactAcceptedReceiptContent(content)).toBe(content),
  )
})

describe('durable game timeline', () => {
  const message = (id: number, content: string, mine: boolean): ChatMessage => ({
    id, content, mine, game_id: 'speed-fly', user_id: mine ? 5 : 0, nickname: mine ? '王者玩家' : '开奖助手',
    room_type: 'group', room_scope: 'agent:2', message_type: mine ? 'text' : 'application',
    created_at: `2026-08-30T05:10:0${id}Z`,
  })

  it('shows one durable success receipt, never an extra synthesized bet summary', () => {
    const messages = [message(1, '4444/88', true), message(2, acceptedContent, false)]
    const entries = buildGameTimelineEntries({ gameId: 'speed-fly', messages, notices: [], tickets: [], feed: [] })
    expect(entries).toHaveLength(2)
    expect(entries.map(entry => entry.kind)).toEqual(['chat', 'chat'])
    expect(entries.map(entry => entry.key)).toEqual(['chat:1', 'chat:2'])
    expect(entries[1].value).toEqual(messages[1])
  })

  it('keeps explicit query replies and the draw in chronological order', () => {
    const queryReply = message(4, '@王者玩家\n【极速飞艇 - 54776105】\n冠军 [4/352.00]\n当期使用积分：352.00', false)
    const draw: DrawResult = { id: 10, game_id: 'speed-fly', issue: '54776105', numbers: [4, 1, 2, 3, 5, 6, 7, 8, 9, 10], draw_at: '2026-08-30T05:12:15Z' }
    const entries = buildGameTimelineEntries({ gameId: 'speed-fly', messages: [queryReply, message(3, '查', true)], draw, notices: [], tickets: [], feed: [] })
    expect(entries.map(entry => entry.key)).toEqual(['chat:3', 'chat:4', 'draw:10'])
    expect(entries[1].value).toEqual(queryReply)
  })

  it('restores a detailed-panel receipt even without a chat message', () => {
    const ticket = { gameId: 'speed-fly', content: '4/88', issue: '54776105', acceptedAt: '2026-08-30T05:10:00Z', lines: ['冠军[4/88.00]'], total: 88, balance: 912 }
    const entries = buildGameTimelineEntries({ gameId: 'speed-fly', messages: [], notices: [], tickets: [ticket], feed: [] })
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: 'ticket', value: ticket })
  })

  it('keeps confirmed tickets across an issue change even if history reload fails', () => {
    const previous = { gameId: 'speed-fly', content: '4/88', issue: '54776105', acceptedAt: '2026-08-30T05:10:00Z', lines: ['冠军[4/88.00]'], total: 88, balance: 912 }
    const nextIssueTickets = ticketsForGame([previous], 'speed-fly')
    expect(nextIssueTickets).toEqual([previous])
    expect(buildGameTimelineEntries({ gameId: 'speed-fly', messages: [], notices: [], tickets: nextIssueTickets, feed: [] })[0].value).toEqual(previous)
    expect(ticketsForGame([previous], 'speed-racing')).toEqual([])
  })

  it('formats chat timestamps in Beijing time without midnight becoming 24:00', () => {
    expect(formatGameMessageTime('2026-08-30T05:10:02Z')).toBe('13:10')
    expect(formatGameMessageTime('2026-08-29T16:00:02Z')).toBe('00:00')
    expect(formatGameMessageTime('invalid')).toBe('刚刚')
  })
})
