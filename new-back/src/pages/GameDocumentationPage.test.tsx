import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'

vi.mock('../api', () => ({ adminApi: { games: vi.fn(), playCatalog: vi.fn(), oddsLimits: vi.fn() } }))

import { GameDocumentationPage } from './GameDocumentationPage'

describe('GameDocumentationPage', () => {
  it('presents original, current and difference sources as separate read-only views', () => {
    const html = renderToStaticMarkup(<GameDocumentationPage />)
    expect(html).toContain('游戏说明')
    expect(html).toContain('原版说明')
    expect(html).toContain('当前所有规则')
    expect(html).toContain('与原版的差异')
    expect(html).toContain('本页面只读')
  })
})
