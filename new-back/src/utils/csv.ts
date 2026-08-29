/**
 * Serialize CSV cells safely for spreadsheet applications.
 *
 * Quoting alone does not stop formula execution. Prefix untrusted string
 * values that begin like a spreadsheet formula while leaving numeric report
 * values numeric.
 */
export function escapeCsvCell(value: unknown) {
  let text = String(value ?? '')
  const formattedNumber = typeof value === 'string' && /^-?(?:\d+|\d*\.\d+)$/.test(text)
  if (typeof value === 'string' && !formattedNumber && (/^[\t\r\n]/.test(text) || /^\s*[=+\-@]/.test(text))) {
    text = `'${text}`
  }
  return `"${text.replaceAll('"', '""')}"`
}

export function createCsv(rows: readonly (readonly unknown[])[]) {
  return rows.map(row => row.map(escapeCsvCell).join(',')).join('\n')
}
