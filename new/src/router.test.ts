import { describe, expect, it } from 'vitest'
import { parseRoute, pathForGame, pathForGameGuide, pathForLobby, pathForResults } from './router'

describe('results route', () => {
  it('keeps the originating game separate from the selected result game', () => {
    const path = pathForResults('speed-racing', 'speed-fly')

    expect(path).toBe('/results?game=speed-racing&from_game=speed-fly')
    expect(parseRoute(path)).toEqual({
      kind: 'results',
      gameId: 'speed-racing',
      returnGameId: 'speed-fly',
    })
  })
})

describe('lobby navigation history', () => {
  it('keeps the selected lobby category in both the lobby and game routes', () => {
    const lobbyPath = pathForLobby('宾果')
    const gamePath = pathForGame('bingo-mark-six', false, '宾果')

    expect(lobbyPath).toBe('/lobby?category=%E5%AE%BE%E6%9E%9C')
    expect(parseRoute(lobbyPath)).toEqual({ kind: 'tab', tab: 'lobby', lobbyFilter: '宾果' })
    expect(gamePath).toBe('/games/bingo-mark-six?from_lobby=%E5%AE%BE%E6%9E%9C')
    expect(parseRoute(gamePath)).toEqual({ kind: 'game', gameId: 'bingo-mark-six', quickMenu: false, returnLobbyFilter: '宾果' })
  })

  it('preserves quick-menu and source category without ambiguous query ordering', () => {
    const path = pathForGame('speed-racing', true, '彩票')
    expect(path).toBe('/games/speed-racing?quick_menu=1&from_lobby=%E5%BD%A9%E7%A5%A8')
    expect(parseRoute(path)).toEqual({ kind: 'game', gameId: 'speed-racing', quickMenu: true, returnLobbyFilter: '彩票' })
  })
})

describe('game guide route', () => {
  it('opens rules and odds as independent pages', () => {
    expect(parseRoute(pathForGameGuide('rules'))).toEqual({ kind: 'game-guide', tab: 'rules' })
    expect(parseRoute(pathForGameGuide('odds'))).toEqual({ kind: 'game-guide', tab: 'odds' })
  })
})
