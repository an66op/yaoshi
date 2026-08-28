export type BetPayload = {
  position: number
  selection: string
  amount: number
  play_code?: string
  play_name?: string
}

export type ParsedBet = { content: string; lines: string[]; total: number; payloads: BetPayload[] }

const positionNames = ['一', '二', '三', '四', '五', '六', '七', '八', '九', '十']
const rankMap: Record<string, number> = {
  冠军: 1, 亚军: 2, 第三名: 3, 第四名: 4, 第五名: 5,
  第六名: 6, 第七名: 7, 第八名: 8, 第九名: 9, 第十名: 10,
}

const dualLabels: Record<string, { position: number; selection: string }> = {
  冠军大: { position: 1, selection: '大' }, 冠军小: { position: 1, selection: '小' },
  冠军单: { position: 1, selection: '单' }, 冠军双: { position: 1, selection: '双' },
  冠军龙: { position: 1, selection: '龙' }, 冠军虎: { position: 1, selection: '虎' },
  亚军大: { position: 2, selection: '大' }, 亚军小: { position: 2, selection: '小' },
  亚军单: { position: 2, selection: '单' }, 亚军双: { position: 2, selection: '双' },
  冠亚和大: { position: 6, selection: '大' }, 冠亚和小: { position: 6, selection: '小' },
  冠亚和单: { position: 6, selection: '单' }, 冠亚和双: { position: 6, selection: '双' },
}

function racingPositions(value: string): number[] | null {
  if (value === '10' || value === '0') return [10]
  if (!/^\d+$/.test(value)) return null
  return [...value].map((digit) => digit === '0' ? 10 : Number(digit))
}

function positionedSelections(positions: number[], selections: string, amount: number): BetPayload[] {
  return positions.flatMap((position) => [...selections].map((selection) => {
    const play_code = /^\d$/.test(selection)
      ? 'ball_1_5'
      : ['大', '小', '单', '双'].includes(selection)
        ? 'two_sided'
        : 'dragon_tiger'
    const play_name = play_code === 'ball_1_5'
      ? `第${position}名号码`
      : play_code === 'two_sided'
        ? `第${position}名两面`
        : '龙虎'
    // 快捷输入沿用 0 表示号码 10；发给接口时统一为真实开奖号码 10。
    const canonicalSelection = play_code === 'ball_1_5' && selection === '0' ? '10' : selection
    return { position, selection: canonicalSelection, amount, play_code, play_name }
  }))
}

function segmentPayload(play: string, amount: number): BetPayload[] {
  const compactPlay = play.replace(/^冠亚(?:和)?\//, '冠亚和').replace(/^冠亚(?=[大小单双\d])/, '冠亚和')
  if (dualLabels[compactPlay]) {
    const item = dualLabels[compactPlay]
    return [{ position: item.position, selection: item.selection, amount, play_code: compactPlay.startsWith('冠亚和') ? 'sum' : undefined, play_name: compactPlay }]
  }
  const crownSum = compactPlay.match(/^冠亚和(1[0-9]|[3-9])$/)
  if (crownSum) {
    return [{ position: 6, selection: crownSum[1], amount, play_code: 'sum', play_name: '冠亚和' }]
  }
  for (const [rank, pos] of Object.entries(rankMap)) {
    if (play.startsWith(rank)) {
      const selection = play.slice(rank.length).replace(/^\//, '')
      if (selection) return [{ position: pos, selection, amount, play_code: /^\d+$/.test(selection) ? 'ball_1_5' : ['大', '小', '单', '双'].includes(selection) ? 'two_sided' : 'dragon_tiger', play_name: play }]
    }
  }
  const ranked = play.match(/^(\d+)([大小单双龙虎])$/)
  if (ranked) {
    return [{ position: Number(ranked[1]), selection: ranked[2], amount, play_code: ['大', '小', '单', '双'].includes(ranked[2]) ? 'two_sided' : 'dragon_tiger', play_name: `第${ranked[1]}名${ranked[2]}` }]
  }
  const positionedSide = play.match(/^(10|[1-9])\/([大小单双龙虎])$/)
  if (positionedSide) {
    return [{ position: Number(positionedSide[1]), selection: positionedSide[2], amount, play_code: ['大', '小', '单', '双'].includes(positionedSide[2]) ? 'two_sided' : 'dragon_tiger', play_name: `第${positionedSide[1]}名${positionedSide[2]}` }]
  }
  const positioned = play.match(/^([0-9]+)\/([0-9大小单双龙虎]+)$/)
  if (positioned) {
    const positions = racingPositions(positioned[1])
    if (positions) return positionedSelections(positions, positioned[2], amount)
  }
  if (/^\d+$/.test(play)) {
    // 与后端开奖助手保持一致：省略名次时默认冠军，末尾金额是
    // 每一个号码的金额，而不是整组号码平分的总金额。赛车中 0 代表 10。
    return positionedSelections([1], play, amount)
  }
  return [{ position: 1, selection: play, amount, play_name: play }]
}

function describePayload(payload: BetPayload): string {
  const position = positionNames[payload.position - 1] ?? String(payload.position)
  const selection = payload.play_code === 'ball_1_5' && payload.selection === '0' ? '10' : payload.selection
  return `第${position}名[${selection}/${payload.amount}]`
}

function inferPlayCode(payload: BetPayload): string {
	if (payload.play_code) return payload.play_code
  if (payload.selection === '龙' || payload.selection === '虎') return 'dragon_tiger'
  if (/^\d$/.test(payload.selection)) return 'ball_1_5'
  if (['大', '小', '单', '双'].includes(payload.selection)) return 'two_sided'
  return 'ball_1_5'
}

/** 将快捷输入文本解析为后端 PlaceBet 参数列表 */
export function parseBetInput(content: string): ParsedBet {
  const lines: string[] = []
  const payloads: BetPayload[] = []
  let total = 0
  const text = content.replace(/^买/, '').trim()

  for (const segment of text.split('#').map((item) => item.trim()).filter(Boolean)) {
    const parts = segment.split('/').map((item) => item.trim()).filter(Boolean)
    if (parts.length < 2) continue
    const amountText = parts.at(-1) ?? ''
    if (!/^\d+(?:\.\d+)?$/.test(amountText)) continue
    const amount = Number(amountText)
    if (amount <= 0) continue
    const play = parts.slice(0, -1).join('/')
    const segmentPayloads = segmentPayload(play, amount)
    for (const payload of segmentPayloads) {
      const play_code = inferPlayCode(payload)
      payloads.push({ ...payload, play_code })
      lines.push(describePayload(payload))
      total += payload.amount
    }
  }

  return {
    content,
    lines: lines.length ? lines : [`号码[${content}]`],
    total,
    payloads,
  }
}
