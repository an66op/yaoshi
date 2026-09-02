import { formatBetAmount } from './betAmount'
import { boardAmountCents } from './fullBetSelection'
import { isDigit5V3Game, lotteryRuleProfile } from './lotteryRules'

export const digitNumbers = Array.from({ length: 10 }, (_, index) => String(index))
export const digitSides = ['大', '小', '单', '双']
export const digitDragons = ['龙', '虎']
export const digitDragonTie = '和'
export const digitPatternPositions = [
  { position: 1, target: '前三', label: '前三形态' },
  { position: 2, target: '中三', label: '中三形态' },
  { position: 3, target: '后三', label: '后三形态' },
] as const
export const digitPatterns = [
  { selection: '豹子', playCode: 'leopard' },
  { selection: '顺子', playCode: 'straight' },
  { selection: '对子', playCode: 'pair' },
  { selection: '半顺', playCode: 'half_straight' },
  { selection: '杂六', playCode: 'mixed' },
] as const

export type DigitBetKind = 'ball' | 'sum' | 'sum_tail' | 'pattern' | 'dragon_tiger'
export type DigitSelection = {
  kind: DigitBetKind
  position: number
  selection: string
  playCode: 'ball_1_5' | 'two_sided' | 'sum' | 'dragon_tiger' | 'dragon_tiger_tie' | typeof digitPatterns[number]['playCode']
}

export const digitBallLabel = (position: number) => `第${['一', '二', '三', '四', '五'][position - 1] ?? position}球`
const digitBallCountForGame = (gameId: string, ruleVersion: string) => isDigit5V3Game(gameId, ruleVersion) ? 5 : lotteryRuleProfile(gameId).family === 'digit3' ? 3 : null
export const digitDragonPositions = (ballCount: 3 | 5, gameId = '', ruleVersion = '') => digitBallCountForGame(gameId, ruleVersion) === ballCount ? [1] : []
export const digitDragonSelections = (gameId = '', ruleVersion = '') => isDigit5V3Game(gameId, ruleVersion) ? [...digitDragons, digitDragonTie] : lotteryRuleProfile(gameId).family === 'digit3' ? digitDragons : []
export const digitDragonLabel = (position: number, ballCount: 3 | 5) => `${digitBallLabel(position)} vs ${digitBallLabel(ballCount + 1 - position)}`
export const digitPatternTarget = (position: number) => digitPatternPositions.find(item => item.position === position)
export const digitSelectionKey = (item: DigitSelection) => `${item.kind}:${item.position}:${item.selection}`

export function digitChoice(kind: DigitBetKind, selection: string, position = 1, gameId = '', ruleVersion = ''): DigitSelection | null {
  const ballCount = digitBallCountForGame(gameId, ruleVersion)
  if (ballCount === null) return null
  if (kind === 'ball') {
    if (!Number.isInteger(position) || position < 1 || position > ballCount) return null
    if (digitNumbers.includes(selection)) return { kind, position, selection, playCode: 'ball_1_5' }
    if (digitSides.includes(selection)) return { kind, position, selection, playCode: 'two_sided' }
  } else if (ballCount === 3 && ((kind === 'sum' && digitSides.includes(selection)) || (kind === 'sum_tail' && digitNumbers.includes(selection)))) {
    return { kind, position: 6, selection, playCode: 'sum' }
  } else if (kind === 'pattern') {
    const pattern = digitPatterns.find(item => item.selection === selection)
    const validPosition = position === 1 || (isDigit5V3Game(gameId, ruleVersion) && (position === 2 || position === 3))
    if (pattern && validPosition) return { kind, position, ...pattern }
  } else if (kind === 'dragon_tiger' && position === 1) {
    if (isDigit5V3Game(gameId, ruleVersion)) {
      if (!digitDragonSelections(gameId, ruleVersion).includes(selection)) return null
      return { kind, position, selection, playCode: selection === digitDragonTie ? 'dragon_tiger_tie' : 'dragon_tiger' }
    }
    if (digitDragons.includes(selection)) return { kind, position, selection, playCode: 'dragon_tiger' }
  }
  return null
}

/** A ball tab is an editing target, never an additional selection. */
export function toggleDigitChoice(items: DigitSelection[], choice: DigitSelection): DigitSelection[] {
  const key = digitSelectionKey(choice)
  return items.some(item => digitSelectionKey(item) === key)
    ? items.filter(item => digitSelectionKey(item) !== key)
    : [...items, choice]
}

export function digitSelectionGroups(items: DigitSelection[], ballCount: 3 | 5 = 5) {
  const groups = new Map<number, DigitSelection[]>()
  for (const item of items) {
    const rank = item.kind === 'ball' ? item.position : item.kind === 'sum' ? 6 : item.kind === 'sum_tail' ? 7 : item.kind === 'pattern' ? 7 + item.position : 11 + item.position
    groups.set(rank, [...(groups.get(rank) ?? []), item])
  }
  const order = [...digitNumbers, ...digitSides, ...digitPatterns.map(item => item.selection), ...digitDragons, digitDragonTie]
  return [...groups.entries()].sort(([left], [right]) => left - right).map(([rank, choices]) => ({
    rank,
    label: rank < 6 ? digitBallLabel(rank) : rank === 6 ? '总和' : rank === 7 ? '总和尾' : rank <= 10 ? digitPatternTarget(rank - 7)!.label : digitDragonLabel(rank - 11, ballCount),
    choices: [...choices].sort((left, right) => order.indexOf(left.selection) - order.indexOf(right.selection)),
  }))
}

/** Keep each option explicit so 0, sum tails and patterns cannot become racing shorthand. */
export function digitSelectionCommand(items: DigitSelection[], amount: string, ballCount: 3 | 5, gameId = '', ruleVersion = ''): string {
  if (digitBallCountForGame(gameId, ruleVersion) !== ballCount) return ''
  const cents = boardAmountCents(amount)
  if (cents === null || !items.length || !Number.isSafeInteger(cents * items.length)) return ''
  if (new Set(items.map(digitSelectionKey)).size !== items.length) return ''
  for (const item of items) {
    const normalized = digitChoice(item.kind, item.selection, item.position, gameId, ruleVersion)
    if (!normalized || normalized.playCode !== item.playCode || normalized.position !== item.position) return ''
    if (item.kind === 'ball' && item.position > ballCount) return ''
    if (item.kind === 'pattern' && (ballCount !== 5 || !isDigit5V3Game(gameId, ruleVersion)) && item.position !== 1) return ''
    if (item.kind === 'dragon_tiger' && !digitDragonPositions(ballCount, gameId, ruleVersion).includes(item.position)) return ''
  }
  const money = formatBetAmount(cents / 100)
  return digitSelectionGroups(items, ballCount).flatMap(group => group.choices.map(item => {
    const target = item.kind === 'ball' || item.kind === 'dragon_tiger' ? String(item.position) : item.kind === 'sum' ? '总和' : item.kind === 'sum_tail' ? '总和尾' : digitPatternTarget(item.position)!.target
    return `${target}/${item.selection}/${money}`
  })).join('#')
}

export function digitCommandLengthError(content: string): string | null {
  const length = Array.from(content).length
  return length > 400 ? `本次指令 ${length} 字，超过单次 400 字上限；请减少所选项，不会自动拆单。` : null
}
