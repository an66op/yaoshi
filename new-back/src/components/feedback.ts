import { createContext, useContext } from 'react'

export type FeedbackTone = 'success' | 'info' | 'warning' | 'error'
export type FeedbackContextValue = { showMessage: (message: string, tone?: FeedbackTone) => void }

export const FeedbackContext = createContext<FeedbackContextValue | null>(null)

export function useFeedback() {
  const context = useContext(FeedbackContext)
  if (!context) throw new Error('useFeedback must be used inside FeedbackProvider')
  return context
}
