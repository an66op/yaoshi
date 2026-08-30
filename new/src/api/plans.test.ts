import { beforeEach, describe, expect, it, vi } from 'vitest'
import { planApi } from './plans'

const transport = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('./client', () => transport)
beforeEach(() => transport.request.mockReset())

describe('racing plan stream requests', () => {
  it('reads one exact position/type stream with a caller-owned abort signal', () => {
    const controller = new AbortController()
    planApi.racingDetail({ position: 10, plan_key: 'three-period-seven-codes' }, controller.signal)
    expect(transport.request).toHaveBeenCalledWith('/member/plans/speed-racing?position=10&plan_key=three-period-seven-codes&history_limit=6', { signal: controller.signal })
  })
  it('activates only the explicitly confirmed position/type, without a room-wide preference', () => {
    const controller = new AbortController()
    planApi.activateRacing({ position: 6, plan_key: 'dragon-tiger-three-periods' }, controller.signal)
    expect(transport.request).toHaveBeenCalledWith('/member/plans/speed-racing/activate?history_limit=6', { method: 'POST', body: '{"position":6,"plan_key":"dragon-tiger-three-periods"}', signal: controller.signal })
  })
  it('limits non-racing reads to six real periods and touches with a separate POST', () => {
    planApi.detail('canada-28')
    expect(transport.request).toHaveBeenCalledWith('/member/plans/canada-28?history_limit=6', { signal: undefined })
    const controller = new AbortController()
    planApi.activate('canada-28', controller.signal)
    expect(transport.request).toHaveBeenLastCalledWith('/member/plans/canada-28/activate?history_limit=6', { method: 'POST', body: '{}', signal: controller.signal })
  })
})
