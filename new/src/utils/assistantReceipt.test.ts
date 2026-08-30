import { describe, expect, it } from 'vitest'
import type { AssistantBetLine } from '../api/bets'
import { assistantReceiptLines } from './assistantReceipt'
import { parseBetInput } from './betParser'

const receipt = (command: string): AssistantBetLine[] => parseBetInput(command).payloads.map(item => ({ ...item, play_code: item.play_code ?? '', play_name: item.play_name ?? '', odds: 9.9, label: '' }))

describe('grouped assistant receipts', () => {
  it('formats the reported multi-rank ticket without redundant .00', () => {
    const items = receipt('1/23/50#2/23/50#3/348/50#4/49/50#0/50/50')
    expect(assistantReceiptLines(items)).toEqual(['冠军[2/50 3/50]', '亚军[2/50 3/50]', '第三名[3/50 4/50 8/50]', '第四名[4/50 9/50]', '第十名[5/50 10/50]'])
    expect(items.reduce((total, item) => total + item.amount, 0)).toBe(550)
  })
  it('does not hide real fractional stakes', () => {
    expect(assistantReceiptLines(receipt('1/2/1.25#2/3/0.50'))).toEqual(['冠军[2/1.25]', '亚军[3/0.50]'])
  })
  it('sorts by explicit rank, not hash order, and groups mixed choices without changing line items', () => {
    const items = receipt('2/大小单双12/20#1/大小单双12/20')
    const original = structuredClone(items)
    expect(assistantReceiptLines(items)).toEqual(['冠军[1/20 2/20 单/20 双/20 大/20 小/20]', '亚军[1/20 2/20 单/20 双/20 大/20 小/20]'])
    expect(items).toEqual(original)
    expect(items).toHaveLength(12)
    expect(items.reduce((total, item) => total + item.amount, 0)).toBe(240)
  })
  it('groups two explicit champion bets as champion and keeps rank six distinct from sum', () => {
    expect(assistantReceiptLines(receipt('1/1/80#1/2/80#6/大/5#冠亚/大/10'))).toEqual(['冠军[1/80 2/80]', '第六名[大/5]', '冠亚和[大/10]'])
  })
  it('shows all ten ordinal ranks correctly, including 0 as rank ten and number ten', () => {
    expect(assistantReceiptLines(receipt('1/3/50#2/0/50#3/0/50#4/9/50#5/0/50#6/9/50#7/4/50#8/7/50#9/6/50#0/6/50'))).toEqual(['冠军[3/50]', '亚军[10/50]', '第三名[10/50]', '第四名[9/50]', '第五名[10/50]', '第六名[9/50]', '第七名[4/50]', '第八名[7/50]', '第九名[6/50]', '第十名[6/50]'])
  })
})
