-- Every odds contract must be activated by an auditable administrator save.
-- The project has not launched, so no code-level or legacy quote is promoted
-- into a live market. Rule-version binding prevents a later rules upgrade from
-- silently reusing a quote confirmed for an older contract.
ALTER TABLE lottery_play_limits
    ADD COLUMN IF NOT EXISTS explicitly_configured boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS rule_version varchar(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS configuration_source varchar(32) NOT NULL DEFAULT 'unconfigured',
    ADD COLUMN IF NOT EXISTS configured_at timestamptz;

UPDATE lottery_play_limits
SET explicitly_configured = false,
    rule_version = '',
    configuration_source = 'unconfigured',
    configured_at = NULL;

CREATE INDEX IF NOT EXISTS idx_lottery_play_limits_confirmation
    ON lottery_play_limits (game_id, rule_version, explicitly_configured, play_code);
