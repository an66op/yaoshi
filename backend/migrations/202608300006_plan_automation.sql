-- No data seeding: a platform administrator must explicitly opt in each room.
CREATE TABLE IF NOT EXISTS plan_automations (
    workspace_id bigint PRIMARY KEY REFERENCES workspaces(id),
    enabled boolean NOT NULL DEFAULT false,
    mode varchar(16) NOT NULL DEFAULT 'demo' CHECK (mode = 'demo'),
    game_ids_json text NOT NULL DEFAULT '[]',
    last_run_at timestamptz,
    last_created_count bigint NOT NULL DEFAULT 0,
    last_error varchar(500) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_plan_automations_enabled ON plan_automations(enabled);

CREATE TABLE IF NOT EXISTS plan_generation_receipts (
    id bigserial PRIMARY KEY,
    workspace_id bigint NOT NULL REFERENCES workspaces(id),
    game_id varchar(40) NOT NULL REFERENCES lottery_games(id),
    issue varchar(64) NOT NULL,
    master_key varchar(32) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT idx_plan_generation_identity UNIQUE (workspace_id, game_id, issue, master_key)
);

ALTER TABLE plan_recommendations
    ADD COLUMN IF NOT EXISTS source varchar(16) NOT NULL DEFAULT 'manual';
