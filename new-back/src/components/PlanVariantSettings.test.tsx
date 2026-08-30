import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import type { PlanVariantOption } from '../api'
import { PlanVariantSettings } from './PlanVariantSettings'

const availablePositions = Array.from({ length: 10 }, (_, i) => ({ position: i + 1, label: ['冠军', '亚军', '第三名', '第四名', '第五名', '第六名', '第七名', '第八名', '第九名', '第十名'][i], opponent_position: 10 - i }))
const options: PlanVariantOption[] = [
  { key: 'four-period-five-codes', label: '四期五码', kind: 'numbers', periods: 4, number_count: 5 },
  { key: 'one-period-eight-codes', label: '一期八码', kind: 'numbers', periods: 1, number_count: 8 },
  { key: 'size-three-periods', label: '大小三期', kind: 'size', periods: 3, number_count: 0 },
  { key: 'parity-five-periods', label: '单双五期', kind: 'parity', periods: 5, number_count: 0 },
  { key: 'dragon-tiger-four-periods', label: '龙虎四期', kind: 'dragon_tiger', periods: 4, number_count: 0 },
]

const props = { positions: [1], planKeys: ['four-period-five-codes'], options, availablePositions, maxActiveStreams: 20, onPositionsChange: vi.fn(), onPlanKeysChange: vi.fn() }

describe('racing plan availability settings', () => {
  it('uses backend-supported positions and variants and shows all four families', () => {
    const html = renderToStaticMarkup(<PlanVariantSettings {...props} />)
    for (const value of [...availablePositions.map(item => item.label), ...options.map(item => item.label), '号码计划', '大小计划', '单双计划', '龙虎计划']) expect(html).toContain(value)
    expect((html.match(/type="checkbox"/g) || []).length).toBe(15)
    expect((html.match(/checked=""/g) || []).length).toBe(2)
    expect(html).toContain('4 个实际开放期')
    expect(html).toContain('最多同时访问 20')
    expect(html).toContain('不预留默认名额')
    expect(html).not.toContain('演示')
    expect(html).not.toContain('默认的“冠军·四期五码”未开放')
  })

  it('explains that removing the default requires an explicit member selection', () => {
    const html = renderToStaticMarkup(<PlanVariantSettings {...props} positions={[2]} />)
    expect(html).toContain('默认的“冠军·四期五码”未开放')
    expect(html).toContain('已发布历史不会被改写')
  })

  it('locks every control while a save is in progress', () => {
    const html = renderToStaticMarkup(<PlanVariantSettings {...props} disabled />)
    expect((html.match(/<input[^>]*disabled=""/g) || []).length).toBe(15)
    expect(html).toMatch(/<button[^>]*disabled=""/)
  })
})
