import { describe, expect, it } from 'vitest'
import { parseBetInput } from './betParser'

describe('parseBetInput racing shorthand', () => {
  it('labels the exact ten-rank instruction from champion through tenth, charging each item 20', () => {
    const content = '1/1/20#2/4/20#3/3/20#4/7/20#5/9/20#6/5/20#7/5/20#8/5/20#9/2/20#0/8/20'
    const parsed = parseBetInput(content)
    expect(parsed.payloads.map(row => row.position)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10])
    expect(parsed.lines).toEqual(['冠军[1/20]', '亚军[4/20]', '第三名[3/20]', '第四名[7/20]', '第五名[9/20]', '第六名[5/20]', '第七名[5/20]', '第八名[5/20]', '第九名[2/20]', '第十名[8/20]'])
    expect(parsed.total).toBe(200)
    expect(parsed.content).toBe(content)
    expect(parsed.payloads[0].play_name).toBe('冠军号码')
    expect(parsed.payloads[1].play_name).toBe('亚军号码')
  })

  it('keeps two explicit first-place selections as champion rather than inferring sequential ranks', () => {
    const parsed = parseBetInput('1/1/80#1/2/80')
    expect(parsed.lines).toEqual(['冠军[1/80]', '冠军[2/80]'])
    expect(parsed.total).toBe(160)
  })

  it.each([
    ['489/0178/48', 12, 576, [4, 8, 9]],
    ['5/045/343', 3, 1029, [5]],
    ['68/单大/811', 4, 3244, [6, 8]],
    ['62437/546', 5, 2730, [1]],
    ['12345/100', 5, 500, [1]],
    ['买12345/1000', 5, 5000, [1]],
    ['冠军/12345/100', 5, 500, [1]],
    ['冠军12345/100', 5, 500, [1]],
    ['第七名/8/200', 1, 200, [7]],
    ['1/123/0.10', 3, 0.3, [1]],
  ])('matches the documented receipt for %s', (content, count, total, positions) => {
    const parsed = parseBetInput(content as string)
    expect(parsed.payloads).toHaveLength(count as number)
    expect(parsed.total).toBe(total)
    expect([...new Set(parsed.payloads.map(row => row.position))]).toEqual(positions)
  })
  it('treats the amount as the amount of every selected number', () => {
    const parsed = parseBetInput('1/12345/100#6/大/200#7/67890/100')

    expect(parsed.payloads).toHaveLength(11)
    expect(parsed.total).toBe(1200)
    expect(parsed.payloads.filter((payload) => payload.position === 1)).toEqual([
      expect.objectContaining({ selection: '1', amount: 100, play_code: 'ball_1_5' }),
      expect.objectContaining({ selection: '2', amount: 100, play_code: 'ball_1_5' }),
      expect.objectContaining({ selection: '3', amount: 100, play_code: 'ball_1_5' }),
      expect.objectContaining({ selection: '4', amount: 100, play_code: 'ball_1_5' }),
      expect.objectContaining({ selection: '5', amount: 100, play_code: 'ball_1_5' }),
    ])
    expect(parsed.payloads.filter((payload) => payload.position === 6)).toEqual([
      expect.objectContaining({ selection: '大', amount: 200, play_code: 'two_sided' }),
    ])
    expect(parsed.payloads.filter((payload) => payload.position === 7)).toEqual([
      expect.objectContaining({ selection: '6', amount: 100, play_code: 'ball_1_5' }),
      expect.objectContaining({ selection: '7', amount: 100, play_code: 'ball_1_5' }),
      expect.objectContaining({ selection: '8', amount: 100, play_code: 'ball_1_5' }),
      expect.objectContaining({ selection: '9', amount: 100, play_code: 'ball_1_5' }),
      expect.objectContaining({ selection: '10', amount: 100, play_code: 'ball_1_5' }),
    ])
    expect(parsed.lines).toHaveLength(11)
    expect(parsed.lines[0]).toBe('冠军[1/100]')
    expect(parsed.lines[5]).toBe('第六名[大/200]')
    expect(parsed.lines.at(-1)).toBe('第七名[10/100]')
  })

  it('defaults unpositioned number selections to the champion', () => {
    const parsed = parseBetInput('123450/20')

    expect(parsed.payloads).toHaveLength(6)
    expect(parsed.total).toBe(120)
    expect(parsed.payloads.every((payload) => payload.position === 1)).toBe(true)
    expect(parsed.payloads.at(-1)?.selection).toBe('10')
    expect(parsed.lines.at(-1)).toBe('冠军[10/20]')
  })

  it('supports crossing multiple positions and selections', () => {
    const parsed = parseBetInput('34/大虎/236#489/0178/48')

    expect(parsed.payloads).toHaveLength(16)
    expect(parsed.total).toBe(4 * 236 + 12 * 48)
    expect(parsed.payloads[0]).toMatchObject({ position: 3, selection: '大', play_code: 'two_sided' })
    expect(parsed.payloads[15]).toMatchObject({ position: 9, selection: '8', play_code: 'ball_1_5' })
  })

  it('supports exact crown sums and all four crown sum sides', () => {
    const parsed = parseBetInput('冠亚/14/9#冠亚和单/20')

    expect(parsed.total).toBe(29)
    expect(parsed.payloads).toEqual([
      expect.objectContaining({ position: 6, selection: '14', amount: 9, play_code: 'sum' }),
      expect.objectContaining({ position: 6, selection: '单', amount: 20, play_code: 'sum' }),
    ])
  })

  it('supports the documented racing aliases, compact amount and named position groups', () => {
    expect(parseBetInput('123大/5').payloads.map(item => [item.position, item.selection])).toEqual([[1, '1'], [1, '2'], [1, '3'], [1, '大']])
    expect(parseBetInput('1大5').lines).toEqual(['冠军[大/5]'])
    expect(parseBetInput('和/大/5#和/345/5#和345/5').payloads.map(item => item.selection)).toEqual(['大', '3', '4', '5', '3', '4', '5'])
    expect(parseBetInput('10大5').payloads.map(item => item.position)).toEqual([1, 10])
    expect(parseBetInput('前三/2/5#后三/2/5#前五/2/5#后五/2/5').payloads.map(item => item.position)).toEqual([
      1, 2, 3, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
    ])
    expect(parseBetInput('1235').payloads).toEqual([])
  })

  it('applies unpositioned five-digit selections to all five balls', () => {
    const side = parseBetInput('大/20', 'speed-ssc')
    expect(side.payloads.map(item => item.position)).toEqual([1, 2, 3, 4, 5])
    expect(side.total).toBe(100)
    const numbers = parseBetInput('12/5', 'au-lucky-5')
    expect(numbers.payloads).toHaveLength(10)
    expect(numbers.total).toBe(50)
    expect(parseBetInput('1大5', 'speed-ssc').payloads).toEqual([
      expect.objectContaining({ position: 1, selection: '大', amount: 5 }),
    ])
  })
})

