import { describe, expect, it, vi } from 'vitest'
import { managePlanSelectionFocus } from './planSelectionFocus'

function fixture() {
  const document: { activeElement: unknown } = { activeElement: null }
  const button = () => ({ isConnected: true, focus: vi.fn(function (this: unknown) { document.activeElement = this }) })
  const trigger = button(), first = button(), selected = button(), last = button()
  let items = [first, selected, last]
  trigger.focus()
  let listener: (event: KeyboardEvent) => void = () => {}
  const dialog = {
    ownerDocument: document, querySelectorAll: () => items, querySelector: () => selected,
    contains: (element: unknown) => items.includes(element as typeof first),
    addEventListener: vi.fn((_type, fn) => { listener = fn }), removeEventListener: vi.fn(),
  }
  const close = vi.fn()
  const cleanup = managePlanSelectionFocus(dialog as unknown as HTMLElement, close)
  const key = (value: string, shiftKey = false) => {
    const event = { key: value, shiftKey, preventDefault: vi.fn(), stopPropagation: vi.fn() }
    listener(event as unknown as KeyboardEvent)
    return event
  }
  return { document, trigger, first, selected, last, dialog, close, cleanup, key, setItems: (value: typeof items) => { items = value } }
}

describe('plan selection keyboard focus', () => {
  it('starts on the selected plan and traps traversal through enabled controls', () => {
    const { document, first, selected, last, key, setItems } = fixture()
    expect(document.activeElement).toBe(selected)
    first.focus(); key('Tab', true)
    expect(document.activeElement).toBe(last)
    key('Tab'); expect(document.activeElement).toBe(first)
    setItems([]); expect(key('Tab').preventDefault).toHaveBeenCalled()
  })
  it('closes on Escape and restores the trigger after removing its listener', () => {
    const { document, trigger, dialog, close, cleanup, key } = fixture()
    expect(key('Escape').stopPropagation).toHaveBeenCalled()
    expect(close).toHaveBeenCalledOnce()
    cleanup()
    expect(dialog.removeEventListener).toHaveBeenCalledWith('keydown', expect.any(Function))
    expect(document.activeElement).toBe(trigger)
  })
})
