export const BET_COMMAND_MAX_LENGTH = 400

/** Match the server's rune limit. Never silently split a financial request. */
export function betCommandError(content: string) {
  const normalized = content.trim().replace(/^买/, '').trim()
  if ([...normalized].length > BET_COMMAND_MAX_LENGTH) return `投注内容超过 ${BET_COMMAND_MAX_LENGTH} 字，请减少选号后提交；本次尚未下注。`
  const segments = normalized.split('#').map(segment => segment.trim()).filter(Boolean)
  const allInCount = normalized.match(/梭哈/g)?.length ?? 0
  if (allInCount > 1) return '每次只能使用一次梭哈。'
  if (allInCount === 1 && segments.length !== 1) return '梭哈必须单独提交，不能与普通金额注单混合。'
  if (allInCount === 1 && !normalized.endsWith('梭哈')) return '梭哈只能填写在金额位置。'
  if (/^\d+梭哈$/.test(normalized)) return '请使用“位置/号码/梭哈”的标准格式。'
  for (const segment of segments) {
    if (!segment.includes('/')) continue
    const amount = segment.split('/').at(-1)?.trim() ?? ''
    if (amount === '梭哈') continue
    if (!/^\d+(?:\.\d{1,2})?$/.test(amount)) return '金额须大于0，最多2位小数；不会自动四舍五入。'
  }
  return ''
}
