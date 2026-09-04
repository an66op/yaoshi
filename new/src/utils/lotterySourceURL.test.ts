import { describe, expect, it } from 'vitest'
import { DEFAULT_LOTTERY_SOURCE_URL, resolveGameLotterySourceURL, resolveLotterySourceURL, SG_SSC_LOTTERY_SOURCE_URL } from './lotterySourceURL'

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

  it('uses the actual SG source before the generic room shortcut', () => {
    expect(resolveGameLotterySourceURL({ id: 'sg-ssc', sourceURL: SG_SSC_LOTTERY_SOURCE_URL }, DEFAULT_LOTTERY_SOURCE_URL)).toBe(SG_SSC_LOTTERY_SOURCE_URL)
    expect(resolveGameLotterySourceURL({ id: 'sg-ssc', sourceURL: 'https://pkk168.com/webapp/html/shishicai_sg/index.html?source=verified' }, 'https://room.example/results')).toBe('https://pkk168.com/webapp/html/shishicai_sg/index.html?source=verified')
  })

  it.each([undefined, '', 'javascript:alert(1)', 'http://wrong.example', 'https://user:secret@wrong.example'])('uses only the fixed SG fallback for unavailable source URL %s', sourceURL => {
    expect(resolveGameLotterySourceURL({ id: 'sg-ssc', sourceURL }, 'https://room.example/results')).toBe(SG_SSC_LOTTERY_SOURCE_URL)
  })

  it('preserves customized room links for every other game', () => {
    expect(resolveGameLotterySourceURL({ id: 'speed-racing', sourceURL: SG_SSC_LOTTERY_SOURCE_URL }, 'https://room.example/results')).toBe('https://room.example/results')
    expect(resolveGameLotterySourceURL({ id: 'unconfigured-game' }, '')).toBe(DEFAULT_LOTTERY_SOURCE_URL)
  })
})
