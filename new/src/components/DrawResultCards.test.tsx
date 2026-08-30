import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { DrawResultCards } from './DrawResultCards'

describe('uncluttered draw image cards', () => {
  it('keeps both accessible preview targets without caption/action text rows', () => {
    const draw = { id: 1, game_id: 'speed-fly', issue: '54776109', draw_at: '2026-08-30T05:15:00Z', numbers: [2, 5, 1, 4, 6, 9, 3, 7, 8, 10] }
    const html = renderToStaticMarkup(<DrawResultCards title="极速飞艇" draw={draw} draws={[draw]} />)
    expect(html.match(/<canvas /g)).toHaveLength(2)
    expect(html.match(/class="draw-image-trigger"/g)).toHaveLength(2)
    expect(html).toContain('aria-label="预览极速飞艇第54776109期开奖号码图片"')
    expect(html).toContain('aria-label="预览极速飞艇最近开奖记录图片"')
    expect(html).not.toContain('<figcaption')
    expect(html).not.toMatch(/>预览<|>保存</)
  })
})
