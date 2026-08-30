import { useEffect, useState, type RefObject } from 'react'

/** Keep a small prepaint margin around the viewport, including the clipping
 * imposed by the room's scroll container. Far-away history keeps its DOM size
 * but does not need a multi-megapixel backing bitmap. */
export function useCanvasVisibility(ref: RefObject<HTMLCanvasElement | null>) {
  const [nearViewport, setNearViewport] = useState(() => typeof IntersectionObserver === 'undefined')

  useEffect(() => {
    const canvas = ref.current
    if (!canvas) return
    if (typeof IntersectionObserver === 'undefined') {
      setNearViewport(true)
      return
    }
    let active = true
    let observer: IntersectionObserver | undefined
    try {
      observer = new IntersectionObserver((entries) => {
        if (!active) return
        for (const entry of entries) {
          if (entry.target === canvas) setNearViewport(entry.isIntersecting)
        }
      }, { rootMargin: '240px 0px', threshold: 0 })
      observer.observe(canvas)
    } catch {
      observer?.disconnect()
      // Some embedded browsers expose an incomplete observer implementation.
      // Correct results take priority over this optional memory optimisation.
      setNearViewport(true)
    }
    return () => {
      active = false
      observer?.disconnect()
    }
  }, [ref])

  return nearViewport
}
