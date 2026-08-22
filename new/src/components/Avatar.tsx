type Props = { index: number; label: string; className?: string }

const photoAvatarCount = 16
const avatarCount = photoAvatarCount * 2

/** 独立裁切头像：-1 代表当前用户选择的头像。 */
export function Avatar({ index, label, className = '' }: Props) {
  let selectedIndex = index
  if (index === -1) {
    try { selectedIndex = Number(JSON.parse(window.localStorage.getItem('seven-star-avatar') ?? '{"index":0}').index) } catch { selectedIndex = 0 }
  }
  const safeIndex = ((selectedIndex % avatarCount) + avatarCount) % avatarCount
  const filename = String(safeIndex % photoAvatarCount).padStart(2, '0')
  const prefix = safeIndex < photoAvatarCount ? 'avatar' : 'avatar-anime'
  return <img alt={label} className={`app-avatar ${className}`} src={`/images/avatars/${prefix}-${filename}.png`} />
}
