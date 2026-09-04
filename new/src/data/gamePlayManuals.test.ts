import { describe, expect, it } from 'vitest'
import type { Game } from '../types'
import { gameManualOptions, manualForGame } from './gamePlayManuals'

const game = (id: string, title = id) => ({ id, title }) as Game

describe('member game manuals stay honest about implementation status', () => {
  it('maps both verified Bingo racing conversions behind exact racing-v2 readiness', () => {
    expect(manualForGame(game('speed-racing', '极速赛车'))).toMatchObject({ status: 'implemented', gameId: 'speed-racing' })
    for (const [id, segment] of [['bingo-racing-a', '前10个'], ['bingo-racing-b', '后10个']] as const) {
      const pending = manualForGame(game(id))
      expect(pending).toMatchObject({ status: 'partial', sourceURL: 'https://www.www-163kai.cc/', betModes: { chat: false, web: false } })
      expect(pending.summary).toContain(segment)
      expect(pending.auditNotes?.join('')).toContain('ID185')
      expect(pending.auditNotes?.join('')).toContain('ID135')
      expect(pending.auditNotes?.join('')).toContain('冠亚和')
      expect(manualForGame({ ...game(id), rulesReady: true, ruleVersion: 'racing-v2' }))
        .toMatchObject({ status: 'implemented', betModes: { chat: true, web: true } })
      expect(manualForGame({ ...game(id), rulesReady: true, ruleVersion: 'racing-v1' }).betModes)
        .toEqual({ chat: false, web: false })
    }
    const bingoB = manualForGame({ ...game('bingo-racing-b'), rulesReady: true, ruleVersion: 'racing-v2' })
    expect(JSON.stringify(bingoB)).toContain('平台扩展')
    expect(JSON.stringify(bingoB)).toContain('不属于原版')
    expect(bingoB.auditNotes?.join('')).toContain('本彩种后台确认赔率')
  })

  it('publishes each exact PC28 version while keeping an unversioned room fail-closed', () => {
    const mapped = manualForGame({ ...game('canada-28', '加拿大28'), rulesReady: true, ruleVersion: 'pc28-v2', sourceName: '王者开奖' })
    expect(mapped).toMatchObject({ status: 'implemented', gameId: 'canada-28', betModes: { chat: true, web: true } })
    expect(mapped.statusText).toContain('玩法二')
    expect(mapped.summary).toContain('pc28-v2')
    expect(mapped.summary).toContain('王者开奖')
    expect(mapped.sections.map(item => item.body).join('')).toContain('组合总注严格大于1且开13或14时')
    expect(mapped.sections.map(item => item.body).join('')).toContain('庄家通吃')
    expect(mapped.sections.map(item => item.body).join('')).toContain('并非所给原版说明中的赔率表条款')
    expect(mapped.auditNotes?.join('')).toContain('原版明确890与901算顺子')
    expect(mapped.auditNotes?.join('')).toContain('019作为同一组号码同样命中')
    expect(manualForGame(game('canada-28', '加拿大28'))).toMatchObject({ status: 'implemented', betModes: { chat: false, web: false } })
    const options = gameManualOptions([game('speed-racing')])
    const animal = options.find(item => item.id === 'reference-animal-1m')!
    expect(animal.status).toBe('reference')
    expect(animal.gameId).toBeUndefined()
    expect(options.filter(item => item.id.startsWith('reference-pc28-play-'))).toHaveLength(3)
  })

  it.each([
    ['pc-canada', 'pc28-v1', '玩法一', '不计入有效流水'],
    ['canada-28', 'pc28-v2', '玩法二', '庄家通吃'],
    ['canada-20', 'pc28-v3', '玩法三', '3.65倍'],
  ])('binds %s only to %s and states the %s financial difference', (id, ruleVersion, label, difference) => {
    const manual = manualForGame({ ...game(id), rulesReady: true, ruleVersion })
    expect(manual.statusText).toContain(label)
    expect(manual.summary).toContain(ruleVersion)
    expect(manual.sections.map(section => section.body).join('')).toContain(difference)
    expect(manual.sections.map(section => section.body).join('')).toContain('第一球对第三球')
  })

  it.each([
    ['pc-canada', 'pc28-v1'],
    ['canada-28', 'pc28-v2'],
  ])('limits the reverse-bet prohibition to sum size/parity for %s', (id, ruleVersion) => {
    const rules = manualForGame({ ...game(id), rulesReady: true, ruleVersion }).sections.map(section => section.body).join('')
    expect(rules).toContain('禁止反向仅指和值大小及和值单双市场')
    expect(rules).toContain('不包含第1–3球的定位两面')
  })

  it('documents the exact compact examples supplied for racing and five-digit games', () => {
    const racing = manualForGame(game('au-lucky-10'))
    expect(racing.sections.flatMap(item => item.examples ?? [])).toEqual(expect.arrayContaining(['1大5 = 1/大/5', '余额100，1/123/梭哈 → 每项33，余额1']))
    const digits = manualForGame(game('speed-ssc'))
    expect(digits.sections.flatMap(item => item.examples ?? [])).toEqual(expect.arrayContaining(['大/20 → 第1至第5球大，各20', '中三/顺子/5']))
    expect(digits.auditNotes?.join('')).toContain('明确绑定 digits5-v3 且规则就绪')
    expect(digits.betModes).toEqual({ chat: false, web: false })
  })

  it.each(['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4'])('publishes the v3 manual only for an exact server-versioned game %s', id => {
    const manual = manualForGame({ ...game(id), rulesReady: true, ruleVersion: 'digits5-v3' })
    expect(manual).toMatchObject({ status: 'implemented', statusText: '三段形态与龙虎和已接入' })
    expect(manual.betModes).toEqual({ chat: true, web: true })
    expect(manual.summary).toContain('前三/中三/后三')
    expect(manual.summary).toContain('总和、总和尾')
    expect(manual.sections.flatMap(item => item.examples ?? [])).toEqual(expect.arrayContaining(['中三/顺子/5']))
    expect(manual.sections.flatMap(item => item.examples ?? []).join('')).toContain('1/和/5')
    expect(JSON.stringify(manual)).not.toContain('digits5-v2')
    expect(JSON.stringify(manual)).not.toContain('旧注单')
    if (id.startsWith('bingo-ssc-')) {
      expect(manual.sections.map(item => item.body).join('')).toContain('ID185')
      expect(manual.sections.map(item => item.body).join('')).toContain('ID135')
      expect(manual.auditNotes?.join('')).toContain('原始顺序与ID135集合')
    }
  })

  it.each(['platform', 'external', 'official'])('documents the exact SG external cross-check contract independently of %s display metadata', sourceKind => {
    const sg = manualForGame({ ...game('sg-ssc'), rulesReady: true, ruleVersion: 'digits5-v3', sourceKind, sourceName: '未经核验的来源标签' })
    expect(sg.status).toBe('implemented')
    expect(sg.summary).toContain('digits5-v3')
    expect(sg.summary).toContain('163目录ID64提供唯一号码母源')
    expect(sg.summary).toContain('115的sgssc产品只读校验')
    expect(sg.summary).not.toContain('未经核验的来源标签')
    expect(JSON.stringify(sg)).not.toContain('王者开奖')
    const source = sg.sections.find(section => section.title === 'SG外部开奖校对')?.body ?? ''
    expect(source).toContain('163目录ID64（站内名称“168SG时时彩”）是唯一号码母源')
    expect(source).toContain('115的sgssc产品是只读校验源')
    expect(source).toContain('最近连续24期')
    expect(source).toContain('不能替代或补写ID64缺失的号码')
    expect(source).toContain('已保存的可信结果仍可结算，按匹配的注单来源快照幂等处理')
    expect(source).toContain('ID169属于另一套结果系统')
    expect(source).not.toContain('api.api168168.com')
    expect(sg.auditNotes?.join('')).toContain('不能证明上游独立')
  })

  it.each(['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4'])('keeps %s closed without its exact version and explicit readiness', id => {
    for (const ruleVersion of [undefined, '', 'digits5-v2', 'digits5-v4']) {
      const manual = manualForGame({ ...game(id), rulesReady: true, ruleVersion })
      expect(manual).toMatchObject({ status: 'partial', betModes: { chat: false, web: false } })
      expect(manual.statusText).toContain('规则版本待核验')
      expect(manual.summary).not.toContain('本地附加玩法')
    }
    expect(manualForGame({ ...game(id), ruleVersion: 'digits5-v3' }).betModes).toEqual({ chat: false, web: false })
    expect(manualForGame({ ...game(id), ruleVersion: 'digits5-v3', rulesReady: false }).betModes).toEqual({ chat: false, web: false })
  })

  it('keeps each Bingo SSC variant tied to its own ordered five-ball segment', () => {
    const expected = {
      'bingo-ssc-1': '第1–5个',
      'bingo-ssc-2': '第6–10个',
      'bingo-ssc-3': '第11–15个',
      'bingo-ssc-4': '第16–20个',
    }
    for (const [id, segment] of Object.entries(expected)) {
      const manual = manualForGame({ ...game(id), rulesReady: true, ruleVersion: 'digits5-v3' })
      expect(manual).toMatchObject({ status: 'implemented', betModes: { chat: true, web: true } })
      expect(manual.summary).toContain(segment)
      expect(manual.sections.map(item => item.body).join('')).toContain(segment)
      if (id !== 'bingo-ssc-1') {
        expect(JSON.stringify(manual)).toContain('平台扩展')
        expect(JSON.stringify(manual)).toContain('不属于原版')
      }
    }
  })

  it('documents existing official three-digit games instead of falsely pausing them', () => {
    expect(manualForGame(game('official-fc3d', '福彩3D'))).toMatchObject({ status: 'implemented', betModes: { chat: true, web: true } })
  })

  it('documents the complete Bingo Mark Six web contract behind exact mark6-v2 readiness', () => {
    const manual = manualForGame({ ...game('bingo-mark-six', '宾果六合彩'), rulesReady: true, ruleVersion: 'mark6-v2' })
    expect(manual).toMatchObject({ status: 'implemented', statusText: '完整详细网投已接入', betModes: { chat: false, web: true } })
    expect(manual.summary).toContain('前6个为正码，第7个为特码')
    expect(manual.summary).toContain('原版全部详细网投市场')
    const rules = manual.sections.map(item => `${item.title}${item.body}`).join('')
    expect(rules).toContain('ID185')
    expect(rules).toContain('ID135')
    expect(rules).toContain('农历年动态轮换')
    expect(rules).toContain('开49均为和局返本')
    expect(rules).toContain('半特开49则不中奖')
    expect(rules).toContain('一肖、一尾与总肖')
    expect(rules).toContain('2–5个生肖')
    expect(rules).toContain('5–11个不同号码')
    expect(rules).toContain('只扣款一次')
    expect(rules).toContain('两档赔率会同时冻结')
    expect(rules).toContain('内部定价行不能单独下注')
    expect(rules).toContain('单注最低、单注最高、会员单期与全房单期限额')
    expect(manual.auditNotes?.join('')).toContain('2–11合肖')
    expect(manual.auditNotes?.join('')).toContain('正肖')
    expect(manual.auditNotes?.join('')).toContain('5–11不中')
    expect(JSON.stringify(manual)).not.toMatch(/尚未完成|分阶段开放|继续保持不可提交/)
    expect(manual.sections.flatMap(item => item.examples ?? [])).toContain('2026马年：马=01、13、25、37、49；猴=11、23、35、47；猪=08、20、32、44')

    for (const ruleVersion of [undefined, '', 'mark6-v1', 'mark6-v3']) {
      expect(manualForGame({ ...game('bingo-mark-six'), rulesReady: true, ruleVersion }))
        .toMatchObject({ status: 'partial', betModes: { chat: false, web: false } })
    }
    expect(manualForGame({ ...game('bingo-mark-six'), ruleVersion: 'mark6-v2' }).betModes).toEqual({ chat: false, web: false })
  })

  it.each([
    ['hong-kong-mark-six', 'hk-mark6-v1', 'ID18'],
    ['happy8-mark-six', 'happy8-mark6-v1', 'ID141'],
    ['new-macau-mark-six', 'new-macau-mark6-v1', 'ID140'],
    ['old-macau-mark-six', 'old-macau-mark6-v1', 'ID70'],
  ])('publishes the complete web-only contract for %s behind %s', (id, version, sourceID) => {
    const manual = manualForGame({ ...game(id), rulesReady: true, ruleVersion: version })
    expect(manual).toMatchObject({ status: 'implemented', betModes: { chat: false, web: true } })
    expect(manual.summary).toContain(version)
    expect(manual.sections.find(section => section.title === '开奖来源与取号')?.body).toContain(sourceID)
    expect(manual.sections.find(section => section.title === '五行')?.body).toContain('原版固定号码组')
    expect(manual.auditNotes?.join('')).toContain('5–11不中')
    expect(manualForGame({ ...game(id), rulesReady: true, ruleVersion: 'mark6-v2' }).betModes).toEqual({ chat: false, web: false })
    expect(manualForGame({ ...game(id), ruleVersion: version }).betModes).toEqual({ chat: false, web: false })
  })

  it('labels 快乐8六合彩 as the direct 163-derived seven-ball product, not official 福彩快乐8', () => {
    const manual = manualForGame({ ...game('happy8-mark-six'), rulesReady: true, ruleVersion: 'happy8-mark6-v1' })
    const source = manual.sections.find(section => section.title === '开奖来源与取号')?.body ?? ''
    expect(source).toContain('163派生的7球私盘产品')
    expect(source).toContain('并非从官方福彩快乐8的20球结果临时筛选')
  })

  it('keeps all three PC28 special-rule variants visible and still non-bettable', () => {
    const options = gameManualOptions([])
    const play1 = options.find(item => item.id === 'reference-pc28-play-1')!
    const play2 = options.find(item => item.id === 'reference-pc28-play-2')!
    const play3 = options.find(item => item.id === 'reference-pc28-play-3')!
    expect(play1.sections.map(item => item.body).join('')).toContain('不计入有效流水')
    expect(play2.sections.map(item => item.body).join('')).toContain('组合总注严格大于1且开13或14时')
    expect(play2.sections.map(item => item.body).join('')).toContain('庄家通吃')
    expect(play3.sections.map(item => item.body).join('')).toContain('3.65倍')
    expect(play1.statusText).toContain('pc-canada')
    expect(play2.statusText).toContain('canada-28')
    expect(play3.statusText).toContain('canada-20')
    expect([play1, play2, play3].every(item => !item.betModes.chat && !item.betModes.web)).toBe(true)
  })
})
