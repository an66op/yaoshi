CREATE TABLE system_event_logs (
    id bigserial PRIMARY KEY,
    category varchar(24) NOT NULL,
    event_type varchar(40) NOT NULL,
    level varchar(12) NOT NULL,
    status varchar(20) NOT NULL,
    source_group varchar(48) NOT NULL DEFAULT '',
    game_id varchar(40) NOT NULL DEFAULT '',
    job_id varchar(64) NOT NULL DEFAULT '',
    message varchar(500) NOT NULL DEFAULT '',
    imported integer NOT NULL DEFAULT 0 CHECK (imported >= 0),
    latest_issue varchar(64) NOT NULL DEFAULT '',
    consecutive_errors integer NOT NULL DEFAULT 0 CHECK (consecutive_errors >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_system_event_category CHECK (category IN ('source', 'scheduler')),
    CONSTRAINT chk_system_event_level CHECK (level IN ('info', 'warning', 'error')),
    CONSTRAINT chk_system_event_status CHECK (status IN ('ok', 'error', 'standby', 'started', 'stopped'))
);

CREATE INDEX idx_system_event_logs_created_at ON system_event_logs (created_at DESC, id DESC);
CREATE INDEX idx_system_event_logs_category ON system_event_logs (category, id DESC);
CREATE INDEX idx_system_event_logs_event_type ON system_event_logs (event_type, id DESC);
CREATE INDEX idx_system_event_logs_status ON system_event_logs (status, id DESC);
CREATE INDEX idx_system_event_logs_game_id ON system_event_logs (game_id, id DESC) WHERE game_id <> '';
CREATE INDEX idx_system_event_logs_source_group ON system_event_logs (source_group, id DESC) WHERE source_group <> '';

CREATE OR REPLACE FUNCTION guard_system_event_log_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'system event logs are append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER system_event_logs_append_only
    BEFORE UPDATE OR DELETE ON system_event_logs
    FOR EACH ROW EXECUTE FUNCTION guard_system_event_log_mutation();
