import { describe, expect, it, vi } from 'vitest'
import { lotteryApi } from './lottery'

const transport = vi.hoisted(() => ({ request: vi.fn(), publicRequest: vi.fn() }))
vi.mock('./client', () => transport)

describe('lottery draw request cancellation', () => {
  it('forwards the request-wide signal through the public transport', () => {
    const controller = new AbortController()
    lotteryApi.draws('speed-racing', 50, controller.signal)
    expect(transport.publicRequest).toHaveBeenLastCalledWith('/public/lottery/games/speed-racing/draws?limit=50', { signal: controller.signal })
  })

  it('keeps existing draw consumers valid without an external signal', () => {
    lotteryApi.draws('speed-fly')
    expect(transport.publicRequest).toHaveBeenLastCalledWith('/public/lottery/games/speed-fly/draws?limit=30', undefined)
  })
})
