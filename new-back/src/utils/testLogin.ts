import type { DevLoginPreset, LoginIdentity } from '../devLoginPresets'
import { validateManagementLoginInput } from '../loginLimits'

export type ManagementTestLogins = Partial<Record<LoginIdentity, DevLoginPreset>>

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function parseTestLogin(value: unknown): ManagementTestLogins | undefined {
  if (!record(value) || value.enabled !== true || !record(value.profiles)) return
  const profiles: ManagementTestLogins = {}
  for (const identity of ['platform', 'tenant', 'agent'] as const) {
    const profile = value.profiles[identity]
    if (profile === undefined) continue
    if (!record(profile)) return
    const { username, password, workspace } = profile
    if (typeof username !== 'string' || typeof password !== 'string' || typeof workspace !== 'string' || !workspace.trim()) return
    if (validateManagementLoginInput(username, password, workspace)) return
    profiles[identity] = { username: username.trim(), password, workspace: workspace.trim() }
  }
  return Object.keys(profiles).length ? profiles : undefined
}

// Served only on an explicitly enabled test installation. Normal builds contain
// no runtime test secrets and a missing file leaves ordinary sign-in unchanged.
export async function loadTestLogin(signal: AbortSignal): Promise<ManagementTestLogins | undefined> {
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
    // Invalid/disabled test configuration must not interrupt manual login.
    return undefined
  } finally {
    clearTimeout(timer)
    signal.removeEventListener('abort', abort)
  }
}
