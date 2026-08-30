/** Scoped to the scratch modal: keep the shared dialog's other consumers intact. */
export function manageScratchDialogFocus(dialog: HTMLElement, onClose: () => void) {
  const document = dialog.ownerDocument
  const previous = document.activeElement as HTMLElement | null
  const controls = () => [...dialog.querySelectorAll<HTMLElement>('button:not([disabled]), a[href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])')]
    .filter((element) => !element.hidden && element.getAttribute('aria-hidden') !== 'true')
  controls()[0]?.focus()
  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); onClose(); return }
    if (event.key !== 'Tab') return
    const items = controls()
    const first = items[0]
    const last = items.at(-1)
    if (!first || !last) { event.preventDefault(); return }
    const active = document.activeElement
    if (!dialog.contains(active) || !items.includes(active as HTMLElement)) {
      event.preventDefault(); (event.shiftKey ? last : first).focus()
    } else if (event.shiftKey && active === first) {
      event.preventDefault(); last.focus()
    } else if (!event.shiftKey && active === last) {
      event.preventDefault(); first.focus()
    }
  }
  dialog.addEventListener('keydown', onKeyDown)
  return () => {
    dialog.removeEventListener('keydown', onKeyDown)
    if (previous?.isConnected && typeof previous.focus === 'function') previous.focus()
  }
}
