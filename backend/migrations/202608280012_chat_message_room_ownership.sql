-- A chat message is a historical room record. A member may later activate a
-- different workspace, but the original room must keep its conversation and
-- the new room must never inherit it. Guard the complete ownership key at the
-- database boundary while leaving presentation snapshots (nickname/avatar)
-- and lifecycle deleted_at transitions independent.

CREATE OR REPLACE FUNCTION guard_chat_message_room_ownership()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND (
        NEW.workspace_id IS DISTINCT FROM OLD.workspace_id OR
        NEW.user_id IS DISTINCT FROM OLD.user_id OR
        NEW.room_type IS DISTINCT FROM OLD.room_type OR
        NEW.scope IS DISTINCT FROM OLD.scope OR
        NEW.room_scope IS DISTINCT FROM OLD.room_scope OR
        NEW.game_id IS DISTINCT FROM OLD.game_id OR
        NEW.message_type IS DISTINCT FROM OLD.message_type OR
        NEW.reference_id IS DISTINCT FROM OLD.reference_id
    ) THEN
        RAISE EXCEPTION 'chat message ownership is immutable'
            USING ERRCODE = '55000';
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM workspaces AS workspace
        WHERE workspace.id = NEW.workspace_id
          AND workspace.scope = NEW.room_scope
    ) THEN
        RAISE EXCEPTION 'chat message workspace and room scope do not match'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_guard_chat_message_room_ownership
    ON member_chat_messages;
CREATE TRIGGER trg_guard_chat_message_room_ownership
    BEFORE INSERT OR UPDATE OF workspace_id, user_id, room_type, scope,
        room_scope, game_id, message_type, reference_id
    ON member_chat_messages
    FOR EACH ROW EXECUTE FUNCTION guard_chat_message_room_ownership();
