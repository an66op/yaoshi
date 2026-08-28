import { describe, expect, it } from 'vitest'
import type { ActivityItem, MemberNotification } from '../api/portal'
import {
  activePromotionTitles,
  configuredHiddenMessageRows,
  notificationMessageRow,
  serverCountedUnreadNotificationCount,
  visibleUnreadNotificationCount,
} from './notificationVisibility'

const notification = (values: Partial<MemberNotification>): MemberNotification => ({
  id: values.id ?? 1,
  title: values.title ?? '通知',
  content: values.content ?? '内容',
  level: values.level ?? 'info',
  category: values.category ?? 'system',
  link: values.link ?? '',
  read: values.read ?? false,
  created_at: values.created_at ?? '2026-08-28T00:00:00Z',
  ...values,
})

describe('notification visibility', () => {
  it('does not count the default-hidden winning/bet-result row', () => {
    const rows = [notification({ category: 'winning', title: '开奖通知' })]
    expect(configuredHiddenMessageRows({})).toEqual(['winning'])
    expect(visibleUnreadNotificationCount(rows, configuredHiddenMessageRows({}), new Set())).toBe(0)
  })

  it('honours an explicit disabled winning notification toggle', () => {
    expect(configuredHiddenMessageRows({ message_hidden_rows: [], bet_notifications_enabled: false })).toEqual(['winning'])
  })

  it('counts only notifications with an entry the member can open', () => {
    const activities = [{ title: '夏日活动', status: 'active', type: 'promotion' }] as ActivityItem[]
    const promotions = activePromotionTitles(activities)
    const rows = [
      notification({ id: 1, category: 'system' }),
      notification({ id: 2, category: 'activity', title: '夏日活动' }),
      notification({ id: 3, category: 'activity', title: '已下线活动' }),
      notification({ id: 4, category: 'account', title: '红包到账' }),
      notification({ id: 5, category: 'winning', title: '开奖通知' }),
      notification({ id: 6, category: 'system', title: '开奖结果' }),
      notification({ id: 7, category: 'system', read: true }),
    ]

    expect(notificationMessageRow(rows[5], promotions)).toBeNull()
    expect(visibleUnreadNotificationCount(rows, ['winning'], promotions)).toBe(2)
    expect(serverCountedUnreadNotificationCount(rows)).toBe(4)
  })
})
