import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { GameSealSecondsOverrides } from './GameSealSecondsOverrides'
import { validGameTimingOverrides } from '../utils/gameTimingOverrides'

describe('GameSealSecondsOverrides', () => {
  it('shows only the aligned lottery games and their inherited or explicit values', () => {
    const html = renderToStaticMarkup(<GameSealSecondsOverrides
      scope="room"
      games={[{ id: 'speed-racing', name: '极速赛车' }, { id: 'speed-ssc', name: '极速时时彩' }, { id: 'other', name: '其他' }]}
      defaultSeconds={30}
      value={{ 'speed-ssc': { seal_seconds: 12 } }}
      onChange={() => undefined}
    />)
    expect(html).toContain('彩票彩种独立封盘')
    expect(html).toContain('极速赛车')
    expect(html).toContain('极速时时彩')
    expect(html).not.toContain('其他')
    expect(html).toContain('value="12"')
    expect(html).toContain('留空继承 30 秒')
  })

  it('writes an override and removes it when the field is cleared', () => {
    const onChange = vi.fn()
    const tree = GameSealSecondsOverrides({
      scope: 'room', games: [{ id: 'speed-racing', name: '极速赛车' }], defaultSeconds: 30,
      value: { 'speed-ssc': { seal_seconds: 10 } }, onChange,
    }) as { props: { children: Array<unknown> } }
    const fields = JSON.stringify(tree)
    expect(fields).toContain('极速赛车')
    // State transformation is covered by invoking the rendered TextField handler.
    const grid = (tree.props.children as Array<{ props: { children: unknown[] } }>)[1]
    const field = (grid.props.children as Array<{ props: { onChange: (event: { target: { value: string } }) => void } }>)[0]
    field.props.onChange({ target: { value: '15' } })
    expect(onChange).toHaveBeenLastCalledWith({ 'speed-ssc': { seal_seconds: 10 }, 'speed-racing': { seal_seconds: 15 } })
    field.props.onChange({ target: { value: '' } })
    expect(onChange).toHaveBeenLastCalledWith({ 'speed-ssc': { seal_seconds: 10 } })
  })

  it('rejects malformed and out-of-range settings before save', () => {
    expect(validGameTimingOverrides(undefined)).toBe(true)
    expect(validGameTimingOverrides({ 'speed-racing': { seal_seconds: 0 } })).toBe(true)
    expect(validGameTimingOverrides({ 'speed-racing': { seal_seconds: 86401 } })).toBe(false)
    expect(validGameTimingOverrides([])).toBe(false)
  })
})
