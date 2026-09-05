import { describe, expect, it } from 'vitest'
import { keepMenuItemVisible } from './menuVisibility'

const rect = (top: number, bottom: number) => ({ top, bottom } as DOMRect)

describe('admin sidebar active menu visibility', () => {
  it('scrolls down only enough to reveal an active item below the viewport', () => {
    const container = { scrollTop: 20, getBoundingClientRect: () => rect(100, 500) } as HTMLElement
    const item = { getBoundingClientRect: () => rect(520, 562) } as HTMLElement
    keepMenuItemVisible(container, item)
    expect(container.scrollTop).toBe(92)
  })

  it('scrolls up only enough to reveal an active item above the viewport', () => {
    const container = { scrollTop: 240, getBoundingClientRect: () => rect(100, 500) } as HTMLElement
    const item = { getBoundingClientRect: () => rect(70, 112) } as HTMLElement
    keepMenuItemVisible(container, item)
    expect(container.scrollTop).toBe(200)
  })

  it('preserves the current sidebar position when the active item is already visible', () => {
    const container = { scrollTop: 160, getBoundingClientRect: () => rect(100, 500) } as HTMLElement
    const item = { getBoundingClientRect: () => rect(260, 302) } as HTMLElement
    keepMenuItemVisible(container, item)
    expect(container.scrollTop).toBe(160)
  })
})
