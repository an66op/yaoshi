import type { Game } from '../types'

export const games: Game[] = [
  { id: 'lucky', title: '澳洲幸运5', tag: 'LUCKY 5', online: '612', period: '51341439', due: '03:24', color: '#d24fda', balls: [7, 0, 6, 0, 6] },
  { id: 'seven', title: '幸运飞艇', tag: 'LUCKY AIRSHIP', online: '888', period: '2026083121', due: '03:19', color: '#3b83ec', balls: [3, 10, 8, 9, 2, 1, 5, 4, 6, 7] },
  { id: 'racing', title: '极速赛车', tag: 'RACING', online: '778', period: '54756993', due: '00:44', color: '#244eaf', balls: [4, 6, 2, 3, 8, 1, 9, 10, 5, 7] },
  { id: 'fast', title: '极速时时彩', tag: 'LOTTO TODAY', online: '1264', period: '14086933', due: '01:15', color: '#ff8d26', balls: [7, 1, 4, 7, 8] },
]

export const ballTone = (number: number) => ['coral', 'lime', 'blue', 'orange', 'violet'][number % 5]
