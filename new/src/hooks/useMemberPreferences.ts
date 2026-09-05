import { useEffect } from 'react'
import { usePersistentState } from './usePersistentState'

export type BetModePreference = 'chat' | 'detail'
export type FontScalePreference = 'standard' | 'large' | 'xlarge'
export type DisplayStylePreference = 'scenic' | 'simple'

/** Older builds stored the three tabs of the chat keyboard in this key. They
 * all belong to the chat surface, so migrate them without inventing a third
 * room mode. Unknown values also fall back to the safest available surface. */
export function normalizeBetModePreference(value: unknown): BetModePreference {
  return value === 'detail' ? 'detail' : 'chat'
}

export function useMemberPreferences() {
  const [drawHistoryLimit, setDrawHistoryLimit] = usePersistentState('seven-star-draw-history', 50)
  const [storedBetMode, setStoredBetMode] = usePersistentState<unknown>('seven-star-bet-mode', 'chat')
  const defaultBetMode = normalizeBetModePreference(storedBetMode)
  const [fontScale, setFontScale] = usePersistentState<FontScalePreference>('seven-star-font-scale', 'standard')
  const [displayStyle, setDisplayStyle] = usePersistentState<DisplayStylePreference>('seven-star-display-style', 'scenic')
  useEffect(() => {
    if (storedBetMode !== defaultBetMode) setStoredBetMode(defaultBetMode)
  }, [defaultBetMode, setStoredBetMode, storedBetMode])
  const setDefaultBetMode = (value: BetModePreference) => setStoredBetMode(value)
  return { drawHistoryLimit, setDrawHistoryLimit, defaultBetMode, setDefaultBetMode, fontScale, setFontScale, displayStyle, setDisplayStyle }
}
