import { lotteryRuleProfile, type LotteryRuleFamily } from './lotteryRules'

export type RoomBettingModeID = `mode${number}`
export type RoomBettingSurface = 'chat' | 'detail'
export type DetailBoardKind = 'racing' | 'digit' | 'pc28' | 'mark-six' | 'pending'

export type RoomBettingMode = Readonly<{
  id: RoomBettingModeID
  label: string
  surface: RoomBettingSurface
  board?: DetailBoardKind
}>

export type RoomBettingAssembly = Readonly<{
  modes: readonly RoomBettingMode[]
  defaultMode: RoomBettingModeID
}>

const CHAT_MODE: RoomBettingMode = { id: 'mode1', label: '聊天', surface: 'chat' }
const detailMode = (board: DetailBoardKind): RoomBettingMode => ({ id: 'mode2', label: '网投', surface: 'detail', board })

function boardForFamily(family: LotteryRuleFamily): DetailBoardKind | null {
  if (family === 'racing') return 'racing'
  if (family === 'ssc' || family === 'digit3') return 'digit'
  if (family === 'pc28') return 'pc28'
  if (family === 'mark-six') return 'mark-six'
  return null
}

/**
 * A room is assembled from ordered betting modes instead of hard-coding one
 * keyboard and one board into GameRoom.  New mode3+ renderers can be appended
 * here without changing the chat/drawer lifecycle.
 */
export function roomBettingAssembly(gameId: string): RoomBettingAssembly {
  // Bingo Mark Six is a web-only product. It enters the dedicated board
  // directly and deliberately has no chat surface or edge mode switch.
  if (gameId === 'bingo-mark-six') {
    return { modes: [detailMode('mark-six')], defaultMode: 'mode2' }
  }

  const board = boardForFamily(lotteryRuleProfile(gameId).family)
  if (!board) return { modes: [CHAT_MODE], defaultMode: 'mode1' }
  return { modes: [CHAT_MODE, detailMode(board)], defaultMode: 'mode1' }
}

export function roomBettingMode(assembly: RoomBettingAssembly, modeId: RoomBettingModeID): RoomBettingMode {
  return assembly.modes.find(mode => mode.id === modeId)
    ?? assembly.modes.find(mode => mode.id === assembly.defaultMode)
    ?? assembly.modes[0]
    ?? CHAT_MODE
}
