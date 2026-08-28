import { describe, expect, it, vi } from 'vitest'
import { createRequestId } from './requestId'

describe('createRequestId', () => {
  it('uses randomUUID when the browser exposes it', () => {
    const randomUUID = vi.fn(() => 'secure-context-id')
    expect(createRequestId({ randomUUID })).toBe('secure-context-id')
    expect(randomUUID).toHaveBeenCalledOnce()
  })

  it('falls back to getRandomValues on LAN HTTP contexts', () => {
    expect(createRequestId({ getRandomValues: (bytes: Uint8Array) => { bytes.fill(1); return bytes } }))
      .toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
  })

  it('still generates an id when the browser exposes no crypto methods', () => {
    expect(createRequestId({})).toMatch(/^[a-z0-9]+-[a-z0-9]+-[a-z0-9]+$/)
  })
})
