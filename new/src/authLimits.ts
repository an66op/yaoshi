export const USERNAME_MIN_LENGTH = 3
export const MEMBER_REGISTER_USERNAME_MAX_LENGTH = 20
export const MEMBER_LOGIN_USERNAME_MAX_LENGTH = 50
export const PASSWORD_MIN_BYTES = 8
export const PASSWORD_MAX_BYTES = 72

export function passwordByteLength(value: string): number {
  return new TextEncoder().encode(value).length
}

export function unicodeLength(value: string): number {
  return Array.from(value).length
}

export function truncateUnicode(value: string, maxLength: number): string {
  return Array.from(value).slice(0, maxLength).join('')
}

export function validPasswordByteLength(value: string): boolean {
  const length = passwordByteLength(value)
  return length >= PASSWORD_MIN_BYTES && length <= PASSWORD_MAX_BYTES
}
