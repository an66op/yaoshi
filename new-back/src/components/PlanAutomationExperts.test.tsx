import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { PlanAutomationExperts } from './PlanAutomationExperts'

describe('automatic plan expert labels', () => {
  it('renders server-provided numbered experts without exposing internal source keys', () => {
    const masters = ['demo-qingyun', 'demo-beidou', 'demo-jinli'].map((key, index) => ({ key, name: `${index + 1}号专家`, title: '系统自动推荐', color: '#2aa9b3', sort_order: (index + 1) * 10 }))
    const html = renderToStaticMarkup(<PlanAutomationExperts masters={masters} />)
    for (const name of ['1号专家', '2号专家', '3号专家']) expect(html).toContain(name)
    expect(html).toContain('专家模板')
    expect(html).toContain('系统自动推荐')
    expect(html).toContain('自动生成')
    expect(html).not.toContain('演示')
    expect(html).not.toContain('demo-')
    expect(html).not.toContain('命中率')
  })

  it('does not remap server-provided names in the browser', () => {
    const html = renderToStaticMarkup(<PlanAutomationExperts masters={[{ key: 'custom', name: '原有专家名称', title: '原有标签', color: '#2aa9b3', sort_order: 10 }]} />)
    expect(html).toContain('原有专家名称')
    expect(html).toContain('原有标签')
  })
})
