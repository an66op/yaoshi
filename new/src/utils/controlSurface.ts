import type { SyntheticEvent } from 'react'

const nativeSelection = 'input, textarea, select, [contenteditable]:not([contenteditable="false"]), [data-allow-selection]'
type MenuEvent = Pick<SyntheticEvent, 'target' | 'preventDefault'>

/** Only use on control panels, never on chat/history or the entire page.
 * Leave native editing menus alone; do not intercept taps, drags or scrolling. */
export function preventControlContextMenu(event: MenuEvent) {
  const target = event.target as (EventTarget & { closest?: Element['closest']; parentElement?: Element | null }) | null
  const element = target?.closest ? target : target?.parentElement
  if (element?.closest?.(nativeSelection)) return
  event.preventDefault()
}

export const controlSurfaceProps = {
  'data-control-surface': '',
  onContextMenu: preventControlContextMenu,
}
