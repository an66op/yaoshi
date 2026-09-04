import type { LotteryTiming } from './lotteryTiming'

export type LotteryRuleFamily = 'racing' | 'ssc' | 'digit3' | 'pc28' | 'mark-six' | 'unknown'
const racingIDs = new Set(['speed-racing', 'speed-fly', 'sg-fly', 'fly-racing', 'au-lucky-10', 'bingo-racing-a', 'bingo-racing-b'])
const sscIDs = new Set(['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4'])
// Only these products have the versioned middle/back-three and 1↔5
// dragon/tiger/tie settlement contract. Do not infer this capability from the
// generic five-ball family.
const digit5V3IDs = new Set(['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4'])
const digit3IDs = new Set(['official-fc3d', 'official-pl3'])
const pc28RuleVersions = {
  'pc-canada': 'pc28-v1',
  'canada-28': 'pc28-v2',
  'canada-20': 'pc28-v3',
} as const
const markSixRuleVersions = {
  'bingo-mark-six': 'mark6-v2',
  'hong-kong-mark-six': 'hk-mark6-v1',
  'happy8-mark-six': 'happy8-mark6-v1',
  'new-macau-mark-six': 'new-macau-mark6-v1',
  'old-macau-mark-six': 'old-macau-mark6-v1',
} as const
// These products changed financial/settlement semantics in place.  Opening a
// board from one endpoint's version while another endpoint still serves the
// previous contract can display selections that the server will reject (or,
// worse, describe a different wager).  Keep this identity-owned allowlist
// explicit and require the catalog, odds and assistant snapshots to agree.
const crossResponseRuleVersions = {
  'bingo-racing-a': 'racing-v2',
  'bingo-racing-b': 'racing-v2',
  'speed-ssc': 'digits5-v3',
  'sg-ssc': 'digits5-v3',
  'au-lucky-5': 'digits5-v3',
  'bingo-ssc-1': 'digits5-v3',
  'bingo-ssc-2': 'digits5-v3',
  'bingo-ssc-3': 'digits5-v3',
  'bingo-ssc-4': 'digits5-v3',
  ...pc28RuleVersions,
  ...markSixRuleVersions,
} as const
// Every seven-ball product owns an exact rule version and independent price
// snapshot, even though they intentionally reuse the same Mark Six board.
const markSixIDs = new Set(Object.keys(markSixRuleVersions))
const markSixDrawPresentationIDs = new Set(Object.keys(markSixRuleVersions))

const redWaveNumbers = new Set([1, 2, 7, 8, 12, 13, 18, 19, 23, 24, 29, 30, 34, 35, 40, 45, 46])
const blueWaveNumbers = new Set([3, 4, 9, 10, 14, 15, 20, 25, 26, 31, 36, 37, 41, 42, 47, 48])

export type MarkSixWave = 'red' | 'blue' | 'green'
export const markSixZodiacOrder = ['鼠', '牛', '虎', '兔', '龙', '蛇', '马', '羊', '猴', '鸡', '狗', '猪'] as const
export type MarkSixZodiac = typeof markSixZodiacOrder[number]

let chineseYearFormatter: Intl.DateTimeFormat | null | undefined

function markSixChineseRelatedYear(value: string | number | Date): number | null {
  const date = value instanceof Date ? new Date(value.getTime()) : new Date(value)
  if (!Number.isFinite(date.getTime())) return null
  try {
    if (chineseYearFormatter === undefined) {
      chineseYearFormatter = new Intl.DateTimeFormat('en-u-ca-chinese', {
        timeZone: 'Asia/Shanghai',
        year: 'numeric',
      })
    }
    if (chineseYearFormatter === null) return null
    // ECMA-402 emits `relatedYear` for non-Gregorian calendars, but older
    // TypeScript DOM declarations do not include it in their narrowed union.
    const parts = chineseYearFormatter.formatToParts(date) as Array<{ type: string; value: string }>
    const relatedYear = parts.find(part => part.type === 'relatedYear')?.value
    if (!relatedYear || !/^\d{4}$/.test(relatedYear)) return null
    const year = Number(relatedYear)
    return Number.isInteger(year) ? year : null
  } catch {
    // A browser without the Chinese calendar must show an unavailable label;
    // silently guessing by Gregorian year is wrong around Lunar New Year.
    chineseYearFormatter = null
    return null
  }
}

