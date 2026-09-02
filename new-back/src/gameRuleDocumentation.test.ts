// @ts-expect-error Vitest executes in Node; the production TS config deliberately excludes Node globals.
import { createHash } from 'node:crypto'
// @ts-expect-error Vitest executes in Node; the production TS config deliberately excludes Node globals.
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { currentRuleBindingReady, currentRuleProfileForGame, originalRuleDocumentLineCount, parseOriginalRuleDocument } from './gameRuleDocumentation'

const game = (values: { id: string; name: string; rules_ready?: boolean; rule_version?: string; rules_message?: string }) => values

describe('original game-rule document snapshot', () => {
  const source = readFileSync(new URL('../public/game-docs/original.txt', import.meta.url))

  it('keeps the user-provided original byte-for-byte', () => {
    expect(source.byteLength).toBe(182_088)
    expect(createHash('sha256').update(source).digest('hex')).toBe('95aedecc382618c1b481926040886f7549bc7e64ba05cf4395b986ae3a6a28c3')
  })

  it('builds a navigable index without rewriting original content', () => {
    const text = source.toString('utf8')
    const sections = parseOriginalRuleDocument(text)
    expect(sections).toHaveLength(23)
    expect(originalRuleDocumentLineCount(text)).toBe(8_776)
    expect(sections.map(section => section.title)).toEqual([
      '极速赛车', '香港六合彩', '宾果赛车(A)', '宾果时时彩(一)', '宾果六合彩',
      '澳洲幸运10', '澳洲幸运5', '幸运飞艇', '极速飞艇', '极速时时彩', 'SG飞艇',
      '幸运时时彩', 'PC蛋蛋', '极速快3', '加拿大28-玩法一', '加拿大28-玩法二',
      '加拿大28-玩法三', '动物运动会', '五分运动会', '福彩3D', '体彩排列3',
      '澳门六合彩', '新澳门六合彩',
    ])
    expect(sections[0].content).toContain('【3/大/5】')
    expect(sections[0].rulesContent).toContain('【3/大/5】')
    expect(sections[0].rulesContent).not.toContain('1-10车号-1-10车号')
    expect(sections[0].odds[0]).toEqual({ play: '1-10车号-1-10车号', multiplier: '9.95' })
    expect(sections.find(section => section.title === '极速时时彩')).toMatchObject({ startLine: 4_125, endLine: 4_163 })
    expect(sections.find(section => section.title === '宾果六合彩')).toMatchObject({ startLine: 2_040, endLine: 3_925 })
    expect(sections.at(-1)?.endLine).toBe(8_776)
  })

  it('parses CRLF documents and preserves section line boundaries', () => {
    const sections = parseOriginalRuleDocument('说明\r\n游戏规则 A \r\n甲\r\n游戏规则 B\r\n乙')
    expect(sections).toEqual([
      { id: '1-A', title: 'A', content: '游戏规则 A \n甲', rulesContent: '游戏规则 A \n甲', odds: [], startLine: 2, endLine: 3 },
      { id: '2-B', title: 'B', content: '游戏规则 B\n乙', rulesContent: '游戏规则 B\n乙', odds: [], startLine: 4, endLine: 5 },
    ])
  })
})

