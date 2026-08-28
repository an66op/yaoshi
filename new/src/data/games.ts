/**
 * 赛车/飞艇采用行业通用的 1—10 固定色；其它大号码循环使用同一色盘。
 * 0 单独保留玫红色，避免与 10 的绿色混淆。
 */
export const ballTone = (number: number) => {
  const normalized = Math.abs(Math.trunc(number))
  if (normalized === 0) return 'lottery-ball ball-0'
  return `lottery-ball ball-${((normalized - 1) % 10) + 1}`
}
