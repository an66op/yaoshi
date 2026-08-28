import type { ReportCenterResult } from '../api'

/** The report API can legitimately return null collections for an empty set. */
export function normalizeReportResult(result: ReportCenterResult | null | undefined): ReportCenterResult {
  const source = result ?? ({} as Partial<ReportCenterResult>)
  return {
    key: typeof source.key === 'string' ? source.key : '',
    title: typeof source.title === 'string' ? source.title : '',
    period_start: typeof source.period_start === 'string' ? source.period_start : '',
    period_end: typeof source.period_end === 'string' ? source.period_end : '',
    metrics: Array.isArray(source.metrics) ? source.metrics : [],
    columns: Array.isArray(source.columns) ? source.columns : [],
    items: Array.isArray(source.items) ? source.items : [],
    total: Number.isFinite(Number(source.total)) ? Number(source.total) : 0,
    page: Number.isFinite(Number(source.page)) ? Number(source.page) : 1,
    page_size: Number.isFinite(Number(source.page_size)) ? Number(source.page_size) : 20,
  }
}
