import { describe, expect, it } from 'vitest'
import { chatCountdownText } from '../utils/chatCountdown'

describe('SG management chat countdown', () => {
  const now = Date.parse('2026-09-03T06:00:00Z')

  it.each([undefined, '', 'invalid', '0001-01-01T00:00:00Z'])('shows unavailable rather than a fabricated zero for %s', target => {
    expect(chatCountdownText(target, now)).toBe('--:--')
  })

  it('waits for a clock sample and otherwise preserves the actual deadline', () => {
    expect(chatCountdownText('2026-09-03T06:01:00Z', 0)).toBe('--:--')
    expect(chatCountdownText('2026-09-03T06:01:00Z', NaN)).toBe('--:--')
    expect(chatCountdownText('2026-09-03T06:01:00Z', now)).toBe('01:00')
    expect(chatCountdownText('2026-09-03T05:59:00Z', now)).toBe('00:00')
  })
})
