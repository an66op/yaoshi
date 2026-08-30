import { describe, expect, it } from 'vitest'
import { parseBetInput } from './betParser'
import { boardAmountCents, boardChoiceState, boardSelectionCommand, toggleBoardChoice, type BoardSelection } from './fullBetSelection'

describe('structured betting board selections', () => {
  it('never interprets a rank/number draft as a number/amount command', () => {
    const items = toggleBoardChoice([], [2], '5')
    expect(boardChoiceState(items, [2], '5')).toBe(true)
    expect(boardSelectionCommand(items, '20')).toBe('2/5/20')
    expect(parseBetInput(boardSelectionCommand(items, '20')).payloads).toEqual([expect.objectContaining({ position: 2, selection: '5', amount: 20 })])
    expect(toggleBoardChoice(items, [2], '5')).toEqual([])
  })

  it('omits the artificial decimal suffix from every integer stake without changing amounts', () => {
    const items = toggleBoardChoice([], [3, 4, 5], '2')
    for (const amount of ['20', '20.00']) {
      const command = boardSelectionCommand(items, amount)
      expect(command).toBe('3/2/20#4/2/20#5/2/20')
      expect(parseBetInput(command).total).toBe(60)
      expect(parseBetInput(command).payloads.map(item => item.position)).toEqual([3, 4, 5])
    }
    expect(boardSelectionCommand(items, '1.25')).toBe('3/2/1.25#4/2/1.25#5/2/1.25')
    expect(boardSelectionCommand(items, '0.10')).toBe('3/2/0.10#4/2/0.10#5/2/0.10')
  })

  it('matches the two-rank mixed screenshot: 12 items at 20, total 240', () => {
    let items: BoardSelection[] = []
    for (const choice of ['大', '小', '单', '双', '1', '2']) items = toggleBoardChoice(items, [2, 1], choice)
    const parsed = parseBetInput(boardSelectionCommand(items, '20'))
    expect(parsed.total).toBe(240)
    expect(parsed.payloads).toHaveLength(12)
    for (const position of [1, 2]) expect(parsed.payloads.filter(item => item.position === position).map(item => item.selection)).toEqual(['大', '小', '单', '双', '1', '2'])
  })

  it('keeps different selections for all ten ranks including rank 0 and number 0', () => {
    let items: BoardSelection[] = []
    for (let position = 1; position <= 10; position++) items = toggleBoardChoice(items, [position], String(11 - position))
    const parsed = parseBetInput(boardSelectionCommand(items, '0.10'))
    expect(parsed.total).toBe(1)
    expect(parsed.payloads.map(item => [item.position, item.selection])).toEqual(Array.from({ length: 10 }, (_, i) => [i + 1, String(10 - i)]))
  })

  it('fills partial selections without duplicating existing bets', () => {
    let items = toggleBoardChoice([], [1], '10')
    expect(boardChoiceState(items, [1, 2], '10')).toBe('mixed')
    items = toggleBoardChoice(items, [1, 2], '10')
    expect(items).toHaveLength(2)
    expect(boardChoiceState(items, [1, 2], '10')).toBe(true)
    expect(toggleBoardChoice(items, [1, 2], '10')).toEqual([])
    expect(toggleBoardChoice(items, [], '1')).toEqual(items)
  })

  it('does not confuse rank six with the sum play or allow invalid dragon/tiger ranks', () => {
    let items = toggleBoardChoice([], [6], '大')
    items = toggleBoardChoice(items, [6], '大', true)
    items = toggleBoardChoice(items, [6], '10', true)
    expect(parseBetInput(boardSelectionCommand(items, '20')).payloads).toEqual([
      expect.objectContaining({ position: 6, selection: '大', play_code: 'two_sided' }),
      expect.objectContaining({ position: 6, selection: '大', play_code: 'sum' }),
      expect.objectContaining({ position: 6, selection: '10', play_code: 'sum' }),
    ])
    expect(toggleBoardChoice(items, [1, 6], '龙')).toEqual(items)
  })

  it('rejects invalid or unsafe amounts without rounding an unintended stake', () => {
    const items = toggleBoardChoice([], [1, 2], '1')
    for (const value of ['', '0', '-1', '1e3', 'NaN', '0.001', '1.234', '90071992547409.92']) {
      expect(boardAmountCents(value)).toBeNull()
      expect(boardSelectionCommand(items, value)).toBe('')
    }
    expect(boardAmountCents('1.25')).toBe(125)
    expect(parseBetInput(boardSelectionCommand(items, '1.25')).total).toBe(2.5)
  })
})
