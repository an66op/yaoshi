import type { ActivityItem, MemberNotification } from '../api/portal'

export type MessageNotificationRow = 'system' | 'activity' | 'winning'

export const defaultHiddenMessageRows = ['winning'] as const

const legacyResultTitles = new Set(['开奖结果', '恭喜中奖', '未中奖', '开奖通知'])
const winningToggleKeys = [
  'bet_notification_enabled',
  'bet_notifications_enabled',
  'winning_notification_enabled',
  'winning_notifications_enabled',
] as const

/** Resolve room message visibility once so the list and bottom-nav badge agree. */
export function configuredHiddenMessageRows(game?: Record<string, unknown> | null) {
  const configured = game?.message_hidden_rows
  const rows = Array.isArray(configured)
    ? configured.filter((item): item is string => typeof item === 'string')
    : [...defaultHiddenMessageRows]

  // Accept both legacy singular and current plural setting names. An explicit
  // disabled bet/result notification must never leave an unreachable badge.
  if (winningToggleKeys.some((key) => game?.[key] === false) && !rows.includes('winning')) {
    rows.push('winning')
  }
  return rows
}

export function activePromotionTitles(activities: ActivityItem[]) {
  return new Set(
    activities
      .filter((item) => item.status === 'active' && item.type === 'promotion')
      .map((item) => item.title.trim())
      .filter(Boolean),
  )
}

/** Return the /messages row that can actually expose this notification. */
export function notificationMessageRow(
  notification: MemberNotification,
  promotionTitles: ReadonlySet<string>,
): MessageNotificationRow | null {
  if (notification.category === 'winning') return 'winning'
  if (notification.category === 'system') {
    // Old result records were stored as system notifications, but the current
    // winning endpoint cannot display or mark them. Do not create a ghost badge.
    return legacyResultTitles.has(notification.title) ? null : 'system'
  }
  if (notification.category === 'activity' && promotionTitles.has(notification.title)) return 'activity'
  return null
}

export function visibleNotificationsForRow(
  row: MessageNotificationRow,
  notifications: MemberNotification[],
  promotionTitles: ReadonlySet<string>,
) {
  return notifications.filter((notification) => notificationMessageRow(notification, promotionTitles) === row)
}

export function visibleUnreadNotificationCount(
  notifications: MemberNotification[],
  hiddenRows: Iterable<string>,
  promotionTitles: ReadonlySet<string>,
) {
  const hidden = new Set(hiddenRows)
  return notifications.reduce((count, notification) => {
    if (notification.read) return count
    const row = notificationMessageRow(notification, promotionTitles)
    return row && !hidden.has(row) ? count + 1 : count
  }, 0)
}

/** Mirrors the existing unread endpoint so pagination can stop once every
 * server-counted unread item has appeared in the downloaded snapshot. */
export function serverCountedUnreadNotificationCount(notifications: MemberNotification[]) {
  return notifications.filter((item) => {
    if (item.read || item.title === '客服回复') return false
    if (item.category !== 'system' && item.category !== 'activity' && item.category !== 'winning') return false
    return item.category !== 'system' || !legacyResultTitles.has(item.title)
  }).length
}
