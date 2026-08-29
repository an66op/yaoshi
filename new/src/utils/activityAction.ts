export type SafeActivityAction = {
  kind: 'internal' | 'external'
  href: string
}

/** Resolve only same-origin app paths or credential-free HTTPS destinations. */
export function resolveActivityAction(
  actionType: unknown,
  actionURL: unknown,
  origin: string,
): SafeActivityAction | null {
  if (typeof actionURL !== 'string') return null
  const value = actionURL.trim()
  if (!value) return null

  try {
    if (actionType === 'internal') {
      // A protocol-relative value starts with '/' too, but would leave the
      // application origin. URL parsing also catches backslash variants.
      if (!value.startsWith('/') || value.startsWith('//')) return null
      const target = new URL(value, origin)
      if (target.origin !== origin || target.username || target.password) return null
      return { kind: 'internal', href: `${target.pathname}${target.search}${target.hash}` }
    }
    if (actionType === 'external') {
      const target = new URL(value)
      if (target.protocol !== 'https:' || target.username || target.password) return null
      return { kind: 'external', href: target.href }
    }
  } catch {
    return null
  }
  return null
}
