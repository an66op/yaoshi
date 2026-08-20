import { defaultNotificationSounds, notificationSounds } from '../data/notificationSounds'
import type { NotificationKind } from '../data/notificationSounds'

export function playNotificationSound(kind: NotificationKind) {
  try {
    const selections = { ...defaultNotificationSounds, ...JSON.parse(window.localStorage.getItem('seven-star-notification-sounds') ?? '{}') }
    const sound = notificationSounds.find((item) => item.id === selections[kind])
    if (!sound) return
    const audio = new Audio(sound.src)
    audio.volume = 0.62
    void audio.play().catch(() => undefined)
  } catch {
    // 浏览器未允许自动播放或本地存储不可用时，静默跳过即可。
  }
}
