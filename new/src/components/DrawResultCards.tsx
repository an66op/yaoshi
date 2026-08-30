import { memo, useEffect, useRef, useState, type RefObject } from 'react'
import type { DrawResult } from '../api/lottery'
import { SPEED_RACING_TRIO_SRC } from '../data/gameArtwork'
import { useCanvasVisibility } from '../hooks/useCanvasVisibility'
import { CURRENT_DRAW_CARD_SIZE, drawCardIssueLabel, paintCurrentDrawCard, paintRecentDrawCard, recentDrawCardSize, releaseDrawCardCanvas } from '../utils/drawResultCardCanvas'

let cachedRacingCars: HTMLImageElement | null = null
let racingCarsRequest: Promise<HTMLImageElement> | null = null

function loadRacingCars() {
  if (cachedRacingCars) return Promise.resolve(cachedRacingCars)
  if (racingCarsRequest) return racingCarsRequest
  racingCarsRequest = new Promise<HTMLImageElement>((resolve, reject) => {
    const image = new Image()
    image.decoding = 'async'
    image.onload = () => {
      cachedRacingCars = image
      resolve(image)
    }
    image.onerror = () => {
      racingCarsRequest = null
      reject(new Error('赛车素材加载失败'))
    }
    image.src = SPEED_RACING_TRIO_SRC
  })
  return racingCarsRequest
}