describe('per-game parsed receipt descriptions', () => {
  it.each(['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4', 'official-fc3d', 'official-pl3'])('keeps zero and names the correct digit groups for %s', gameId => {
    const parsed = parseBetInput('1/0/20#总和/大/20#总和尾/7/20#前三/豹子/20', gameId)
    expect(parsed.lines).toEqual(['第1球[0/20]', '总和[大/20]', '总和尾[7/20]', '前三形态[豹子/20]'])
    expect(parsed.payloads.map(item => item.selection)).toEqual(['0', '大', '7', '豹子'])
    expect(parsed.payloads.map(item => item.play_code)).toEqual(['ball_1_5', 'sum', 'sum', 'leopard'])
    expect(parsed.total).toBe(80)
  })

  it('keeps individual shapes outside ball-position descriptions and retains cents', () => {
    const parsed = parseBetInput('3/9/1.25#总和尾/0/0.50#前三/顺子/20#前三/对子/20#前三/半顺/20#前三/杂六/20', 'official-fc3d')
    expect(parsed.lines).toEqual(['第3球[9/1.25]', '总和尾[0/0.50]', '前三形态[顺子/20]', '前三形态[对子/20]', '前三形态[半顺/20]', '前三形态[杂六/20]'])
    expect(parsed.total).toBe(81.75)
  })

  it.each(['pc-canada', 'canada-28'])('leaves versioned PC28 chat parsing to the authoritative server for %s', gameId => {
    const parsed = parseBetInput('1/0/20#总和/大/20#前三/豹子/20', gameId)
    expect(parsed).toMatchObject({ lines: [], payloads: [], total: 0 })
  })

  it('never invents wager rules for an unknown game', () => {
    const gameId = 'official-tw-bingo'
    const parsed = parseBetInput('1/0/20#总和/大/20#前三/豹子/20', gameId)
    expect(parsed).toMatchObject({ lines: [], payloads: [], total: 0 })
  })

  it('retains the legacy default and explicit racing descriptions', () => {
    const command = '1/0/20#冠亚/14/20'
    expect(parseBetInput(command, 'speed-racing').lines).toEqual(['冠军[10/20]', '冠亚和[14/20]'])
    expect(parseBetInput(command, 'speed-racing').lines).toEqual(parseBetInput(command).lines)
  })

  it.each(['speed-ssc', 'au-lucky-5', 'bingo-ssc-1'])('expands exact digits5-v3 shapes and tie for %s', gameId => {
    const ruleVersion = 'digits5-v3'
    expect(parseBetInput('中三顺子/5', gameId, ruleVersion)).toMatchObject({
      lines: ['中三形态[顺子/5]'],
      payloads: [expect.objectContaining({ position: 2, selection: '顺子', play_code: 'straight' })],
    })
    expect(parseBetInput('前三豹子5', gameId, ruleVersion).payloads).toEqual([
      expect.objectContaining({ position: 1, selection: '豹子', amount: 5, play_code: 'leopard' }),
    ])
    expect(parseBetInput('后三/对子/5', gameId, ruleVersion).lines).toEqual(['后三形态[对子/5]'])
    expect(parseBetInput('豹子5', gameId, ruleVersion)).toMatchObject({
      total: 15,
      lines: ['前三形态[豹子/5]', '中三形态[豹子/5]', '后三形态[豹子/5]'],
      payloads: [
        expect.objectContaining({ position: 1, play_code: 'leopard' }),
        expect.objectContaining({ position: 2, play_code: 'leopard' }),
        expect.objectContaining({ position: 3, play_code: 'leopard' }),
      ],
    })
    expect(parseBetInput('1/和/5', gameId, ruleVersion)).toMatchObject({
      total: 5,
      lines: ['第一球 vs 第五球[和/5]'],
      payloads: [expect.objectContaining({ position: 1, selection: '和', play_code: 'dragon_tiger_tie' })],
    })
    expect(parseBetInput('2/龙/5', gameId, ruleVersion).payloads).toEqual([])
    expect(parseBetInput('总和/大/5', gameId, ruleVersion).payloads).toEqual([])
    expect(parseBetInput('总和尾7/5', gameId, ruleVersion).payloads).toEqual([])
  })

  it('keeps SG and unversioned matching games on the legacy five-ball parser', () => {
    for (const [gameId, version] of [['speed-ssc', ''], ['speed-ssc', 'digits5-v2'], ['sg-ssc', 'digits5-v3'], ['bingo-ssc-2', 'digits5-v3'], ['bingo-ssc-3', 'digits5-v3'], ['bingo-ssc-4', 'digits5-v3']] as const) {
      expect(parseBetInput('中三顺子/5', gameId, version).payloads).toEqual([])
      expect(parseBetInput('1/和/5', gameId, version).payloads).toEqual([])
      expect(parseBetInput('豹子5', gameId, version).payloads).toHaveLength(1)
    }
  })
})

