-- Only the SG history recovery queue and its attempt journal are new. No
-- existing draw, ticket, balance, source binding or rule version is rewritten.
CREATE TABLE lottery_sgssc_backfill_items (
    issue varchar(11) PRIMARY KEY CHECK (issue ~ '^[0-9]{11}$'),
    draw_at timestamptz NOT NULL,
    status varchar(24) NOT NULL CHECK (status IN ('pending', 'running', 'retry', 'settlement_retry', 'completed', 'blocked')),
    reason varchar(32) NOT NULL,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error varchar(500) NOT NULL DEFAULT '',
    next_retry_at timestamptz NOT NULL,
    lease_until timestamptz,
    requested_by varchar(100) NOT NULL,
    request_trigger varchar(12) NOT NULL CHECK (request_trigger IN ('auto', 'admin')),
    request_id varchar(100) NOT NULL,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_sgssc_backfill_due ON lottery_sgssc_backfill_items (next_retry_at, draw_at)
    WHERE status IN ('pending', 'retry', 'settlement_retry');
CREATE INDEX idx_sgssc_backfill_lease ON lottery_sgssc_backfill_items (lease_until)
    WHERE status = 'running';

CREATE TABLE lottery_sgssc_backfill_attempts (
    id bigserial PRIMARY KEY,
    issue varchar(11) NOT NULL REFERENCES lottery_sgssc_backfill_items(issue) ON DELETE RESTRICT,
    attempt integer NOT NULL CHECK (attempt > 0),
    status varchar(24) NOT NULL CHECK (status IN ('running', 'recovered', 'source_error', 'conflict', 'settlement_error', 'interrupted', 'blocked')),
    trigger varchar(12) NOT NULL CHECK (trigger IN ('auto', 'admin')),
    operator varchar(100) NOT NULL,
    request_id varchar(100) NOT NULL,
    started_at timestamptz NOT NULL,
    finished_at timestamptz,
    numbers varchar(20) NOT NULL DEFAULT '',
    imported boolean NOT NULL DEFAULT false,
    settled_bets bigint NOT NULL DEFAULT 0 CHECK (settled_bets >= 0),
    error varchar(500) NOT NULL DEFAULT '',
    source_revision varchar(64) NOT NULL,
    conversion_revision varchar(64) NOT NULL,
    UNIQUE (issue, attempt),
    CHECK ((status = 'running' AND finished_at IS NULL) OR (status <> 'running' AND finished_at IS NOT NULL))
);

CREATE INDEX idx_sgssc_backfill_attempt_issue ON lottery_sgssc_backfill_attempts (issue, id DESC);

-- A running receipt may record its import and then close exactly once. Its
-- immutable identity, and every finished failure/success, remain evidence.
CREATE FUNCTION guard_sgssc_backfill_attempt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'SG history recovery attempts cannot be deleted';
    END IF;
    IF OLD.status <> 'running'
       OR (NEW.id, NEW.issue, NEW.attempt, NEW.trigger, NEW.operator, NEW.request_id,
           NEW.started_at, NEW.source_revision, NEW.conversion_revision)
          IS DISTINCT FROM
          (OLD.id, OLD.issue, OLD.attempt, OLD.trigger, OLD.operator, OLD.request_id,
           OLD.started_at, OLD.source_revision, OLD.conversion_revision)
       OR (OLD.imported AND NOT NEW.imported) THEN
        RAISE EXCEPTION 'SG history recovery attempt evidence is immutable';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_sgssc_backfill_attempt_evidence
    BEFORE UPDATE OR DELETE ON lottery_sgssc_backfill_attempts
    FOR EACH ROW EXECUTE FUNCTION guard_sgssc_backfill_attempt();

SELECT public.install_application_truncate_guards();
