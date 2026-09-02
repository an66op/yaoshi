import { describe, expect, it } from 'vitest'
import { betCommandError } from './betCommand'
import { parseBetInput } from './betParser'

describe('atomic bet command validation', () => {
  it('uses the same 400-codepoint boundary as the server, without splitting', () => {
    expect(betCommandError('大'.repeat(400))).toBe('')
    expect(betCommandError('大'.repeat(401))).toContain('400')
    expect(betCommandError(`买 ${'大'.repeat(400)}`)).toBe('')
    expect(parseBetInput('1/1/20#'.repeat(100)).payloads).toEqual([])
  })

  it.each(['1.005', '0.009', '1.999', '1e2', 'NaN', '-1', '1.'])('rejects invalid precision/syntax %s without keeping an earlier valid segment', amount => {
    expect(betCommandError(`1/1/20#2/2/${amount}`)).not.toBe('')
    expect(parseBetInput(`1/1/20#2/2/${amount}`)).toMatchObject({ payloads: [], total: 0 })
  })

  it('preserves cent-exact and integer amounts', () => {
    expect(parseBetInput('1/1/1.25#2/2/20')).toMatchObject({ total: 21.25 })
    expect(betCommandError('1/1/梭哈')).toBe('')
  })

  it('requires all-in to be one unambiguous standalone command', () => {
    expect(betCommandError('1/1/20#2/2/梭哈')).toContain('单独提交')
    expect(betCommandError('1/1/梭哈#2/2/梭哈')).toContain('一次')
    expect(betCommandError('123梭哈')).toContain('标准格式')
    expect(betCommandError('大单梭哈')).toBe('')
  })

  it('validates digit-board commands without changing zero into ten', () => {
    expect(parseBetInput('1/0/20#总和/大/20#总和尾/7/20#前三/豹子/20', 'official-fc3d', 'digits3-v2').payloads).toEqual([
      expect.objectContaining({ position: 1, selection: '0', play_code: 'ball_1_5', amount: 20 }),
      expect.objectContaining({ position: 6, selection: '大', play_code: 'sum', amount: 20 }),
      expect.objectContaining({ position: 6, selection: '7', play_code: 'sum', amount: 20 }),
      expect.objectContaining({ position: 1, selection: '豹子', play_code: 'leopard', amount: 20 }),
    ])
  })
})
