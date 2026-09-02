/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { BottomNav } from './BottomNav'
import { GameChatMessage } from './GameChatMessage'
import type { ChatMessage } from '../api/chat'

describe('control surface scope', () => {
  it('protects navigation labels/icons without losing normal click targets', () => {
    const onSelect = vi.fn()
    const nav = BottomNav({ activeTab: 'lobby', theme: 'day', unreadCount: 0, onSelect })
    const html = renderToStaticMarkup(nav)
    expect(html).toContain('class="bottom-nav" data-control-surface=""')
    expect(html.match(/draggable="false"/g)).toHaveLength(4)
    nav.props.children[1].props.onClick()
    expect(onSelect).toHaveBeenCalledWith('shop')
  })

  it.each([true, false])('does not protect normal chat or assistant receipt text (mine=%s)', mine => {
    const message = { id: 1, mine, nickname: '会员', user_id: 0, game_id: 'speed-racing', message_type: mine ? 'text' : 'application', content: mine ? '1/2/50' : '【极速赛车 - 34137294】下单成功\n冠军[2/50]', created_at: '2026-08-31T01:00:00Z' } as ChatMessage
    const html = renderToStaticMarkup(<GameChatMessage message={message} nickname="会员" />)
    expect(html).not.toContain('data-control-surface')
    expect(html).toContain(mine ? '1/2/50' : '冠军[2/50]')
  })

  it('scopes CSS to marked panels, restores editing, and does not change scroll/touch behavior', () => {
    const css = readFileSync(new URL('../control-surface.css', import.meta.url), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '')
    const rules = [...css.matchAll(/([^{}]+)\{([^{}]+)\}/g)]
    expect(rules).toHaveLength(3)
    for (const [, selectors] of rules) {
      // Each top-level selector is on its own line; commas inside :is/:not
      // belong to the scoped selector, not to additional global selectors.
      for (const selector of selectors.trim().split('\n')) expect(selector.trim()).toMatch(/^\[data-control-surface\]/)
    }
    expect(rules[0][2]).toContain('-webkit-user-select: none')
    expect(rules[0][2]).toContain('-webkit-touch-callout: none')
    expect(rules[2][1]).toContain('input, textarea, select')
    expect(rules[2][2]).toContain('user-select: text')
    expect(rules[2][2]).toContain('-webkit-touch-callout: default')
    expect(css).not.toMatch(/touch-action|pointer-events|overflow/)
  })
})
