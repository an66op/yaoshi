import { describe, expect, it } from 'vitest'
import type { AssistantBetLine } from '../api/bets'
import { assistantReceiptLines } from './assistantReceipt'
import { parseBetInput } from './betParser'

const receipt = (command: string, gameId?: string, ruleVersion = ''): AssistantBetLine[] => parseBetInput(command, gameId, ruleVersion).payloads.map(item => ({ ...item, play_code: item.play_code ?? '', play_name: item.play_name ?? '', odds: 9.9, label: '' }))

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

  it.each(['speed-racing', 'speed-fly', 'sg-fly', 'fly-racing', 'au-lucky-10', 'bingo-racing-a', 'bingo-racing-b'])('preserves existing grouped racing receipts when %s is supplied explicitly', gameId => {
    const ruleVersion = gameId.startsWith('bingo-racing-') ? 'racing-v2' : ''
    const items = receipt('1/0/20#1/大/20#6/小/20#冠亚/大/20', gameId, ruleVersion)
    expect(assistantReceiptLines(items, gameId, ruleVersion)).toEqual(['冠军[10/20 大/20]', '第六名[小/20]', '冠亚和[大/20]'])
    expect(assistantReceiptLines(items, gameId, ruleVersion)).toEqual(assistantReceiptLines(items))
  })

  it.each(['official-fc3d', 'official-pl3'])('separates ball, sum, sum-tail and first-three shapes for %s', gameId => {
    const items = receipt('1/0/20#总和/大/20#总和尾/7/20#前三/豹子/20', gameId)
    const original = structuredClone(items)
    expect(assistantReceiptLines(items, gameId)).toEqual(['第1球[0/20]', '总和[大/20]', '总和尾[7/20]', '前三形态[豹子/20]'])
    expect(items).toEqual(original)
    expect(items.reduce((sum, item) => sum + item.amount, 0)).toBe(80)
  })

  it('groups only matching digit categories and preserves real fractional amounts and zero tails', () => {
    const gameId = 'official-fc3d'
    const items = receipt('前三/杂六/0.50#3/0/1.25#2/9/20#总和尾/0/1.25#总和/双/10#2/单/20#前三/豹子/20#总和尾/7/0.50', gameId)
    expect(assistantReceiptLines(items, gameId)).toEqual(['第2球[9/20 单/20]', '第3球[0/1.25]', '总和[双/10]', '总和尾[0/1.25 7/0.50]', '前三形态[豹子/20 杂六/0.50]'])
  })

  it.each(['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4'])('groups all exact digits5-v3 segments and tie independently for %s', gameId => {
    const ruleVersion = 'digits5-v3'
    const items = receipt('前三/豹子/5#中三/顺子/5#后三/对子/5#1/龙/5#1/和/5', gameId, ruleVersion)
    expect(assistantReceiptLines(items, gameId, ruleVersion)).toEqual([
      '前三形态[豹子/5]',
      '中三形态[顺子/5]',
      '后三形态[对子/5]',
      '第一球 vs 第五球[龙/5 和/5]',
    ])
  })

  it('renders canonical backend shape selections as the documented Chinese labels', () => {
    const lines: AssistantBetLine[] = [
      { position: 1, selection: 'leopard', amount: 5, play_code: 'leopard', play_name: '前三豹子', label: '前三形态[豹子/5]', odds: 50 },
      { position: 2, selection: 'straight', amount: 5, play_code: 'straight', play_name: '中三顺子', label: '中三形态[顺子/5]', odds: 15 },
      { position: 3, selection: 'pair', amount: 5, play_code: 'pair', play_name: '后三对子', label: '后三形态[对子/5]', odds: 8 },
    ]
    expect(assistantReceiptLines(lines, 'speed-ssc', 'digits5-v3')).toEqual([
      '前三形态[豹子/5]', '中三形态[顺子/5]', '后三形态[对子/5]',
    ])
  })

  it.each(['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4'])('preserves server labels without inferring a contract for unversioned %s receipts', gameId => {
    const items: AssistantBetLine[] = [
      { position: 1, selection: 'leopard', amount: 5, play_code: 'leopard', play_name: '服务端形态说明', label: '服务端原始回执', odds: 50 },
      { position: 1, selection: '龙', amount: 5, play_code: 'dragon_tiger', play_name: '服务端比较说明', label: '', odds: 2 },
    ]
    for (const ruleVersion of [undefined, '', 'digits5-v2', 'digits5-v4']) {
      expect(assistantReceiptLines(items, gameId, ruleVersion)).toEqual(['服务端原始回执', '服务端比较说明[龙/5]'])
    }
  })

  it('does not label invalid three-ball positions as supported ball bets', () => {
    const item = { ...receipt('5/0/20', 'official-fc3d')[0], label: '原始第五球记录' }
    expect(assistantReceiptLines([item], 'official-fc3d')).toEqual(['原始第五球记录'])
  })

  it.each(['pc-canada', 'canada-28', 'canada-20', 'bingo-mark-six', 'official-tw-bingo', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4'])('uses original labels instead of inventing rules for unversioned %s', gameId => {
    const items: AssistantBetLine[] = [
      { position: 6, selection: '7', amount: 20, play_code: 'sum', play_name: '原始和值玩法', label: '平台原始记录：7/20', odds: 2 },
      { position: 1, selection: '0', amount: 1.25, play_code: 'ball_1_5', play_name: '', label: '', odds: 2 },
      { position: 1, selection: '豹子', amount: 20, play_code: 'leopard', play_name: '原始形态', label: '', odds: 2 },
    ]
    expect(assistantReceiptLines(items, gameId)).toEqual(['平台原始记录：7/20', '号码[0/1.25]', '原始形态[豹子/20]'])
  })
})
