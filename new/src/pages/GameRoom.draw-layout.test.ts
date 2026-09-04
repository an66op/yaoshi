import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

type CSSRule = { selectors: string[]; declarations: Map<string, string>; media: string[] }

// This is a bounded CSS contract, not a browser layout engine. Preserve media
// context and source order so a later narrow-screen override cannot go unnoticed.
function styleRules(source: string, media: string[] = []): CSSRule[] {
  const css = source.replace(/\/\*[\s\S]*?\*\//g, '')
  const rules: CSSRule[] = []
  let cursor = 0
  while (cursor < css.length) {
    const opening = css.indexOf('{', cursor)
    if (opening < 0) break
    let closing = opening + 1
    let depth = 1
    for (; closing < css.length && depth; closing++) {
      if (css[closing] === '{') depth++
      if (css[closing] === '}') depth--
    }
    const selector = css.slice(cursor, opening).trim()
    const body = css.slice(opening + 1, closing - 1)
    if (selector.startsWith('@media')) {
      rules.push(...styleRules(body, [...media, selector]))
    } else if (!selector.startsWith('@')) {
      const declarations = new Map<string, string>()
      for (const declaration of body.split(';')) {
        const colon = declaration.indexOf(':')
        if (colon >= 0) declarations.set(declaration.slice(0, colon).trim(), declaration.slice(colon + 1).trim().replace(/\s+/g, ' '))
      }
      rules.push({ selectors: selector.split(',').map(value => value.trim()), declarations, media })
    }
    cursor = closing
  }
  return rules
}

const css = readFileSync(new URL('../game-room.css', import.meta.url), 'utf8')
const rules = styleRules(css)
const markSix = '.game-room .mark-six-draw-ui'
const racing = '.game-room .racing-draw-ui'
const ballTracks = 'repeat(6, minmax(0, 1fr)) minmax(0, 1.4fr)'

function matchesWidth(rule: CSSRule, width: number) {
  return rule.media.every(query => [...query.matchAll(/\((min|max)-width:\s*(\d+)px\)/g)]
    .every(([, comparison, pixels]) => comparison === 'max' ? width <= Number(pixels) : width >= Number(pixels)))
}

// Only the explicit, simple selectors listed by each test are in scope. Class
// count plus type count gives their specificity; !important still wins first.
function effective(selectors: string[], property: string, width: number) {
  let result: { rank: number; value: string } | undefined
  for (const rule of rules) {
    if (!matchesWidth(rule, width)) continue
    const value = rule.declarations.get(property)
    if (value === undefined) continue
    for (const selector of rule.selectors.filter(candidate => selectors.includes(candidate))) {
      const classes = (selector.match(/\.[\w-]+/g) ?? []).length
      const types = (selector.replace(/\.[\w-]+/g, '').match(/\b[a-z][\w-]*\b/g) ?? []).length
      const rank = classes * 100 + types + (value.includes('!important') ? 10_000 : 0)
      if (!result || rank >= result.rank) result = { rank, value: value.replace(/\s*!important\s*$/, '') }
    }
  }
  return result?.value
}

const last = ['.last-draw', '.game-room .last-draw', `${markSix} .last-draw`]
const latestNumbers = ['.last-draw div', '.game-room .last-draw > div', `${markSix} .last-draw > div`]
const latestSummary = ['.last-draw small', '.last-draw > small', '.game-room .last-draw > small', `${markSix} .last-draw > small`]
const historyRow = ['.recent-draws article', '.game-room .recent-draws article', `${markSix} .recent-draws article`]
const historyNumbers = ['.recent-draws article > div', '.game-room .recent-draws article > div', `${markSix} .recent-draws article > div`]
const historySummary = ['.recent-draws article > small', '.game-room .recent-draws article > small', `${markSix} .recent-draws article > small`]

describe('GameRoom Mark Six draw layout CSS contract', () => {
  // At desktop viewport widths the application is still a 430px phone frame.
  // Test both sides of the old 390px override as well as that desktop case.
  it.each([320, 375, 390, 393, 430, 1440])('keeps results out of the seven-ball row at viewport width %i', width => {
    expect(effective(last, 'grid-template-columns', width)).toBe('minmax(0, 1fr) max-content')
    expect(effective(latestNumbers, 'grid-column', width)).toBe('1 / -1')
    expect(effective(latestNumbers, 'grid-row', width)).toBe('2')
    expect(effective(latestSummary, 'grid-column', width)).toBe('2')
    expect(effective(latestSummary, 'grid-row', width)).toBe('1')
    expect(effective(latestSummary, 'white-space', width)).toBe('normal')
    expect(effective(['.game-room .last-draw > span', `${markSix} .last-draw > span`], 'max-width', width)).toBe('none')

    expect(effective(historyRow, 'grid-template-columns', width)).toBe('minmax(76px, 22%) minmax(0, 1fr)')
    expect(effective(historyNumbers, 'grid-column', width)).toBe('2')
    expect(effective(historyNumbers, 'grid-row', width)).toBe('1')
    expect(effective(historySummary, 'grid-column', width)).toBe('2')
    expect(effective(historySummary, 'grid-row', width)).toBe('2')
    // display:flex makes the obsolete inherited 38px/6px/1fr grid irrelevant.
    expect(effective(historySummary, 'display', width)).toBe('flex')
    expect(effective(historySummary, 'flex-wrap', width)).toBe('wrap')
    expect(effective(historySummary, 'white-space', width)).toBe('normal')
    expect(effective(historySummary, 'overflow', width)).toBe('visible')
  })

  it.each([320, 390, 393, 430, 1440])('uses fluid tracks and reserves the special separator inside its cell at width %i', width => {
    const headings = ['.recent-draws header b', '.game-room .recent-draws header > b', `${markSix} .recent-draws header > b`]
    for (const selectors of [latestNumbers, historyNumbers, headings]) {
      expect(effective(selectors, 'grid-template-columns', width)).toBe(ballTracks)
      expect(effective(selectors, 'grid-auto-columns', width)).toBe('auto')
      expect(effective(selectors, 'width', width)).toBe('100%')
    }
    for (const row of ['.last-draw', '.recent-draws article']) {
      const cell = [`${markSix} .mark-six-draw-cell`, `${markSix} ${row} > div > .mark-six-draw-cell`]
      const special = [...cell, `${markSix} .mark-six-draw-cell.is-special`, `${markSix} ${row} > div > .mark-six-draw-cell.is-special`]
      expect(effective(cell, 'box-sizing', width)).toBe('border-box')
      expect(effective(cell, 'width', width)).toBe('min(100%, 29px)')
      expect(effective(cell, 'min-width', width)).toBe('0')
      expect(effective(special, 'width', width)).toBe('min(100%, 40px)')
      expect(effective(special, 'padding-left', width)).toBe('11px')
      expect(effective(special, 'margin-left', width)).toBe('0')
    }
    const ball = [`${markSix} .mark-six-draw-cell > .mark-six-ball`]
    expect(effective(ball, 'width', width)).toBe('100%')
    expect(effective(ball, 'min-width', width)).toBe('0')
    expect(effective(ball, 'height', width)).toBe('auto')
    expect(effective(ball, 'aspect-ratio', width)).toBe('1')
    const specialHeading = [`${markSix} .recent-draws header > b > i:last-child`]
    expect(effective(specialHeading, 'width', width)).toBe('min(100%, 40px)')
    expect(effective(specialHeading, 'padding-left', width)).toBe('11px')
    expect(effective([`${markSix} .mark-six-draw-cell.is-special::before`], 'left', width)).toBe('0')
  })

  it('hides only the obsolete Mark Six summary heading and keeps expanded history scrollable', () => {
    expect(effective(['.game-room .recent-draws header > small', `${markSix} .recent-draws header > small`], 'display', 375)).toBe('none')
    const openHistory = ['.recent-draws', '.draw-history.open .recent-draws', '.game-room .draw-history.open .recent-draws']
    expect(effective(openHistory, 'overflow-y', 375)).toBe('auto')
    expect(effective(openHistory, 'max-height', 375)).toBe('360px')
  })

  it.each([375, 430, 1440])('preserves the racing single-line layout at viewport width %i', width => {
    const racingLast = ['.last-draw', '.game-room .last-draw', `${racing} .last-draw`]
    const racingNumbers = ['.game-room .last-draw > div', `${racing} .last-draw > div`]
    expect(effective(racingLast, 'grid-template-columns', width)).toBe('max-content minmax(0, 1fr) max-content')
    expect(effective(racingNumbers, 'grid-row', width)).toBeUndefined()
    expect(effective(racingNumbers, 'grid-template-columns', width)).toBeUndefined()
    expect(effective(racingNumbers, 'grid-auto-columns', width)).toBe('16px')
    const racingSummary = ['.game-room .recent-draws article > small', `${racing} .recent-draws article > small`]
    expect(effective(racingSummary, 'display', width)).toBe('grid')
    expect(effective(racingSummary, 'grid-column', width)).toBe('3')
    expect(effective(racingSummary, 'grid-row', width)).toBe('1')
  })
})
