import { isValidSealSeconds } from './sealSeconds'

export const alignedLotteryGameIds = [
  'speed-racing',
  'speed-fly',
  'sg-fly',
  'fly-racing',
  'au-lucky-10',
  'speed-ssc',
  'au-lucky-5',
  'sg-ssc',
] as const

export function validGameTimingOverrides(value: unknown): boolean {
  if (value == null) return true
  if (typeof value !== 'object' || Array.isArray(value)) return false
  return Object.values(value).every(item => {
    if (typeof item !== 'object' || item == null || Array.isArray(item)) return false
    const seconds = (item as { seal_seconds?: unknown }).seal_seconds
    return seconds === undefined || isValidSealSeconds(seconds)
  })
}
