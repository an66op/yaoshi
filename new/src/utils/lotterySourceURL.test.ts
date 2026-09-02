import { describe, expect, it } from 'vitest'
import { DEFAULT_LOTTERY_SOURCE_URL, resolveLotterySourceURL } from './lotterySourceURL'

describe('member lottery source URL', () => {
  it('keeps configured credential-free HTTPS links', () => {
    expect(resolveLotterySourceURL(' https://draw.example/mobile?q=1 ')).toBe('https://draw.example/mobile?q=1')
  })

  it.each([
    undefined,
    '',
    'javascript:alert(1)',
    'data:text/html,unsafe',
    'http://draw.example/mobile',
    '//draw.example/mobile',
    'https://user:secret@draw.example/mobile',
  ])('falls back safely for %s', value => {
    expect(resolveLotterySourceURL(value)).toBe(DEFAULT_LOTTERY_SOURCE_URL)
  })
})
