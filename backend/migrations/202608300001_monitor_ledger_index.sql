-- Keep the minute-level accounting invariant bounded by one newest ledger row
-- per user in each hot/archive table instead of sorting the full history.
CREATE INDEX IF NOT EXISTS idx_balance_archive_user_id_id
    ON user_balance_transaction_archives (user_id, id DESC);

-- The full chain audit is intentionally low-frequency, but this covering index
-- lets PostgreSQL evaluate the ordered LAG without sorting or fetching every
-- hot-ledger heap row. The small partial index makes the common "no arithmetic
-- error" result an empty index-only scan.
CREATE INDEX IF NOT EXISTS idx_balance_ledger_monitor_chain
    ON user_balance_transactions (user_id, id)
    INCLUDE (before_cents, after_cents);

CREATE INDEX IF NOT EXISTS idx_balance_ledger_monitor_arithmetic_error
    ON user_balance_transactions (id)
    WHERE after_cents <> before_cents + amount_cents
       OR before_cents < 0
       OR after_cents < 0;
