import { describe, expect, it, vi } from 'vitest'
import type { Game } from '../types'
import type { DrawResult } from '../api/lottery'
import { ScratchCoverage, createScratchMask, scratchPoint, scratchResult } from './scratchDraw'

const game = { id: 'speed-fly', period: '54776212', latestIssue: '54776211', balls: [7, 4, 3, 8, 1, 5, 2, 6, 9, 10] } as Game
const draw: DrawResult = { id: 1, game_id: game.id, issue: '54776210', numbers: [2, 4, 3, 8, 1, 5, 7, 6, 9, 10], draw_at: '2026-08-30T08:00:00Z' }

function canvasFixture() {
  const context = {
    clearRect: vi.fn(), fillRect: vi.fn(), stroke: vi.fn(), beginPath: vi.fn(), moveTo: vi.fn(), lineTo: vi.fn(),
    createLinearGradient: vi.fn(() => ({ addColorStop: vi.fn() })), arc: vi.fn(), fill: vi.fn(),
    save: vi.fn(), restore: vi.fn(), setTransform: vi.fn(), drawImage: vi.fn(), globalCompositeOperation: 'source-over',
  }
  const copyContext = { drawImage: vi.fn() }
  const copy = { width: 0, height: 0, getContext: vi.fn(() => copyContext) }
  const canvas = { width: 300, height: 150, getContext: vi.fn(() => context), ownerDocument: { createElement: vi.fn(() => copy) } }
  const revealed = vi.fn()
  const mask = createScratchMask(canvas as unknown as HTMLCanvasElement, revealed)!
  return { canvas, context, copy, copyContext, revealed, mask }
}

describe('scratch draw result identity', () => {
  it('uses only a matching draw or an explicitly identified latest result, never the accepting issue', () => {
    expect(scratchResult(game, draw)).toMatchObject({ issue: draw.issue, numbers: draw.numbers })
    expect(scratchResult(game)).toMatchObject({ issue: game.latestIssue, numbers: game.balls })
    expect(scratchResult({ ...game, latestIssue: '' })).toBeNull()
    expect(scratchResult({ ...game, latestIssue: '—' })).toBeNull()
    expect(scratchResult({ ...game, latestIssue: '--' })).toBeNull()
    expect(scratchResult({ ...game, latestIssue: '—' }, { ...draw, issue: '--' })).toBeNull()
    expect(scratchResult({ ...game, latestIssue: '', balls: [] })).toBeNull()
    expect(scratchResult({ ...game, latestIssue: '' }, { ...draw, game_id: 'speed-racing' })).toBeNull()
    expect(scratchResult({ ...game, balls: [NaN] })).toBeNull()
    expect(scratchResult({ ...game, balls: [-1] })).toBeNull()
  })

  it('does not reset on clock ticks, current-issue changes or identical API payloads; new actual results reset', () => {
    const key = scratchResult(game, draw)!.key
    expect(scratchResult({ ...game, due: '00:01', period: 'next' }, { ...draw, numbers: [...draw.numbers] })!.key).toBe(key)
    expect(scratchResult(game, { ...draw, issue: '54776212' })!.key).not.toBe(key)
    expect(scratchResult(game, { ...draw, numbers: [1, 2, 3] })!.key).not.toBe(key)
    expect(scratchResult({ ...game, id: 'other' }, { ...draw, game_id: 'other' })!.key).not.toBe(key)
  })
})

