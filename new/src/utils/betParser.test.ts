import { describe, expect, it } from 'vitest'
import { parseBetInput } from './betParser'

describe('parseBetInput racing shorthand', () => {
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
    expect(parsed.lines[0]).toBe('第一名[1/100]')
    expect(parsed.lines[5]).toBe('第六名[大/200]')
    expect(parsed.lines.at(-1)).toBe('第七名[10/100]')
  })

  it('defaults unpositioned number selections to the champion', () => {
    const parsed = parseBetInput('123450/20')

    expect(parsed.payloads).toHaveLength(6)
    expect(parsed.total).toBe(120)
    expect(parsed.payloads.every((payload) => payload.position === 1)).toBe(true)
    expect(parsed.payloads.at(-1)?.selection).toBe('10')
    expect(parsed.lines.at(-1)).toContain('[10/20]')
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
