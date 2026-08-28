import { describe, expect, it } from 'vitest'
import type { ReportCenterResult } from '../api'
import { normalizeReportResult } from './reportData'

describe('normalizeReportResult', () => {
  it('把空报表和 null 集合转换成可渲染数据', () => {
    const malformed = {
      key: '28', title: '28报表', period_start: '2026-08-21', period_end: '2026-08-27',
      metrics: null, columns: null, items: null, total: null, page: 1, page_size: 20,
    } as unknown as ReportCenterResult
    const result = normalizeReportResult(malformed)
    expect(result.metrics).toEqual([])
    expect(result.columns).toEqual([])
    expect(result.items).toEqual([])
    expect(result.total).toBe(0)
  })

  it('接口返回 null 时仍提供完整默认结构', () => {
    expect(normalizeReportResult(null)).toMatchObject({ metrics: [], columns: [], items: [], total: 0 })
  })
})
