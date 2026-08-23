// A phone on the development LAN must call this computer's backend, not its
// own localhost. Production deployments continue to provide VITE_API_BASE_URL.
const apiBase = (() => {
  const configured = String(import.meta.env.VITE_API_BASE_URL ?? '').trim()
  if (configured) return configured.replace(/\/$/, '')
  return `${window.location.protocol}//${window.location.hostname}:8080/api`
})()

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
  const controller = init?.signal ? undefined : new AbortController()
  const timeout = controller ? window.setTimeout(() => controller.abort(), 15_000) : undefined
  let response: Response
  try {
    response = await fetch(`${apiBase}${path}`, {
      ...init,
      signal: init?.signal ?? controller?.signal,
      headers: { 'Content-Type': 'application/json', ...authHeaders(), ...init?.headers },
    })
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') throw new Error('请求超时，请稍后重试')
    throw error
  } finally {
    if (timeout !== undefined) window.clearTimeout(timeout)
  }
  const raw = await response.text()
  let body: ApiResponse<T>
  try { body = JSON.parse(raw) as ApiResponse<T> } catch { throw new Error('服务返回了无效响应') }
  if (response.status === 401) {
    clearToken()
    window.dispatchEvent(new CustomEvent('yaotu-member-auth-expired'))
    throw new AuthError(body.message || '请先登录')
  }
  if (!response.ok) throw new Error(body.message || '服务暂时不可用')
  return body.data
}

export async function publicRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const controller = init?.signal ? undefined : new AbortController()
  const timeout = controller ? window.setTimeout(() => controller.abort(), 15_000) : undefined
  let response: Response
  try {
    response = await fetch(`${apiBase}${path}`, {
      ...init,
      signal: init?.signal ?? controller?.signal,
      headers: { 'Content-Type': 'application/json', ...init?.headers },
    })
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') throw new Error('请求超时，请稍后重试')
    throw error
  } finally {
    if (timeout !== undefined) window.clearTimeout(timeout)
  }
  const raw = await response.text()
  let body: ApiResponse<T>
  try { body = JSON.parse(raw) as ApiResponse<T> } catch { throw new Error('服务返回了无效响应') }
  if (!response.ok) throw new Error(body.message || '服务暂时不可用')
  return body.data
}

export { apiBase }
