export const LEGACY_MEMBER_TOKEN_KEY = 'yaotu-member-token'
export const MEMBER_SESSION_KEY = 'seven-star-session'
export const MEMBER_ROOM_HISTORY_KEY = 'seven-star-room-history'
export const MEMBER_DEMO_STATE_KEY = 'seven-star-demo-state'
export const MEMBER_AUTH_EVENT_KEY = 'yaotu-member-auth-event'
export const LOGIN_ANNOUNCEMENT_PREFIX = 'wangzhe-login-announcements-shown'

type StorageLike = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>
type EnumerableStorageLike = StorageLike & Pick<Storage, 'length' | 'key'>

type StoredDemoState = {
  theme?: unknown
}

function storedTheme(storage: StorageLike): 'day' | 'night' | null {
  try {
    const parsed = JSON.parse(storage.getItem(MEMBER_DEMO_STATE_KEY) ?? 'null') as StoredDemoState | null
    return parsed?.theme === 'day' || parsed?.theme === 'night' ? parsed.theme : null
  } catch {
    return null
  }
}

/** 清除一次登录会话内的公告展示标记，不影响任何长期个人偏好。 */
export function clearLoginAnnouncementMarkers(storage: EnumerableStorageLike = window.sessionStorage) {
  try {
    for (let index = storage.length - 1; index >= 0; index -= 1) {
      const key = storage.key(index)
      if (key?.startsWith(LOGIN_ANNOUNCEMENT_PREFIX)) storage.removeItem(key)
    }
  } catch {
    // Session storage is optional in hardened/private browser contexts.
  }
}

/**
 * 清除与后端账号、房间和通知计数绑定的数据。
 *
 * 主题仍保存在原来的组合键中；字体、头像、声音、投注模式和开奖条数
 * 使用其他独立键，本函数不会读写它们。
 */
export function clearMemberBusinessStorage(
  local: StorageLike = window.localStorage,
  session: EnumerableStorageLike = window.sessionStorage,
) {
  const theme = storedTheme(local)
  // Remove tokens written by pre-cookie builds during the migration. New
  // builds never write an access token into browser storage.
  try {
    local.removeItem(LEGACY_MEMBER_TOKEN_KEY)
    local.removeItem(MEMBER_SESSION_KEY)
    local.removeItem(MEMBER_ROOM_HISTORY_KEY)
    local.removeItem(MEMBER_DEMO_STATE_KEY)
    if (theme) {
      local.setItem(MEMBER_DEMO_STATE_KEY, JSON.stringify({ theme, checkedIn: false, chatUnread: 0 }))
    }
  } catch {
    // Authentication still expires in memory when persistent storage is denied.
  }
  clearLoginAnnouncementMarkers(session)
  return { theme }
}

/**
 * Notify other tabs that the HttpOnly-backed member session is no longer
 * usable. The payload contains only an event timestamp and never credentials
 * or cached profile data.
 */
export function broadcastMemberLogout(
  local: StorageLike = window.localStorage,
  session: EnumerableStorageLike = window.sessionStorage,
) {
  const result = clearMemberBusinessStorage(local, session)
  try {
    local.setItem(MEMBER_AUTH_EVENT_KEY, JSON.stringify({ type: 'logout', at: Date.now() }))
  } catch {
    // Storage may be disabled by browser policy; local logout still succeeds.
  }
  return result
}