export const DrawResultCards = memo(function DrawResultCards({ title, draw, draws }: { title: string; draw: DrawResult; draws: DrawResult[] }) {
  const currentRef = useRef<HTMLCanvasElement>(null)
  const rangeRef = useRef<HTMLCanvasElement>(null)
  const currentNearViewport = useCanvasVisibility(currentRef)
  const rangeNearViewport = useCanvasVisibility(rangeRef)
  const previewDialogRef = useRef<HTMLElement>(null)
  const [racingCars, setRacingCars] = useState<HTMLImageElement | null>(cachedRacingCars)
  const [artworkError, setArtworkError] = useState('')
  const [artworkAttempt, setArtworkAttempt] = useState(0)
  const [preview, setPreview] = useState<{ src: string; title: string; filename: string } | null>(null)

  useEffect(() => {
    if (racingCars) return
    let active = true
    void loadRacingCars().then((image) => {
      if (active) setRacingCars(image)
    }).catch(() => {
      if (active) setArtworkError('赛车图片加载失败，请重试后预览或保存。')
    })
    return () => { active = false }
  }, [artworkAttempt, racingCars])

  useEffect(() => {
    const canvas = currentRef.current
    if (!canvas) return
    if (currentNearViewport) paintCurrentDrawCard(canvas, { title }, draw, racingCars, window.devicePixelRatio)
    else releaseDrawCardCanvas(canvas)
    return () => releaseDrawCardCanvas(canvas)
  }, [draw, title, racingCars, currentNearViewport])

  useEffect(() => {
    const canvas = rangeRef.current
    if (!canvas) return
    if (rangeNearViewport) paintRecentDrawCard(canvas, { title }, draws, racingCars, window.devicePixelRatio)
    else releaseDrawCardCanvas(canvas)
    return () => releaseDrawCardCanvas(canvas)
  }, [draws, title, racingCars, rangeNearViewport])

  useEffect(() => {
    if (!preview) return
    const dialog = previewDialogRef.current
    if (!dialog) return
    const trigger = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const controls = Array.from(dialog.querySelectorAll<HTMLElement>('button:not([disabled]), a[href]'))
    const first = controls[0]
    const last = controls.at(-1)
    first?.focus({ preventScroll: true })
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        event.stopPropagation()
        setPreview(null)
      }
      if (event.key !== 'Tab' || !first || !last) return
      if (event.shiftKey && (document.activeElement === first || !dialog.contains(document.activeElement))) {
        event.preventDefault()
        last.focus({ preventScroll: true })
      } else if (!event.shiftKey && (document.activeElement === last || !dialog.contains(document.activeElement))) {
        event.preventDefault()
        first.focus({ preventScroll: true })
      }
    }
    const onFocus = (event: FocusEvent) => {
      if (event.target instanceof Node && !dialog.contains(event.target)) first?.focus({ preventScroll: true })
    }
    document.addEventListener('keydown', onKeyDown, true)
    document.addEventListener('focusin', onFocus)
    return () => {
      document.removeEventListener('keydown', onKeyDown, true)
      document.removeEventListener('focusin', onFocus)
      if (trigger?.isConnected) trigger.focus({ preventScroll: true })
    }
  }, [preview])

  // Never export the temporary, artwork-free canvas while the shared PNG loads.
  const imageData = (ref: RefObject<HTMLCanvasElement | null>) => {
    const canvas = ref.current
    if (!racingCars || !canvas) return ''
    // Paint synchronously as well: a click immediately after an image/draw
    // update must not capture the preceding passive-effect frame.
    const current = ref === currentRef
    try {
      if (current) paintCurrentDrawCard(canvas, { title }, draw, racingCars, window.devicePixelRatio)
      else paintRecentDrawCard(canvas, { title }, draws, racingCars, window.devicePixelRatio)
      return canvas.toDataURL('image/png')
    } finally {
      // Keyboard activation may target a card before its observer callback.
      // Its exported snapshot remains valid without retaining a hidden bitmap.
      if (!(current ? currentNearViewport : rangeNearViewport)) releaseDrawCardCanvas(canvas)
    }
  }
  const openPreview = (ref: RefObject<HTMLCanvasElement | null>, title: string, filename: string) => {
    const src = imageData(ref)
    if (src) setPreview({ src, title, filename })
  }
  const issue = drawCardIssueLabel(draw.issue)
  const currentTitle = `${title}第${issue}期开奖号码图片`
  const rangeTitle = `${title}最近开奖记录图片`
  const currentFilename = `${title}-${issue}-开奖号码.png`
  const rangeFilename = `${title}-最近开奖记录.png`
  const recentSize = recentDrawCardSize(draws.length)
  return <div className="draw-result-image-cards" aria-busy={!racingCars && !artworkError}>
    {!racingCars && <div className="draw-result-artwork-status" role={artworkError ? 'alert' : 'status'}><span>{artworkError || '正在加载赛车图片…'}</span>{artworkError && <button type="button" onClick={() => { setArtworkError(''); setArtworkAttempt(attempt => attempt + 1) }}>重试</button>}</div>}
    <figure className="draw-result-image-card">
      <button className="draw-image-trigger" type="button" disabled={!racingCars} aria-label={`预览${currentTitle}`} onClick={() => openPreview(currentRef, currentTitle, currentFilename)}><canvas aria-label={currentTitle} ref={currentRef} width={0} height={0} style={{ width: '100%', height: 'auto', aspectRatio: `${CURRENT_DRAW_CARD_SIZE.width} / ${CURRENT_DRAW_CARD_SIZE.height}` }} /></button>
    </figure>
    <figure className="draw-result-image-card">
      <button className="draw-image-trigger" type="button" disabled={!racingCars} aria-label={`预览${rangeTitle}`} onClick={() => openPreview(rangeRef, rangeTitle, rangeFilename)}><canvas aria-label={rangeTitle} ref={rangeRef} width={0} height={0} style={{ width: '100%', height: 'auto', aspectRatio: `${recentSize.width} / ${recentSize.height}` }} /></button>
    </figure>
    {preview && <div className="draw-image-preview-layer" role="presentation" onClick={() => setPreview(null)}><section ref={previewDialogRef} role="dialog" aria-modal="true" aria-label={preview.title} onClick={(event) => event.stopPropagation()}><header><b>{preview.title}</b><button type="button" aria-label="关闭图片预览" onClick={() => setPreview(null)}>×</button></header><img alt={preview.title} src={preview.src} /><footer><small>长按图片也可以保存到手机</small><a download={preview.filename} href={preview.src}>下载图片</a></footer></section></div>}
  </div>
})
