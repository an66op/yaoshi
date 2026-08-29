import { describe, expect, it } from 'vitest'
import {
  MANAGEMENT_LOGIN_PASSWORD_MAX_BYTES,
  MANAGEMENT_LOGIN_USERNAME_MAX_RUNES,
  MANAGEMENT_LOGIN_WORKSPACE_MAX_RUNES,
  truncateCodePoints,
  utf8ByteLength,
  validateManagementLoginInput,
} from './loginLimits'

describe('management login limits', () => {
  it('accepts the full provisioned account and bcrypt boundaries', () => {
    expect(validateManagementLoginInput('a'.repeat(MANAGEMENT_LOGIN_USERNAME_MAX_RUNES), 'a'.repeat(MANAGEMENT_LOGIN_PASSWORD_MAX_BYTES), '平台')).toBe('')
  })

  it('counts account and workspace limits as Unicode code points', () => {
    expect(validateManagementLoginInput('王'.repeat(51), 'abcdefgh', '平台')).toContain('50')
    expect(validateManagementLoginInput('admin', 'abcdefgh', '王'.repeat(MANAGEMENT_LOGIN_WORKSPACE_MAX_RUNES + 1))).toContain('80')
    const astralAccount = '🚀'.repeat(MANAGEMENT_LOGIN_USERNAME_MAX_RUNES)
    expect(astralAccount.length).toBe(MANAGEMENT_LOGIN_USERNAME_MAX_RUNES * 2)
    expect(validateManagementLoginInput(astralAccount, 'abcdefgh', '平台')).toBe('')
    expect(truncateCodePoints(`${astralAccount}🚀`, MANAGEMENT_LOGIN_USERNAME_MAX_RUNES)).toBe(astralAccount)
  })

  it('enforces the bcrypt limit in UTF-8 bytes', () => {
    expect(utf8ByteLength('王者密码')).toBe(12)
    expect(validateManagementLoginInput('admin', '王'.repeat(24), '平台')).toBe('')
    expect(validateManagementLoginInput('admin', '王'.repeat(25), '平台')).toContain('72')
    expect(validateManagementLoginInput('admin', '1234567', '平台')).toContain('8')
  })
})
