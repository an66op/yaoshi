import { usePersistentState } from './usePersistentState'

export type BetModePreference = 'quick' | 'dual' | 'numbers'

export function useMemberPreferences() {
  const [drawHistoryLimit, setDrawHistoryLimit] = usePersistentState('seven-star-draw-history', 50)
  const [defaultBetMode, setDefaultBetMode] = usePersistentState<BetModePreference>('seven-star-bet-mode', 'quick')
  return { drawHistoryLimit, setDrawHistoryLimit, defaultBetMode, setDefaultBetMode }
}
