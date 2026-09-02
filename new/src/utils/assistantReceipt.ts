import type { AssistantBetLine } from '../api/bets'
import { boardRankLabel } from './fullBetSelection'
import { formatBetAmount } from './betAmount'
import { isDigit5V3Game, lotteryRuleProfile } from './lotteryRules'

const shapeCodes = new Set(['leopard', 'straight', 'pair', 'half_straight', 'mixed'])
const shapeOrder = ['豹子', '顺子', '对子', '半顺', '杂六']
const shapeLabels: Record<string, string> = { leopard: '豹子', straight: '顺子', pair: '对子', half_straight: '半顺', mixed: '杂六' }

const choiceOrder = (selection: string) => {
  if (/^\d+$/.test(selection)) return Number(selection)
  const index = ['单', '双', '大', '小', '龙', '虎', '和'].indexOf(selection)
  return index < 0 ? 200 : 100 + index
}

function originalReceiptLine(line: AssistantBetLine) {
  if (line.label?.trim()) return line.label
  const label = line.play_name || (/^\d+$/.test(line.selection) ? '号码' : '投注项')
  return `${label}[${line.selection}/${formatBetAmount(line.amount)}]`
}

function digitReceiptLines(lines: AssistantBetLine[], ballCount: number, gameId: string, ruleVersion: string): string[] {
  const v3 = isDigit5V3Game(gameId, ruleVersion)
  const groups = new Map<number, AssistantBetLine[]>()
  const fallback: string[] = []
  for (const line of lines) {
    let group = 0
    if (line.play_code === 'sum') {
      group = /^[大小单双]$/.test(line.selection) ? 6 : /^\d$/.test(line.selection) ? 7 : 0
    } else if (shapeCodes.has(line.play_code)) {
      group = Number.isInteger(line.position) && line.position >= 1 && line.position <= (v3 ? 3 : 1) ? 7 + line.position : 0
    } else if (v3 && ['dragon_tiger', 'dragon_tiger_tie'].includes(line.play_code) && line.position === 1) {
      group = 12
    } else if (['ball_1_5', 'two_sided', 'dragon_tiger'].includes(line.play_code) && Number.isInteger(line.position) && line.position >= 1 && line.position <= ballCount) {
      group = line.position
    }
    if (!group || !line.selection) { fallback.push(originalReceiptLine(line)); continue }
    groups.set(group, [...(groups.get(group) ?? []), line])
  }
  return [...groups.entries()].sort(([left], [right]) => left - right).map(([group, items]) => {
    const label = group < 6 ? `第${group}球` : group === 6 ? '总和' : group === 7 ? '总和尾' : group === 8 ? '前三形态' : group === 9 ? '中三形态' : group === 10 ? '后三形态' : '第一球 vs 第五球'
    const displaySelection = (item: AssistantBetLine) => shapeCodes.has(item.play_code) ? shapeLabels[item.play_code] : item.selection
    const choices = [...items].sort((left, right) => group >= 8 && group <= 10
      ? shapeOrder.indexOf(displaySelection(left)) - shapeOrder.indexOf(displaySelection(right))
      : choiceOrder(left.selection) - choiceOrder(right.selection))
    return `${label}[${choices.map(item => `${displaySelection(item)}/${formatBetAmount(item.amount)}`).join(' ')}]`
  }).concat(fallback)
}

/** Group only visible receipts. Existing callers without an ID retain racing labels. */
export function assistantReceiptLines(lines: AssistantBetLine[], gameId?: string, ruleVersion = ''): string[] {
  if (gameId !== undefined) {
    const profile = lotteryRuleProfile(gameId)
    if (profile.family === 'unknown') return lines.map(originalReceiptLine)
    if (profile.family === 'mark-six' || profile.family === 'pc28') return lines.map(originalReceiptLine)
    if (profile.family === 'ssc' || profile.family === 'digit3') return digitReceiptLines(lines, profile.ballCount!, gameId, ruleVersion)
  }
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