describe('normalized scratch coordinates and coverage', () => {
  it('maps CSS pointer coordinates independently of DPR and clamps captured pointer movement outside the card', () => {
    const bounds = { left: 80, top: 120, width: 240, height: 100 }
    expect(scratchPoint(200, 145, bounds)).toEqual({ x: .5, y: .25 })
    expect(scratchPoint(40, 300, bounds)).toEqual({ x: 0, y: 1 })
    expect(scratchPoint(80, 120, { ...bounds, width: 0 })).toBeNull()
    expect(scratchPoint(NaN, 120, bounds)).toBeNull()
  })

  it('accounts for full continuous strokes, makes repeated strokes idempotent, and can reset', () => {
    const coverage = new ScratchCoverage()
    const first = coverage.mark({ x: .1, y: .5 }, { x: .9, y: .5 }, .05, .1)
    expect(first).toBeGreaterThan(.12)
    expect(first).toBeLessThan(.25)
    expect(coverage.mark({ x: .1, y: .5 }, { x: .9, y: .5 }, .05, .1)).toBe(first)
    expect(coverage.mark({ x: .5, y: .2 }, { x: .5, y: .8 }, .08, .08)).toBeGreaterThan(first)
    coverage.reset()
    expect(coverage.fraction).toBe(0)
    expect(coverage.mark({ x: .5, y: .5 }, { x: .5, y: .5 }, .1, .1)).toBeGreaterThan(0)
  })

  it('counts all cells after a complete sweep without reading canvas pixels', () => {
    const coverage = new ScratchCoverage()
    for (let y = 0; y <= 1; y += .1) coverage.mark({ x: 0, y }, { x: 1, y }, .08, .08)
    expect(coverage.fraction).toBe(1)
  })
})

describe('scratch mask canvas controller', () => {
  it('paints an opaque cover at DPR resolution without timers or automatic reveal', () => {
    const { mask, canvas, context, revealed } = canvasFixture()
    mask.resize(240, 120, 2)
    expect([canvas.width, canvas.height]).toEqual([480, 240])
    expect(context.setTransform).toHaveBeenLastCalledWith(2, 0, 0, 2, 0, 0)
    expect(context.fillRect).toHaveBeenCalledWith(0, 0, 240, 120)
    expect(revealed).not.toHaveBeenCalled()
  })

  it('uses destination-out continuous strokes and reveals only after manual coverage or explicit reveal', () => {
    const { mask, context, revealed } = canvasFixture()
    mask.resize(240, 120, 1)
    mask.scratch({ x: .1, y: .1 }, { x: .1, y: .1 })
    expect(context.globalCompositeOperation).toBe('destination-out')
    expect(context.arc).toHaveBeenLastCalledWith(24, 12, 18, 0, Math.PI * 2)
    expect(revealed).not.toHaveBeenCalled()
    for (let y = 0; y <= 1; y += .1) mask.scratch({ x: 0, y }, { x: 1, y })
    expect(revealed).toHaveBeenCalledOnce()
    mask.reveal()
    expect(revealed).toHaveBeenCalledOnce()
    mask.reset()
    mask.scratch({ x: .1, y: .1 }, { x: .1, y: .1 })
    expect(revealed).toHaveBeenCalledOnce()
    mask.reveal()
    expect(revealed).toHaveBeenCalledTimes(2)
  })

  it('preserves already-erased pixels on resize/DPR changes, without repainting the cover', () => {
    const { mask, canvas, context, copy, copyContext } = canvasFixture()
    mask.resize(240, 120, 1)
    mask.scratch({ x: .1, y: .1 }, { x: .4, y: .4 })
    context.fillRect.mockClear()
    mask.resize(300, 180, 2)
    expect(copyContext.drawImage).toHaveBeenCalledWith(canvas, 0, 0)
    expect(context.drawImage).toHaveBeenCalledWith(copy, 0, 0, 240, 120, 0, 0, 300, 180)
    expect(context.fillRect).not.toHaveBeenCalled()
    expect([canvas.width, canvas.height]).toEqual([600, 360])
    mask.resize(300, 180, 2)
    expect(context.drawImage).toHaveBeenCalledOnce()
    mask.reveal()
    mask.resize(320, 160, 2)
    expect(context.fillRect).not.toHaveBeenCalled()
  })

  it('leaves the current mask intact for hidden/invalid sizes, and returns null without canvas support', () => {
    const { mask, context } = canvasFixture()
    mask.resize(0, 0, 1)
    mask.resize(Infinity, 100, 1)
    expect(context.fillRect).not.toHaveBeenCalled()
    expect(createScratchMask({ getContext: () => null } as unknown as HTMLCanvasElement, vi.fn())).toBeNull()
  })
})
