import { describe, expect, it } from 'vitest'
import {
  MEMBER_LOGIN_USERNAME_MAX_LENGTH,
  MEMBER_REGISTER_USERNAME_MAX_LENGTH,
  PASSWORD_MAX_BYTES,
  passwordByteLength,
  truncateUnicode,
  unicodeLength,
  validPasswordByteLength,
} from './authLimits'

describe('member authentication limits', () => {
  it('accepts every username length that an administrator may provision', () => {
    expect(MEMBER_LOGIN_USERNAME_MAX_LENGTH).toBe(50)
    expect(MEMBER_LOGIN_USERNAME_MAX_LENGTH).toBeGreaterThan(MEMBER_REGISTER_USERNAME_MAX_LENGTH)
  })

  it('measures the bcrypt boundary in bytes rather than characters', () => {
    expect(passwordByteLength('abcdefgh')).toBe(8)
    expect(passwordByteLength('王者密码')).toBe(12)
    expect(validPasswordByteLength('abcdefgh')).toBe(true)
    expect(validPasswordByteLength('a'.repeat(PASSWORD_MAX_BYTES))).toBe(true)
    expect(validPasswordByteLength('a'.repeat(PASSWORD_MAX_BYTES + 1))).toBe(false)
    expect(validPasswordByteLength('王'.repeat(25))).toBe(false)
  })

  it('counts and truncates astral characters by Unicode code point', () => {
    const account = '🚀'.repeat(MEMBER_LOGIN_USERNAME_MAX_LENGTH)
    expect(account.length).toBe(MEMBER_LOGIN_USERNAME_MAX_LENGTH * 2)
    expect(unicodeLength(account)).toBe(MEMBER_LOGIN_USERNAME_MAX_LENGTH)
    expect(truncateUnicode(`${account}🚀`, MEMBER_LOGIN_USERNAME_MAX_LENGTH)).toBe(account)
  })
})
