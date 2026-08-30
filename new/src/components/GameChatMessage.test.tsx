import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { ChatMessage } from '../api/chat'
import { GameChatMessage } from './GameChatMessage'

const base: ChatMessage = {
  id: 1, game_id: 'speed-fly', user_id: 0, nickname: '开奖助手', room_type: 'group', room_scope: 'agent:2',
  mine: false, message_type: 'application', created_at: '2026-08-30T05:10:00Z', content: '',
}

describe('game chat presentation', () => {
  it('renders old success receipts with one compact mention and no odds suffix', () => {
    const content = '@王者玩家\n【极速飞艇 - 54776105】下单成功\n冠军[4/352.00] · 赔率 9.900\n\n使用：352.00\n剩余：10035483.00'
    const html = renderToStaticMarkup(<GameChatMessage message={{ ...base, content }} nickname="王者玩家" />)
    expect(html.match(/class="assistant-mention"/g)).toHaveLength(1)
    expect(html).toContain('冠军[4/352]')
    expect(html).toContain('使用：352')
    expect(html).not.toContain('使用：352.00')
    expect(html).toContain('剩余：10035483.00')
    expect(html).not.toContain('赔率')
    expect(html).not.toContain('我的本期注单')
  })

  it('keeps short outgoing content, timestamp, and the shared bubble wrapper', () => {
    const html = renderToStaticMarkup(<GameChatMessage message={{ ...base, mine: true, user_id: 5, content: '3', message_type: 'text' }} nickname="王者玩家" />)
    expect(html).toContain('player-bet game-chat-message mine')
    expect(html).toContain('<span class="game-chat-content">3</span>')
    expect(html).toContain('13:10</time>')
  })

  it('shows a parsing failure as a normal durable assistant reply', () => {
    const html = renderToStaticMarkup(<GameChatMessage message={{ ...base, content: '@王者玩家\n解析失败，“3”缺少金额，请使用 玩法/金额' }} nickname="王者玩家" />)
    expect(html).toContain('解析失败，“3”缺少金额，请使用 玩法/金额')
    expect(html).not.toContain('下单成功')
  })

  it('never strips ordinary member text or interprets it as HTML', () => {
    const html = renderToStaticMarkup(<GameChatMessage message={{ ...base, user_id: 6, message_type: 'text', content: '<script>bad()</script> · 赔率 9.900' }} nickname="王者玩家" />)
    expect(html).toContain('赔率 9.900')
    expect(html).toContain('&lt;script&gt;')
    expect(html).not.toContain('<script>')
  })
})
