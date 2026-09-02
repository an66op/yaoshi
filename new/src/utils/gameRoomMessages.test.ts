import { describe, expect, it } from 'vitest'
import type { ChatMessage } from '../api/chat'
import type { DrawResult } from '../api/lottery'
import { buildGameTimelineEntries, compactAcceptedReceiptContent, drawHistoryAtIssue, formatGameMessageTime, isRepeatableBetInput, isRoomCommandContent, keyboardShortcutInput, latestBetInput, ticketsForGame } from './gameRoomMessages'
import { GAME_TIMELINE_LIMIT } from './gameTimelineBudget'

const acceptedContent = '@王者玩家\n【极速飞艇 - 54776105】下单成功\n冠军[4/352.00] · 赔率 9.900\n\n使用：352.00\n剩余：10035483.00'

describe('game-room command routing', () => {
  it.each(['3', '6', '4444', '单', '冠军', '冠军4', '买123', '3,4,5', '4/88', '4444/88', '1大5', '和大5', '豹子5', '前三豹子5', '查', '取消', '重复', '大梭哈', '申请上分 200.50', '下分/10'])(
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
  it('compacts odds and integer stakes without changing the stored receipt or balance', () => {
    expect(compactAcceptedReceiptContent(acceptedContent)).toBe('@王者玩家\n【极速飞艇 - 54776105】下单成功\n冠军[4/352]\n\n使用：352\n剩余：10035483.00')
    expect(acceptedContent).toContain('赔率 9.900')
  })

  it('works for future game names and multiple lines without changing their order', () => {
    const receipt = '【另一个彩种 - 123】下单成功\n第一名[8/20.00] · 赔率 9.8\n第二名[7/30.00] · 赔率 10\n使用：50.00'
    expect(compactAcceptedReceiptContent(receipt)).toBe('【另一个彩种 - 123】下单成功\n第一名[8/20]\n第二名[7/30]\n使用：50')
  })

  it('compacts grouped historical stakes while preserving genuine cents, balance and other decimals', () => {
    const receipt = '【极速赛车 - 34137294】下单成功\n冠军[2/50.00 3/50.00]\n亚军[2/1.25 3/0.50]\n使用：101.75\n剩余：100.00\n备注：50.00'
    const compact = '【极速赛车 - 34137294】下单成功\n冠军[2/50 3/50]\n亚军[2/1.25 3/0.50]\n使用：101.75\n剩余：100.00\n备注：50.00'
    expect(compactAcceptedReceiptContent(receipt)).toBe(compact)
    expect(compactAcceptedReceiptContent(compact)).toBe(compact)
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
    const entries = buildGameTimelineEntries({ gameId: 'speed-fly', messages, tickets: [], feed: [] })
    expect(entries).toHaveLength(2)
    expect(entries.map(entry => entry.kind)).toEqual(['chat', 'chat'])
    expect(entries.map(entry => entry.key)).toEqual(['chat:1', 'chat:2'])
    expect(entries[1].value).toEqual(messages[1])
  })

  it('keeps explicit query replies and the draw in chronological order', () => {
    const queryReply = message(4, '@王者玩家\n【极速飞艇 - 54776105】\n冠军 [4/352.00]\n当期使用积分：352.00', false)
    const draw: DrawResult = { id: 10, game_id: 'speed-fly', issue: '54776105', numbers: [4, 1, 2, 3, 5, 6, 7, 8, 9, 10], draw_at: '2026-08-30T05:12:15Z' }
    const entries = buildGameTimelineEntries({ gameId: 'speed-fly', messages: [queryReply, message(3, '查', true)], draws: [draw], tickets: [], feed: [] })
    expect(entries.map(entry => entry.key)).toEqual(['chat:3', 'chat:4', 'draw:speed-fly:54776105'])
    expect(entries[1].value).toEqual(queryReply)
  })

  it('restores a detailed-panel receipt even without a chat message', () => {
    const ticket = { gameId: 'speed-fly', content: '4/88', issue: '54776105', acceptedAt: '2026-08-30T05:10:00Z', lines: ['冠军[4/88.00]'], total: 88, balance: 912 }
    const entries = buildGameTimelineEntries({ gameId: 'speed-fly', messages: [], tickets: [ticket], feed: [] })
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: 'ticket', value: ticket })
  })

  it('keeps confirmed tickets across an issue change even if history reload fails', () => {
    const previous = { gameId: 'speed-fly', content: '4/88', issue: '54776105', acceptedAt: '2026-08-30T05:10:00Z', lines: ['冠军[4/88.00]'], total: 88, balance: 912 }
    const nextIssueTickets = ticketsForGame([previous], 'speed-fly')
    expect(nextIssueTickets).toEqual([previous])
    expect(buildGameTimelineEntries({ gameId: 'speed-fly', messages: [], tickets: nextIssueTickets, feed: [] })[0].value).toEqual(previous)
    expect(ticketsForGame([previous], 'speed-racing')).toEqual([])
  })

  it('formats chat timestamps in Beijing time without midnight becoming 24:00', () => {
    expect(formatGameMessageTime('2026-08-30T05:10:02Z')).toBe('13:10')
    expect(formatGameMessageTime('2026-08-29T16:00:02Z')).toBe('00:00')
    expect(formatGameMessageTime('invalid')).toBe('刚刚')
  })

  const draw = (issue: number): DrawResult => ({ id: issue, game_id: 'speed-fly', issue: String(issue), numbers: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10], draw_at: `2026-08-30T05:${String(issue).padStart(2, '0')}:00Z` })

  it('appends every confirmed draw without any member bet, message or winning notice', () => {
    const before = buildGameTimelineEntries({ gameId: 'speed-fly', messages: [], draws: [draw(2), draw(1)], feed: [], tickets: [] })
    const after = buildGameTimelineEntries({ gameId: 'speed-fly', messages: [], draws: [draw(3), draw(2), draw(1)], feed: [], tickets: [] })
    expect(after.map(entry => entry.key)).toEqual([...before.map(entry => entry.key), 'draw:speed-fly:3'])
  })

  it('deduplicates overlapping push/recovery rows by game and issue, keeping order stable', () => {
    const before = buildGameTimelineEntries({ gameId: 'speed-fly', messages: [], draws: [draw(2), draw(1)], feed: [], tickets: [] })
    const after = buildGameTimelineEntries({ gameId: 'speed-fly', messages: [], draws: [draw(1), draw(2), { ...draw(2), id: 999 }, { ...draw(3), game_id: 'speed-racing' }], feed: [], tickets: [] })
    expect(after.map(entry => entry.key)).toEqual(before.map(entry => entry.key))
  })

  it('does not cut off draws already accumulated in the current visit after eight issues', () => {
    const rows = Array.from({ length: 40 }, (_, index) => draw(index + 1))
    const entries = buildGameTimelineEntries({ gameId: 'speed-fly', messages: [], draws: rows, feed: [], tickets: [] })
    expect(entries).toHaveLength(40)
    expect(entries[0].key).toBe('draw:speed-fly:1')
    expect(entries.at(-1)?.key).toBe('draw:speed-fly:40')
  })

  it('starts with the anchor draw and applies its inclusive boundary to chat, tickets and feeds', () => {
    const anchor = draw(10)
    const rows = [message(1, 'older', true), message(2, 'settlement', false), message(3, 'newer', true)]
    rows[0].created_at = '2026-08-30T05:09:59Z'
    rows[1].created_at = anchor.draw_at
    const ticket = { gameId: 'speed-fly', content: '1/5', issue: '11', acceptedAt: anchor.draw_at, lines: [], total: 5, balance: 100 }
    const entries = buildGameTimelineEntries({ gameId: 'speed-fly', startAt: Date.parse(anchor.draw_at), draws: [anchor], messages: rows,
      tickets: [{ ...ticket, acceptedAt: rows[0].created_at }, ticket],
      feed: [{ nickname: 'old', detail: '', amount: 1, created_at: rows[0].created_at }, { nickname: 'new', detail: '', amount: 1, created_at: rows[2].created_at }],
    })
    expect(entries[0].key).toBe('draw:speed-fly:10')
    expect(entries.filter(entry => entry.kind === 'chat').map(entry => entry.key)).toEqual(['chat:2', 'chat:3'])
    expect(entries.filter(entry => entry.kind === 'ticket')).toHaveLength(1)
    expect(entries.filter(entry => entry.kind === 'feed')).toHaveLength(1)
  })

  it('freezes each historical image before or at its own issue, never including future or other-game draws', () => {
    const snapshot = drawHistoryAtIssue([draw(4), draw(3), draw(2), draw(1), { ...draw(2), game_id: 'speed-racing' }], draw(2))
    expect(snapshot.map(row => row.issue)).toEqual(['2', '1'])
  })

  it('keeps the entry announcement first even if its verified timestamp is corrected', () => {
    const startAt = Date.parse(draw(10).draw_at)
    for (const at of [draw(9).draw_at, draw(11).draw_at]) {
      const entries = buildGameTimelineEntries({ gameId: 'speed-fly', startAt, anchorIssue: '10', draws: [{ ...draw(10), draw_at: at }],
        messages: [{ ...message(1, 'at boundary', false), created_at: draw(10).draw_at }], feed: [], tickets: [] })
      expect(entries.map(entry => entry.kind)).toEqual(['draw', 'chat'])
    }
  })

  it('keeps public settlement and scoreboard messages without synthesizing a personal settlement', () => {
    const publicSettlement = { ...message(5, '【极速飞艇 - 54776105】\n结算内容如下：\n[王者玩家]\n得分：+231.30', false), message_type: 'settlement' }
    const entries = buildGameTimelineEntries({ gameId: 'speed-fly', messages: [publicSettlement], feed: [], tickets: [] })
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: 'chat', value: publicSettlement })
  })

  it('uses one combined budget for chat, feed, detailed tickets and announcements', () => {
    const at = (index: number) => new Date(Date.parse(draw(10).draw_at) + index * 1000).toISOString()
    const count = GAME_TIMELINE_LIMIT
    const messages = Array.from({ length: count }, (_, index) => ({ ...message(index + 1, 'chat', false), created_at: at(index * 3 + 1) }))
    const feed = Array.from({ length: count }, (_, index) => ({ nickname: `会员${index}`, detail: '冠军 1', amount: 20, created_at: at(index * 3 + 2) }))
    const tickets = Array.from({ length: count }, (_, index) => ({ gameId: 'speed-fly', content: `1/${index + 1}`, issue: '11', acceptedAt: at(index * 3 + 3), lines: [], total: 20, balance: 100 }))
    const latest = draw(11)
    const entries = buildGameTimelineEntries({ gameId: 'speed-fly', startAt: Date.parse(draw(10).draw_at), anchorIssue: '10', draws: [draw(10), latest], messages, feed, tickets })
    expect(entries).toHaveLength(GAME_TIMELINE_LIMIT)
    expect(entries[0]).toMatchObject({ kind: 'draw', value: latest })
    expect(entries.at(-1)).toMatchObject({ kind: 'ticket', value: tickets.at(-1) })
    expect(new Set(entries.map(entry => entry.kind))).toEqual(new Set(['chat', 'feed', 'ticket', 'draw']))
    expect(messages).toHaveLength(count)
    expect(tickets).toHaveLength(count)
  })

  it('retains the newest same-timestamp message IDs rather than sorting their digits as text', () => {
    const messages = Array.from({ length: GAME_TIMELINE_LIMIT + 50 }, (_, index) => ({ ...message(index + 1, 'chat', false), created_at: draw(10).draw_at }))
    const entries = buildGameTimelineEntries({ gameId: 'speed-fly', messages, feed: [], tickets: [] })
    expect(entries).toHaveLength(GAME_TIMELINE_LIMIT)
    expect(entries[0].key).toBe('chat:51')
    expect(entries.at(-1)?.key).toBe(`chat:${GAME_TIMELINE_LIMIT + 50}`)
  })

  it('does not add a draw slot outside the budget when the latest result is already in the recent window', () => {
    const messages = Array.from({ length: GAME_TIMELINE_LIMIT + 1 }, (_, index) => ({ ...message(index + 1, 'chat', false), created_at: draw(10).draw_at }))
    const entries = buildGameTimelineEntries({ gameId: 'speed-fly', draws: [draw(11)], messages, feed: [], tickets: [] })
    expect(entries).toHaveLength(GAME_TIMELINE_LIMIT)
    expect(entries.at(-1)).toMatchObject({ kind: 'draw', value: draw(11) })
    expect(entries.filter(entry => entry.kind === 'draw')).toHaveLength(1)
  })

  it('restores the latest bet input but never repeats applications, cancellation, queries or chat', () => {
    const messages = [message(1, '1/12345/100#6/大/200#7/67890/100', true), message(2, '上分/100', true), message(3, '查', true), message(4, '大家好', true)]
    expect(latestBetInput(messages, [], 'speed-fly')).toBe('1/12345/100#6/大/200#7/67890/100')
    expect(latestBetInput(messages, [], 'speed-racing')).toBe('')
    expect(latestBetInput(messages, [{ gameId: 'speed-fly', content: '4/88', issue: '100', acceptedAt: '2026-08-30T06:00:00Z', lines: [], total: 88, balance: 912 }], 'speed-fly')).toBe('4/88')
  })
})

