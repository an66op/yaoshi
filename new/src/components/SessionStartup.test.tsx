import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { SessionStartup } from './SessionStartup'

describe('themed session startup shell', () => {
  it.each(['day', 'night'] as const)('uses the %s login surface, not the legacy mobile-app fallback', theme => {
    const html = renderToStaticMarkup(<SessionStartup theme={theme} />)
    expect(html).toContain(`class="login-page session-startup theme-${theme}"`)
    expect(html).not.toContain('mobile-app')
    expect(html).toContain('aria-busy="true"')
    expect(html).toContain('role="status"')
    expect(html).toContain('正在确认登录状态')
    expect(html).not.toContain('<input')
    expect(html).not.toContain('余额')
    expect(html).not.toContain('88001')
  })

  it('provides themed recovery without revealing unverified business content', () => {
    const html = renderToStaticMarkup(<SessionStartup theme="day" error="请求超时，请稍后重试" onRetry={() => undefined} onLogout={() => undefined} />)
    expect(html).toContain('aria-busy="false"')
    expect(html).toContain('role="alert"')
    expect(html).toContain('重新连接')
    expect(html).toContain('退出登录')
    expect(html).not.toContain('session-check-spinner')
    expect(html).not.toContain('余额')
  })

  it('prevents a second recovery action while logout is pending', () => {
    const html = renderToStaticMarkup(<SessionStartup theme="night" error="连接失败" onRetry={() => undefined} onLogout={() => undefined} loggingOut />)
    expect(html.match(/disabled=""/g)).toHaveLength(2)
    expect(html).toContain('退出中…')
  })
})
