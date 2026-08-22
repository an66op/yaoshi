import { Alert, Snackbar } from '@mui/material'
import { useCallback, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { FeedbackContext } from './feedback'
import type { FeedbackTone } from './feedback'

export function FeedbackProvider({ children }: { children: ReactNode }) {
  const [feedback, setFeedback] = useState<{ message: string; tone: FeedbackTone } | null>(null)
  const showMessage = useCallback((message: string, tone: FeedbackTone = 'success') => setFeedback({ message, tone }), [])
  const value = useMemo(() => ({ showMessage }), [showMessage])

  return <FeedbackContext.Provider value={value}>
    {children}
    <Snackbar open={Boolean(feedback)} autoHideDuration={2600} onClose={() => setFeedback(null)} anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}>
      <Alert variant="filled" severity={feedback?.tone ?? 'success'} onClose={() => setFeedback(null)} sx={{ minWidth: { xs: 280, sm: 360 }, boxShadow: 8 }}>
        {feedback?.message}
      </Alert>
    </Snackbar>
  </FeedbackContext.Provider>
}
