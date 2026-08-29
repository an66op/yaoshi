import { describe, expect, it } from 'vitest'
import { resolveActivityAction } from './activityAction'

const origin = 'https://member.example.com'

describe('resolveActivityAction', () => {
  it('keeps internal activity links on the member origin', () => {
    expect(resolveActivityAction('internal', '/wallet?action=deposit', origin)).toEqual({
      kind: 'internal',
      href: '/wallet?action=deposit',
    })
    expect(resolveActivityAction('internal', '//evil.example/steal', origin)).toBeNull()
    expect(resolveActivityAction('internal', '/\\evil.example/steal', origin)).toBeNull()
  })

  it('allows only credential-free HTTPS external links', () => {
    expect(resolveActivityAction('external', 'https://help.example/path', origin)).toEqual({
      kind: 'external',
      href: 'https://help.example/path',
    })
    expect(resolveActivityAction('external', 'http://help.example/path', origin)).toBeNull()
    expect(resolveActivityAction('external', 'https://user:secret@help.example/path', origin)).toBeNull()
    expect(resolveActivityAction('external', 'javascript:alert(1)', origin)).toBeNull()
  })
})
