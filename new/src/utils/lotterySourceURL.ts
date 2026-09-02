export const DEFAULT_LOTTERY_SOURCE_URL = 'https://www.www-163kai.cc/mobile.html'

/** Defense in depth for configuration returned by an older or modified API. */
export function resolveLotterySourceURL(value: unknown): string {
  if (typeof value !== 'string') return DEFAULT_LOTTERY_SOURCE_URL
  const candidate = value.trim() || DEFAULT_LOTTERY_SOURCE_URL
  if (candidate.length > 2048) return DEFAULT_LOTTERY_SOURCE_URL
  try {
    const target = new URL(candidate)
    if (target.protocol !== 'https:' || !target.hostname || target.username || target.password) return DEFAULT_LOTTERY_SOURCE_URL
    return target.href
  } catch {
    return DEFAULT_LOTTERY_SOURCE_URL
  }
}
