import { describe, expect, it } from 'vitest'
import { selectedRoomAnnouncement } from '../utils/roomAnnouncements'
import type { RoomSettings } from '../api/portal'

function settings(announcements: RoomSettings['announcements'], roomNotice = ''): RoomSettings {
  return {
    room_name: '测试房间', room_logo: '', room_notice: roomNotice, announcements,
    show_odds: true, sound_enabled: true, prediction_enabled: true,
    min_credit_amount: 1, min_debit_amount: 1, min_chat_score: 0,
    chat_nickname: '', chat_avatar: '', lottery_source_url: '', game: {}, quick_replies: [],
  }
}

describe('chat room announcements', () => {
  it('shows every enabled announcement in configured order', () => {
    const result = selectedRoomAnnouncement(settings([
      { id: 'later', title: '维护', content: '今晚维护', enabled: true, popup_on_login: false, sort_order: 20 },
      { id: 'first', title: '欢迎', content: '欢迎进入房间', enabled: true, popup_on_login: true, sort_order: 10 },
      { id: 'off', title: '停用', content: '不应显示', enabled: false, popup_on_login: false, sort_order: 1 },
    ]))
    expect(result).toBe('欢迎：欢迎进入房间\n维护：今晚维护')
  })

  it('keeps the compact single-announcement presentation and legacy fallback', () => {
    expect(selectedRoomAnnouncement(settings([
      { id: 'only', title: '公告', content: '单条内容', enabled: true, popup_on_login: false, sort_order: 10 },
    ]))).toBe('单条内容')
    expect(selectedRoomAnnouncement(settings([], '旧公告'))).toBe('旧公告')
  })
})
