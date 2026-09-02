import { formatBetAmount } from './betAmount'
import { betCommandError } from './betCommand'
import { isDigit5V3Game, lotteryRuleProfile } from './lotteryRules'

export type BetPayload = {
  position: number
  selection: string
  amount: number
  play_code?: string
  play_name?: string
}

export type ParsedBet = { content: string; lines: string[]; total: number; payloads: BetPayload[] }

const positionNames = ['冠军', '亚军', '第三名', '第四名', '第五名', '第六名', '第七名', '第八名', '第九名', '第十名']
const rankLabel = (position: number) => positionNames[position - 1] ?? `第${position}名`
const rankMap: Record<string, number> = {
  冠军: 1, 亚军: 2, 第三名: 3, 第四名: 4, 第五名: 5,
  第六名: 6, 第七名: 7, 第八名: 8, 第九名: 9, 第十名: 10,
}

type RuleProfile = ReturnType<typeof lotteryRuleProfile>
const shapeCodes: Record<string, string> = { 豹子: 'leopard', 顺子: 'straight', 对子: 'pair', 半顺: 'half_straight', 杂六: 'mixed' }
const shapeTargets = { 前三: 1, 中三: 2, 后三: 3 } as const
const shapeTargetLabel = (position: number) => position === 1 ? '前三' : position === 2 ? '中三' : position === 3 ? '后三' : ''

function betPositions(value: string, rules: RuleProfile): number[] | null {
  const racing = rules.family === 'racing'
  if (rankMap[value]) return racing ? [rankMap[value]] : null
  const ball = value.match(/^第([+-]?\d+)球$/)
  if (ball && !racing) {
    const position = Number(ball[1])
    return position >= 1 && position <= rules.ballCount! ? [position] : null
  }
  if (value === '10' || value === '0') return racing ? [10] : null
  if (!/^\d+$/.test(value)) return null
  const positions = [...value].map(digit => racing && digit === '0' ? 10 : Number(digit))
  return positions.every(position => position >= 1 && position <= rules.ballCount!) ? positions : null
}

function namedPositionGroup(value: string, rules: RuleProfile): number[] | null {
  if (rules.family !== 'racing') return null
  if (value === '前三') return [1, 2, 3]
  if (value === '后三') return [8, 9, 10]
  if (value === '前五') return [1, 2, 3, 4, 5]
  if (value === '后五') return [6, 7, 8, 9, 10]
  return null
}

function positionedSelections(positions: number[], selections: string, amount: number, rules: RuleProfile, gameId: string, ruleVersion: string): BetPayload[] | null {
  const racing = rules.family === 'racing'
  const v3 = isDigit5V3Game(gameId, ruleVersion)
  if (!(v3 && !racing ? /^[0-9大小单双龙虎和]+$/ : /^[0-9大小单双龙虎]+$/).test(selections)) return null
  const hasDragon = /[龙虎和]/.test(selections)
  if (positions.some(position => position < 1 || position > rules.ballCount! || (hasDragon && (v3 ? position !== 1 : position > Math.floor(rules.ballCount! / 2))))) return null
  return positions.flatMap((position) => [...selections].map((selection) => {
    const play_code = /^\d$/.test(selection)
      ? 'ball_1_5'
      : ['大', '小', '单', '双'].includes(selection)
        ? 'two_sided'
        : selection === '和' ? 'dragon_tiger_tie' : 'dragon_tiger'
    const label = racing ? rankLabel(position) : `第${position}球`
    const play_name = play_code === 'ball_1_5'
      ? `${label}号码`
      : play_code === 'two_sided'
        ? `${label}两面`
        : racing ? '龙虎' : v3 ? '第一球与第五球龙虎和' : `${label}龙虎`
    // 快捷输入沿用 0 表示号码 10；发给接口时统一为真实开奖号码 10。
    const canonicalSelection = racing && play_code === 'ball_1_5' && selection === '0' ? '10' : selection
    return { position, selection: canonicalSelection, amount, play_code, play_name }
  }))
}

