import { describe, expect, it, vi } from 'vitest'
import { chatApi } from './chat'
const transport = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('./client', () => transport)

describe('visit-bound message requests', () => {
  it('sends a timestamp plus an incremental cursor without a workspace override', () => {
    chatApi.messages('group', 'speed-racing', 50, { since: '2026-08-30T12:34:00.000Z', after_id: 120 })
    const url = new URL(transport.request.mock.calls.at(-1)![0], 'http://localhost')
    expect(Object.fromEntries(url.searchParams)).toEqual({ room_type: 'group', game_id: 'speed-racing', limit: '50', after_id: '120', since: '2026-08-30T12:34:00.000Z' })
  })
})
