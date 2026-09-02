import { describe, expect, it, vi } from 'vitest'
import { dispatchProfileNavigation, profileGuideTabForTarget } from '../utils/profileNavigation'

describe('Profile game guide navigation contract', () => {
  it('maps gameplay and odds rows to routed page tabs', () => {
    expect(profileGuideTabForTarget('guideRules')).toBe('rules')
    expect(profileGuideTabForTarget('guideOdds')).toBe('odds')
  })

  it('keeps ordinary profile settings in sheets', () => {
    expect(profileGuideTabForTarget('avatar')).toBeNull()
    expect(profileGuideTabForTarget('theme')).toBeNull()
  })

  it('opens guides through page navigation and preserves sheets for other settings', () => {
    const onOpenGuide = vi.fn()
    const onOpenSheet = vi.fn()
    dispatchProfileNavigation('guideOdds', onOpenGuide, onOpenSheet)
    dispatchProfileNavigation('theme', onOpenGuide, onOpenSheet)

    expect(onOpenGuide).toHaveBeenCalledWith('odds')
    expect(onOpenSheet).toHaveBeenCalledWith('theme')
  })
})
