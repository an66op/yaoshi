/** Go's bcrypt boundary is defined in UTF-8 bytes, not JavaScript string
 * code units or displayed characters. */
export function passwordUTF8ByteLength(value: string) {
  return new TextEncoder().encode(value).length
}
