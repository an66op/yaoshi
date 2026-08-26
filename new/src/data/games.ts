// 用户端彩票白名单。后台可以继续保留其他数据源和彩种，但只有这里
// 明确列出的彩种能进入大厅、开奖结果选择器和游戏路由。
export const memberLotteryGameIds = [
  'speed-racing',
  'speed-fly',
  'speed-ssc',
  'sg-fly',
  'sg-ssc',
  'fly-racing',
  'au-lucky-5',
  'au-lucky-10',
  'pc-canada',
  'canada-28',
  'canada-20',
  'bingo-mark-six',
  'bingo-racing-a',
  'bingo-racing-b',
  'bingo-ssc-1',
  'bingo-ssc-2',
  'bingo-ssc-3',
  'bingo-ssc-4',
  'hong-kong-mark-six',
  'happy8-mark-six',
  'new-macau-mark-six',
  'old-macau-mark-six',
] as const

/**
 * 赛车/飞艇采用行业通用的 1—10 固定色；其它大号码循环使用同一色盘。
 * 0 单独保留玫红色，避免与 10 的绿色混淆。
 */
export const ballTone = (number: number) => {
  const normalized = Math.abs(Math.trunc(number))
  if (normalized === 0) return 'lottery-ball ball-0'
  return `lottery-ball ball-${((normalized - 1) % 10) + 1}`
}
