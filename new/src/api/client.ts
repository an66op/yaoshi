const apiBase = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080/api'

export type ApiResponse<T> = {
  code: number
  message: string
  data: T
}

export class AuthError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'AuthError'
  }
}

const TOKEN_KEY = 'yaotu-member-token'

export function getToken() {
  return window.localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string) {
  window.localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken() {
  window.localStorage.removeItem(TOKEN_KEY)
}

function authHeaders(): Record<string, string> {
  const token = getToken()
  return token ? { Authorization: `Bearer ${token}` } : {}
}

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...authHeaders(), ...init?.headers },
  })
  const body = (await response.json()) as ApiResponse<T>
  if (response.status === 401) {
    clearToken()
    window.dispatchEvent(new CustomEvent('yaotu-member-auth-expired'))
    throw new AuthError(body.message || '请先登录')
  }
  if (!response.ok) throw new Error(body.message || '服务暂时不可用')
  return body.data
}

export async function publicRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  })
  const body = (await response.json()) as ApiResponse<T>
  if (!response.ok) throw new Error(body.message || '服务暂时不可用')
  return body.data
}

export { apiBase }
