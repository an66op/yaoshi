export const DEFAULT_LOTTERY_SOURCE_URL = 'https://www.www-163kai.cc/mobile.html'
export const SG_SSC_LOTTERY_SOURCE_URL = 'https://pkk168.com/webapp/html/shishicai_sg/index.html'

/** Defense in depth for configuration returned by an older or modified API. */
export function resolveLotterySourceURL(value: unknown, fallback = DEFAULT_LOTTERY_SOURCE_URL): string {
  if (typeof value !== 'string') return fallback
  const candidate = value.trim() || fallback
  if (candidate.length > 2048) return fallback
  try {
    const target = new URL(candidate)
    if (target.protocol !== 'https:' || !target.hostname || target.username || target.password) return fallback
    return target.href
  } catch {
    return fallback
  }
}

/** SG has a fixed verified feed; a room shortcut must not impersonate it. */
export function resolveGameLotterySourceURL(game: { id: string; sourceURL?: string }, roomURL: unknown): string {
  return game.id === 'sg-ssc'
    ? resolveLotterySourceURL(game.sourceURL, SG_SSC_LOTTERY_SOURCE_URL)
    : resolveLotterySourceURL(roomURL)
}
