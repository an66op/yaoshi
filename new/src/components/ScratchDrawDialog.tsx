import { useEffect, useRef, useState, type CSSProperties, type PointerEvent } from 'react'
import type { DrawResult } from '../api/lottery'
import { ballTone } from '../data/games'
import type { Game } from '../types'
import { createScratchMask, scratchPoint, scratchResult, type ScratchMask, type ScratchPoint } from '../utils/scratchDraw'
import { manageScratchDialogFocus } from '../utils/scratchDialogFocus'
import { ActionDialog } from './Dialogs'
import { controlSurfaceProps } from '../utils/controlSurface'
import { lotteryResultSummary, lotteryRuleProfile, markSixDrawBallClass } from '../utils/lotteryRules'
import './scratch-draw.css'

export function ScratchDrawDialog({ game, draw, onClose }: { game: Game; draw?: DrawResult; onClose: () => void }) {
  const contentRef = useRef<HTMLDivElement | null>(null)
  const closeRef = useRef(onClose)
  closeRef.current = onClose
  useEffect(() => {
    const dialog = contentRef.current?.closest<HTMLElement>('[role="dialog"]')
    if (dialog) return manageScratchDialogFocus(dialog, () => closeRef.current())
  }, [])
  const result = scratchResult(game, draw)
  return <ActionDialog title="涂抹开奖" description={result ? `${game.title} · 第 ${result.issue} 期` : game.title} confirmLabel="关闭" onClose={onClose}>
    <div ref={contentRef}>{result ? <ScratchSurface key={result.key} gameId={game.id} ruleVersion={game.ruleVersion} numbers={result.numbers} /> : <p className="scratch-draw-empty" role="status">等待开奖结果</p>}</div>
  </ActionDialog>
}

function ScratchSurface({ numbers, gameId, ruleVersion = '' }: { numbers: number[]; gameId: string; ruleVersion?: string }) {
  const surfaceRef = useRef<HTMLDivElement | null>(null)
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const maskRef = useRef<ScratchMask | null>(null)
  const pointerRef = useRef<number | null>(null)
  const pointRef = useRef<ScratchPoint | null>(null)
  const [ready, setReady] = useState(false)
  const [unavailable, setUnavailable] = useState(false)
  const [revealed, setRevealed] = useState(false)

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const mask = createScratchMask(canvas, () => {
      if (pointerRef.current !== null && canvas.hasPointerCapture(pointerRef.current)) canvas.releasePointerCapture(pointerRef.current)
      pointerRef.current = null; pointRef.current = null
      setRevealed(true)
    })
    if (!mask) { setUnavailable(true); return }
    maskRef.current = mask
    const resize = () => {
      const bounds = canvas.getBoundingClientRect()
      if (!bounds.width || !bounds.height) return
      mask.resize(bounds.width, bounds.height, window.devicePixelRatio || 1)
      setReady(true)
    }
    resize()
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(resize)
    observer?.observe(canvas)
    window.addEventListener('resize', resize)
    return () => {
      observer?.disconnect()
      window.removeEventListener('resize', resize)
      if (pointerRef.current !== null && canvas.hasPointerCapture(pointerRef.current)) canvas.releasePointerCapture(pointerRef.current)
      pointerRef.current = null; pointRef.current = null; maskRef.current = null
    }
  }, [])

  const pointFor = (event: PointerEvent<HTMLCanvasElement>) => scratchPoint(event.clientX, event.clientY, event.currentTarget.getBoundingClientRect())
  const start = (event: PointerEvent<HTMLCanvasElement>) => {
    if (revealed || !ready || !event.isPrimary || event.button !== 0 || pointerRef.current !== null) return
    const point = pointFor(event)
    if (!point) return
    event.preventDefault()
    event.currentTarget.setPointerCapture(event.pointerId)
    pointerRef.current = event.pointerId; pointRef.current = point
    maskRef.current?.scratch(point, point)
  }
  const move = (event: PointerEvent<HTMLCanvasElement>) => {
    if (event.pointerId !== pointerRef.current || !pointRef.current || revealed) return
    const point = pointFor(event)
    if (!point) return
    event.preventDefault()
    maskRef.current?.scratch(pointRef.current, point)
    pointRef.current = point
  }
  const stop = (event: PointerEvent<HTMLCanvasElement>) => {
    if (event.pointerId !== pointerRef.current) return
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
    pointerRef.current = null; pointRef.current = null
  }
  const reveal = () => { maskRef.current?.reveal(); setRevealed(true) }
  const reset = () => {
    if (surfaceRef.current) surfaceRef.current.scrollLeft = 0
    maskRef.current?.reset()
    setRevealed(false)
  }
  const columns = numbers.length
  const summary = lotteryResultSummary(gameId, numbers, ruleVersion)
  const markSix = lotteryRuleProfile(gameId).family === 'mark-six'

  return <section className="scratch-draw-board" {...controlSurfaceProps}>
    <p className="scratch-draw-hint" role="status">{revealed ? '开奖结果已揭晓' : unavailable ? '点击全部揭晓查看结果' : '按住涂抹，刮开查看结果'}</p>
    <div ref={surfaceRef} className={`scratch-draw-surface${ready ? ' is-ready' : ''}${revealed ? ' is-revealed' : ''}`}>
      <div className="scratch-draw-grid" style={{ '--scratch-columns': columns } as CSSProperties} aria-hidden={!revealed} aria-label={revealed ? `开奖号码：${numbers.join('、')}` : undefined}>
        {numbers.map((number, index) => <span className="scratch-draw-cell" key={index}><b className={markSix ? markSixDrawBallClass(number, index, numbers.length) : ballTone(number)}>{number}</b><small>{markSix && index === 6 ? '特' : index + 1}</small></span>)}
      </div>
      <canvas ref={canvasRef} className="scratch-draw-mask" aria-hidden="true" onPointerDown={start} onPointerMove={move} onPointerUp={stop} onPointerCancel={stop} onLostPointerCapture={stop} />
    </div>
    {revealed && summary && <p className="scratch-draw-total">{summary.label}：<b>{summary.total}</b><span>{summary.size} · {summary.parity}</span></p>}
    <div className="scratch-draw-controls">
      <button type="button" onClick={reset} disabled={!ready || !revealed}>再刮一次</button>
      <button type="button" onClick={reveal} disabled={revealed}>全部揭晓</button>
    </div>
  </section>
}