describe('current rule documentation profiles', () => {
  it('documents every implemented rules family without inferring from display names', () => {
    expect(currentRuleProfileForGame(game({ id: 'speed-racing', name: '任意名称', rules_ready: true, rule_version: 'racing-v2' })).expectedVersion).toBe('racing-v2')
    expect(currentRuleProfileForGame(game({ id: 'speed-ssc', name: '任意名称', rules_ready: true, rule_version: 'digits5-v3' }))).toMatchObject({
      expectedVersion: 'digits5-v3',
      summary: expect.stringContaining('中三'),
    })
    expect(currentRuleProfileForGame(game({ id: 'au-lucky-5', name: '任意名称', rules_ready: true, rule_version: 'digits5-v3' })).rules.join('')).toContain('第一球与第五球')
    expect(currentRuleProfileForGame(game({ id: 'bingo-ssc-1', name: '宾果时时彩(一)', rules_ready: true, rule_version: 'digits5-v3' }))).toMatchObject({
      expectedVersion: 'digits5-v3',
      modes: '聊天投注 + 详细网投',
      summary: expect.stringContaining('原始20号的前5个号码尾数'),
    })
    expect(currentRuleProfileForGame(game({ id: 'bingo-ssc-1', name: '宾果时时彩(一)', rules_ready: true, rule_version: 'digits5-v3' })).rules.join('')).toContain('交叉核对')
    expect(currentRuleProfileForGame(game({ id: 'official-fc3d', name: '任意名称', rules_ready: true, rule_version: 'digits3-v2' })).expectedVersion).toBe('digits3-v2')
    expect(currentRuleProfileForGame(game({ id: 'bingo-mark-six', name: '任意名称', rules_ready: true, rule_version: 'mark6-v2' }))).toMatchObject({ expectedVersion: 'mark6-v2', modes: '仅详细网投' })
  })

  it.each(['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1'])('documents only the current v3 contract for %s', id => {
    const profile = currentRuleProfileForGame(game({ id, name: id, rules_ready: true, rule_version: 'digits5-v3' }))
    expect(profile.rules.join('')).toContain('不开放投注')
    expect(profile.rules.join('')).toContain('循环顺子')
    expect(profile.differences.find(item => item.topic === '本地附加玩法')).toMatchObject({ status: 'same' })
    expect(JSON.stringify(profile)).not.toContain('digits5-v2')
    expect(JSON.stringify(profile)).not.toContain('历史')
    expect(currentRuleBindingReady(game({ id, name: id, rules_ready: true, rule_version: 'digits5-v3' }))).toBe(true)
    for (const rule_version of [undefined, '', 'digits5-v2', 'digits5-v4']) {
      const snapshot = game({ id, name: id, rules_ready: true, rule_version })
      expect(currentRuleProfileForGame(snapshot).expectedVersion).toBe('digits5-v3')
      expect(currentRuleBindingReady(snapshot)).toBe(false)
    }
    expect(currentRuleBindingReady(game({ id, name: id, rules_ready: false, rule_version: 'digits5-v3' }))).toBe(false)
  })

  it.each(['platform', 'external', 'official'])('keeps SG complete gameplay separate from its platform draw identity with %s metadata', source_kind => {
    const sg = currentRuleProfileForGame({ ...game({ id: 'sg-ssc', name: '任意名称', rules_ready: true, rule_version: 'digits5-v3' }), source_kind, source_name: '未经核验的来源标签' })
    expect(sg).toMatchObject({ expectedVersion: 'digits5-v3', modes: '聊天投注 + 详细网投' })
    expect(sg.summary).toContain('完整支持前三、中三、后三')
    expect(sg.summary).toContain('王者平台自开（王者开奖）')
    expect(sg.summary).toContain('不宣称与SG外部开奖同步')
    expect(sg.summary).not.toContain('未经核验的来源标签')
    expect(sg.differences.find(item => item.topic === '开奖身份')).toMatchObject({ status: 'different' })
  })

  it.each(['bingo-racing-b', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4'])('keeps unverified %s display-only even when a server snapshot borrows another version', id => {
    for (const rule_version of [undefined, '', 'racing-v2', 'digits5-v2', 'digits5-v3']) {
      const snapshot = game({ id, name: id, rules_ready: true, rule_version })
      const profile = currentRuleProfileForGame(snapshot)
      expect(profile).toMatchObject({ expectedVersion: '未绑定', modes: '仅展示 · 不受理投注' })
      expect(profile.summary).toContain('待核验')
      expect(profile.differences[0].status).toBe('pending')
      expect(currentRuleBindingReady(snapshot)).toBe(false)
    }
  })

  it('binds the three PC games to distinct versioned rule contracts', () => {
    const play1 = currentRuleProfileForGame(game({ id: 'pc-canada', name: 'PC加拿大', rules_ready: true, rule_version: 'pc28-v1' }))
    const play2 = currentRuleProfileForGame(game({ id: 'canada-28', name: '加拿大28', rules_ready: true, rule_version: 'pc28-v2' }))
    expect(play1).toMatchObject({ expectedVersion: 'pc28-v1', modes: '聊天投注 + 详细网投' })
    expect(play1.rules.join('')).toContain('不含定位两面')
    expect(play2).toMatchObject({ expectedVersion: 'pc28-v2', summary: expect.stringContaining('玩法二') })
    expect(play2.rules.join('')).toContain('总注大于1且开13/14时')
    expect(currentRuleProfileForGame(game({ id: 'canada-20', name: '加拿大2.0', rules_ready: true, rule_version: 'pc28-v3' })).rules.join('')).toContain('1.98倍')
  })

  it('keeps unknown games visibly pending instead of borrowing another game ruleset', () => {
    expect(currentRuleProfileForGame(game({ id: 'unknown', name: '未知彩种', rules_ready: false })).differences[0].status).toBe('pending')
  })

  it('documents Bingo Racing A as pending until both source readiness and racing-v2 are confirmed', () => {
    expect(currentRuleProfileForGame(game({ id: 'bingo-racing-a', name: '宾果赛车(A)', rules_ready: false }))).toMatchObject({ expectedVersion: '未绑定', modes: '不受理投注' })
    expect(currentRuleProfileForGame(game({ id: 'bingo-racing-a', name: '宾果赛车(A)', rules_ready: true, rule_version: 'racing-v1' })).expectedVersion).toBe('未绑定')
    expect(currentRuleProfileForGame(game({ id: 'bingo-racing-a', name: '宾果赛车(A)', rules_ready: true, rule_version: 'racing-v2' }))).toMatchObject({ expectedVersion: 'racing-v2', modes: '聊天投注 + 详细网投' })
    expect(currentRuleBindingReady(game({ id: 'bingo-racing-a', name: '宾果赛车(A)', rules_ready: true, rule_version: 'racing-v1' }))).toBe(false)
    expect(currentRuleBindingReady(game({ id: 'bingo-racing-a', name: '宾果赛车(A)', rules_ready: true, rule_version: 'racing-v2' }))).toBe(true)
  })
})
