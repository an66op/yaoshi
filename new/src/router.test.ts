import { describe, expect, it } from 'vitest'
import { parseRoute, pathForResults } from './router'

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