describe('server-aligned game syntax and atomic rejection', () => {
  it.each(['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4', 'official-fc3d', 'official-pl3'])('recognizes compact totals, tails, shapes and explicit ball labels for %s', gameId => {
    const parsed = parseBetInput('总和单/100#总和尾7/100#总和/大小单双/20#前三豹子/50#第1球/09/20', gameId)
    expect(parsed.total).toBe(370)
    expect(parsed.payloads.map(({ position, selection, amount, play_code }) => ({ position, selection, amount, play_code }))).toEqual([
      { position: 6, selection: '单', amount: 100, play_code: 'sum' },
      { position: 6, selection: '7', amount: 100, play_code: 'sum' },
      ...['大', '小', '单', '双'].map(selection => ({ position: 6, selection, amount: 20, play_code: 'sum' })),
      { position: 1, selection: '豹子', amount: 50, play_code: 'leopard' },
      { position: 1, selection: '0', amount: 20, play_code: 'ball_1_5' },
      { position: 1, selection: '9', amount: 20, play_code: 'ball_1_5' },
    ])
    expect(parsed.lines).toContain('第1球[0/20]')
    expect(parsed.lines).toContain('总和尾[7/100]')
  })

  it('keeps compact single positions distinct from slash-separated multi-position plays', () => {
    const parsed = parseBetInput('0大/20#10小/20#34/大/20#冠亚/大小单双/20')
    expect(parsed.total).toBe(180)
    expect(parsed.payloads.map(item => item.position)).toEqual([10, 1, 10, 3, 4, 6, 6, 6, 6])
    expect(parsed.lines[0]).toBe('第十名[大/20]')
    expect(parseBetInput('34大/20').payloads.map(item => item.selection)).toEqual(['3', '4', '大'])
  })

  it('charges every repeated item and preserves the zero semantics of each family', () => {
    expect(parseBetInput(' 买 11/00/0.10#1/0/0.10 ', 'speed-racing')).toMatchObject({ total: 0.5 })
    expect(parseBetInput('11/00/0.10', 'speed-ssc').payloads).toEqual(Array.from({ length: 4 }, () => expect.objectContaining({ position: 1, selection: '0', amount: 0.1 })))
    expect(parseBetInput('总和7/20', 'speed-ssc').lines).toEqual(['总和尾[7/20]'])
    expect(parseBetInput('冠亚和/014/20').lines).toEqual(['冠亚和[14/20]'])
    expect(parseBetInput('第2球09/20', 'speed-ssc').lines).toEqual(['第2球[0/20]', '第2球[9/20]'])
    expect(parseBetInput('总和 单/20#前三 LEOPARD/20', 'speed-ssc').lines).toEqual(['总和[单/20]', '前三形态[豹子/20]'])
  })

  it.each([
    ['speed-racing', '6/龙/20'], ['speed-racing', '0龙/20'],
    ['speed-racing', '总和/大/20'], ['speed-racing', '前三/豹子/20'], ['speed-racing', '冠亚/2/20'],
    ['speed-racing', '冠亚/34/20'], ['speed-racing', '冠亚/大3/20'], ['speed-racing', '第1球/2/20'],
    ['speed-ssc', '冠军/2/20'], ['speed-ssc', '0/2/20'], ['speed-ssc', '6/2/20'],
    ['speed-ssc', '3/龙/20'], ['speed-ssc', '总和/27/20'], ['speed-ssc', '总和尾/大/20'],
    ['speed-ssc', '中三豹子/20'], ['speed-ssc', '冠亚/大/20'], ['official-fc3d', '4/9/20'],
    ['official-fc3d', '第4球9/20'], ['official-fc3d', '2/虎/20'], ['speed-racing', '1//20'],
    ['speed-racing', '1/2//20'], ['speed-racing', '/2/20'], ['speed-racing', '1/2/3/20'],
    ['speed-racing', '不存在的玩法/20'], ['speed-racing', '1/和/20'],
  ])('rejects the entire command for unsupported %s syntax %s', (gameId, invalid) => {
    const content = `1/1/20#${invalid}`
    expect(parseBetInput(content, gameId)).toEqual({ content, lines: [], total: 0, payloads: [] })
  })

  it('allows only the backend-supported mirror pairs and never adds a draw bet', () => {
    expect(parseBetInput('1/龙虎/20#2/虎/20', 'speed-ssc')).toMatchObject({ total: 60 })
    expect(parseBetInput('1/龙虎/20', 'official-fc3d').payloads.map(item => item.play_code)).toEqual(['dragon_tiger', 'dragon_tiger'])
    expect(parseBetInput('1/和/20', 'speed-ssc').payloads).toEqual([])
  })
})
