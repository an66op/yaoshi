import { describe, expect, it } from 'vitest'
import { isRetiredAccountPath, resolveAdminPath } from './adminRoutes'

describe('management route normalization', () => {
  it.each(['/users', '/users/'])('redirects the retired %s account page to member management', path => {
    expect(isRetiredAccountPath(path)).toBe(true)
    expect(resolveAdminPath(path)).toBe('/members')
  })

  it.each(['/', '/members', '/agents', '/tenants', '/menu-management', '/game-guide', '/applications'])('retains %s', path => {
    expect(isRetiredAccountPath(path)).toBe(false)
    expect(resolveAdminPath(path)).toBe(path)
  })

  it.each(['/users/1', '/unknown', 'https://untrusted.example/users', '//untrusted.example'])('safely falls back for unsupported path %s', path => {
    expect(resolveAdminPath(path)).toBe('/')
  })
})