const positiveModulo = (value: number, divisor: number) => ((value % divisor) + divisor) % divisor

/**
 * Resolve a 1–49 Mark Six number to its zodiac for the draw's actual date.
 * Number 1 belongs to that lunar year's zodiac; every following number walks
 * backwards through the canonical 鼠→猪 cycle. The Shanghai timezone keeps
 * the Lunar New Year boundary identical to the lottery room.
 */
export function markSixZodiac(number: number, drawAt: string | number | Date | null | undefined): MarkSixZodiac | null {
  if (!Number.isInteger(number) || number < 1 || number > 49 || drawAt == null) return null
  const relatedYear = markSixChineseRelatedYear(drawAt)
  if (relatedYear === null) return null
  // 2020 is a known Rat year. Modulo keeps the mapping valid in both
  // directions without freezing the product to a single yearly lookup table.
  const yearZodiacIndex = positiveModulo(relatedYear - 2020, markSixZodiacOrder.length)
  return markSixZodiacOrder[positiveModulo(yearZodiacIndex - (number - 1), markSixZodiacOrder.length)]
}

export function markSixZodiacLabel(number: number, drawAt: string | number | Date | null | undefined) {
  return markSixZodiac(number, drawAt) ?? '—'
}

/** Fixed 1–49 wave colours; never reuse the racing ten-colour palette. */
export function markSixWave(number: number): MarkSixWave | null {
  if (!Number.isInteger(number) || number < 1 || number > 49) return null
  if (redWaveNumbers.has(number)) return 'red'
  if (blueWaveNumbers.has(number)) return 'blue'
  return 'green'
}

export function markSixWaveLabel(number: number) {
  const wave = markSixWave(number)
  return wave === 'red' ? '红波' : wave === 'blue' ? '蓝波' : wave === 'green' ? '绿波' : '—'
}

export function markSixBallClass(number: number) {
  const wave = markSixWave(number)
  return `lottery-ball mark-six-ball${wave ? ` wave-${wave}` : ''}`
}

export function markSixDrawBallClass(number: number, index: number, length: number) {
  return `${markSixBallClass(number)}${index === 6 && length === 7 ? ' mark-six-special-ball' : ''}`
}

/** Seven-ball draw styling only; this helper never implies a rule family. */
export function usesMarkSixDrawPresentation(gameId: string) {
  return markSixDrawPresentationIDs.has(gameId)
}

/** Identity, not a mutable name/category or the latest draw's shape, owns rules. */
export function lotteryRuleProfile(gameId: string) {
  const family: LotteryRuleFamily = racingIDs.has(gameId) ? 'racing' : sscIDs.has(gameId) ? 'ssc' : digit3IDs.has(gameId) ? 'digit3' : pc28RuleVersionForGame(gameId) !== null ? 'pc28' : markSixIDs.has(gameId) ? 'mark-six' : 'unknown'
  return {
    family,
    ballCount: family === 'racing' ? 10 : family === 'ssc' ? 5 : family === 'digit3' || family === 'pc28' ? 3 : family === 'mark-six' ? 7 : null,
    sumLabel: family === 'racing' ? '冠亚和' : family === 'pc28' ? '和值' : family === 'mark-six' ? '特码' : '总和',
    sumThreshold: family === 'racing' ? 12 : family === 'ssc' ? 23 : family === 'mark-six' ? 25 : 14,
    numberThreshold: family === 'racing' ? 6 : family === 'mark-six' ? 25 : 5,
  } as const
}

