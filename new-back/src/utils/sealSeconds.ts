export const SEAL_SECONDS_ERROR = '封盘秒数必须为 0～86400 的整数'

export function isValidSealSeconds(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value >= 0 && value <= 86400
}
