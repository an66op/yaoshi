export const MANAGEMENT_LOGIN_USERNAME_MAX_RUNES = 50
export const MANAGEMENT_LOGIN_PASSWORD_MIN_BYTES = 8
export const MANAGEMENT_LOGIN_PASSWORD_MAX_BYTES = 72
export const MANAGEMENT_LOGIN_WORKSPACE_MAX_RUNES = 80

export function utf8ByteLength(value: string): number {
  return new TextEncoder().encode(value).length
}

export function truncateCodePoints(value: string, maxLength: number): string {
  return Array.from(value).slice(0, maxLength).join('')
}

export function validateManagementLoginInput(username: string, password: string, workspace = ''): string {
  const account = username.trim()
  if (!account) return '请输入登录帐号'
  if (Array.from(username).length > MANAGEMENT_LOGIN_USERNAME_MAX_RUNES) {
    return `登录帐号最多 ${MANAGEMENT_LOGIN_USERNAME_MAX_RUNES} 个字符`
  }

  const passwordBytes = utf8ByteLength(password)
  if (passwordBytes < MANAGEMENT_LOGIN_PASSWORD_MIN_BYTES) {
    return `密码至少 ${MANAGEMENT_LOGIN_PASSWORD_MIN_BYTES} 字节`
  }
  if (passwordBytes > MANAGEMENT_LOGIN_PASSWORD_MAX_BYTES) {
    return `密码最多 ${MANAGEMENT_LOGIN_PASSWORD_MAX_BYTES} 字节`
  }

  if (Array.from(workspace).length > MANAGEMENT_LOGIN_WORKSPACE_MAX_RUNES) {
    return `所属工作区最多 ${MANAGEMENT_LOGIN_WORKSPACE_MAX_RUNES} 个字符`
  }
  return ''
}
