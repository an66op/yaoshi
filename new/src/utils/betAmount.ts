/** Compact stakes only. Never truncate real cents or change accounting values. */
export function formatBetAmount(value: number): string {
  return value.toFixed(2).replace(/\.00$/, '')
}
