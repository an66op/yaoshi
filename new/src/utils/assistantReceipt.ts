import type { AssistantBetLine } from '../api/bets'
import { boardRankLabel } from './fullBetSelection'
import { formatBetAmount } from './betAmount'

const choiceOrder = (selection: string) => {
  if (/^\d+$/.test(selection)) return Number(selection)
  const index = ['单', '双', '大', '小', '龙', '虎'].indexOf(selection)
  return index < 0 ? 200 : 100 + index
}

/** Group only the visible receipt, not the order items or their amounts/odds. */
export function assistantReceiptLines(lines: AssistantBetLine[]): string[] {
  const groups = new Map<number, AssistantBetLine[]>()
  const fallback: string[] = []
  for (const line of lines) {
    const key = line.play_code === 'sum' ? 11 : line.position
    if (!key || key < 1 || key > 11 || (key === 11 && line.play_code !== 'sum') || !line.selection) { fallback.push(line.label); continue }
    groups.set(key, [...(groups.get(key) ?? []), line])
  }
  return [...groups.entries()].sort(([a], [b]) => a - b).map(([rank, items]) => {
    const label = rank === 11 ? '冠亚和' : boardRankLabel(rank)
    const choices = [...items].sort((a, b) => choiceOrder(a.selection) - choiceOrder(b.selection))
    return `${label}[${choices.map(item => `${item.selection}/${formatBetAmount(item.amount)}`).join(' ')}]`
  }).concat(fallback)
}
