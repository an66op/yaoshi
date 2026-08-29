import { describe, expect, it } from 'vitest'
import {
  broadcastMemberLogout,
  clearMemberBusinessStorage,
	LEGACY_MEMBER_TOKEN_KEY,
  MEMBER_AUTH_EVENT_KEY,
  MEMBER_DEMO_STATE_KEY,
  MEMBER_ROOM_HISTORY_KEY,
  MEMBER_SESSION_KEY,
} from './businessStorage'

class MemoryStorage implements Storage {
  private values = new Map<string, string>()

  get length() { return this.values.size }
  clear() { this.values.clear() }
  getItem(key: string) { return this.values.get(key) ?? null }
  key(index: number) { return [...this.values.keys()][index] ?? null }
  removeItem(key: string) { this.values.delete(key) }
  setItem(key: string, value: string) { this.values.set(key, String(value)) }
}

describe('clearMemberBusinessStorage', () => {
  it('清除账号、房间和会话公告，同时保留个人显示与声音偏好', () => {
    const local = new MemoryStorage()
    const session = new MemoryStorage()
	local.setItem(LEGACY_MEMBER_TOKEN_KEY, 'old-token')
    local.setItem(MEMBER_SESSION_KEY, JSON.stringify({ account: 'old-user', balance: 999 }))
    local.setItem(MEMBER_ROOM_HISTORY_KEY, JSON.stringify({ 'old-user': [{ code: '8801' }] }))
    local.setItem(MEMBER_DEMO_STATE_KEY, JSON.stringify({ theme: 'night', checkedIn: true, chatUnread: 8 }))
    local.setItem('seven-star-font-scale', JSON.stringify('large'))
    local.setItem('seven-star-display-style', JSON.stringify('simple'))
    local.setItem('seven-star-avatar', JSON.stringify({ index: 3 }))
    local.setItem('seven-star-notification-sounds', JSON.stringify({ chat: 'bell' }))
    session.setItem('wangzhe-login-announcements-shown:8801', '1')
    session.setItem('unrelated-session-key', 'keep')

    expect(clearMemberBusinessStorage(local, session)).toEqual({ theme: 'night' })
	expect(local.getItem(LEGACY_MEMBER_TOKEN_KEY)).toBeNull()
    expect(local.getItem(MEMBER_SESSION_KEY)).toBeNull()
    expect(local.getItem(MEMBER_ROOM_HISTORY_KEY)).toBeNull()
    expect(JSON.parse(local.getItem(MEMBER_DEMO_STATE_KEY) ?? '{}')).toEqual({ theme: 'night', checkedIn: false, chatUnread: 0 })
    expect(local.getItem('seven-star-font-scale')).toBe('"large"')
    expect(local.getItem('seven-star-display-style')).toBe('"simple"')
    expect(local.getItem('seven-star-avatar')).toBe('{"index":3}')
    expect(local.getItem('seven-star-notification-sounds')).toBe('{"chat":"bell"}')
    expect(session.getItem('wangzhe-login-announcements-shown:8801')).toBeNull()
    expect(session.getItem('unrelated-session-key')).toBe('keep')
  })

  it('损坏的组合状态不会阻止业务数据清理', () => {
    const local = new MemoryStorage()
    const session = new MemoryStorage()
	local.setItem(LEGACY_MEMBER_TOKEN_KEY, 'old-token')
    local.setItem(MEMBER_DEMO_STATE_KEY, '{broken')

    expect(clearMemberBusinessStorage(local, session)).toEqual({ theme: null })
	expect(local.getItem(LEGACY_MEMBER_TOKEN_KEY)).toBeNull()
    expect(local.getItem(MEMBER_DEMO_STATE_KEY)).toBeNull()
  })

  it('跨标签注销事件不包含账号或凭据', () => {
    const local = new MemoryStorage()
    const session = new MemoryStorage()
    local.setItem(MEMBER_SESSION_KEY, JSON.stringify({ account: 'sensitive-user' }))
    local.setItem(LEGACY_MEMBER_TOKEN_KEY, 'legacy-secret')

    broadcastMemberLogout(local, session)

    const raw = local.getItem(MEMBER_AUTH_EVENT_KEY) ?? ''
    expect(JSON.parse(raw)).toMatchObject({ type: 'logout' })
    expect(raw).not.toContain('sensitive-user')
    expect(raw).not.toContain('legacy-secret')
    expect(local.getItem(MEMBER_SESSION_KEY)).toBeNull()
    expect(local.getItem(LEGACY_MEMBER_TOKEN_KEY)).toBeNull()
  })
})
