import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { PlanRecommendationNumberFields } from './PlanRecommendationNumberFields'
import { SPEED_RACING_PLAN_RULE } from '../utils/planRecommendation'

describe('plan recommendation number fields', () => {
  it('explains exactly five racing numbers and hides legacy size/parity controls', () => {
    const html = renderToStaticMarkup(<PlanRecommendationNumberFields gameId="speed-racing" value={{ numbersText: '1,3,5,7,10', size: '大', parity: '单' }} onChange={() => undefined} />)
    expect(html).toContain(SPEED_RACING_PLAN_RULE)
    expect(html).not.toContain('大小')
    expect(html).not.toContain('单双')
    expect(html).not.toContain('aria-invalid="true"')
  })

  it('marks historical three-number racing recommendations invalid until explicitly edited', () => {
    const html = renderToStaticMarkup(<PlanRecommendationNumberFields gameId="speed-racing" value={{ numbersText: '1,3,5', size: '', parity: '' }} onChange={() => undefined} />)
    expect(html).toContain('value="1,3,5"')
    expect(html).toContain('aria-invalid="true"')
  })

  it.each(['speed-fly', 'canada-28'])('keeps size/parity and the existing numbers help for %s', gameId => {
    const html = renderToStaticMarkup(<PlanRecommendationNumberFields gameId={gameId} value={{ numbersText: '1,3,5', size: '大', parity: '单' }} onChange={() => undefined} />)
    expect(html).toContain('大小')
    expect(html).toContain('单双')
    expect(html).toContain('使用逗号分隔，例如 1,5,9')
    expect(html).not.toContain(SPEED_RACING_PLAN_RULE)
    expect(html).not.toContain('aria-invalid="true"')
  })
})
