/** An unavailable source deadline is unknown, not a completed countdown. */
export function chatCountdownText(target: string | undefined, now: number): string {
  const targetTime = new Date(target || '').getTime()
  if (!Number.isFinite(targetTime) || targetTime <= 0 || !Number.isFinite(now) || now <= 0) return '--:--'
  const seconds = Math.max(0, Math.ceil((targetTime - now) / 1000))
  return `${String(Math.floor(seconds / 60)).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`
}
