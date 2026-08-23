import type { Game } from '../types'

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

/** 离线回退与后台开放目录保持一致，接口恢复后会立即切回服务端数据。 */
export const games: Game[] = [
  { id: 'speed-racing', title: '极速赛车', tag: '赛车', category: '赛车', online: '186', period: '34129131', due: '02:58', color: '#e85656', balls: [4, 9, 2, 1, 3, 7, 5, 10, 8, 6] },
  { id: 'speed-fly', title: '极速飞艇', tag: '飞艇', category: '飞艇', online: '168', period: '34129131', due: '02:58', color: '#3389c8', balls: [5, 1, 8, 2, 10, 4, 7, 3, 9, 6] },
  { id: 'speed-ssc', title: '极速时时彩', tag: '时时彩', category: '时时彩', online: '152', period: '34129131', due: '02:58', color: '#ef8a3c', balls: [8, 2, 6, 1, 9] },
  { id: 'sg-fly', title: 'SG飞艇', tag: '飞艇', category: '飞艇', online: '144', period: '20260822001', due: '04:58', color: '#3389c8', balls: [3, 6, 1, 8, 5, 2, 10, 4, 7, 9] },
  { id: 'sg-ssc', title: 'SG时时彩', tag: '时时彩', category: '时时彩', online: '138', period: '20260822001', due: '04:58', color: '#ef8a3c', balls: [3, 7, 1, 9, 4] },
  { id: 'fly-racing', title: '幸运飞艇', tag: '飞艇', category: '飞艇', online: '161', period: '20260822001', due: '04:58', color: '#3389c8', balls: [9, 1, 6, 3, 8, 10, 4, 2, 5, 7] },
  { id: 'au-lucky-5', title: '澳洲幸运5', tag: '幸运5', category: '幸运5', online: '173', period: '20260822001', due: '04:58', color: '#8066ca', balls: [7, 2, 8, 4, 1] },
  { id: 'au-lucky-10', title: '澳洲幸运10', tag: '幸运10', category: '幸运10', online: '179', period: '20260822001', due: '04:58', color: '#8066ca', balls: [1, 7, 5, 10, 3, 9, 2, 6, 4, 8] },
  { id: 'pc-canada', title: 'PC加拿大', tag: 'PC', category: 'PC', online: '96', period: '20260822001', due: '03:28', color: '#20a39e', balls: [3, 7, 8] },
  { id: 'canada-28', title: '加拿大28', tag: '28', category: 'PC', online: '112', period: '20260822001', due: '03:28', color: '#8066ca', balls: [5, 9, 6] },
  { id: 'canada-20', title: '加拿大2.0', tag: '2.0', category: 'PC', online: '105', period: '20260822001', due: '01:58', color: '#3b83ec', balls: [2, 8, 7] },
  { id: 'bingo-mark-six', title: '宾果六合彩', tag: '六合彩', category: '六合彩', online: '118', period: '20260822001', due: '09:58', color: '#3b83ec', balls: [3, 8, 12, 21, 29, 36, 47] },
  { id: 'bingo-racing-a', title: '宾果赛车(A)', tag: '赛车', category: '赛车', online: '210', period: '20260822001', due: '04:58', color: '#244eaf', balls: [4, 6, 2, 3, 8, 1, 9, 10, 5, 7] },
  { id: 'bingo-racing-b', title: '宾果赛车(B)', tag: '赛车', category: '赛车', online: '192', period: '20260822001', due: '04:58', color: '#244eaf', balls: [7, 2, 9, 1, 5, 8, 3, 10, 6, 4] },
  { id: 'bingo-ssc-1', title: '宾果时时彩(一)', tag: '时时彩', category: '时时彩', online: '128', period: '20260822001', due: '04:58', color: '#ff8d26', balls: [3, 7, 1, 9, 4] },
  { id: 'bingo-ssc-2', title: '宾果时时彩(二)', tag: '时时彩', category: '时时彩', online: '126', period: '20260822001', due: '04:58', color: '#ff8d26', balls: [8, 2, 5, 4, 7] },
  { id: 'bingo-ssc-3', title: '宾果时时彩(三)', tag: '时时彩', category: '时时彩', online: '131', period: '20260822001', due: '04:58', color: '#ff8d26', balls: [6, 1, 9, 3, 5] },
  { id: 'bingo-ssc-4', title: '宾果时时彩(四)', tag: '时时彩', category: '时时彩', online: '124', period: '20260822001', due: '04:58', color: '#ff8d26', balls: [2, 4, 7, 8, 1] },
  { id: 'hong-kong-mark-six', title: '香港六合彩', tag: '六合彩', category: '六合彩', online: '165', period: '20260822001', due: '09:58', color: '#d64155', balls: [2, 9, 17, 23, 31, 42, 48] },
  { id: 'happy8-mark-six', title: '快乐8六合彩', tag: '六合彩', category: '六合彩', online: '141', period: '20260822001', due: '09:58', color: '#36a16b', balls: [5, 11, 19, 27, 33, 41, 46] },
  { id: 'new-macau-mark-six', title: '新澳门六合彩', tag: '六合彩', category: '六合彩', online: '158', period: '20260822001', due: '09:58', color: '#d69a32', balls: [4, 13, 18, 25, 34, 39, 49] },
  { id: 'old-macau-mark-six', title: '老澳门六合彩', tag: '六合彩', category: '六合彩', online: '149', period: '20260822001', due: '09:58', color: '#a76f4b', balls: [1, 7, 16, 22, 30, 38, 45] },
]

export const ballTone = (number: number) => ['coral', 'lime', 'blue', 'orange', 'violet'][number % 5]
