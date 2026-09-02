import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { DEFAULT_LOTTERY_SOURCE_URL } from '../utils/lotterySourceURL'
import { QuickActions } from './GameRoom'

vi.mock('../api/client', () => ({ apiBase: 'http://localhost:8080/api', request: vi.fn(), publicRequest: vi.fn() }))

const noop = () => undefined
const render = (lotterySourceURL: string) => renderToStaticMarkup(
  <QuickActions
    lotterySourceURL={lotterySourceURL}
    hasRedPacket={false}
    keyboardOpen={false}
    onSwitchGame={noop}
    onCustomerService={noop}
    onQuickBet={noop}
    onCheckIn={noop}
    onOpenRedPacket={noop}
  />,
)

describe('game room quick actions', () => {
  it('puts the configured lottery source first and opens it safely in a new window', () => {
    const html = render('https://draw.example/mobile')
    expect(html.indexOf('aria-label="查看开奖源（新窗口）"')).toBeLessThan(html.indexOf('aria-label="切换游戏"'))
    expect(html).toContain('href="https://draw.example/mobile"')
    expect(html).toContain('target="_blank"')
    expect(html).toContain('rel="noopener noreferrer"')
  })

  it('uses one linear icon system and never emits an unsafe configured destination', () => {
    const html = render('javascript:alert(1)')
    expect(html).toContain(`href="${DEFAULT_LOTTERY_SOURCE_URL}"`)
    expect(html.match(/<svg /g)).toHaveLength(5)
    expect(html).not.toMatch(/>(?:⇄|🎧|☷|签)</)
    for (const label of ['切换游戏', '联系客服', '快捷投注', '每日签到']) expect(html).toContain(`aria-label="${label}"`)
  })
})
