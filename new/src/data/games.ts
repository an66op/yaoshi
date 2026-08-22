import type { Game } from '../types'

/** 离线回退：与后端目标彩种 ID 对齐的 11 款演示数据。 */
export const games: Game[] = [
  { id: 'bingo-ssc-1', title: '宾果时时彩(一)', tag: '时时彩', online: '128', period: '20260822001', due: '04:58', color: '#ff8d26', balls: [3, 7, 1, 9, 4] },
  { id: 'bingo-ssc-2', title: '宾果时时彩(二)', tag: '时时彩', online: '96', period: '20260822001', due: '04:52', color: '#ff8d26', balls: [2, 8, 0, 5, 6] },
  { id: 'bingo-racing-a', title: '宾果赛车(A)', tag: '赛车', online: '210', period: '20260822001', due: '02:18', color: '#244eaf', balls: [4, 6, 2, 3, 8, 1, 9, 10, 5, 7] },
  { id: 'bingo-racing-b', title: '宾果赛车(B)', tag: '赛车', online: '188', period: '20260822001', due: '02:11', color: '#244eaf', balls: [7, 3, 9, 1, 5, 8, 2, 10, 4, 6] },
  { id: 'bingo-mark-six', title: '宾果六合彩', tag: '六合彩', online: '356', period: '20260822001', due: '08:44', color: '#3b83ec', balls: [12, 18, 24, 31, 42, 7] },
  { id: 'hong-kong-mark-six', title: '香港六合彩', tag: '六合彩', online: '512', period: '20260822001', due: '12:30', color: '#ffffff', balls: [5, 11, 19, 28, 36, 44, 9] },
  { id: 'happy8-mark-six', title: '快乐8六合彩', tag: '六合彩', online: '142', period: '20260822001', due: '09:05', color: '#2eaf7b', balls: [8, 14, 21, 27, 33, 41, 2] },
  { id: 'new-macau-mark-six', title: '新澳门六合彩', tag: '六合彩', online: '298', period: '20260822001', due: '10:18', color: '#d69a32', balls: [6, 13, 22, 29, 37, 45, 16] },
  { id: 'old-macau-mark-six', title: '老澳门六合彩', tag: '六合彩', online: '276', period: '20260822001', due: '10:22', color: '#8b5a2b', balls: [4, 10, 17, 26, 35, 43, 11] },
  { id: 'bingo-ssc-3', title: '宾果时时彩(三)', tag: '时时彩', online: '84', period: '20260822001', due: '03:47', color: '#ff8d26', balls: [1, 5, 8, 2, 9] },
  { id: 'bingo-ssc-4', title: '宾果时时彩(四)', tag: '时时彩', online: '73', period: '20260822001', due: '03:39', color: '#ff8d26', balls: [0, 6, 4, 7, 3] },
]

export const ballTone = (number: number) => ['coral', 'lime', 'blue', 'orange', 'violet'][number % 5]
