import { describe, expect, it } from 'vitest'
import type { AdminApplication } from '../api'
import { isBalanceApplication, reviewedBalance } from './workspaceReview'

const application = (requestType: AdminApplication['request_type']): AdminApplication => ({
  id: 1,
  workspace_id: 8,
  user_id: 10,
  username: 'member',
  user_balance: 500,
  account_type: 'member',
  request_type: requestType,
  payment_type: 'manual',
  requested_amount: 120,
  received_amount: 0,
  remark: '',
  status: 'pending',
  operator: '',
  review_remark: '',
  reviewed_at: null,
  created_at: '2026-08-27T00:00:00Z',
  updated_at: '2026-08-27T00:00:00Z',
})

describe('workspace application review balance preview', () => {
  it('上分按实际到账金额增加余额', () => {
    expect(reviewedBalance(application('credit'), 100)).toBe(600)
  })

  it('下分按申请金额扣除，实际出款只作为审核记录', () => {
    expect(reviewedBalance(application('debit'), 95)).toBe(380)
  })

  it('入房申请不改变余额', () => {
    const row = application('join')
    expect(isBalanceApplication(row)).toBe(false)
    expect(reviewedBalance(row, 0)).toBe(500)
  })
})
