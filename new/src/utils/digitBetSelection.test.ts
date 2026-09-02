import { describe, expect, it } from 'vitest'
import { digitChoice, digitCommandLengthError, digitDragonPositions, digitDragonSelections, digitNumbers, digitPatterns, digitSelectionCommand, digitSelectionGroups, digitSelectionKey, digitSides, toggleDigitChoice, type DigitSelection } from './digitBetSelection'
import { parseBetInput } from './betParser'

describe('digit lottery selection contract', () => {
  it('keeps mirror-pair selections independent and serializes them to supported ball positions', () => {
    const firstDragon = digitChoice('dragon_tiger', '龙', 1)!
    const secondTiger = digitChoice('dragon_tiger', '虎', 2)!
    let items = toggleDigitChoice([], firstDragon)
    items = toggleDigitChoice(items, secondTiger)
    expect(digitDragonPositions(5)).toEqual([1, 2])
    expect(digitDragonPositions(3)).toEqual([1])
    expect(digitSelectionGroups(items, 5).map(group => group.label)).toEqual(['第一球 vs 第五球', '第二球 vs 第四球'])
    expect(digitSelectionCommand(items, '20', 5)).toBe('1/龙/20#2/虎/20')
    expect(parseBetInput(digitSelectionCommand(items, '20', 5), 'speed-ssc')).toMatchObject({ total: 40, payloads: [expect.objectContaining({ position: 1, play_code: 'dragon_tiger' }), expect.objectContaining({ position: 2, play_code: 'dragon_tiger' })] })
    expect(toggleDigitChoice(items, secondTiger)).toEqual([firstDragon])
    expect(digitSelectionCommand(items, '20', 3)).toBe('')
    expect(digitSelectionCommand([firstDragon], '20', 3)).toBe('1/龙/20')
    expect(digitSelectionGroups([firstDragon], 3)[0].label).toBe('第一球 vs 第三球')
    for (const selection of ['和', '大', '1', '龙虎']) expect(digitChoice('dragon_tiger', selection)).toBeNull()
    for (const position of [0, 3, 1.5]) expect(digitChoice('dragon_tiger', '龙', position)).toBeNull()
  })

  it.each(['speed-ssc', 'au-lucky-5', 'bingo-ssc-1'])('enables the exact v3 three-segment and first-versus-fifth tie contract for %s', gameId => {
    const ruleVersion = 'digits5-v3'
    const items = [
      digitChoice('pattern', '豹子', 1, gameId, ruleVersion)!,
      digitChoice('pattern', '顺子', 2, gameId, ruleVersion)!,
      digitChoice('pattern', '对子', 3, gameId, ruleVersion)!,
      digitChoice('dragon_tiger', '和', 1, gameId, ruleVersion)!,
    ]
    expect(digitDragonPositions(5, gameId, ruleVersion)).toEqual([1])
    expect(digitDragonSelections(gameId, ruleVersion)).toEqual(['龙', '虎', '和'])
    expect(items.map(item => item.playCode)).toEqual(['leopard', 'straight', 'pair', 'dragon_tiger_tie'])
    expect(digitSelectionGroups(items, 5).map(group => group.label)).toEqual(['前三形态', '中三形态', '后三形态', '第一球 vs 第五球'])
    const command = digitSelectionCommand(items, '5', 5, gameId, ruleVersion)
    expect(command).toBe('前三/豹子/5#中三/顺子/5#后三/对子/5#1/和/5')
    expect(parseBetInput(command, gameId, ruleVersion).payloads.map(({ position, play_code }) => ({ position, play_code }))).toEqual([
      { position: 1, play_code: 'leopard' },
      { position: 2, play_code: 'straight' },
      { position: 3, play_code: 'pair' },
      { position: 1, play_code: 'dragon_tiger_tie' },
    ])
    expect(digitChoice('sum', '大', 1, gameId, ruleVersion)).toBeNull()
    expect(digitChoice('sum_tail', '7', 1, gameId, ruleVersion)).toBeNull()
  })

  it('does not infer v3 from a game id or from the generic five-ball family', () => {
    expect(digitChoice('pattern', '豹子', 2, 'speed-ssc')).toBeNull()
    expect(digitChoice('dragon_tiger', '和', 1, 'speed-ssc')).toBeNull()
    expect(digitChoice('pattern', '豹子', 2, 'sg-ssc', 'digits5-v3')).toBeNull()
    expect(digitChoice('dragon_tiger', '和', 1, 'sg-ssc', 'digits5-v3')).toBeNull()
    expect(digitDragonPositions(5, 'sg-ssc', 'digits5-v3')).toEqual([1, 2])
    for (const id of ['bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4']) {
      expect(digitChoice('pattern', '顺子', 2, id, 'digits5-v3')).toBeNull()
      expect(digitChoice('dragon_tiger', '和', 1, id, 'digits5-v3')).toBeNull()
      expect(digitDragonPositions(5, id, 'digits5-v3')).toEqual([1, 2])
    }
  })
  it('keeps each ball independent, including zero, without racer rank/number aliases', () => {
    const first = digitChoice('ball', '0', 1)!
    const third = digitChoice('ball', '0', 3)!
    let items = toggleDigitChoice([], first)
    items = toggleDigitChoice(items, third)
    expect(items).toHaveLength(2)
    expect(digitSelectionCommand(items, '20', 5)).toBe('1/0/20#3/0/20')
    expect(toggleDigitChoice(items, third)).toEqual([first])
    expect(digitChoice('ball', '10', 1)).toBeNull()
    expect(digitChoice('ball', '龙', 1)).toBeNull()
    expect(digitChoice('ball', '大单', 1)).toBeNull()
  })

  it('serializes every option explicitly, preserving totals, tails and first-three patterns', () => {
    const items = [digitChoice('pattern', '豹子')!, digitChoice('sum_tail', '7')!, digitChoice('sum', '大')!, digitChoice('ball', '单', 2)!, digitChoice('ball', '0', 2)!]
    expect(digitSelectionCommand(items, '1.25', 5)).toBe('2/0/1.25#2/单/1.25#总和/大/1.25#总和尾/7/1.25#前三/豹子/1.25')
    expect(digitSelectionGroups(items).map(group => group.label)).toEqual(['第二球', '总和', '总和尾', '前三形态'])
    expect(new Set(items.map(digitSelectionKey)).size).toBe(5)
    expect(digitSelectionCommand(digitPatterns.map(item => digitChoice('pattern', item.selection)!), '50.00', 3))
      .toBe('前三/豹子/50#前三/顺子/50#前三/对子/50#前三/半顺/50#前三/杂六/50')
  })

  it('rejects unimplemented shapes, precise sums, invalid positions and forged play codes', () => {
    expect(digitChoice('pattern', '中三豹子')).toBeNull()
    expect(digitChoice('sum', '27')).toBeNull()
    expect(digitChoice('sum_tail', '10')).toBeNull()
    expect(digitChoice('ball', '1', 0)).toBeNull()
    expect(digitChoice('ball', '1', 6)).toBeNull()
    const fourth = digitChoice('ball', '0', 4)!
    expect(digitSelectionCommand([fourth], '20', 3)).toBe('')
    expect(digitSelectionCommand([fourth], '20', 5)).toBe('4/0/20')
    expect(digitSelectionCommand([{ ...fourth, playCode: 'sum' }], '20', 5)).toBe('')
    expect(digitSelectionCommand([{ ...digitChoice('sum', '大')!, position: 2 }], '20', 5)).toBe('')
    expect(digitSelectionCommand([fourth, fourth], '20', 5)).toBe('')
  })

  it.each(['', '0', '-1', '0.001', '1.000', '1.', '.5', '1e2', '1,000', 'Infinity', 'NaN', '90071992547409.92'])('rejects unsafe or non-canonical money %j', amount => {
    expect(digitSelectionCommand([digitChoice('ball', '1')!], amount, 5)).toBe('')
  })

  it('retains real cents, canonicalizes harmless whitespace and rejects unsafe totals', () => {
    const items = [digitChoice('ball', '0')!, digitChoice('ball', '9')!]
    expect(digitSelectionCommand(items, ' 01.20 ', 5)).toBe('1/0/1.20#1/9/1.20')
    expect(digitSelectionCommand(items, '0.01', 5)).toBe('1/0/0.01#1/9/0.01')
    expect(digitSelectionCommand(items, '90071992547409.90', 5)).toBe('')
  })

  it('measures the 400 limit as Unicode code points, not UTF-16 units', () => {
    expect(digitCommandLengthError('总'.repeat(400))).toBeNull()
    expect(digitCommandLengthError('🚗'.repeat(400))).toBeNull()
    expect(digitCommandLengthError('🚗'.repeat(401))).toContain('401 字')
    expect(digitCommandLengthError('a'.repeat(401))).toContain('不会自动拆单')
  })

  it('never truncates or splits an over-limit cart into additional submissions', () => {
    const items: DigitSelection[] = Array.from({ length: 5 }, (_, index) => [...digitNumbers, ...digitSides].map(selection => digitChoice('ball', selection, index + 1)!)).flat()
    const command = digitSelectionCommand(items, '200', 5)
    expect(command.split('#')).toHaveLength(70)
    expect(digitCommandLengthError(command)).not.toBeNull()
    expect(command).toContain('5/双/200')
  })
})
