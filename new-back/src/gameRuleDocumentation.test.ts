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
      summary: expect.stringContaining('第1–5个号码尾数'),
    })
    expect(currentRuleProfileForGame(game({ id: 'bingo-ssc-1', name: '宾果时时彩(一)', rules_ready: true, rule_version: 'digits5-v3' })).rules.join('')).toContain('交叉核对')
    expect(currentRuleProfileForGame(game({ id: 'official-fc3d', name: '任意名称', rules_ready: true, rule_version: 'digits3-v2' })).expectedVersion).toBe('digits3-v2')
    expect(currentRuleProfileForGame(game({ id: 'bingo-mark-six', name: '任意名称', rules_ready: true, rule_version: 'mark6-v2' }))).toMatchObject({ expectedVersion: 'mark6-v2', modes: '仅详细网投' })
    expect(currentRuleProfileForGame(game({ id: 'hong-kong-mark-six', name: '任意名称', rules_ready: true, rule_version: 'hk-mark6-v1' }))).toMatchObject({ expectedVersion: 'hk-mark6-v1', modes: '仅详细网投' })
    expect(currentRuleProfileForGame(game({ id: 'happy8-mark-six', name: '任意名称', rules_ready: true, rule_version: 'happy8-mark6-v1' }))).toMatchObject({ expectedVersion: 'happy8-mark6-v1', modes: '仅详细网投' })
    expect(currentRuleProfileForGame(game({ id: 'new-macau-mark-six', name: '任意名称', rules_ready: true, rule_version: 'new-macau-mark6-v1' }))).toMatchObject({ expectedVersion: 'new-macau-mark6-v1', modes: '仅详细网投' })
    expect(currentRuleProfileForGame(game({ id: 'old-macau-mark-six', name: '任意名称', rules_ready: true, rule_version: 'old-macau-mark6-v1' }))).toMatchObject({ expectedVersion: 'old-macau-mark6-v1', modes: '仅详细网投' })
  })

  it('documents the complete Mark Six web contract and keeps internal prices non-bettable', () => {
    const profile = currentRuleProfileForGame(game({ id: 'bingo-mark-six', name: '宾果六合彩', rules_ready: true, rule_version: 'mark6-v2' }))
    const rules = profile.rules.join('')
    expect(profile.summary).toContain('原版全部详细网投市场')
    expect(profile.summary).toContain('ID185')
    expect(profile.summary).toContain('ID135')
    expect(rules).toContain('一肖、一尾')
    expect(rules).toContain('2–11肖')
    expect(rules).toContain('正肖')
    expect(rules).toContain('2–5连肖与2–5连尾')
    expect(rules).toContain('5–11不中')
    expect(rules).toContain('一次扣款的一张组合票')
    expect(rules).toContain('两档互斥派彩的赔率在下注时同时冻结')
    expect(rules).toContain('仅后台定价')
    expect(rules).toContain('会员不能')
    expect(rules).toContain('单注最低、单注最高、会员单期和全房单期限额')
    expect(profile.differences.find(item => item.topic === '复杂组合')).toMatchObject({ status: 'same' })
    expect(JSON.stringify(profile)).not.toMatch(/尚未全部开放|模型完整前保持不可提交/)
  })

  it.each([
    ['hong-kong-mark-six', 'hk-mark6-v1', 'ID18'],
    ['happy8-mark-six', 'happy8-mark6-v1', 'ID141'],
    ['new-macau-mark-six', 'new-macau-mark6-v1', 'ID140'],
    ['old-macau-mark-six', 'old-macau-mark6-v1', 'ID70'],
  ])('documents %s as an independently versioned, web-only Mark Six product', (id, version, sourceID) => {
    const gameState = game({ id, name: id, rules_ready: true, rule_version: version })
    const profile = currentRuleProfileForGame(gameState)
    expect(profile).toMatchObject({ expectedVersion: version, modes: '仅详细网投' })
    expect(profile.summary).toContain(sourceID)
    expect(profile.summary).toContain('赔率、限额、来源和注单快照均按本彩种独立保存')
    expect(profile.rules.join('')).toContain('五行使用原版固定号码表')
    expect(profile.rules.join('')).toContain('旧的五位模拟记录')
    expect(currentRuleBindingReady(gameState)).toBe(true)
    expect(currentRuleBindingReady({ ...gameState, rule_version: 'mark6-v2' })).toBe(false)
  })

  it('does not mislabel 快乐8六合彩 as official 福彩快乐8', () => {
    const profile = currentRuleProfileForGame(game({ id: 'happy8-mark-six', name: '快乐8六合彩', rules_ready: true, rule_version: 'happy8-mark6-v1' }))
    expect(profile.summary).toContain('163派生7球私盘')
    expect(profile.summary).toContain('不从官方福彩快乐8的20球临时筛选')
    expect(profile.differences.find(item => item.topic === '开奖来源')).toMatchObject({ status: 'current-only' })
  })

  it.each(['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4'])('documents only the current v3 contract for %s', id => {
    const profile = currentRuleProfileForGame(game({ id, name: id, rules_ready: true, rule_version: 'digits5-v3' }))
    expect(profile.rules.join('')).toContain('不开放投注')
    expect(profile.rules.join('')).toContain('循环顺子')
    expect(profile.differences.find(item => item.topic === '本地附加玩法')).toMatchObject({ status: 'same' })
    expect(JSON.stringify(profile)).not.toContain('digits5-v2')
    if (id === 'sg-ssc') expect(JSON.stringify(profile)).toContain('有限历史')
    else expect(JSON.stringify(profile)).not.toContain('历史')
    expect(currentRuleBindingReady(game({ id, name: id, rules_ready: true, rule_version: 'digits5-v3' }))).toBe(true)
    for (const rule_version of [undefined, '', 'digits5-v2', 'digits5-v4']) {
      const snapshot = game({ id, name: id, rules_ready: true, rule_version })
      expect(currentRuleProfileForGame(snapshot).expectedVersion).toBe('digits5-v3')
      expect(currentRuleBindingReady(snapshot)).toBe(false)
    }
    expect(currentRuleBindingReady(game({ id, name: id, rules_ready: false, rule_version: 'digits5-v3' }))).toBe(false)
  })

  it.each(['platform', 'external', 'official'])('keeps the exact SG external cross-check contract separate from %s display metadata', source_kind => {
    const sg = currentRuleProfileForGame({ ...game({ id: 'sg-ssc', name: '任意名称', rules_ready: true, rule_version: 'digits5-v3' }), source_kind, source_name: '未经核验的来源标签' })
    expect(sg).toMatchObject({ expectedVersion: 'digits5-v3', modes: '聊天投注 + 详细网投' })
    expect(sg.summary).toContain('完整支持前三、中三、后三')
    expect(sg.summary).toContain('163目录ID64是唯一号码母源')
    expect(sg.summary).toContain('115的sgssc产品只读校验')
    expect(sg.summary).not.toContain('未经核验的来源标签')
    expect(JSON.stringify(sg)).not.toContain('王者开奖')
    expect(sg.rules.join('')).toContain('163目录ID64（站内名称“168SG时时彩”）是唯一号码母源')
    expect(sg.rules.join('')).toContain('115的sgssc产品是只读校验源')
    expect(sg.rules.join('')).toContain('最近连续24期')
    expect(sg.rules.join('')).toContain('不能替代或补写ID64缺失的号码')
    expect(sg.rules.join('')).toContain('已保存的可信结果仍可结算，按匹配的注单来源快照幂等处理')
    expect(sg.rules.join('')).toContain('ID169属于另一套开奖结果系统')
    expect(JSON.stringify(sg)).not.toContain('api.api168168.com')
    expect(sg.rules.join('')).toContain('一致不能证明上游独立')
    expect(sg.differences.find(item => item.topic === '开奖身份')).toMatchObject({ status: 'different', current: expect.stringContaining('已保存的可信结果仍可结算') })
  })

  it.each([
    ['bingo-racing-b', 'racing-v2', '后10个'],
    ['bingo-ssc-2', 'digits5-v3', '第6–10个'],
    ['bingo-ssc-3', 'digits5-v3', '第11–15个'],
    ['bingo-ssc-4', 'digits5-v3', '第16–20个'],
  ])('documents the verified conversion and exact rule version for %s', (id, version, segment) => {
    const profile = currentRuleProfileForGame(game({ id, name: id, rules_ready: true, rule_version: version }))
    expect(profile).toMatchObject({ expectedVersion: version, modes: '聊天投注 + 详细网投' })
    expect(profile.summary).toContain(segment)
    expect(profile.rules.join('')).toContain('ID185')
    expect(profile.rules.join('')).toContain('ID135')
    expect(JSON.stringify(profile)).toContain('平台扩展')
    expect(JSON.stringify(profile)).toContain('不属于原版')
    if (id === 'bingo-racing-b') {
      expect(profile.differences.find(item => item.topic === '冠亚和赔率')).toMatchObject({ status: 'current-only', current: expect.stringContaining('21个') })
    }
    expect(currentRuleBindingReady(game({ id, name: id, rules_ready: true, rule_version: version }))).toBe(true)
    expect(currentRuleBindingReady(game({ id, name: id, rules_ready: true, rule_version: `${version}-stale` }))).toBe(false)
  })

  it('binds the three PC games to distinct versioned rule contracts', () => {
    const play1 = currentRuleProfileForGame(game({ id: 'pc-canada', name: 'PC加拿大', rules_ready: true, rule_version: 'pc28-v1' }))
    const play2 = currentRuleProfileForGame(game({ id: 'canada-28', name: '加拿大28', rules_ready: true, rule_version: 'pc28-v2' }))
    expect(play1).toMatchObject({ expectedVersion: 'pc28-v1', modes: '聊天投注 + 详细网投' })
    expect(play1.rules.join('')).toContain('不含定位两面')
    expect(play1.rules.join('')).toContain('原版明确890和901算顺子')
    expect(play1.differences.find(item => item.topic === '顺子边界')).toMatchObject({ status: 'same', original: expect.stringContaining('算顺子') })
    expect(play1.differences.find(item => item.topic === '13/14规则')).toMatchObject({ status: 'current-only', original: expect.stringContaining('没有13/14动态') })
    expect(play2).toMatchObject({ expectedVersion: 'pc28-v2', summary: expect.stringContaining('玩法二') })
    expect(play2.rules.join('')).toContain('总注大于1且开13/14时')
    expect(currentRuleProfileForGame(game({ id: 'canada-20', name: '加拿大2.0', rules_ready: true, rule_version: 'pc28-v3' })).rules.join('')).toContain('1.98倍')
  })

  it('keeps unknown games visibly pending instead of borrowing another game ruleset', () => {
    expect(currentRuleProfileForGame(game({ id: 'unknown', name: '未知彩种', rules_ready: false })).differences[0].status).toBe('pending')
  })

  it('documents Bingo Racing A contract while binding readiness only to exact racing-v2', () => {
    expect(currentRuleProfileForGame(game({ id: 'bingo-racing-a', name: '宾果赛车(A)', rules_ready: false }))).toMatchObject({ expectedVersion: 'racing-v2', modes: '聊天投注 + 详细网投' })
    expect(currentRuleProfileForGame(game({ id: 'bingo-racing-a', name: '宾果赛车(A)', rules_ready: true, rule_version: 'racing-v1' })).expectedVersion).toBe('racing-v2')
    expect(currentRuleProfileForGame(game({ id: 'bingo-racing-a', name: '宾果赛车(A)', rules_ready: true, rule_version: 'racing-v2' }))).toMatchObject({ expectedVersion: 'racing-v2', modes: '聊天投注 + 详细网投' })
    expect(currentRuleBindingReady(game({ id: 'bingo-racing-a', name: '宾果赛车(A)', rules_ready: true, rule_version: 'racing-v1' }))).toBe(false)
    expect(currentRuleBindingReady(game({ id: 'bingo-racing-a', name: '宾果赛车(A)', rules_ready: true, rule_version: 'racing-v2' }))).toBe(true)
  })
})
