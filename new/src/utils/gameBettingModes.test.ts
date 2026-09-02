import { describe, expect, it } from 'vitest'
import { roomBettingAssembly, roomBettingMode } from './gameBettingModes'

describe('roomBettingAssembly', () => {
  it('assembles chat first and a compatible detail board for verified game families', () => {
    expect(roomBettingAssembly('speed-racing')).toEqual({
      modes: [
        { id: 'mode1', label: '聊天', surface: 'chat' },
        { id: 'mode2', label: '网投', surface: 'detail', board: 'racing' },
      ],
      defaultMode: 'mode1',
    })
    expect(roomBettingAssembly('speed-ssc').modes[1]?.board).toBe('digit')
    expect(roomBettingAssembly('pc-canada')).toEqual({
      modes: [
        { id: 'mode1', label: '聊天', surface: 'chat' },
        { id: 'mode2', label: '网投', surface: 'detail', board: 'pc28' },
      ],
      defaultMode: 'mode1',
    })
  })

  it('opens Bingo Mark Six directly in its only web board mode', () => {
    const assembly = roomBettingAssembly('bingo-mark-six')
    expect(assembly).toEqual({
      modes: [{ id: 'mode2', label: '网投', surface: 'detail', board: 'mark-six' }],
      defaultMode: 'mode2',
    })
    expect(roomBettingMode(assembly, 'mode1')).toEqual(assembly.modes[0])
  })

  it('keeps unknown games chat-only rather than guessing a financial board', () => {
    expect(roomBettingAssembly('unverified-game')).toEqual({
      modes: [{ id: 'mode1', label: '聊天', surface: 'chat' }],
      defaultMode: 'mode1',
    })
  })
})
