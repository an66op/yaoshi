export function managePlanSelectionFocus(dialog: HTMLElement, onClose: () => void) {
  const document = dialog.ownerDocument
  const previous = document.activeElement as HTMLElement | null
  const controls = () => [...dialog.querySelectorAll<HTMLButtonElement>('button:not([disabled])')]
  const selected = dialog.querySelector<HTMLButtonElement>('button[aria-pressed="true"]:not([disabled])')
  const initial = selected ?? controls()[0]
  initial?.focus()
  const keydown = (event: KeyboardEvent) => {
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); onClose(); return }
    if (event.key !== 'Tab') return
    const buttons = controls()
    const first = buttons[0]
    const last = buttons.at(-1)
    if (!first || !last) { event.preventDefault(); return }
    const active = document.activeElement
    if (!dialog.contains(active) || !buttons.includes(active as HTMLButtonElement)) {
      event.preventDefault(); (event.shiftKey ? last : first).focus()
    } else if (event.shiftKey && active === first) {
      event.preventDefault(); last.focus()
    } else if (!event.shiftKey && active === last) {
      event.preventDefault(); first.focus()
    }
  }
  dialog.addEventListener('keydown', keydown)
  return () => {
    dialog.removeEventListener('keydown', keydown)
    if (previous?.isConnected) previous.focus()
  }
}
