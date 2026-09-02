import { describe, expect, it } from 'vitest'
import { DEFAULT_LOTTERY_SOURCE_URL, isValidLotterySourceURL, normalizeLotterySourceURL } from './lotterySourceURL'

describe('lottery source settings URL', () => {
  it('defaults an empty value and accepts a credential-free HTTPS destination', () => {
    expect(normalizeLotterySourceURL('')).toBe(DEFAULT_LOTTERY_SOURCE_URL)
    expect(normalizeLotterySourceURL(' https://draw.example/mobile?q=1 ')).toBe('https://draw.example/mobile?q=1')
  })

  it.each([
    'javascript:alert(1)',
    'data:text/html,unsafe',
    'http://draw.example/mobile',
    '//draw.example/mobile',
    '/mobile',
    'https://user:secret@draw.example/mobile',
  ])('rejects unsafe or non-HTTPS setting %s', value => {
    expect(isValidLotterySourceURL(value)).toBe(false)
  })
})
