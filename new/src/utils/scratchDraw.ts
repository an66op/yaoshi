import type { DrawResult } from '../api/lottery'
import type { Game } from '../types'

export type ScratchResult = { issue: string; numbers: number[]; key: string }
export type ScratchPoint = { x: number; y: number }

const validNumbers = (numbers: number[]) => numbers.length > 0 && numbers.every((number) => Number.isSafeInteger(number) && number >= 0)
const validIssue = (issue: string) => typeof issue === 'string' && /\d/.test(issue)

/** The current accepting issue is never a substitute for a verified draw. */
export function scratchResult(game: Game, draw?: DrawResult): ScratchResult | null {
  const result = draw?.game_id === game.id && validIssue(draw.issue) && validNumbers(draw.numbers)
    ? { issue: draw.issue, numbers: draw.numbers }
    : validIssue(game.latestIssue) && validNumbers(game.balls)
      ? { issue: game.latestIssue, numbers: game.balls }
      : null
  return result ? { ...result, key: JSON.stringify([game.id, result.issue, result.numbers]) } : null
}

export function scratchPoint(clientX: number, clientY: number, bounds: { left: number; top: number; width: number; height: number }): ScratchPoint | null {
  if (!(bounds.width > 0 && bounds.height > 0) || !Number.isFinite(clientX) || !Number.isFinite(clientY)) return null
  return {
    x: Math.max(0, Math.min(1, (clientX - bounds.left) / bounds.width)),
    y: Math.max(0, Math.min(1, (clientY - bounds.top) / bounds.height)),
  }
}

/** Small normalized sampling grid: no synchronous canvas pixel readbacks. */
export class ScratchCoverage {
  private cells: Uint8Array
  private covered = 0
  readonly columns: number
  readonly rows: number

  constructor(columns = 40, rows = 24) {
    this.columns = columns
    this.rows = rows
    this.cells = new Uint8Array(columns * rows)
  }

  get fraction() { return this.covered / this.cells.length }

  reset() { this.cells.fill(0); this.covered = 0 }

  mark(from: ScratchPoint, to: ScratchPoint, radiusX: number, radiusY: number) {
    if (!(radiusX > 0 && radiusY > 0)) return this.fraction
    const minX = Math.max(0, Math.floor((Math.min(from.x, to.x) - radiusX) * this.columns))
    const maxX = Math.min(this.columns - 1, Math.floor((Math.max(from.x, to.x) + radiusX) * this.columns))
    const minY = Math.max(0, Math.floor((Math.min(from.y, to.y) - radiusY) * this.rows))
    const maxY = Math.min(this.rows - 1, Math.floor((Math.max(from.y, to.y) + radiusY) * this.rows))
    const dx = (to.x - from.x) / radiusX
    const dy = (to.y - from.y) / radiusY
    const lengthSquared = dx * dx + dy * dy
    for (let y = minY; y <= maxY; y++) for (let x = minX; x <= maxX; x++) {
      const index = y * this.columns + x
      if (this.cells[index]) continue
      const px = ((x + 0.5) / this.columns - from.x) / radiusX
      const py = ((y + 0.5) / this.rows - from.y) / radiusY
      const along = lengthSquared ? Math.max(0, Math.min(1, (px * dx + py * dy) / lengthSquared)) : 0
      if ((px - along * dx) ** 2 + (py - along * dy) ** 2 <= 1) {
        this.cells[index] = 1
        this.covered++
      }
    }
    return this.fraction
  }
}

export type ScratchMask = {
  resize: (width: number, height: number, pixelRatio: number) => void
  scratch: (from: ScratchPoint, to: ScratchPoint) => void
  reveal: () => void
  reset: () => void
}

export function createScratchMask(canvas: HTMLCanvasElement, onReveal: () => void): ScratchMask | null {
  const context = canvas.getContext('2d')
  if (!context) return null
  const coverage = new ScratchCoverage()
  let width = 0
  let height = 0
  let revealed = false

  const paint = () => {
    context.clearRect(0, 0, width, height)
    const gradient = context.createLinearGradient(0, 0, width, height)
    gradient.addColorStop(0, '#a8c7cc')
    gradient.addColorStop(0.45, '#e0ecee')
    gradient.addColorStop(1, '#8fb5bf')
    context.fillStyle = gradient
    context.fillRect(0, 0, width, height)
    context.strokeStyle = 'rgba(255,255,255,.22)'
    context.lineWidth = 1
    for (let x = -height; x < width; x += 8) {
      context.beginPath(); context.moveTo(x, 0); context.lineTo(x + height, height); context.stroke()
    }
    context.strokeStyle = '#4e7d88'
    context.lineWidth = 3
    context.lineCap = 'round'
    for (let offset = -9; offset <= 9; offset += 9) {
      context.beginPath()
      context.moveTo(width / 2 - 10, height / 2 + offset + 5)
      context.lineTo(width / 2 + 10, height / 2 + offset - 5)
      context.stroke()
    }
  }

  const reveal = () => {
    if (revealed) return
    revealed = true
    context.clearRect(0, 0, width, height)
    onReveal()
  }

  return {
    resize(nextWidth, nextHeight, pixelRatio) {
      if (!(nextWidth > 0 && nextHeight > 0) || !Number.isFinite(nextWidth + nextHeight)) return
      const ratio = Math.max(1, Math.min(3, Number.isFinite(pixelRatio) ? pixelRatio : 1))
      const backingWidth = Math.max(1, Math.round(nextWidth * ratio))
      const backingHeight = Math.max(1, Math.round(nextHeight * ratio))
      if (width === nextWidth && height === nextHeight && canvas.width === backingWidth && canvas.height === backingHeight) return
      // Preserve the actual erased pixels through layout/DPR changes. Resizing
      // never restarts the scratch game or stores an unbounded stroke history.
      let previous: HTMLCanvasElement | null = null
      if (width && height && !revealed) {
        previous = canvas.ownerDocument.createElement('canvas')
        previous.width = canvas.width
        previous.height = canvas.height
        const previousContext = previous.getContext('2d')
        if (previousContext) previousContext.drawImage(canvas, 0, 0)
        else { previous = null; coverage.reset() }
      }
      width = nextWidth; height = nextHeight
      canvas.width = backingWidth; canvas.height = backingHeight
      context.setTransform(backingWidth / width, 0, 0, backingHeight / height, 0, 0)
      if (revealed) context.clearRect(0, 0, width, height)
      else if (previous) context.drawImage(previous, 0, 0, previous.width, previous.height, 0, 0, width, height)
      else paint()
    },
    scratch(from, to) {
      if (revealed || !width || !height) return
      const radius = Math.max(17, Math.min(25, width * 0.075))
      context.save()
      context.globalCompositeOperation = 'destination-out'
      context.lineWidth = radius * 2
      context.lineCap = 'round'
      context.beginPath()
      context.moveTo(from.x * width, from.y * height)
      context.lineTo(to.x * width, to.y * height)
      context.stroke()
      context.beginPath()
      context.arc(to.x * width, to.y * height, radius, 0, Math.PI * 2)
      context.fill()
      context.restore()
      if (coverage.mark(from, to, radius / width, radius / height) >= 0.7) reveal()
    },
    reveal,
    reset() { revealed = false; coverage.reset(); if (width && height) paint() },
  }
}
