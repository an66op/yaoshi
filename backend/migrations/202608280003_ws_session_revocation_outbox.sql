-- Security-sensitive account changes must leave a durable revocation receipt
-- in the same PostgreSQL transaction.  Redis is only the delivery transport:
-- a backend crash or Redis outage must not erase the intent to close sockets
-- authenticated with the previous credential generation.

CREATE TABLE IF NOT EXISTS ws_session_revocation_outbox (
    id bigserial PRIMARY KEY,
    user_id bigint NOT NULL,
    revoked_auth_version bigint NOT NULL,
    attempt_count integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    stream_id varchar(80),
    last_error text,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    delivered_at timestamptz,
    CONSTRAINT chk_ws_revocation_user_positive CHECK (user_id > 0),
    CONSTRAINT chk_ws_revocation_version_positive CHECK (revoked_auth_version > 0),
    CONSTRAINT chk_ws_revocation_attempt_nonnegative CHECK (attempt_count >= 0),
    CONSTRAINT chk_ws_revocation_delivery_state CHECK (
        (delivered_at IS NULL AND stream_id IS NULL)
        OR (delivered_at IS NOT NULL AND stream_id IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_ws_session_revocation_pending
    ON ws_session_revocation_outbox (next_attempt_at, id)
    WHERE delivered_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_ws_session_revocation_delivered
    ON ws_session_revocation_outbox (delivered_at, id)
    WHERE delivered_at IS NOT NULL;

-- All fields below participate in session authorization.  Advance the shared
-- JWT/WebSocket generation even if an application call site forgets to do so.
-- Password updates that already increment auth_version remain single-step.
CREATE OR REPLACE FUNCTION bump_user_session_generation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF (
        OLD.password IS DISTINCT FROM NEW.password
        OR OLD.status IS DISTINCT FROM NEW.status
        OR OLD.role IS DISTINCT FROM NEW.role
        OR OLD.workspace_id IS DISTINCT FROM NEW.workspace_id
        OR OLD.login_scope IS DISTINCT FROM NEW.login_scope
        OR OLD.parent_agent_id IS DISTINCT FROM NEW.parent_agent_id
        OR OLD.parent_tenant_id IS DISTINCT FROM NEW.parent_tenant_id
        OR OLD.agent_room_code IS DISTINCT FROM NEW.agent_room_code
        OR OLD.deleted_at IS DISTINCT FROM NEW.deleted_at
    ) AND NEW.auth_version IS NOT DISTINCT FROM OLD.auth_version THEN
        NEW.auth_version := OLD.auth_version + 1;
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_bump_user_session_generation ON "user";
CREATE TRIGGER trg_bump_user_session_generation
    BEFORE UPDATE OF password, status, role, workspace_id, login_scope,
        parent_agent_id, parent_tenant_id, agent_room_code, deleted_at, auth_version
    ON "user"
    FOR EACH ROW
    EXECUTE FUNCTION bump_user_session_generation();

CREATE OR REPLACE FUNCTION enqueue_user_session_revocation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.auth_version IS DISTINCT FROM NEW.auth_version THEN
        INSERT INTO ws_session_revocation_outbox (user_id, revoked_auth_version)
        VALUES (OLD.user_id, OLD.auth_version);
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_enqueue_user_session_revocation ON "user";
CREATE TRIGGER trg_enqueue_user_session_revocation
    AFTER UPDATE OF password, status, role, workspace_id, login_scope,
        parent_agent_id, parent_tenant_id, agent_room_code, deleted_at, auth_version
    ON "user"
    FOR EACH ROW
    EXECUTE FUNCTION enqueue_user_session_revocation();