function sumPayload(selection: string, amount: number, rules: RuleProfile, tailOnly = false, splitShortNumbers = false): BetPayload[] | null {
  selection = selection.trim()
  const racing = rules.family === 'racing'
  let play_name = racing ? '冠亚和' : '总和'
  let selections: string[]
  if (/^\d+$/.test(selection)) {
    if (racing && splitShortNumbers) {
      selections = [...selection]
      if (!selections.every(value => Number(value) >= 3 && Number(value) <= 9)) return null
      return selections.map(value => ({ position: 6, selection: value, amount, play_code: 'sum', play_name }))
    }
    const sum = Number(selection)
    if (racing ? sum < 3 || sum > 19 : selection.length !== 1) return null
    selections = [String(sum)]
    if (!racing) play_name = '总和尾'
  } else {
    if (tailOnly || !/^[大小单双]+$/.test(selection)) return null
    selections = [...selection]
  }
  return selections.map(value => ({ position: 6, selection: value, amount, play_code: 'sum', play_name }))
}

function shapePayload(selection: string, amount: number, rules: RuleProfile, gameId: string, ruleVersion: string, requestedPosition?: number): BetPayload[] | null {
  if (rules.family === 'racing') return null
  selection = selection.trim().toLowerCase()
  const shape = Object.entries(shapeCodes).find(([label, code]) => selection === label || selection === code)
  if (!shape) return null
  const v3 = isDigit5V3Game(gameId, ruleVersion)
  const positions = requestedPosition === undefined ? (v3 ? [1, 2, 3] : [1]) : [requestedPosition]
  if (positions.some(position => position !== 1 && (!v3 || position < 2 || position > 3))) return null
  return positions.map(position => ({ position, selection: shape[0], amount, play_code: shape[1], play_name: `${shapeTargetLabel(position)}${shape[0]}` }))
}

/** Mirror assistantSegmentEntries/assistantPlayEntries; never guess a first-ball bet. */
function segmentPayload(parts: string[], amount: number, rules: RuleProfile, gameId: string, ruleVersion: string): BetPayload[] | null {
  const racing = rules.family === 'racing'
  const v3 = isDigit5V3Game(gameId, ruleVersion)
  if (parts.length === 2) {
    const [target, selection] = parts
    if (target === '和') return racing ? sumPayload(selection, amount, rules, false, true) : null
    if (/^冠亚(?:和)?$/.test(target.replaceAll('军', ''))) return racing ? sumPayload(selection, amount, rules) : null
    if (target === '总和' || target === '总和尾') return racing || v3 ? null : sumPayload(selection, amount, rules, target === '总和尾')
    const group = namedPositionGroup(target, rules)
    if (group) return positionedSelections(group, selection, amount, rules, gameId, ruleVersion)
    if (target in shapeTargets) return shapePayload(selection, amount, rules, gameId, ruleVersion, shapeTargets[target as keyof typeof shapeTargets])
    const positions = betPositions(target, rules)
    return positions ? positionedSelections(positions, selection, amount, rules, gameId, ruleVersion) : null
  }
  if (parts.length !== 1) return null
  const play = parts[0]
  if (racing && play.startsWith('和')) return sumPayload(play.slice(1), amount, rules, false, true)
  if (racing && /^10[大小单双龙虎]$/.test(play)) return positionedSelections([1, 10], play.slice(2), amount, rules, gameId, ruleVersion)
  const compactPosition = play.match(/^(10|[0-9])([大小单双龙虎和])$/)
  if (compactPosition) {
    const positions = betPositions(compactPosition[1], rules)
    return positions ? positionedSelections(positions, compactPosition[2], amount, rules, gameId, ruleVersion) : null
  }
  const rank = play.match(/^(冠军|亚军|第三名|第四名|第五名|第六名|第七名|第八名|第九名|第十名)([0-9大小单双龙虎]+)$/)
  if (rank) return racing ? positionedSelections([rankMap[rank[1]]], rank[2], amount, rules, gameId, ruleVersion) : null
  const ball = play.match(/^第([1-5])球([0-9大小单双龙虎和]+)$/)
  if (ball && !racing) return positionedSelections([Number(ball[1])], ball[2], amount, rules, gameId, ruleVersion)
  const crownSum = play.match(/^冠亚(?:和)?(.*)$/)
  if (crownSum) return racing ? sumPayload(crownSum[1], amount, rules) : null
  const sum = play.match(/^总和(尾)?(.*)$/)
  if (sum) return racing || v3 ? null : sumPayload(sum[2], amount, rules, Boolean(sum[1]))
  const positionedShape = play.match(/^(前三|中三|后三)(.+)$/)
  if (positionedShape) return shapePayload(positionedShape[2], amount, rules, gameId, ruleVersion, shapeTargets[positionedShape[1] as keyof typeof shapeTargets])
  const shape = shapePayload(play, amount, rules, gameId, ruleVersion)
  if (shape) return shape
  if (/^[0-9大小单双龙虎和]+$/.test(play)) {
    const positions = racing || /[龙虎和]/.test(play) ? [1] : Array.from({ length: rules.ballCount! }, (_, index) => index + 1)
    return positionedSelections(positions, play, amount, rules, gameId, ruleVersion)
  }
  return null
}

