import { describe, expect, it, vi } from 'vitest'
import { controlSurfaceProps, preventControlContextMenu } from './controlSurface'

type Event = Parameters<typeof preventControlContextMenu>[0]
const eventFor = (target: unknown) => ({ target, preventDefault: vi.fn() }) as Event & { preventDefault: ReturnType<typeof vi.fn> }

describe('control panel long-press protection', () => {
  it('blocks a control label or number menu without adding touch/click/clipboard handlers', () => {
    const event = eventFor({ closest: vi.fn(() => null) })
    controlSurfaceProps.onContextMenu(event)
    expect(event.preventDefault).toHaveBeenCalledOnce()
    expect(Object.keys(controlSurfaceProps).sort()).toEqual(['data-control-surface', 'onContextMenu'])
  })

  it.each(['input', 'textarea', 'select', '[contenteditable]:not([contenteditable="false"])', '[data-allow-selection]'])(
    'leaves the native menu on %s available', selector => {
      const closest = vi.fn((selectors: string) => selectors.split(', ').includes(selector) ? {} : null)
      const event = eventFor({ closest })
      preventControlContextMenu(event)
      expect(closest).toHaveBeenCalledOnce()
      expect(event.preventDefault).not.toHaveBeenCalled()
    },
  )

  it('also respects an editable parent when the event targets a text node', () => {
    const event = eventFor({ parentElement: { closest: () => ({}) } })
    preventControlContextMenu(event)
    expect(event.preventDefault).not.toHaveBeenCalled()
  })
})
