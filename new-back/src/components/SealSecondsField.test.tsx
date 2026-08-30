import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { SealSecondsField } from './SealSecondsField'
import { SEAL_SECONDS_ERROR, isValidSealSeconds } from '../utils/sealSeconds'

describe('seal seconds settings field', () => {
  it.each(['platform', 'room'] as const)('keeps the %s field editable with bounded integer inputs', (scope) => {
    const html = renderToStaticMarkup(<SealSecondsField scope={scope} value={30} onChange={() => undefined} />)
    expect(html).toContain(scope === 'platform' ? '默认封盘秒数' : '本房间封盘秒数')
    expect(html).toContain('type="number"')
    expect(html).toContain('name="seal_seconds"')
    expect(html).toContain('min="0"')
    expect(html).toContain('max="86400"')
    expect(html).toContain('step="1"')
    expect(html).toContain('inputMode="numeric"')
    expect(html).not.toMatch(/\s(?:readonly|disabled)=/i)
  })

  it('explains creation-time defaults without implying a platform override', () => {
    const html = renderToStaticMarkup(<SealSecondsField scope="platform" value={30} onChange={() => undefined} />)
    expect(html).toContain('新租户/代理房间创建时复制为初始值')
    expect(html).toContain('已有房间不随平台后续修改同步')
  })

  it('explains room-local changes and the already-sealed-period restriction', () => {
    const html = renderToStaticMarkup(<SealSecondsField scope="room" value={15} onChange={() => undefined} />)
    expect(html).toContain('仅当前房间生效，可独立调整')
    expect(html).toContain('减小提前量不会重新开放已封盘的当期')
  })

  it('passes an edited integer, including zero, to the owning page', () => {
    const onChange = vi.fn()
    const field = SealSecondsField({ scope: 'room', value: 30, onChange })
    field.props.onChange({ target: { value: '12' } })
    expect(onChange).toHaveBeenLastCalledWith(12)
    field.props.onChange({ target: { value: '0' } })
    expect(onChange).toHaveBeenLastCalledWith(0)
    field.props.onChange({ target: { value: '86400' } })
    expect(onChange).toHaveBeenLastCalledWith(86400)
  })

  it('does not silently turn an empty input into zero or round fractional input', () => {
    const onChange = vi.fn()
    const field = SealSecondsField({ scope: 'platform', value: 30, onChange })
    field.props.onChange({ target: { value: '' } })
    expect(onChange).toHaveBeenLastCalledWith(undefined)
    field.props.onChange({ target: { value: '1.5' } })
    expect(onChange).toHaveBeenLastCalledWith(1.5)
    expect(isValidSealSeconds(onChange.mock.lastCall?.[0])).toBe(false)
  })

  it.each([undefined, -1, 1.5, 86401])('shows validation feedback for %s', (value) => {
    const html = renderToStaticMarkup(<SealSecondsField scope="room" value={value} onChange={() => undefined} />)
    expect(html).toContain('aria-invalid="true"')
    expect(html).toContain(SEAL_SECONDS_ERROR)
  })
})

describe('seal seconds save validation', () => {
  it.each([0, 1, 15, 30, 86400])('accepts the integer %i', (value) => {
    expect(isValidSealSeconds(value)).toBe(true)
  })

  it.each([undefined, null, '', '30', -1, 1.5, 86401, Number.NaN, Number.POSITIVE_INFINITY])('rejects invalid input %s', (value) => {
    expect(isValidSealSeconds(value)).toBe(false)
  })
})
