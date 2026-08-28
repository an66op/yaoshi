import { describe, expect, it } from 'vitest'
import { createRequestId } from './requestId'

describe('createRequestId', () => {
  it('works without secure-context crypto methods', () => {
    expect(createRequestId({})).toMatch(/^[a-z0-9]+-[a-z0-9]+-[a-z0-9]+$/)
  })
})