describe('repeatable bet drafts', () => {
  it.each(['4444/88', '1/12345/100#6/大/200#7/67890/100', '大梭哈', '12345/梭哈'])('can refill %s for editing without a command', content => expect(isRepeatableBetInput(content)).toBe(true))
  it.each(['重复', '取消', '查', '上分/100', '申请 下分 20', '聊天', '', '3'])('does not reuse %s as a bet', content => expect(isRepeatableBetInput(content)).toBe(false))

  it('refills the exact previous draft rather than the word repeat, without submitting it', () => {
    expect(keyboardShortcutInput('repeat', '', '4444/88')).toBe('4444/88')
    expect(keyboardShortcutInput('repeat', '', '1/12345/100#6/大/200')).toBe('1/12345/100#6/大/200')
    expect(keyboardShortcutInput('repeat', '编辑中的草稿', '')).toBe('编辑中的草稿')
    expect(keyboardShortcutInput('repeat', '', '下分/100')).toBe('')
  })

  it('leaves every shortcut as editable input and avoids duplicate all-in suffixes', () => {
    expect(keyboardShortcutInput('all-in', '12345/', '')).toBe('12345/梭哈')
    expect(keyboardShortcutInput('all-in', '大梭哈', '')).toBe('大梭哈')
    expect(keyboardShortcutInput('cancel', '', '')).toBe('取消')
    expect(keyboardShortcutInput('credit', '', '')).toBe('上分 ')
    expect(keyboardShortcutInput('debit', '', '')).toBe('下分 ')
    expect(keyboardShortcutInput('check', '', '')).toBe('查')
  })
})
