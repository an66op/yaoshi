import { describe, expect, it } from 'vitest'
import { createCsv, escapeCsvCell } from './csv'

describe('CSV export hardening', () => {
  it('neutralizes spreadsheet formulas in untrusted strings', () => {
    expect(escapeCsvCell('=HYPERLINK("https://evil.example")')).toBe('"\'=HYPERLINK(""https://evil.example"")"')
    expect(escapeCsvCell('  +SUM(1,2)')).toBe('"\'  +SUM(1,2)"')
    expect(escapeCsvCell('\t@payload')).toBe('"\'\t@payload"')
    expect(escapeCsvCell('-1+CMD')).toBe('"\'-1+CMD"')
  })

  it('preserves numeric values and quotes commas and line breaks', () => {
    expect(escapeCsvCell('-12.50')).toBe('"-12.50"')
    expect(createCsv([['name', 'amount'], ['a,b', -12.5], ['line\nbreak', 0]])).toBe(
      '"name","amount"\n"a,b","-12.5"\n"line\nbreak","0"',
    )
  })
})
