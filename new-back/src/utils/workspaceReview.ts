import type { AdminApplication } from '../api'

export function isBalanceApplication(application: AdminApplication) {
  return application.request_type === 'credit' || application.request_type === 'debit'
}
export function reviewedBalance(application: AdminApplication, receivedAmount: number) {
  if (application.request_type === 'credit') return application.user_balance + receivedAmount
  if (application.request_type === 'debit') return application.user_balance - application.requested_amount
  return application.user_balance
}
