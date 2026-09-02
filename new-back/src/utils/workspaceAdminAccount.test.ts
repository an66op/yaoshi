import { describe, expect, it } from 'vitest'
import { createdWorkspaceAdmin, managementLoginURL, validateWorkspaceAdminAccount, workspaceAdminRoleLabel } from './workspaceAdminAccount'

describe('workspace administrator account validation', () => {
  it.each(['abc', 'a'.repeat(50), '管'.repeat(50), '😀'.repeat(50)])('accepts a 3–50 code-point account (%s)', username => {
    expect(validateWorkspaceAdminAccount(username, 'password-123')).toBe('')
  })

  it.each(['', '  ', 'ab', '😀😀', 'a'.repeat(51), '管'.repeat(51)])('rejects an account outside the creation boundary (%s)', username => {
    expect(validateWorkspaceAdminAccount(username, 'password-123')).not.toBe('')
  })

  it('validates the same trimmed account that will be submitted', () => {
    expect(validateWorkspaceAdminAccount(`  ${'a'.repeat(50)}  `, 'password-123')).toBe('')
    expect(validateWorkspaceAdminAccount(' ab ', 'password-123')).not.toBe('')
  })

  it.each(['room_robot_owner', 'ROOM_ROBOT_OWNER', ' room_activity_owner '])('reserves system-account prefixes (%s)', username => {
    expect(validateWorkspaceAdminAccount(username, 'password-123')).toContain('保留')
  })

  it.each(['12345678', 'a'.repeat(72), '密'.repeat(24), '😀'.repeat(18)])('accepts 8–72 UTF-8 password bytes', password => {
    expect(validateWorkspaceAdminAccount('new-admin', password)).toBe('')
  })

  it.each(['', '1234567', '密密', 'a'.repeat(73), '密'.repeat(25), '😀'.repeat(19)])('rejects invalid password byte lengths', password => {
    expect(validateWorkspaceAdminAccount('new-admin', password)).toMatch(/密码.*字节/)
  })
})

describe('workspace administrator creation confirmation', () => {
  it.each(['tenant', 'agent'] as const)('keeps only returned, non-secret %s account details', role => {
    const response = { username: 'api-canonical-account', room_code: '987654', status: 0, password: 'private-initial-password', token: 'private-session', balance: 10 }
    const summary = createdWorkspaceAdmin(role, response)
    expect(summary).toEqual({ role, username: response.username, roomCode: response.room_code, status: 0 })
    expect(JSON.stringify(summary)).not.toMatch(/password|token|private/)
    response.username = 'later-response-mutation'
    expect(summary.username).toBe('api-canonical-account')
  })

  it('labels tenant and agent administrators separately', () => {
    expect(workspaceAdminRoleLabel('tenant')).toBe('租户')
    expect(workspaceAdminRoleLabel('agent')).toBe('代理')
  })

  it.each(['http://127.0.0.1:5174', 'https://admin.example.test', 'https://management.example.test:8443'])('uses the current management origin (%s)', origin => {
    expect(managementLoginURL(origin)).toBe(`${origin}/login`)
  })

  it('uses a relative management login link when rendering without a browser', () => {
    expect(managementLoginURL()).toBe('/login')
  })
})
