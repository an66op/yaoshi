import { describe, expect, it } from 'vitest'
import { digitChoice, digitCommandLengthError, digitDragonPositions, digitDragonSelections, digitNumbers, digitPatterns, digitSelectionCommand, digitSelectionGroups, digitSelectionKey, digitSides, toggleDigitChoice, type DigitBetKind, type DigitSelection } from './digitBetSelection'
import { parseBetInput } from './betParser'

const fiveIDs = ['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1']
const fiveChoice = (kind: DigitBetKind, selection: string, position = 1) => digitChoice(kind, selection, position, 'speed-ssc', 'digits5-v3')
const threeChoice = (kind: DigitBetKind, selection: string, position = 1) => digitChoice(kind, selection, position, 'official-fc3d', 'digits3-v2')
const fiveCommand = (items: DigitSelection[], amount: string) => digitSelectionCommand(items, amount, 5, 'speed-ssc', 'digits5-v3')
const threeCommand = (items: DigitSelection[], amount: string) => digitSelectionCommand(items, amount, 3, 'official-fc3d', 'digits3-v2')

describe('digit lottery selection contract', () => {
  it('keeps the three-ball first-versus-third comparison on its own current contract', () => {
    const dragon = threeChoice('dragon_tiger', '龙')!
    const tiger = threeChoice('dragon_tiger', '虎')!
    let items = toggleDigitChoice([], dragon)
    items = toggleDigitChoice(items, tiger)
    expect(digitDragonPositions(3, 'official-fc3d', 'digits3-v2')).toEqual([1])
    expect(digitDragonSelections('official-fc3d', 'digits3-v2')).toEqual(['龙', '虎'])
    expect(digitSelectionGroups(items, 3).map(group => group.label)).toEqual(['第一球 vs 第三球'])
    expect(threeCommand(items, '20')).toBe('1/龙/20#1/虎/20')
    expect(parseBetInput(threeCommand(items, '20'), 'official-fc3d', 'digits3-v2')).toMatchObject({ total: 40 })
    expect(toggleDigitChoice(items, tiger)).toEqual([dragon])
    for (const selection of ['和', '大', '1', '龙虎']) expect(threeChoice('dragon_tiger', selection)).toBeNull()
    for (const position of [0, 2, 3, 1.5]) expect(threeChoice('dragon_tiger', '龙', position)).toBeNull()
  })

  it.each(fiveIDs)('enables the exact v3 three-segment and first-versus-fifth tie contract for %s', gameId => {
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
    expect(digitChoice('dragon_tiger', '龙', 2, gameId, ruleVersion)).toBeNull()
    expect(digitSelectionCommand([items[0]], '5', 3, gameId, ruleVersion)).toBe('')
  })

  it.each(fiveIDs)('never generates choices or commands for %s without the exact current version', gameId => {
    const item = fiveChoice('ball', '0')!
    for (const ruleVersion of [undefined, '', 'digits5-v2', 'digits5-v4']) {
      expect(digitChoice('ball', '0', 1, gameId, ruleVersion)).toBeNull()
      expect(digitChoice('pattern', '豹子', 1, gameId, ruleVersion)).toBeNull()
      expect(digitChoice('pattern', '豹子', 2, gameId, ruleVersion)).toBeNull()
      expect(digitChoice('dragon_tiger', '和', 1, gameId, ruleVersion)).toBeNull()
      expect(digitDragonPositions(5, gameId, ruleVersion)).toEqual([])
      expect(digitDragonSelections(gameId, ruleVersion)).toEqual([])
      expect(digitSelectionCommand([item], '20', 5, gameId, ruleVersion)).toBe('')
    }
  })

  it.each(['', 'bingo-racing-b', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4'])('never borrows a five-ball selection contract for unverified %j', gameId => {
    for (const ruleVersion of ['', 'digits5-v2', 'digits5-v3']) {
      expect(digitChoice('ball', '0', 1, gameId, ruleVersion)).toBeNull()
      expect(digitChoice('pattern', '顺子', 1, gameId, ruleVersion)).toBeNull()
      expect(digitChoice('dragon_tiger', '龙', 1, gameId, ruleVersion)).toBeNull()
      expect(digitDragonPositions(5, gameId, ruleVersion)).toEqual([])
      expect(digitDragonSelections(gameId, ruleVersion)).toEqual([])
      expect(digitSelectionCommand([fiveChoice('ball', '0')!], '20', 5, gameId, ruleVersion)).toBe('')
    }
  })

  it('keeps each ball independent, including zero, without racer rank/number aliases', () => {
    const first = fiveChoice('ball', '0', 1)!
    const third = fiveChoice('ball', '0', 3)!
    let items = toggleDigitChoice([], first)
    items = toggleDigitChoice(items, third)
    expect(items).toHaveLength(2)
    expect(fiveCommand(items, '20')).toBe('1/0/20#3/0/20')
    expect(toggleDigitChoice(items, third)).toEqual([first])
    expect(fiveChoice('ball', '10', 1)).toBeNull()
    expect(fiveChoice('ball', '龙', 1)).toBeNull()
    expect(fiveChoice('ball', '大单', 1)).toBeNull()
  })

  it('serializes the three-ball totals, tails and first-three patterns explicitly', () => {
    const items = [threeChoice('pattern', '豹子')!, threeChoice('sum_tail', '7')!, threeChoice('sum', '大')!, threeChoice('ball', '单', 2)!, threeChoice('ball', '0', 2)!]
    expect(threeCommand(items, '1.25')).toBe('2/0/1.25#2/单/1.25#总和/大/1.25#总和尾/7/1.25#前三/豹子/1.25')
    expect(digitSelectionGroups(items, 3).map(group => group.label)).toEqual(['第二球', '总和', '总和尾', '前三形态'])
    expect(new Set(items.map(digitSelectionKey)).size).toBe(5)
    expect(threeCommand(digitPatterns.map(item => threeChoice('pattern', item.selection)!), '50.00'))
      .toBe('前三/豹子/50#前三/顺子/50#前三/对子/50#前三/半顺/50#前三/杂六/50')
    expect(fiveCommand(items, '1.25')).toBe('')
  })

  it('rejects unimplemented shapes, precise sums, invalid positions and forged play codes', () => {
    expect(fiveChoice('pattern', '中三豹子')).toBeNull()
    expect(threeChoice('sum', '27')).toBeNull()
    expect(threeChoice('sum_tail', '10')).toBeNull()
    expect(fiveChoice('ball', '1', 0)).toBeNull()
    expect(fiveChoice('ball', '1', 6)).toBeNull()
    expect(threeChoice('ball', '0', 4)).toBeNull()
    const fourth = fiveChoice('ball', '0', 4)!
    expect(threeCommand([fourth], '20')).toBe('')
    expect(fiveCommand([fourth], '20')).toBe('4/0/20')
    expect(fiveCommand([{ ...fourth, playCode: 'sum' }], '20')).toBe('')
    expect(threeCommand([{ ...threeChoice('sum', '大')!, position: 2 }], '20')).toBe('')
    expect(fiveCommand([fourth, fourth], '20')).toBe('')
  })

  it.each(['', '0', '-1', '0.001', '1.000', '1.', '.5', '1e2', '1,000', 'Infinity', 'NaN', '90071992547409.92'])('rejects unsafe or non-canonical money %j', amount => {
    expect(fiveCommand([fiveChoice('ball', '1')!], amount)).toBe('')
  })

  it('retains real cents, canonicalizes harmless whitespace and rejects unsafe totals', () => {
    const items = [fiveChoice('ball', '0')!, fiveChoice('ball', '9')!]
    expect(fiveCommand(items, ' 01.20 ')).toBe('1/0/1.20#1/9/1.20')
    expect(fiveCommand(items, '0.01')).toBe('1/0/0.01#1/9/0.01')
    expect(fiveCommand(items, '90071992547409.90')).toBe('')
  })

  it('measures the 400 limit as Unicode code points, not UTF-16 units', () => {
    expect(digitCommandLengthError('总'.repeat(400))).toBeNull()
    expect(digitCommandLengthError('🚗'.repeat(400))).toBeNull()
    expect(digitCommandLengthError('🚗'.repeat(401))).toContain('401 字')
    expect(digitCommandLengthError('a'.repeat(401))).toContain('不会自动拆单')
  })

  it('never truncates or splits an over-limit cart into additional submissions', () => {
    const items: DigitSelection[] = Array.from({ length: 5 }, (_, index) => [...digitNumbers, ...digitSides].map(selection => fiveChoice('ball', selection, index + 1)!)).flat()
    const command = fiveCommand(items, '200')
    expect(command.split('#')).toHaveLength(70)
    expect(digitCommandLengthError(command)).not.toBeNull()
    expect(command).toContain('5/双/200')
  })
})
