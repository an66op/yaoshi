import { describe, expect, it } from 'vitest'
import { exactRuleResponsesReady, gameRulesReady, isBingoRacingAReady, isBingoSSC1Ready, lotteryResultSummary, lotteryRuleProfile, markSixBallClass, markSixDrawBallClass, markSixWave, markSixZodiac, markSixZodiacLabel, pc28RuleVersionForGame, pc28TripleShape, requiredRuleVersionForGame } from './lotteryRules'

describe('explicit lottery result profiles', () => {
  it.each(['speed-racing', 'speed-fly', 'sg-fly', 'fly-racing', 'au-lucky-10', 'bingo-racing-a', 'bingo-racing-b'])('uses only the first two numbers and five mirrored pairs for %s', id => {
    expect(lotteryRuleProfile(id).family).toBe('racing')
    expect(lotteryResultSummary(id, [4, 7, 1, 2, 3, 5, 6, 8, 9, 10])).toMatchObject({ label: '冠亚和', total: 11, size: '小', parity: '单', dragons: ['虎', '虎', '虎', '虎', '虎'] })
  })

  it.each(['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4'])('uses all five digits and two mirrored pairs for %s', id => {
    expect(lotteryRuleProfile(id).family).toBe('ssc')
    expect(lotteryResultSummary(id, [9, 8, 1, 2, 3])).toMatchObject({ label: '总和', total: 23, size: '大', parity: '单', dragons: ['龙', '龙'] })
    expect(lotteryResultSummary(id, [7, 1, 2, 3, 7])).toMatchObject({ total: 20, size: '小', dragons: ['和', '虎'] })
  })

  it.each(['speed-ssc', 'au-lucky-5', 'bingo-ssc-1'])('uses only first versus fifth for exact digits5-v3 game %s', id => {
    expect(lotteryResultSummary(id, [7, 1, 2, 3, 7], 'digits5-v3')).toMatchObject({ dragons: ['和'], dragonLabel: '第一球 vs 第五球 龙虎和' })
    expect(lotteryResultSummary(id, [9, 8, 1, 2, 3], 'digits5-v3')).toMatchObject({ dragons: ['龙'] })
  })

  it('does not grant the v3 result shape to SG or to a matching game without the exact server version', () => {
    expect(lotteryResultSummary('sg-ssc', [9, 8, 1, 2, 3], 'digits5-v3')?.dragons).toEqual(['龙', '龙'])
    expect(lotteryResultSummary('speed-ssc', [9, 8, 1, 2, 3])?.dragons).toEqual(['龙', '龙'])
    expect(lotteryResultSummary('speed-ssc', [9, 8, 1, 2, 3], 'digits5-v2')?.dragons).toEqual(['龙', '龙'])
    for (const id of ['bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4']) {
      expect(lotteryResultSummary(id, [9, 8, 1, 2, 3], 'digits5-v3')?.dragons).toEqual(['龙', '龙'])
    }
  })

  it('keeps Bingo Racing A closed until the server confirms both readiness and racing-v2', () => {
    expect(isBingoRacingAReady({ id: 'bingo-racing-a', rulesReady: true, ruleVersion: 'racing-v2' })).toBe(true)
    expect(gameRulesReady({ id: 'bingo-racing-a', rulesReady: true, ruleVersion: 'racing-v2' })).toBe(true)
    expect(gameRulesReady({ id: 'bingo-racing-a', ruleVersion: 'racing-v2' })).toBe(false)
    expect(gameRulesReady({ id: 'bingo-racing-a', rulesReady: true })).toBe(false)
    expect(gameRulesReady({ id: 'bingo-racing-a', rulesReady: true, ruleVersion: 'racing-v1' })).toBe(false)
    expect(gameRulesReady({ id: 'bingo-racing-a', rulesReady: false, ruleVersion: 'racing-v2' })).toBe(false)
  })

  it('keeps Bingo SSC (1) closed until the server confirms readiness and digits5-v3', () => {
    expect(isBingoSSC1Ready({ id: 'bingo-ssc-1', rulesReady: true, ruleVersion: 'digits5-v3' })).toBe(true)
    expect(gameRulesReady({ id: 'bingo-ssc-1', rulesReady: true, ruleVersion: 'digits5-v3' })).toBe(true)
    expect(gameRulesReady({ id: 'bingo-ssc-1', ruleVersion: 'digits5-v3' })).toBe(false)
    expect(gameRulesReady({ id: 'bingo-ssc-1', rulesReady: true, ruleVersion: 'digits5-v2' })).toBe(false)
    expect(gameRulesReady({ id: 'bingo-ssc-1', rulesReady: false, ruleVersion: 'digits5-v3' })).toBe(false)
  })

  it.each([
    ['bingo-racing-a', 'racing-v2'],
    ['speed-ssc', 'digits5-v3'],
    ['au-lucky-5', 'digits5-v3'],
    ['bingo-ssc-1', 'digits5-v3'],
    ['pc-canada', 'pc28-v1'],
    ['canada-28', 'pc28-v2'],
    ['canada-20', 'pc28-v3'],
    ['bingo-mark-six', 'mark6-v2'],
  ])('requires matching catalog, odds and assistant versions for %s', (id, version) => {
    const game = { id, rulesReady: true, ruleVersion: version }
    const matching = { game_id: id, rules_ready: true, rule_version: version }
    expect(requiredRuleVersionForGame(id)).toBe(version)
    expect(gameRulesReady(game)).toBe(true)
    expect(exactRuleResponsesReady(game, matching, matching)).toBe(true)
    expect(exactRuleResponsesReady(game, null, matching)).toBe(false)
    expect(exactRuleResponsesReady(game, matching, undefined)).toBe(false)
    expect(exactRuleResponsesReady(game, { rules_ready: true }, matching)).toBe(false)
    expect(exactRuleResponsesReady(game, { ...matching, game_id: 'another-game' }, matching)).toBe(false)
    expect(exactRuleResponsesReady(game, { ...matching, rule_version: `${version}-stale` }, matching)).toBe(false)
    expect(exactRuleResponsesReady(game, matching, { ...matching, rules_ready: false })).toBe(false)
    expect(exactRuleResponsesReady({ ...game, ruleVersion: `${version}-stale` }, matching, matching)).toBe(false)
  })

  it('does not impose the upgrade gate on unrelated legacy products', () => {
    const legacy = { id: 'bingo-ssc-2', rulesReady: true }
    expect(requiredRuleVersionForGame(legacy.id)).toBeNull()
    expect(exactRuleResponsesReady(legacy, null, null)).toBe(true)
    expect(exactRuleResponsesReady(legacy, { rules_ready: true, rule_version: 'digits5-v3' }, { rules_ready: true, rule_version: 'racing-v2' })).toBe(true)
  })

  it.each(['official-fc3d', 'official-pl3'])('uses three-digit sum boundaries for %s', id => {
    expect(lotteryResultSummary(id, [9, 1, 4])).toMatchObject({ total: 14, size: '大', dragons: ['龙'] })
    expect(lotteryResultSummary(id, [6, 1, 6])).toMatchObject({ total: 13, size: '小', dragons: ['和'] })
  })

  it.each([
    ['pc-canada', 'pc28-v1'],
    ['canada-28', 'pc28-v2'],
    ['canada-20', 'pc28-v3'],
  ])('binds %s to %s and shows three balls, their sum and 1↔3 dragon/tiger/tie', (id, version) => {
    expect(pc28RuleVersionForGame(id)).toBe(version)
    expect(lotteryRuleProfile(id)).toMatchObject({ family: 'pc28', ballCount: 3, sumLabel: '和值' })
    expect(lotteryResultSummary(id, [9, 1, 9], version)).toMatchObject({
      label: '和值', total: 19, size: '大', parity: '单', dragons: ['和'], dragonLabel: '第一球 vs 第三球 龙虎和',
    })
    expect(gameRulesReady({ id, rulesReady: true, ruleVersion: version })).toBe(true)
    expect(gameRulesReady({ id, rulesReady: true, ruleVersion: 'pc28-v9' })).toBe(false)
    expect(gameRulesReady({ id, rulesReady: true })).toBe(false)
  })

  it('locks the PC28 non-circular straight boundary', () => {
    expect(pc28TripleShape([0, 1, 2])).toBe('顺子')
    expect(pc28TripleShape([2, 0, 1])).toBe('顺子')
    expect(pc28TripleShape([8, 9, 0])).toBe('杂六')
    expect(pc28TripleShape([9, 0, 1])).toBe('杂六')
    expect(pc28TripleShape([0, 1, 9])).toBe('杂六')
    expect(pc28TripleShape([6, 6, 6])).toBe('豹子')
    expect(pc28TripleShape([6, 1, 6])).toBe('对子')
  })

  it('uses the explicit seven-ball Bingo Mark Six profile and fixed wave colours', () => {
    expect(lotteryRuleProfile('bingo-mark-six')).toMatchObject({ family: 'mark-six', ballCount: 7 })
    expect(lotteryResultSummary('bingo-mark-six', [1, 7, 18, 25, 30, 42, 49])).toMatchObject({
      label: '特码', total: 49, size: '和', parity: '和', text: '49 绿波 和局', dragonText: '',
    })
    expect(lotteryResultSummary('bingo-mark-six', [1, 7, 18, 25, 30, 42, 46])).toMatchObject({ total: 46, size: '大', parity: '双', text: '46 红波 大双' })
    expect(markSixWave(1)).toBe('red')
    expect(markSixWave(48)).toBe('blue')
    expect(markSixWave(49)).toBe('green')
    expect(markSixBallClass(49)).toContain('wave-green')
    expect(markSixDrawBallClass(49, 6, 7)).toContain('mark-six-special-ball')
    expect(markSixDrawBallClass(49, 5, 7)).not.toContain('mark-six-special-ball')
    expect(gameRulesReady({ id: 'bingo-mark-six' })).toBe(false)
    expect(gameRulesReady({ id: 'bingo-mark-six', rulesReady: true, ruleVersion: 'mark6-v2' })).toBe(true)
  })

  it('maps numbers by the draw-date lunar zodiac and changes exactly at Lunar New Year', () => {
    const horseYear = '2026-09-01T12:00:00+08:00'
    expect([35, 34, 23, 30, 22, 6, 20].map(number => markSixZodiac(number, horseYear))).toEqual(['猴', '鸡', '猴', '牛', '鸡', '牛', '猪'])
    expect(markSixZodiac(1, '2026-02-16T12:00:00+08:00')).toBe('蛇')
    expect(markSixZodiac(1, '2026-02-17T12:00:00+08:00')).toBe('马')
    expect(markSixZodiac(49, horseYear)).toBe('马')
    expect(markSixZodiacLabel(1, 'not-a-date')).toBe('—')
    expect(markSixZodiac(0, horseYear)).toBeNull()
    expect(markSixZodiac(50, horseYear)).toBeNull()
  })

  it.each(['hong-kong-mark-six', 'official-kl8', 'official-tw-bingo', 'renamed-speed-racing'])('does not guess result/betting rules for %s', id => {
    expect(lotteryRuleProfile(id).family).toBe('unknown')
    expect(lotteryResultSummary(id, [1, 2, 3])).toBeNull()
    expect(gameRulesReady({ id, rulesReady: true })).toBe(false)
  })

  it('does not derive an outcome from malformed known-game numbers or override explicit server denial', () => {
    expect(lotteryResultSummary('speed-racing', [1, 2, 3])).toBeNull()
    expect(lotteryResultSummary('speed-racing', [1, 2, 3, 4, 5, 6, 7, 8, 9, 9])).toBeNull()
    expect(lotteryResultSummary('speed-ssc', [1, 2, 3, 4, 10])).toBeNull()
    expect(lotteryResultSummary('bingo-mark-six', [1, 2, 3, 4, 5, 6, 6])).toBeNull()
    expect(lotteryResultSummary('bingo-mark-six', [1, 2, 3, 4, 5, 6, 50])).toBeNull()
    expect(gameRulesReady({ id: 'speed-racing', rulesReady: false })).toBe(false)
    expect(gameRulesReady({ id: 'speed-racing' })).toBe(true)
  })
})
