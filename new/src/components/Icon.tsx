import type { ReactElement } from 'react'
import type { IconName } from '../types'

export function Icon({ name }: { name: IconName }) {
  const paths: Record<IconName, ReactElement> = {
    game: <><rect x="3" y="9" width="18" height="10" rx="3"/><path d="M7 13h4m-2-2v4m7-1h.01M19 13h.01"/></>,
    shop: <><path d="M5 4h14l1 5H4l1-5Z"/><path d="M5 9v10h14V9M9 19v-5h6v5"/></>,
    chat: <><path d="M4 5h16v12H9l-5 3V5Z"/><path d="M8 10h8m-8 3h5"/></>,
    user: <><circle cx="12" cy="8" r="3.5"/><path d="M5 20c.7-3.2 3.1-5 7-5s6.3 1.8 7 5"/></>,
    back: <path d="m14 5-7 7 7 7"/>,
    bell: <><path d="M18 10a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9"/><path d="M10 22h4"/></>,
    plus: <path d="M12 5v14M5 12h14"/>,
    more: <path d="M6 12h.01M12 12h.01M18 12h.01"/>,
    gift: <><rect x="4" y="8" width="16" height="12" rx="2"/><path d="M3 8h18v4H3zM12 8v12M12 8S7 7 7 4c0-2 3-1 5 4Zm0 0s5-1 5-4c0-2-3-1-5 4Z"/></>,
    arrow: <path d="m9 18 6-6-6-6"/>,
    switch: <><path d="M7 7h11l-3-3M18 7l-3 3M17 17H6l3 3M6 17l3-3"/></>,
    room: <><circle cx="8.5" cy="9.5" r="4.5"/><circle cx="8.5" cy="9.5" r="1"/><path d="m11.8 12.8 8.2 8.2M15.2 16.2l2.2-2.2M17.7 18.7l2.1-2.1"/></>,
  }
  return <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{paths[name]}</svg>
}
