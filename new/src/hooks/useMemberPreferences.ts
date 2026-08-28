import { usePersistentState } from './usePersistentState'

export type BetModePreference = 'quick' | 'dual' | 'numbers'
export type FontScalePreference = 'standard' | 'large' | 'xlarge'
export type DisplayStylePreference = 'scenic' | 'simple'

export function useMemberPreferences() {
  const [drawHistoryLimit, setDrawHistoryLimit] = usePersistentState('seven-star-draw-history', 50)
  const [defaultBetMode, setDefaultBetMode] = usePersistentState<BetModePreference>('seven-star-bet-mode', 'quick')
  const [fontScale, setFontScale] = usePersistentState<FontScalePreference>('seven-star-font-scale', 'standard')
  const [displayStyle, setDisplayStyle] = usePersistentState<DisplayStylePreference>('seven-star-display-style', 'scenic')
  return { drawHistoryLimit, setDrawHistoryLimit, defaultBetMode, setDefaultBetMode, fontScale, setFontScale, displayStyle, setDisplayStyle }
}