export type PC28RuleVersion = typeof pc28RuleVersions[keyof typeof pc28RuleVersions]

/** The product identity owns the financial PC/28 version; names never do. */
export function pc28RuleVersionForGame(gameId: string): PC28RuleVersion | null {
  return pc28RuleVersions[gameId as keyof typeof pc28RuleVersions] ?? null
}

export function isPC28RuleVersion(gameId: string, ruleVersion: string | null | undefined) {
  const expected = pc28RuleVersionForGame(gameId)
  return expected !== null && ruleVersion === expected
}

export type CrossResponseRuleVersion = typeof crossResponseRuleVersions[keyof typeof crossResponseRuleVersions]

/** Exact current contract for products that must never mix response versions. */
export function requiredRuleVersionForGame(gameId: string): CrossResponseRuleVersion | null {
  return crossResponseRuleVersions[gameId as keyof typeof crossResponseRuleVersions] ?? null
}

type RuleVersionSnapshot = { game_id?: string; rules_ready?: boolean; rule_version?: string } | null | undefined

/**
 * Version-sensitive rooms stay closed until all three independently loaded
 * snapshots confirm the same exact contract.  Missing fields and missing
 * responses are not treated as legacy-compatible: a partial rollout must be
 * fail-closed and must not mount the previous detailed board.
 */
export function exactRuleResponsesReady(
  game: { id: string; rulesReady?: boolean; ruleVersion?: string },
  odds: RuleVersionSnapshot,
  assistant: RuleVersionSnapshot,
) {
  const expected = requiredRuleVersionForGame(game.id)
  if (expected === null) return gameRulesReady(game)
  const responseMatches = (response: RuleVersionSnapshot) => response !== null
    && response !== undefined
    && response.game_id === game.id
    && response.rules_ready === true
    && response.rule_version === expected
  return game.rulesReady === true
    && game.ruleVersion === expected
    && responseMatches(odds)
    && responseMatches(assistant)
}

export type PC28TripleShape = '豹子' | '对子' | '顺子' | '杂六'

/** PC/28 follows the original rules: 890 and 901 are circular straights. */
export function pc28TripleShape(numbers: readonly number[]): PC28TripleShape | null {
  if (numbers.length !== 3 || numbers.some(number => !Number.isInteger(number) || number < 0 || number > 9)) return null
  const unique = new Set(numbers).size
  if (unique === 1) return '豹子'
  if (unique === 2) return '对子'
  const sorted = [...numbers].sort((left, right) => left - right)
  const consecutive = sorted[1] === sorted[0] + 1 && sorted[2] === sorted[1] + 1
  const circular = sorted[0] === 0 && (sorted[1] === 8 && sorted[2] === 9 || sorted[1] === 1 && sorted[2] === 9)
  return consecutive || circular ? '顺子' : '杂六'
}

export function isDigit5V3Game(gameId: string, ruleVersion: string | null | undefined) {
  return digit5V3IDs.has(gameId) && ruleVersion === 'digits5-v3'
}

/**
 * Bingo Racing (A) needs the normal racing-v2 bet contract *and* an audited
 * ordered source conversion.  The server expresses the latter by withholding
 * readiness/version until the source is safe; never infer it from the title.
 */
export function isBingoRacingAReady(game: { id: string; rulesReady?: boolean; ruleVersion?: string }) {
  return game.id === 'bingo-racing-a' && game.rulesReady === true && game.ruleVersion === 'racing-v2'
}

/** Bingo SSC (1) also depends on the audited ordered Bingo feed. */
export function isBingoSSC1Ready(game: { id: string; rulesReady?: boolean; ruleVersion?: string }) {
  return game.id === 'bingo-ssc-1' && game.rulesReady === true && game.ruleVersion === 'digits5-v3'
}

export const UNCONFIGURED_RULES_MESSAGE = '暂未配置完整玩法，暂停受理；仍可查看开奖结果和聊天。'

