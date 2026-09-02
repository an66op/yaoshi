import type { GameGuideTab } from '../components/GameGuidePanel'

export type ProfileSheetPanel = 'avatar' | 'nickname' | 'security' | 'history' | 'betMode' | 'fontSize' | 'sounds' | 'line' | 'theme' | 'help'
export type ProfileNavigationTarget = ProfileSheetPanel | 'guideRules' | 'guideOdds'

export function profileGuideTabForTarget(target: ProfileNavigationTarget): GameGuideTab | null {
  if (target === 'guideRules') return 'rules'
  if (target === 'guideOdds') return 'odds'
  return null
}

export function dispatchProfileNavigation(
  target: ProfileNavigationTarget,
  onOpenGuide: (tab: GameGuideTab) => void,
  onOpenSheet: (panel: ProfileSheetPanel) => void,
) {
  if (target === 'guideRules') {
    onOpenGuide('rules')
    return
  }
  if (target === 'guideOdds') {
    onOpenGuide('odds')
    return
  }
  onOpenSheet(target)
}
