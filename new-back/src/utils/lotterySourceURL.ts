export const DEFAULT_LOTTERY_SOURCE_URL = 'https://www.www-163kai.cc/mobile.html'

/** Match the backend boundary before a settings save reaches the network. */
export function normalizeLotterySourceURL(value: unknown): string | null {
  if (typeof value !== 'string') return null
  const candidate = value.trim() || DEFAULT_LOTTERY_SOURCE_URL
  if (candidate.length > 2048) return null
  try {
    const target = new URL(candidate)
    if (target.protocol !== 'https:' || !target.hostname || target.username || target.password) return null
    return target.href
  } catch {
    return null
  }
}

export function isValidLotterySourceURL(value: unknown) {
  return normalizeLotterySourceURL(value) !== null
}