export function gameRulesReady(game: { id: string; rulesReady?: boolean; ruleVersion?: string }) {
  // Older known-game snapshots may omit readiness, but no label/category can
  // turn an unknown game into a supported one. Explicit server denial wins.
  const family = lotteryRuleProfile(game.id).family
  if (family === 'unknown' || game.rulesReady === false) return false
  const expected = requiredRuleVersionForGame(game.id)
  // Products with materially different historical contracts require an
  // explicit ready bit and their identity-owned current version.  Generic
  // unchanged products retain the legacy omission-compatible read behaviour.
  return expected === null || (game.rulesReady === true && game.ruleVersion === expected)
}

export function rulesBlockedTiming(timing: LotteryTiming): LotteryTiming {
  return { ...timing, phase: 'unavailable', phaseLabel: '仅开奖', statusLabel: '仅展示已公布开奖 · 投注未开放', accepting: false, due: '--:--', remainingSeconds: null }
}

export function sourcePausedTiming(timing: LotteryTiming): LotteryTiming {
  return { ...timing, phase: 'unavailable', phaseLabel: '开奖暂停', statusLabel: '开奖同步暂停 · 投注已暂停', accepting: false, due: '--:--', remainingSeconds: null }
}

export function lotteryResultSummary(gameId: string, numbers: number[], ruleVersion = '') {
  const profile = lotteryRuleProfile(gameId)
  if (profile.family === 'ssc' && !isDigit5V3Game(gameId, ruleVersion)) return null
  // The four direct Mark Six products were promoted from results-only shelves
  // to independent betting contracts.  Keep their seven-ball presentation for
  // historic rows, but never infer a playable outcome unless the response
  // carries that exact product version. Bingo retains its established
  // version-omission-compatible result display.
  if (profile.family === 'mark-six' && gameId !== 'bingo-mark-six' && ruleVersion !== requiredRuleVersionForGame(gameId)) return null
  const minimum = profile.family === 'racing' || profile.family === 'mark-six' ? 1 : 0
  const maximum = profile.family === 'racing' ? 10 : profile.family === 'mark-six' ? 49 : 9
  const valid = profile.ballCount !== null && numbers.length === profile.ballCount
    && numbers.every(number => Number.isInteger(number) && number >= minimum && number <= maximum)
    && ((profile.family !== 'racing' && profile.family !== 'mark-six') || new Set(numbers).size === profile.ballCount)
  if (!valid) return null
  if (profile.family === 'mark-six') {
    const special = numbers[6]
    const push = special === 49
    const size = push ? '和' : special >= 25 ? '大' : '小'
    const parity = push ? '和' : special % 2 ? '单' : '双'
    const wave = markSixWaveLabel(special)
    const text = `${special} ${wave} ${push ? '和局' : `${size}${parity}`}`
    return { label: '特码', total: special, size, parity, text, spacedText: text, dragons: [], dragonText: '', dragonLabel: '' }
  }
  const total = (profile.family === 'racing' ? numbers.slice(0, 2) : numbers).reduce((sum, number) => sum + number, 0)
  const size = total >= profile.sumThreshold ? '大' : '小'
  const parity = total % 2 ? '单' : '双'
  const digit5V3 = isDigit5V3Game(gameId, ruleVersion)
  const dragonCount = digit5V3 ? 1 : Math.floor(numbers.length / 2)
  const dragons = numbers.slice(0, dragonCount).map((number, index) => {
    const opponent = numbers[numbers.length - 1 - index]
    return number > opponent ? '龙' : number < opponent ? '虎' : '和'
  })
  return { label: profile.sumLabel, total, size, parity, text: `${total}${size}${parity}`, spacedText: `${total} ${size} ${parity}`, dragons, dragonText: dragons.join(''), dragonLabel: digit5V3 ? '第一球 vs 第五球 龙虎和' : profile.family === 'pc28' ? '第一球 vs 第三球 龙虎和' : `1–${dragons.length} 龙虎` }
}
