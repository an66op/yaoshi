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
})
