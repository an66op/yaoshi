-- Keep administrator odds mutations separate from ordinary draw/source
-- updates. A durable monotonic revision prevents stale forms from passing
-- after another administrator saves and then clears a configuration.
ALTER TABLE lottery_games
    ADD COLUMN IF NOT EXISTS odds_config_revision bigint NOT NULL DEFAULT 0;

ALTER TABLE lottery_games
    ADD CONSTRAINT chk_lottery_games_odds_config_revision
    CHECK (odds_config_revision >= 0);

-- An omitted price must never manufacture a numeric quote, even in a direct
-- insert. Activation still requires the explicit current-rule save markers.
ALTER TABLE lottery_play_limits ALTER COLUMN odds SET DEFAULT 0;
