import type { BasePlayCode } from './gameRoomSafety'
import { formatBetAmount } from './betAmount'

export const racingRanks = ['冠军', '亚军', '第三名', '第四名', '第五名', '第六名', '第七名', '第八名', '第九名', '第十名']
export type BoardSelection = { position: number; selection: string; playCode: BasePlayCode }
export const boardSelectionKey = (item: BoardSelection) => `${item.playCode}:${item.position}:${item.selection}`
export const boardRankLabel = (position: number) => racingRanks[position - 1] ?? `第${position}名`
export const boardChoiceCode = (choice: string): BasePlayCode => /^\d+$/.test(choice) ? 'ball_1_5' : /[龙虎]/.test(choice) ? 'dragon_tiger' : 'two_sided'

export function boardChoiceState(items: BoardSelection[], positions: number[], choice: string, sum = false): boolean | 'mixed' {
  const count = positions.filter(position => items.some(item => item.position === position && item.selection === choice && item.playCode === (sum ? 'sum' : boardChoiceCode(choice)))).length
  return count === 0 ? false : count === positions.length ? true : 'mixed'
}

/** The cart stores actual rank/choice tuples, never incomplete text commands. */
export function toggleBoardChoice(items: BoardSelection[], positions: number[], choice: string, sum = false): BoardSelection[] {
  if (!positions.length || (!sum && /[龙虎]/.test(choice) && positions.some(position => position > 5))) return items
  const playCode = sum ? 'sum' : boardChoiceCode(choice)
  const keys = new Set(positions.map(position => boardSelectionKey({ position, selection: choice, playCode })))
  if (boardChoiceState(items, positions, choice, sum) === true) return items.filter(item => !keys.has(boardSelectionKey(item)))
  const existing = new Set(items.map(boardSelectionKey))
  return [...items, ...positions.map(position => ({ position, selection: choice, playCode })).filter(item => !existing.has(boardSelectionKey(item)))]
}

export function boardAmountCents(value: string): number | null {
  if (!/^\d+(?:\.\d{1,2})?$/.test(value.trim())) return null
  const cents = Math.round(Number(value) * 100)
  return Number.isSafeInteger(cents) && cents > 0 ? cents : null
}

export function boardSelectionGroups(items: BoardSelection[]) {
  const groups = new Map<number, BoardSelection[]>()
  for (const item of items) {
    const rank = item.playCode === 'sum' ? 11 : item.position
    groups.set(rank, [...(groups.get(rank) ?? []), item])
  }
  return [...groups.entries()].sort(([left], [right]) => left - right).map(([rank, choices]) => ({
    rank, label: rank === 11 ? '冠亚和' : boardRankLabel(rank), choices,
  }))
}

/** Only serialize at the boundary. `#` separates groups; prefixes identify ranks. */
export function boardSelectionCommand(items: BoardSelection[], amount: string): string {
  const cents = boardAmountCents(amount)
  if (cents === null || !Number.isSafeInteger(cents * items.length)) return ''
  // 与手动键盘输入一致：整数金额不自动补 .00，真实小数不截断。
  const money = formatBetAmount(cents / 100)
  return boardSelectionGroups(items).flatMap(group => group.rank === 11
    ? group.choices.map(item => `冠亚/${item.selection}/${money}`)
    : [`${group.rank === 10 ? '0' : group.rank}/${group.choices.map(item => item.selection === '10' ? '0' : item.selection).join('')}/${money}`],
  ).join('#')
}
