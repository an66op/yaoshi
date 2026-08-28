-- Member odds adjustments are scoped to one room membership. A member who
-- moves to another workspace must inherit that workspace's own configuration.
ALTER TABLE workspace_memberships
    ADD COLUMN IF NOT EXISTS odds_multiplier numeric(6,4) NOT NULL DEFAULT 1.0000;

ALTER TABLE user_applications
    ADD COLUMN IF NOT EXISTS review_odds_multiplier numeric(6,4) NOT NULL DEFAULT 1.0000;

ALTER TABLE workspace_memberships
    DROP CONSTRAINT IF EXISTS chk_workspace_membership_odds_multiplier;
ALTER TABLE workspace_memberships
    ADD CONSTRAINT chk_workspace_membership_odds_multiplier
    CHECK (odds_multiplier >= 0.5000 AND odds_multiplier <= 1.5000);

ALTER TABLE user_applications
    DROP CONSTRAINT IF EXISTS chk_application_review_odds_multiplier;
ALTER TABLE user_applications
    ADD CONSTRAINT chk_application_review_odds_multiplier
    CHECK (review_odds_multiplier >= 0.5000 AND review_odds_multiplier <= 1.5000);

COMMENT ON COLUMN workspace_memberships.odds_multiplier IS
    'Room-scoped member multiplier. Exact user play odds override it; otherwise it scales room/platform odds.';
COMMENT ON COLUMN user_applications.review_odds_multiplier IS
    'Multiplier selected by the reviewer for an approved join request.';
