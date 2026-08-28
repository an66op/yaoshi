import { avatarSrcForIndex } from '../data/avatars'

type Props = { index: number; label: string; className?: string; src?: string }

/** 独立裁切头像：-1 代表当前用户选择的头像。 */
export function Avatar({ index, label, className = '', src = '' }: Props) {
  let selectedIndex = index
  let selectedSrc = src.trim()
  if (index === -1) {
    try {
      const stored = JSON.parse(window.localStorage.getItem('seven-star-avatar') ?? '{"index":0}') as { index?: unknown; src?: unknown }
      selectedIndex = Number(stored.index)
      if (!selectedSrc && typeof stored.src === 'string') selectedSrc = stored.src.trim()
    } catch { selectedIndex = 0 }
  }
  if (!Number.isFinite(selectedIndex)) selectedIndex = 0
  return <img alt={label} className={`app-avatar ${className}`} src={selectedSrc || avatarSrcForIndex(selectedIndex)} />
}
