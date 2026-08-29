export const LEGACY_ADMIN_TOKEN_KEY = 'yaotu-admin-token'
export const LEGACY_ADMIN_USER_KEY = 'yaotu-admin-user'
export const ADMIN_AUTH_EVENT_KEY = 'yaotu-admin-auth-event'

type StorageLike = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>

export type AuthUser = {
  id: number
  username: string
  email: string
  nickname: string
  role: string
  status: number
}

// The authenticated profile is deliberately process-local. Authorization is
// carried by the HttpOnly cookie and revalidated through /api/session after
// every reload; storing this object in localStorage would let stale role data
// survive logout or an account permission change.
let currentUser: AuthUser | null = null

export function setCurrentUser(user: AuthUser | null) {
  currentUser = user
}

/** @deprecated Name retained for existing panels; this never reads storage. */
export function getStoredUser() {
  return currentUser
}

/** Remove credentials left by pre-cookie versions without touching UI prefs. */
export function clearLegacyAdminSession(storage: StorageLike = window.localStorage) {
  try {
    storage.removeItem(LEGACY_ADMIN_TOKEN_KEY)
    storage.removeItem(LEGACY_ADMIN_USER_KEY)
  } catch {
    // Browser policy may disable storage; cookie logout remains authoritative.
  }
}

/** A timestamp-only event synchronizes logout across tabs; it grants no access. */
export function broadcastAdminLogout(storage: StorageLike = window.localStorage) {
  setCurrentUser(null)
  clearLegacyAdminSession(storage)
  try {
    storage.setItem(ADMIN_AUTH_EVENT_KEY, JSON.stringify({ type: 'logout', at: Date.now() }))
  } catch {
    // Storage may be disabled by browser policy; local logout still succeeds.
  }
}
