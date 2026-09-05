import { describe, expect, it } from 'vitest'
import { preferredRoomBettingMode, roomBettingAssembly, roomBettingMode } from './gameBettingModes'

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
    expect(roomBettingAssembly('bingo-racing-b').modes[1]?.board).toBe('racing')
    for (const id of ['bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4']) expect(roomBettingAssembly(id).modes[1]?.board).toBe('digit')
    for (const id of ['pc-canada', 'canada-28', 'canada-20']) {
      expect(roomBettingAssembly(id)).toEqual({
        modes: [
          { id: 'mode1', label: '聊天', surface: 'chat' },
          { id: 'mode2', label: '网投', surface: 'detail', board: 'pc28' },
        ],
        defaultMode: 'mode1',
      })
    }
  })

  it.each(['bingo-mark-six', 'hong-kong-mark-six', 'happy8-mark-six', 'new-macau-mark-six', 'old-macau-mark-six'])('opens %s directly in its only web board mode', id => {
    const assembly = roomBettingAssembly(id)
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

  it('uses the saved surface when available and falls back to the only real mode', () => {
    const dual = roomBettingAssembly('speed-racing')
    expect(preferredRoomBettingMode(dual, 'chat')).toBe('mode1')
    expect(preferredRoomBettingMode(dual, 'detail')).toBe('mode2')
    expect(preferredRoomBettingMode(roomBettingAssembly('unverified-game'), 'detail')).toBe('mode1')
    expect(preferredRoomBettingMode(roomBettingAssembly('bingo-mark-six'), 'chat')).toBe('mode2')
  })
})
