export const avatars = [
  '晨风', '可可', '阿泽', '小葵', '海岸',
  '安娜', '子言', '桃桃', '书航', '晚樱',
  '银川', '初夏', '达伦', '南枝', '星野',
  '牧歌', '夜空', '糖果', '晴川', '木棉',
  '霜华', '暖阳', '赤焰', '冰蓝', '紫音',
  '海风', '星野二号', '青柠', '书航二号', '达伦二号',
  '桃桃二号', '银川二号',
]

const photoAvatarCount = 16
const avatarCount = photoAvatarCount * 2

export function avatarSrcForIndex(index: number) {
  const safeIndex = ((index % avatarCount) + avatarCount) % avatarCount
  const filename = String(safeIndex % photoAvatarCount).padStart(2, '0')
  const prefix = safeIndex < photoAvatarCount ? 'avatar' : 'avatar-anime'
  return `/images/avatars/${prefix}-${filename}.png`
}

export function avatarIndexFromSrc(src: string) {
  const match = src.match(/\/avatar(-anime)?-(\d{2})\.png(?:[?#].*)?$/i)
  if (!match) return null
  const fileIndex = Number(match[2])
  if (!Number.isInteger(fileIndex) || fileIndex < 0 || fileIndex >= photoAvatarCount) return null
  return match[1] ? photoAvatarCount + fileIndex : fileIndex
}
