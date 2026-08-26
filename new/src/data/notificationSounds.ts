export type NotificationKind = 'lottery' | 'message' | 'reward' | 'announcement'

export type NotificationSound = {
  id: string
  name: string
  description: string
  src?: string
  tones?: number[]
  /** Compensates for loudness differences between imported WAV files. */
  sourceLevel?: number
}

export const notificationKinds: Array<{ id: NotificationKind; label: string; description: string }> = [
  { id: 'lottery', label: '彩票开奖通知', description: '开奖、封盘、结果更新' },
  { id: 'message', label: '普通消息通知', description: '客服与聊天室新消息' },
  { id: 'reward', label: '红包与奖励', description: '红包、签到、积分到账' },
  { id: 'announcement', label: '系统公告', description: '维护、活动与重要提醒' },
]

export const notificationSounds: NotificationSound[] = [
  { id: 'blessing-chime', name: '祝福和弦', description: '温和上行，适合奖励到账', tones: [523, 659, 784] },
  { id: 'aurora-chime', name: '极光编钟', description: '清亮上行，适合开奖', tones: [659, 784, 1047] },
  { id: 'lucky-triad', name: '幸运三连', description: '明快三音，适合中奖', tones: [523, 659, 784] },
  { id: 'midnight-bell', name: '夜航铃', description: '低调沉稳，适合夜间', tones: [392, 523, 659] },
  { id: 'wave-signal', name: '海浪讯号', description: '轻盈起伏，适合消息', tones: [587, 698, 587] },
  { id: 'starlight', name: '星光提示', description: '短促干净，适合公告', tones: [880, 1175] },
  { id: 'rising-ticket', name: '开奖号角', description: '有仪式感，适合开奖', tones: [440, 554, 659, 880] },
  { id: 'mint-pop', name: '薄荷弹跳', description: '轻快活泼，适合奖励', tones: [740, 880, 740, 988] },
  { id: 'soft-knock', name: '柔和叩响', description: '不打扰，适合普通消息', tones: [494, 523] },
]

export const defaultNotificationSounds: Record<NotificationKind, string> = {
  lottery: 'aurora-chime',
  message: 'soft-knock',
  reward: 'lucky-triad',
  announcement: 'starlight',
}
