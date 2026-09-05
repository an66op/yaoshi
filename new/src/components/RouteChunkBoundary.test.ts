import { describe, expect, it, vi } from 'vitest'
import { isRouteChunkLoadError, tryReloadStaleRouteChunk } from '../utils/routeChunkRecovery'

function memoryStorage(initial = '') {
  let value = initial
  return {
    getItem: vi.fn(() => value || null),
    setItem: vi.fn((_key: string, next: string) => { value = next }),
  }
}

describe('route chunk release recovery', () => {
  it.each([
    new Error('Failed to fetch dynamically imported module: /assets/Lobby-old.js'),
    Object.assign(new Error('Loading chunk 17 failed'), { name: 'ChunkLoadError' }),
    new TypeError('error loading dynamically imported module'),
    new Error('Importing a module script failed'),
  ])('recognizes stale lazy imports', error => {
    expect(isRouteChunkLoadError(error)).toBe(true)
  })

  it('does not mistake an ordinary render failure for a stale release', () => {
    expect(isRouteChunkLoadError(new Error('Cannot read properties of undefined'))).toBe(false)
  })

  it('reloads a failed route only once until a chunk successfully renders', () => {
    const storage = memoryStorage()
    const reload = vi.fn()
    const error = new Error('Failed to fetch dynamically imported module')
    const runtime = { currentPath: '/game/speed-racing?quick=1', reload, storage }

    expect(tryReloadStaleRouteChunk(error, runtime)).toBe(true)
    expect(reload).toHaveBeenCalledOnce()
    expect(tryReloadStaleRouteChunk(error, runtime)).toBe(false)
    expect(reload).toHaveBeenCalledOnce()
  })

  it('fails safely when session storage is unavailable', () => {
    const reload = vi.fn()
    expect(tryReloadStaleRouteChunk(new Error('Loading chunk 1 failed'), {
      currentPath: '/lobby',
      reload,
      storage: {
        getItem: () => { throw new Error('blocked') },
        setItem: vi.fn(),
      },
    })).toBe(false)
    expect(reload).not.toHaveBeenCalled()
  })
})
