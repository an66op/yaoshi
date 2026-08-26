import { defaultNotificationSounds, notificationSounds } from '../data/notificationSounds'
import type { NotificationKind, NotificationSound } from '../data/notificationSounds'

const mutedStorageKey = 'seven-star-notification-muted'
let activeNotification: SoundPlayback | null = null
let activeNotificationTimer: number | null = null

export type SoundPlayback = { stop: () => void; durationMs: number }

export function isNotificationMuted() {
  try {
    return window.localStorage.getItem(mutedStorageKey) === '1'
  } catch {
    return false
  }
}

export function setNotificationMuted(muted: boolean) {
  try {
    window.localStorage.setItem(mutedStorageKey, muted ? '1' : '0')
  } catch {
    // Local preferences are optional; playback continues to work in memory.
  }
}

export function startNotificationSound(sound: NotificationSound, volume = 0.62): SoundPlayback | null {
  try {
    if (sound.src) {
      const audio = new Audio(sound.src)
      // The bundled recordings have noticeably different mastering levels.
      // Keep their perceived loudness close to the generated chimes.
      audio.volume = Math.min(1, volume * (sound.sourceLevel ?? 0.75))
      void audio.play().catch(() => undefined)
      return { stop: () => { audio.pause(); audio.currentTime = 0 }, durationMs: 1800 }
    }
    if (!sound.tones?.length) return null
    const AudioContextCtor = window.AudioContext ?? (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
    if (!AudioContextCtor) return null
    const context = new AudioContextCtor()
    const startedAt = context.currentTime + 0.01
    const toneLength = 0.16
    sound.tones.forEach((frequency, index) => {
      const oscillator = context.createOscillator()
      const gain = context.createGain()
      const at = startedAt + index * toneLength
      oscillator.type = index % 2 ? 'sine' : 'triangle'
      oscillator.frequency.setValueAtTime(frequency, at)
      gain.gain.setValueAtTime(0.0001, at)
      gain.gain.exponentialRampToValueAtTime(Math.max(0.01, volume * 0.16), at + 0.012)
      gain.gain.exponentialRampToValueAtTime(0.0001, at + toneLength)
      oscillator.connect(gain).connect(context.destination)
      oscillator.start(at)
      oscillator.stop(at + toneLength + 0.02)
    })
    return { stop: () => { void context.close() }, durationMs: sound.tones.length * toneLength * 1000 + 180 }
  } catch {
    return null
  }
}

export function playNotificationSound(kind: NotificationKind) {
  try {
    if (isNotificationMuted()) return
    const selections = { ...defaultNotificationSounds, ...JSON.parse(window.localStorage.getItem('seven-star-notification-sounds') ?? '{}') }
    const sound = notificationSounds.find((item) => item.id === selections[kind])
      ?? notificationSounds.find((item) => item.id === defaultNotificationSounds[kind])
    if (!sound) return
    stopNotificationSounds()
    activeNotification = startNotificationSound(sound)
    if (activeNotification) {
      activeNotificationTimer = window.setTimeout(() => {
        activeNotification = null
        activeNotificationTimer = null
      }, activeNotification.durationMs)
    }
  } catch {
    // 浏览器未允许自动播放或本地存储不可用时，静默跳过即可。
  }
}

export function stopNotificationSounds() {
  if (activeNotificationTimer != null) {
    window.clearTimeout(activeNotificationTimer)
    activeNotificationTimer = null
  }
  activeNotification?.stop()
  activeNotification = null
}
