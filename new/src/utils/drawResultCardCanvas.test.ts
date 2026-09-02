import { describe, expect, it, vi } from 'vitest'
import type { DrawResult } from '../api/lottery'
import { CURRENT_DRAW_CARD_SIZE, currentDrawCardSize, drawCardIssueLabel, paintCurrentDrawCard, paintRecentDrawCard, recentDrawCardSize, releaseDrawCardCanvas } from './drawResultCardCanvas'

function recordingCanvas() {
  const gradient = { addColorStop: vi.fn() }
  const ctx = {
    scale: vi.fn(), save: vi.fn(), restore: vi.fn(),
    createLinearGradient: vi.fn(() => gradient), createRadialGradient: vi.fn(() => gradient),
    fillRect: vi.fn(), beginPath: vi.fn(), roundRect: vi.fn(), fill: vi.fn(),
    stroke: vi.fn(), moveTo: vi.fn(), lineTo: vi.fn(), arc: vi.fn(),
    fillText: vi.fn(), drawImage: vi.fn(),
  }
  const canvas = { width: 0, height: 0, getContext: vi.fn(() => ctx) }
  return { canvas: canvas as unknown as HTMLCanvasElement, ctx }
}

const draw: DrawResult = {
  id: 7, game_id: 'speed-fly', issue: '54776094',
  numbers: [7, 6, 10, 4, 9, 2, 8, 5, 1, 3], draw_at: '2026-08-30T05:02:15Z',
}
const artwork = {} as CanvasImageSource

