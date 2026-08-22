const TOKEN_KEY = 'yaotu-admin-token'
const USER_KEY = 'yaotu-admin-user'

export type AuthUser = {
  id: number
  username: string
  email: string
  nickname: string
  role: string
  status: number
}

export type AuthSession = {
  token: string
  user: AuthUser
}

export function getToken() {
  return window.localStorage.getItem(TOKEN_KEY) ?? ''
}

export function getStoredUser(): AuthUser | null {
  const raw = window.localStorage.getItem(USER_KEY)
  if (!raw) return null
  try {
    return JSON.parse(raw) as AuthUser
  } catch {
    return null
  }
}

export function saveSession(session: AuthSession) {
  window.localStorage.setItem(TOKEN_KEY, session.token)
  window.localStorage.setItem(USER_KEY, JSON.stringify(session.user))
}

export function clearSession() {
  window.localStorage.removeItem(TOKEN_KEY)
  window.localStorage.removeItem(USER_KEY)
}

export function readSession(): AuthSession | null {
  const token = getToken()
  const user = getStoredUser()
  if (!token || !user) return null
  return { token, user }
}
