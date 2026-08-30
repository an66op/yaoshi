import { MEMBER_LOGIN_USERNAME_MAX_LENGTH, USERNAME_MIN_LENGTH, unicodeLength, validPasswordByteLength } from '../authLimits'

export type MemberTestLogin = { username: string; password: string; room_code?: string }

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function parseTestLogin(value: unknown): MemberTestLogin | undefined {
  if (!record(value) || value.enabled !== true || !record(value.profiles) || !record(value.profiles.member)) return
  const { username, password, room_code } = value.profiles.member
  if (typeof username !== 'string' || typeof password !== 'string') return
  const account = username.trim()
  if (unicodeLength(account) < USERNAME_MIN_LENGTH || unicodeLength(account) > MEMBER_LOGIN_USERNAME_MAX_LENGTH || !validPasswordByteLength(password)) return
  if (room_code !== undefined && (typeof room_code !== 'string' || !/^\d{1,20}$/.test(room_code))) return
  return { username: account, password, ...(typeof room_code === 'string' ? { room_code } : {}) }
}

// Test credentials belong to an explicitly enabled, same-origin runtime file,
// never a production JS bundle. Missing/disabled/invalid files fail closed.
export async function loadTestLogin(signal: AbortSignal): Promise<MemberTestLogin | undefined> {
  if (signal.aborted) return
  const request = new AbortController()
  const abort = () => request.abort()
  signal.addEventListener('abort', abort, { once: true })
  const timer = setTimeout(abort, 3000)
  try {
    const response = await fetch('/test-login.json', {
      signal: request.signal, cache: 'no-store', credentials: 'omit', redirect: 'error',
      headers: { Accept: 'application/json' },
    })
    if (!response.ok || !response.headers.get('content-type')?.toLowerCase().includes('application/json')) return
    const text = await response.text()
    if (request.signal.aborted || text.length > 12_000) return
    return parseTestLogin(JSON.parse(text))
  } catch {
    // Do not log a response body or credentials, or block normal sign-in.
    return undefined
  } finally {
    clearTimeout(timer)
    signal.removeEventListener('abort', abort)
  }
}
