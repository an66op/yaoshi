import { renderToStaticMarkup } from 'react-dom/server'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { setCurrentUser } from '../auth'
import { FeedbackProvider } from '../components/FeedbackProvider'
import { PlanManagementPanel } from '../components/PlanManagementPanel'
import { PlanGenerationPolicy, PlanManagementPage } from './PlanManagementPage'
import type { PlanAutomationConfig } from '../api'

vi.mock('../api', () => ({ adminApi: {}, tenantApi: {}, agentApi: {} }))

const signIn = (role: string) => setCurrentUser({ id: 1, username: role, email: '', nickname: role, role, status: 1 })

afterEach(() => setCurrentUser(null))

describe('plan management role and workspace boundaries', () => {
  it('describes bounded visitor-only generation without an administrative publish action', () => {
    const config = { stream_ttl_seconds: 60, history_default_periods: 6, history_max_periods: 10, history_retention_periods: 20 } as PlanAutomationConfig
    const html = renderToStaticMarkup(<PlanGenerationPolicy config={config} />)
    for (const text of ['无人浏览不推进', '每 15 秒', '隐藏或离开即暂停', '默认冠军计划也不常驻', '访问租期 60 秒', '最近 6 期', '最多 10 期', '最近 20 期']) expect(html).toContain(text)
    expect(html).not.toContain('生成本期')
    expect(html).not.toContain('30 分钟')
    expect(html).not.toContain('演示')
  })
  it.each(['tenant', 'agent', 'member'])('does not render administrator automation controls for %s', role => {
    signIn(role)
    const html = renderToStaticMarkup(<PlanManagementPage />)
    expect(html).toContain('仅总管理员可访问计划自动化管理')
    expect(html).not.toContain('开启自动推荐')
    expect(html).not.toContain('配置房间')
  })

  it('renders the standalone management page for the administrator', () => {
    signIn('admin')
    const html = renderToStaticMarkup(<PlanManagementPage />)
    expect(html).toContain('计划管理')
    expect(html).toContain('配置房间')
    expect(html).not.toContain('仅总管理员可访问')
    expect(html).toContain('专家推荐与自动生成配置')
    expect(html).not.toContain('演示')
  })

  it('uses the owning page workspace without a competing selector in the manual panel', () => {
    signIn('admin')
    const controlled = renderToStaticMarkup(<FeedbackProvider><PlanManagementPanel workspaceId={37} /></FeedbackProvider>)
    const standalone = renderToStaticMarkup(<FeedbackProvider><PlanManagementPanel /></FeedbackProvider>)
    expect(controlled).toContain('新增推荐')
    expect(controlled).not.toContain('配置房间')
    expect(standalone).toContain('配置房间')
  })

  it.each(['tenant', 'agent'])('retains %s manual editing without platform automation controls', role => {
    signIn(role)
    const html = renderToStaticMarkup(<FeedbackProvider><PlanManagementPanel /></FeedbackProvider>)
    expect(html).toContain('新增推荐')
    expect(html).not.toContain('开启自动推荐')
    expect(html).not.toContain('演示')
    expect(html).not.toContain('配置房间')
  })
})
