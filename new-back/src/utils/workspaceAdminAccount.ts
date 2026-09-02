import { validateManagementLoginInput } from '../loginLimits'

export type WorkspaceAdminRole = 'tenant' | 'agent'
export type CreatedWorkspaceAdmin = {
  role: WorkspaceAdminRole
  username: string
  roomCode: string
  status: number
}

export const workspaceAdminRoleLabel = (role: WorkspaceAdminRole) => role === 'tenant' ? '租户' : '代理'

export function validateWorkspaceAdminAccount(username: string, password: string): string {
  const account = username.trim()
  if (Array.from(account).length < 3) return '登录账号需为 3–50 个字符'
  if (/^room_(robot|activity)_/i.test(account)) return '该账号前缀为系统机器人保留，请更换登录账号'
  return validateManagementLoginInput(account, password)
}

/** Keep only non-secret details in the post-creation confirmation. */
export function createdWorkspaceAdmin(role: WorkspaceAdminRole, account: { username: string; room_code: string; status: number }): CreatedWorkspaceAdmin {
  return { role, username: account.username, roomCode: account.room_code, status: account.status }
}

export function managementLoginURL(origin?: string): string {
  return origin ? new URL('/login', origin).href : '/login'
}
