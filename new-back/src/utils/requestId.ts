/**
 * Generate an idempotency key in both secure HTTPS contexts and local/LAN HTTP.
 * `crypto.randomUUID()` is unavailable in some non-secure browser contexts,
 * while `getRandomValues()` remains widely available.
 */
type CryptoLike = {
  randomUUID?: () => string
  getRandomValues?: (bytes: Uint8Array) => Uint8Array
}

export function createRequestId(cryptoAPI: CryptoLike | undefined = globalThis.crypto as unknown as CryptoLike) {
  if (typeof cryptoAPI?.randomUUID === 'function') return cryptoAPI.randomUUID()

  if (typeof cryptoAPI?.getRandomValues === 'function') {
    const bytes = new Uint8Array(16)
    cryptoAPI.getRandomValues(bytes)
    bytes[6] = (bytes[6] & 0x0f) | 0x40
    bytes[8] = (bytes[8] & 0x3f) | 0x80
    const hex = Array.from(bytes, value => value.toString(16).padStart(2, '0')).join('')
    return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
  }

  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}-${Math.random().toString(36).slice(2, 12)}`
}
