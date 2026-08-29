import { broadcastMemberLogout } from '../utils/businessStorage'

// A phone on the development LAN must call this computer's backend, not its
// own localhost. Production is served through Nginx on the same origin, where
// `/api` is proxied to the Go service. Keep the result absolute because the
// WebSocket client derives its ws(s) URL from this value.
const apiBase = (() => {
  const configured = String(import.meta.env.VITE_API_BASE_URL ?? '').trim()
  if (configured) return configured.replace(/\/$/, '')
  if (import.meta.env.DEV) return `${window.location.protocol}//${window.location.hostname}:8080/api`
  return `${window.location.origin}/api`
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

function responseErrorMessage(response: Response, value: unknown, fallback: string) {
  // Never render database/proxy diagnostics returned by an unexpected server
  // failure. Validation and rate-limit messages remain available to users.
  if (response.status >= 500) return fallback
  if (typeof value !== 'string') return fallback
  const message = [...value].map(character => {
    const code = character.charCodeAt(0)
    return code < 32 || code === 127 ? ' ' : character
  }).join('').trim()
  return message ? message.slice(0, 240) : fallback
}

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const controller = init?.signal ? undefined : new AbortController()
  const timeout = controller ? window.setTimeout(() => controller.abort(), 15_000) : undefined
  let response: Response
  try {
    response = await fetch(`${apiBase}${path}`, {
      ...init,
	  credentials: 'include',
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
  try {
    body = JSON.parse(raw) as ApiResponse<T>
  } catch {
    if (response.status === 401) {
      broadcastMemberLogout()
      window.dispatchEvent(new CustomEvent('yaotu-member-auth-expired'))
      throw new AuthError('登录状态已失效，请重新登录')
    }
    throw new Error('服务返回了无效响应')
  }
  if (response.status === 401) {
    broadcastMemberLogout()
    window.dispatchEvent(new CustomEvent('yaotu-member-auth-expired'))
    throw new AuthError(responseErrorMessage(response, body.message, '请先登录'))
  }
  if (!response.ok) throw new Error(responseErrorMessage(response, body.message, '服务暂时不可用'))
  return body.data
}

export async function publicRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const controller = init?.signal ? undefined : new AbortController()
  const timeout = controller ? window.setTimeout(() => controller.abort(), 15_000) : undefined
  let response: Response
  try {
    response = await fetch(`${apiBase}${path}`, {
      ...init,
	  credentials: 'include',
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
  if (!response.ok) throw new Error(responseErrorMessage(response, body.message, '服务暂时不可用'))
  return body.data
}

export { apiBase }