describe('shared draw-result racing artwork', () => {
  // Only the title is needed by the renderer. No ID/category whitelist may
  // leave a different lottery with the former empty glowing-number podium.
  it.each(['极速赛车', '极速飞艇', 'SG飞艇', '澳洲幸运10', '宾果赛车', '极速时时彩', 'PC加拿大', '六合彩', '以后新增的彩种'])(
    'paints the same three cars in current and recent result images for %s', title => {
      const current = recordingCanvas()
      const recent = recordingCanvas()
      paintCurrentDrawCard(current.canvas, { title }, draw, artwork)
      paintRecentDrawCard(recent.canvas, { title }, [draw], artwork)

      expect(current.ctx.drawImage).toHaveBeenCalledExactlyOnceWith(artwork, 160, 148, 400, 227)
      expect(recent.ctx.drawImage).toHaveBeenCalledExactlyOnceWith(artwork, 563, 1, 145, 82)
      expect(current.ctx.fillText).toHaveBeenCalledWith(title, 28, 48)
      expect(recent.ctx.fillText).toHaveBeenCalledWith(`${title} · 最近 1 期`, 18, 31)
    },
  )

  it('preserves the supplied issue, draw order and top-three numbers', () => {
    const { canvas, ctx } = recordingCanvas()
    const before = structuredClone(draw)
    paintCurrentDrawCard(canvas, { title: '极速飞艇' }, draw, artwork)
    expect(draw).toEqual(before)
    expect(ctx.fillText).toHaveBeenCalledWith('第 54776094 期 · 开奖结果', 29, 76, 662)
    expect(ctx.fillText).toHaveBeenCalledWith('7', 360, 303)
    expect(ctx.fillText).toHaveBeenCalledWith('6', 225, 279)
    expect(ctx.fillText).toHaveBeenCalledWith('10', 495, 281)
    expect(ctx.fillText).toHaveBeenCalledWith('13 大 单', 40, 423)
    const printedNumbers = ctx.fillText.mock.calls.slice(2, 12).map(args => args[0])
    expect(printedNumbers).toEqual(draw.numbers.map(String))
  })

  it('keeps the recent-result canvas height stable while its artwork is loading', () => {
    const { canvas, ctx } = recordingCanvas()
    paintRecentDrawCard(canvas, { title: '极速飞艇' }, [draw], null)
    const pendingHeight = canvas.height
    expect(ctx.drawImage).not.toHaveBeenCalled()
    paintRecentDrawCard(canvas, { title: '极速飞艇' }, [draw], artwork)
    expect(canvas.height).toBe(pendingHeight)
    expect(canvas.height).toBe(155)
  })

  it('renders all supplied numbers even for short lottery results', () => {
    const { canvas, ctx } = recordingCanvas()
    paintCurrentDrawCard(canvas, { title: '三位数彩种' }, { ...draw, numbers: [0, 5, 9] }, artwork)
    expect(ctx.drawImage).toHaveBeenCalledOnce()
    expect(ctx.fillText).toHaveBeenCalledWith('0', 360, 303)
    expect(ctx.fillText).toHaveBeenCalledWith('5', 225, 279)
    expect(ctx.fillText).toHaveBeenCalledWith('9', 495, 281)
  })

  it('uses the same five-ball sum and tie labels in both image layouts', () => {
    const result = { ...draw, game_id: 'speed-ssc', numbers: [7, 1, 2, 3, 7] }
    const current = recordingCanvas()
    const recent = recordingCanvas()
    paintCurrentDrawCard(current.canvas, { title: '极速时时彩' }, result, artwork)
    paintRecentDrawCard(recent.canvas, { title: '极速时时彩' }, [result], artwork)
    expect(current.ctx.fillText).toHaveBeenCalledWith('20 小 双', 40, 423)
    expect(current.ctx.fillText).toHaveBeenCalledWith('和 虎', 275, 423)
    expect(recent.ctx.fillText).toHaveBeenCalledWith('20 小 双', 520, 128)
    expect(recent.ctx.fillText).toHaveBeenCalledWith('和 虎', 625, 128, 80)
  })

  it('paints one first-versus-fifth outcome only for an exact v3 product', () => {
    const result = { ...draw, game_id: 'speed-ssc', numbers: [7, 1, 2, 3, 7] }
    const v3 = recordingCanvas()
    paintCurrentDrawCard(v3.canvas, { title: '极速时时彩', ruleVersion: 'digits5-v3' }, result, artwork)
    expect(v3.ctx.fillText).toHaveBeenCalledWith('和', 275, 423)
    expect(v3.ctx.fillText).not.toHaveBeenCalledWith('和 虎', 275, 423)

    const sg = recordingCanvas()
    paintCurrentDrawCard(sg.canvas, { title: 'SG时时彩', ruleVersion: 'digits5-v3' }, { ...result, game_id: 'sg-ssc' }, artwork)
    expect(sg.ctx.fillText).toHaveBeenCalledWith('和 虎', 275, 423)
  })

  it('keeps all twenty numbers and a long issue without inventing unknown-game derivatives', () => {
    const result = { ...draw, game_id: 'official-tw-bingo', issue: '9'.repeat(80), numbers: Array.from({ length: 20 }, (_, index) => index + 40) }
    const current = recordingCanvas()
    const recent = recordingCanvas()
    paintCurrentDrawCard(current.canvas, { title: '宾果' }, result, artwork)
    paintRecentDrawCard(recent.canvas, { title: '宾果' }, [result], artwork)
    const currentText = current.ctx.fillText.mock.calls.map(call => call[0])
    const recentText = recent.ctx.fillText.mock.calls.map(call => call[0])
    for (const number of result.numbers) {
      expect(currentText).toContain(String(number))
      expect(recentText).toContain(String(number))
    }
    expect(currentText.join(' ')).not.toMatch(/冠亚和|龙虎| 大 | 小 /)
    expect(recentText.join(' ')).not.toMatch(/冠亚和|龙虎| 大 | 小 /)
    expect(currentText.filter(value => typeof value === 'string' && value.includes('999')).join('')).toBe(`第 ${result.issue} 期 · 开奖结果`)
    expect(recentText.filter(value => typeof value === 'string' && /^9+$/.test(value)).join('')).toBe(result.issue)
    expect(current.canvas.height).toBe(currentDrawCardSize(20, result.issue).height)
    expect(recent.canvas.height).toBe(recentDrawCardSize([result]).height)
    expect(current.canvas.height).toBeGreaterThan(450)
  })

  it('caps retina scaling and keeps the recent table within its fifteen-row limit', () => {
    const current = recordingCanvas()
    const recent = recordingCanvas()
    paintCurrentDrawCard(current.canvas, { title: '极速飞艇' }, draw, artwork, 3)
    paintRecentDrawCard(recent.canvas, { title: '极速飞艇' }, Array.from({ length: 20 }, () => draw), artwork, 2)
    expect(current.canvas.width).toBe(1440)
    expect(current.canvas.height).toBe(900)
    expect(current.ctx.scale).toHaveBeenCalledWith(2, 2)
    expect(recent.canvas.height).toBe((84 + 22 + 15 * 35 + 14) * 2)
    expect(recent.ctx.fillText).toHaveBeenCalledWith('极速飞艇 · 最近 15 期', 18, 31)
  })

  it('preserves the complete source issue, including date-prefixed identifiers', () => {
    expect(drawCardIssueLabel('20260830-12345')).toBe('20260830-12345')
    expect(drawCardIssueLabel('54776094')).toBe('54776094')
  })

  it('reserves the same display proportions as the painted bitmap before and after release', () => {
    const { canvas } = recordingCanvas()
    paintCurrentDrawCard(canvas, { title: '极速赛车' }, draw, artwork, 2)
    expect(canvas.width / canvas.height).toBe(CURRENT_DRAW_CARD_SIZE.width / CURRENT_DRAW_CARD_SIZE.height)
    releaseDrawCardCanvas(canvas)
    expect([canvas.width, canvas.height]).toEqual([0, 0])
    for (const count of [0, 1, 8, 15, 20]) {
      const rows = Array.from({ length: count }, () => draw)
      const size = recentDrawCardSize(count)
      paintRecentDrawCard(canvas, { title: '极速赛车' }, rows, artwork, 2)
      expect(canvas.width / canvas.height).toBe(size.width / size.height)
      releaseDrawCardCanvas(canvas)
      expect([canvas.width, canvas.height]).toEqual([0, 0])
    }
  })
})
