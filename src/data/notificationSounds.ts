export type NotificationKind = 'lottery' | 'message' | 'reward' | 'announcement'

export type NotificationSound = {
  id: string
  name: string
  description: string
  src: string
}

export const notificationKinds: Array<{ id: NotificationKind; label: string; description: string }> = [
  { id: 'lottery', label: '彩票开奖通知', description: '开奖、封盘、结果更新' },
  { id: 'message', label: '普通消息通知', description: '客服与聊天室新消息' },
  { id: 'reward', label: '红包与奖励', description: '红包、签到、积分到账' },
  { id: 'announcement', label: '系统公告', description: '维护、活动与重要提醒' },
]

export const notificationSounds: NotificationSound[] = [
  { id: 'crystal-bell', name: '水晶铃', description: '清脆、适合开奖提醒', src: '/sounds/bell1.wav' },
  { id: 'quick-switch', name: '轻触提示', description: '短促、适合普通消息', src: '/sounds/switch1.wav' },
  { id: 'golden-pop', name: '金币礼花', description: '明亮、适合红包奖励', src: '/sounds/cracker1.wav' },
  { id: 'festival-pop', name: '节庆礼花', description: '热闹、适合活动通知', src: '/sounds/cracker2.wav' },
  { id: 'metal-ping', name: '金属叮', description: '明确、适合重要公告', src: '/sounds/metal1.wav' },
  { id: 'silver-tone', name: '银铃提示', description: '柔和、适合连续通知', src: '/sounds/steel1.wav' },
  { id: 'cheer-reward', name: '欢呼彩蛋', description: '趣味、适合奖励到账', src: '/sounds/wahaha.wav' },
]

export const defaultNotificationSounds: Record<NotificationKind, string> = {
  lottery: 'crystal-bell',
  message: 'quick-switch',
  reward: 'golden-pop',
  announcement: 'metal-ping',
}
