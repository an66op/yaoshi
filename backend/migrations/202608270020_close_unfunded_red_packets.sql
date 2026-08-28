-- Older chat envelopes credited members without first reserving room funds.
-- They cannot remain claimable after the reserve ledger becomes authoritative,
-- otherwise a legacy row could still mint points. Preserve them for history,
-- but close the outstanding liability without changing any account balance.
UPDATE chat_red_packets
SET status = 'expired',
    remaining_cents = 0,
    closed_at = COALESCE(closed_at, now()),
    closed_by = CASE WHEN closed_by = '' THEN '系统迁移' ELSE closed_by END,
    close_reason = CASE WHEN close_reason = '' THEN '历史红包未预留资金，已安全关闭' ELSE close_reason END,
    updated_at = now()
WHERE funding_user_id = 0
  AND status = 'active';
