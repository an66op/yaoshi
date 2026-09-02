import { describe, expect, it } from 'vitest'
import type { Game } from '../types'
import { gameManualOptions, manualForGame } from './gamePlayManuals'

const game = (id: string, title = id) => ({ id, title }) as Game

describe('member game manuals stay honest about implementation status', () => {
  it('maps implemented racing syntax and keeps Bingo A source conversion visibly pending', () => {
    expect(manualForGame(game('speed-racing', '极速赛车'))).toMatchObject({ status: 'implemented', gameId: 'speed-racing' })
    const bingo = manualForGame(game('bingo-racing-a', '宾果赛车(A)'))
    expect(bingo).toMatchObject({ status: 'partial', sourceURL: 'https://www.www-163kai.cc/', betModes: { chat: false, web: false } })
    expect(bingo.auditNotes?.join('')).toContain('前10个宾果原号')
    expect(bingo.auditNotes?.join('')).toContain('严格升序')
    expect(manualForGame({ ...game('bingo-racing-a', '宾果赛车(A)'), rulesReady: true, ruleVersion: 'racing-v2' }))
      .toMatchObject({ status: 'implemented', betModes: { chat: true, web: true } })
    expect(manualForGame({ ...game('bingo-racing-a'), rulesReady: true, ruleVersion: 'racing-v1' }).betModes)
      .toEqual({ chat: false, web: false })
  })

  it('publishes each exact PC28 version while keeping an unversioned room fail-closed', () => {
    const mapped = manualForGame({ ...game('canada-28', '加拿大28'), rulesReady: true, ruleVersion: 'pc28-v2', sourceName: '王者开奖' })
    expect(mapped).toMatchObject({ status: 'implemented', gameId: 'canada-28', betModes: { chat: true, web: true } })
    expect(mapped.statusText).toContain('玩法二')
    expect(mapped.summary).toContain('pc28-v2')
    expect(mapped.summary).toContain('王者开奖')
    expect(mapped.sections.map(item => item.body).join('')).toContain('组合总注严格大于1且开13或14时')
    expect(mapped.sections.map(item => item.body).join('')).toContain('庄家通吃')
    expect(mapped.auditNotes?.join('')).toContain('890、901与019')
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

  it.each(['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1'])('publishes the v3 manual only for an exact server-versioned game %s', id => {
    const manual = manualForGame({ ...game(id), rulesReady: true, ruleVersion: 'digits5-v3' })
    expect(manual).toMatchObject({ status: 'implemented', statusText: '三段形态与龙虎和已接入' })
    expect(manual.betModes).toEqual({ chat: true, web: true })
    expect(manual.summary).toContain('前三/中三/后三')
    expect(manual.summary).toContain('总和、总和尾')
    expect(manual.sections.flatMap(item => item.examples ?? [])).toEqual(expect.arrayContaining(['中三/顺子/5']))
    expect(manual.sections.flatMap(item => item.examples ?? []).join('')).toContain('1/和/5')
    expect(JSON.stringify(manual)).not.toContain('digits5-v2')
    expect(JSON.stringify(manual)).not.toContain('旧注单')
    if (id === 'bingo-ssc-1') {
      expect(manual.sections.map(item => item.body).join('')).toContain('前5个原号的个位数')
      expect(manual.auditNotes?.join('')).toContain('原始顺序交叉核对')
    }
  })

  it.each(['platform', 'external', 'official'])('documents SG as complete v3 gameplay and platform-owned draws with %s metadata', sourceKind => {
    const sg = manualForGame({ ...game('sg-ssc'), rulesReady: true, ruleVersion: 'digits5-v3', sourceKind, sourceName: '未经核验的来源标签' })
    expect(sg.status).toBe('implemented')
    expect(sg.summary).toContain('王者平台自开（王者开奖）')
    expect(sg.summary).toContain('不宣称与SG外部开奖同步')
    expect(sg.summary).not.toContain('未经核验的来源标签')
  })

  it.each(['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1'])('keeps %s closed without its exact version and explicit readiness', id => {
    for (const ruleVersion of [undefined, '', 'digits5-v2', 'digits5-v4']) {
      const manual = manualForGame({ ...game(id), rulesReady: true, ruleVersion })
      expect(manual).toMatchObject({ status: 'partial', betModes: { chat: false, web: false } })
      expect(manual.statusText).toContain('规则版本待核验')
      expect(manual.summary).not.toContain('本地附加玩法')
    }
    expect(manualForGame({ ...game(id), ruleVersion: 'digits5-v3' }).betModes).toEqual({ chat: false, web: false })
    expect(manualForGame({ ...game(id), ruleVersion: 'digits5-v3', rulesReady: false }).betModes).toEqual({ chat: false, web: false })
  })

  it.each(['bingo-racing-b', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4'])('keeps unverified %s display-only even with borrowed rule metadata', id => {
    for (const ruleVersion of [undefined, '', 'racing-v2', 'digits5-v2', 'digits5-v3']) {
      const manual = manualForGame({ ...game(id), rulesReady: true, ruleVersion })
      expect(manual).toMatchObject({ status: 'reference', betModes: { chat: false, web: false } })
      expect(manual.statusText).toBe('玩法待核验 · 仅展示')
      expect(manual.summary).toContain('不接受投注')
    }
  })

  it('documents existing official three-digit games instead of falsely pausing them', () => {
    expect(manualForGame(game('official-fc3d', '福彩3D'))).toMatchObject({ status: 'implemented', betModes: { chat: true, web: true } })
  })

  it('documents Bingo Mark Six as web-only partial implementation instead of a pending placeholder', () => {
    const manual = manualForGame(game('bingo-mark-six', '宾果六合彩'))
    expect(manual).toMatchObject({ status: 'partial', betModes: { chat: false, web: true } })
    expect(manual.statusText).not.toContain('待配置')
    expect(manual.summary).toContain('前6个为正码，第7个为特码')
    const rules = manual.sections.map(item => `${item.title}${item.body}`).join('')
    expect(rules).toContain('农历年动态轮换')
    expect(rules).toContain('开49均为和局返本')
    expect(rules).toContain('半特开49则不中奖')
    expect(rules).toContain('三中二有“中二”与“中三”两档赔率')
    expect(manual.sections.flatMap(item => item.examples ?? [])).toContain('2026马年：马=01、13、25、37、49；猴=11、23、35、47；猪=08、20、32、44')
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