function segmentParts(segment: string) {
  const separated = segment.split('/').map((item) => item.trim())
  if (separated.length > 1) return separated
  // Pure-number plays remain slash-only. Otherwise split the trailing compact
  // amount: 1大5 => 1大/5, 大单20 => 大单/20.
  const compact = segment.trim().match(/^((?:.*[大小单双龙虎和]|(?:前三|中三|后三)?(?:豹子|顺子|对子|半顺|杂六)))(\d+(?:\.\d{1,2})?)$/)
  return compact ? [compact[1].trim(), compact[2]] : separated
}

function describePayload(payload: BetPayload, gameId?: string, ruleVersion = ''): string {
  const family = gameId === undefined ? 'racing' : lotteryRuleProfile(gameId).family
  let position = payload.play_code === 'sum' ? '冠亚和' : rankLabel(payload.position)
  if (family === 'ssc' || family === 'digit3') {
    position = payload.play_code === 'sum'
      ? /^\d$/.test(payload.selection) ? '总和尾' : '总和'
      : ['leopard', 'straight', 'pair', 'half_straight', 'mixed'].includes(payload.play_code ?? '') ? `${shapeTargetLabel(payload.position)}形态`
        : isDigit5V3Game(gameId ?? '', ruleVersion) && ['dragon_tiger', 'dragon_tiger_tie'].includes(payload.play_code ?? '') ? '第一球 vs 第五球'
          : `第${payload.position}球`
  } else if (family === 'unknown') {
    position = /^\d+$/.test(payload.selection) ? '号码' : '投注项'
  }
  const selection = family === 'racing' && payload.play_code === 'ball_1_5' && payload.selection === '0' ? '10' : payload.selection
  return `${position}[${selection}/${formatBetAmount(payload.amount)}]`
}

/** 将快捷输入文本解析为后端 PlaceBet 参数列表 */
export function parseBetInput(content: string, gameId?: string, ruleVersion = ''): ParsedBet {
  const lines: string[] = []
  const payloads: BetPayload[] = []
  let totalCents = 0
  const text = content.trim().replace(/^买/, '').trim()
  const invalid = (): ParsedBet => ({ content, lines: [], total: 0, payloads: [] })
  if (betCommandError(content)) return invalid()
  const rules = lotteryRuleProfile(gameId ?? 'speed-racing')
  // PC/28 chat commands are parsed and version-checked by the server. The
  // browser only builds its typed detailed-web rows and must not reinterpret
  // PC syntax through the generic three-digit parser.
  if (rules.family !== 'racing' && rules.family !== 'ssc' && rules.family !== 'digit3') return invalid()
  // Five-ball products have one current contract. A missing/stale version
  // must not silently revive a different parser or produce financial rows.
  if (rules.family === 'ssc' && !isDigit5V3Game(gameId ?? '', ruleVersion)) return invalid()

  for (const segment of text.split('#').map((item) => item.trim()).filter(Boolean)) {
    const parts = segmentParts(segment)
    if (parts.length < 2) return invalid()
    const amountText = parts.at(-1) ?? ''
    if (!/^\d+(?:\.\d{1,2})?$/.test(amountText)) return invalid()
    const amountCents = Math.round(Number(amountText) * 100)
    if (!Number.isSafeInteger(amountCents) || amountCents <= 0) return invalid()
    const amount = amountCents / 100
    const segmentPayloads = segmentPayload(parts.slice(0, -1), amount, rules, gameId ?? '', ruleVersion)
    if (!segmentPayloads?.length) return invalid()
    for (const payload of segmentPayloads) {
      payloads.push(payload)
      lines.push(describePayload(payload, gameId, ruleVersion))
      totalCents += amountCents
      if (!Number.isSafeInteger(totalCents)) return invalid()
    }
  }

  return {
    content,
    lines,
    total: totalCents / 100,
    payloads,
  }
}
