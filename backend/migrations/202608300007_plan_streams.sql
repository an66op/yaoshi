-- Additive only: published legacy picks and their receipts are immutable.
-- The matrix is permitted only when the existing room opt-in is enabled.
ALTER TABLE plan_automations ADD COLUMN IF NOT EXISTS positions_json text NOT NULL DEFAULT '[1,2,3,4,5,6,7,8,9,10]';
ALTER TABLE plan_automations ADD COLUMN IF NOT EXISTS plan_keys_json text NOT NULL DEFAULT '["four-period-five-codes","three-period-five-codes","four-period-six-codes","three-period-six-codes","four-period-seven-codes","three-period-seven-codes","two-period-eight-codes","one-period-eight-codes","size-five-periods","size-four-periods","size-three-periods","parity-five-periods","parity-four-periods","parity-three-periods","dragon-tiger-five-periods","dragon-tiger-four-periods","dragon-tiger-three-periods"]';
CREATE TABLE IF NOT EXISTS plan_streams (
 id bigserial PRIMARY KEY, workspace_id bigint NOT NULL REFERENCES workspaces(id),
 game_id varchar(40) NOT NULL REFERENCES lottery_games(id), position integer NOT NULL CHECK(position BETWEEN 1 AND 10),
 plan_key varchar(48) NOT NULL, active_until timestamptz, revoked boolean NOT NULL DEFAULT false, cycle_id bigint NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT idx_plan_stream_identity UNIQUE(workspace_id, game_id, position, plan_key)
);
CREATE TABLE IF NOT EXISTS plan_stream_cycles (
 id bigserial PRIMARY KEY, stream_id bigint NOT NULL REFERENCES plan_streams(id),
 periods integer NOT NULL CHECK(periods BETWEEN 1 AND 5), published_periods integer NOT NULL DEFAULT 0,
 status varchar(16) NOT NULL DEFAULT 'active' CHECK(status IN ('active','completed','interrupted')),
 start_issue varchar(64) NOT NULL, last_issue_id bigint NOT NULL DEFAULT 0, last_scheduled_at timestamptz,
 payload_json text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_plan_stream_cycles_stream ON plan_stream_cycles(stream_id,id DESC);
CREATE TABLE IF NOT EXISTS plan_stream_periods (
 id bigserial PRIMARY KEY, stream_id bigint NOT NULL REFERENCES plan_streams(id),
 issue_id bigint NOT NULL REFERENCES lottery_issues(id), issue varchar(64) NOT NULL,
 cycle_id bigint NOT NULL REFERENCES plan_stream_cycles(id), period_index integer NOT NULL CHECK(period_index BETWEEN 1 AND 5),
 scheduled_draw_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT idx_plan_stream_period UNIQUE(stream_id,issue_id)
);
CREATE INDEX IF NOT EXISTS idx_plan_stream_periods_recent ON plan_stream_periods(stream_id,id DESC);
