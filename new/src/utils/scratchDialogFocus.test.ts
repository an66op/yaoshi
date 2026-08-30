import { describe, expect, it, vi } from 'vitest'
import { manageScratchDialogFocus } from './scratchDialogFocus'

function fixture() {
  const document: { activeElement: unknown } = { activeElement: null }
  const button = () => ({ hidden: false, isConnected: true, getAttribute: vi.fn(() => null), focus: vi.fn(function (this: unknown) { document.activeElement = this }) })
  const trigger = button()
  const first = button()
  const last = button()
  let items = [first, last]
  trigger.focus()
  let listener: (event: KeyboardEvent) => void = () => {}
  const dialog = {
    ownerDocument: document,
    querySelectorAll: vi.fn(() => items),
    contains: (element: unknown) => element === first || element === last,
    addEventListener: vi.fn((_type, fn) => { listener = fn }),
    removeEventListener: vi.fn(),
  }
  const close = vi.fn()
  const cleanup = manageScratchDialogFocus(dialog as unknown as HTMLElement, close)
  const key = (value: string, shiftKey = false) => {
    const event = { key: value, shiftKey, preventDefault: vi.fn(), stopPropagation: vi.fn() }
    listener(event as unknown as KeyboardEvent)
    return event
  }
  return { document, trigger, first, last, dialog, close, cleanup, key, setItems: (value: typeof items) => { items = value } }
}

describe('scratch dialog keyboard focus', () => {
  it('moves focus into the portal and loops Tab/Shift+Tab through current enabled controls', () => {
    const { document, first, last, key, setItems } = fixture()
    expect(document.activeElement).toBe(first)
    expect(key('Tab', true).preventDefault).toHaveBeenCalled()
    expect(document.activeElement).toBe(last)
    expect(key('Tab').preventDefault).toHaveBeenCalled()
    expect(document.activeElement).toBe(first)
    expect(key('Tab').preventDefault).not.toHaveBeenCalled()
    setItems([last])
    key('Tab')
    expect(document.activeElement).toBe(last)
  })

  it('closes on Escape, removes listeners and restores the connected trigger', () => {
    const { document, trigger, dialog, close, cleanup, key } = fixture()
    expect(key('Escape').stopPropagation).toHaveBeenCalled()
    expect(close).toHaveBeenCalledOnce()
    cleanup()
    expect(dialog.removeEventListener).toHaveBeenCalledWith('keydown', expect.any(Function))
    expect(document.activeElement).toBe(trigger)
  })
})
