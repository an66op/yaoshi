import { beforeEach, describe, expect, it, vi } from 'vitest'
import { betsApi } from './bets'

const transport = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('./client', () => transport)
beforeEach(() => transport.request.mockReset())

describe('period-bound betting requests', () => {
  it('preserves the confirmed next period when submitting a bet', () => {
    betsApi.assistantPlace('speed-racing', { issue: '34137174', content: '1/5/9', request_id: 'next-period-1' })
    expect(transport.request).toHaveBeenCalledWith('/member/games/speed-racing/assistant/bets', {
      method: 'POST', body: JSON.stringify({ issue: '34137174', content: '1/5/9', request_id: 'next-period-1' }),
    })
  })

  it('binds cancellation to the confirmed period instead of a later current period', () => {
    betsApi.cancelCurrent('speed-racing', '34137174')
    expect(transport.request).toHaveBeenCalledWith('/member/bets/cancel-current', {
      method: 'POST', body: JSON.stringify({ game_id: 'speed-racing', issue: '34137174' }),
    })
  })

  it('keeps callers without an explicit period valid for server-side resolution', () => {
    betsApi.cancelCurrent('speed-racing')
    expect(transport.request).toHaveBeenCalledWith('/member/bets/cancel-current', {
      method: 'POST', body: JSON.stringify({ game_id: 'speed-racing' }),
    })
  })
})
