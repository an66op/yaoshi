-- Scheduling metadata is not a published draw. Never rewrite historic results,
-- bets or balances while upgrading the future-period scheduling contract.
ALTER TABLE public.lottery_games
    ADD COLUMN IF NOT EXISTS next_issue varchar(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS timing_source varchar(24) NOT NULL DEFAULT 'configured';

ALTER TABLE public.lottery_issues
    ADD COLUMN IF NOT EXISTS scheduled_draw_at timestamptz;

CREATE TABLE IF NOT EXISTS public.lottery_issue_windows (
    id bigserial PRIMARY KEY,
    workspace_id bigint NOT NULL,
    game_id varchar(40) NOT NULL,
    issue varchar(64) NOT NULL,
    accept_at timestamptz NOT NULL,
    seal_at timestamptz NOT NULL,
    scheduled_draw_at timestamptz NOT NULL,
    draw_interval integer NOT NULL CHECK (draw_interval > 0),
    seal_seconds integer NOT NULL CHECK (seal_seconds >= 0),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT idx_lottery_issue_window UNIQUE (workspace_id, game_id, issue),
    CONSTRAINT lottery_issue_window_cutoff CHECK (seal_at <= scheduled_draw_at)
);
