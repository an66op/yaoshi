import { renderToStaticMarkup } from 'react-dom/server'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { Game } from '../types'
import type { DrawResult } from '../api/lottery'
import { ScratchDrawDialog } from './ScratchDrawDialog'

vi.mock('./Dialogs', () => ({
  ActionDialog: ({ title, description, children }: { title: string; description: string; children?: ReactNode }) => <section aria-label={title}><p>{description}</p>{children}</section>,
}))

const game = { id: 'speed-fly', title: '极速飞艇', period: '54776212', latestIssue: '54776211', balls: [7, 4, 3, 8, 1, 5, 2, 6, 9, 10] } as Game
const draw: DrawResult = { id: 1, game_id: game.id, issue: '54776211', numbers: [...game.balls], draw_at: '2026-08-30T08:00:00Z' }

describe('scratch draw dialog content', () => {
  it('identifies the real drawn issue, hides covered numbers from assistive technology and offers keyboard reveal', () => {
    const html = renderToStaticMarkup(<ScratchDrawDialog game={game} draw={draw} onClose={() => {}} />)
    expect(html).toContain('涂抹开奖')
    expect(html).toContain('class="scratch-draw-board" data-control-surface=""')
    expect(html).toContain('第 54776211 期')
    expect(html).not.toContain('54776212')
    expect(html).toContain('class="scratch-draw-grid" style="--scratch-columns:10" aria-hidden="true"')
    expect(html).toContain('按住涂抹，刮开查看结果')
    expect(html).toContain('<button type="button">全部揭晓</button>')
    expect(html).not.toContain('scratch-draw-total')
    expect(html).not.toContain('正在揭晓')
  })

  it.each([3, 5, 7, 10, 20])('keeps all %i numbers in one row', (count) => {
    const html = renderToStaticMarkup(<ScratchDrawDialog game={game} draw={{ ...draw, numbers: Array.from({ length: count }, (_, index) => index + 1) }} onClose={() => {}} />)
    expect(html.match(/class="scratch-draw-cell"/g)).toHaveLength(count)
    expect(html).toContain(`--scratch-columns:${count}`)
  })

  it('waits for real data rather than making current-period results', () => {
    const html = renderToStaticMarkup(<ScratchDrawDialog game={{ ...game, latestIssue: '—' }} onClose={() => {}} />)
    expect(html).toContain('等待开奖结果')
    expect(html).not.toContain('<canvas')
    expect(html).not.toContain('54776212')
    expect(html).not.toContain('全部揭晓')
  })

  it('keeps Bingo Mark Six fixed wave colours and marks the seventh ball as special', () => {
    const markSix = { ...game, id: 'bingo-mark-six', title: '宾果六合彩', balls: [5, 9, 40, 47, 29, 2, 49] }
    const html = renderToStaticMarkup(<ScratchDrawDialog game={markSix} draw={{ ...draw, game_id: markSix.id, numbers: markSix.balls }} onClose={() => {}} />)
    expect(html).toContain('mark-six-ball wave-green')
    expect(html).toContain('mark-six-ball wave-blue')
    expect(html).toContain('mark-six-ball wave-green mark-six-special-ball')
    expect(html).toContain('<small>特</small>')
  })
})
