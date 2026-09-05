import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { PlanRecommendationNumberFields } from './PlanRecommendationNumberFields'
import { planRecommendationNumberRule, RACING_MANUAL_PLAN_RULE, RACING_PLAN_GAME_IDS } from '../utils/planRecommendation'

describe('plan recommendation number fields', () => {
  it('explains that racing products use the automatic matrix and hides legacy dimensions', () => {
    const html = renderToStaticMarkup(<PlanRecommendationNumberFields gameId="speed-racing" value={{ numbersText: '1,3,5,7,10', size: '大', parity: '单' }} onChange={() => undefined} />)
    expect(html).toContain(RACING_MANUAL_PLAN_RULE)
    expect(html).not.toContain('大小')
    expect(html).not.toContain('单双')
    expect(html).toContain('aria-invalid="true"')
  })

  it('applies the same prohibition to all seven racing-v2 products', () => {
    for (const gameId of RACING_PLAN_GAME_IDS) {
      const html = renderToStaticMarkup(<PlanRecommendationNumberFields gameId={gameId} value={{ numbersText: '1,3,5', size: '', parity: '' }} onChange={() => undefined} />)
      expect(html).toContain(RACING_MANUAL_PLAN_RULE)
      expect(html).toContain('aria-invalid="true"')
    }
  })

  it.each(['speed-ssc', 'canada-28'])('keeps size/parity and shows the exact numbers help for %s', gameId => {
    const html = renderToStaticMarkup(<PlanRecommendationNumberFields gameId={gameId} value={{ numbersText: '1,3,5', size: '大', parity: '单' }} onChange={() => undefined} />)
    expect(html).toContain('大小')
    expect(html).toContain('单双')
    expect(html).toContain(planRecommendationNumberRule(gameId).replace('–', '–'))
    expect(html).not.toContain(RACING_MANUAL_PLAN_RULE)
    expect(html).not.toContain('aria-invalid="true"')
  })
})
