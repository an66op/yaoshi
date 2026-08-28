export const LEGACY_MEMBER_TOKEN_KEY = 'yaotu-member-token'
export const MEMBER_SESSION_KEY = 'seven-star-session'
export const MEMBER_ROOM_HISTORY_KEY = 'seven-star-room-history'
export const MEMBER_DEMO_STATE_KEY = 'seven-star-demo-state'
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
  for (let index = storage.length - 1; index >= 0; index -= 1) {
    const key = storage.key(index)
    if (key?.startsWith(LOGIN_ANNOUNCEMENT_PREFIX)) storage.removeItem(key)
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
  local.removeItem(LEGACY_MEMBER_TOKEN_KEY)
  local.removeItem(MEMBER_SESSION_KEY)
  local.removeItem(MEMBER_ROOM_HISTORY_KEY)
  local.removeItem(MEMBER_DEMO_STATE_KEY)
  if (theme) {
    local.setItem(MEMBER_DEMO_STATE_KEY, JSON.stringify({ theme, checkedIn: false, chatUnread: 0 }))
  }
  clearLoginAnnouncementMarkers(session)
  return { theme }
}
