-- Legacy robot schedulers ran without daily or pending-bet circuit breakers.
-- Pause every existing scheduler once during upgrade so an operator explicitly
-- reviews its room, games and limits before resuming it.

ALTER TABLE IF EXISTS workspace_robot_settings
    ADD COLUMN IF NOT EXISTS daily_bet_limit integer NOT NULL DEFAULT 200;
ALTER TABLE IF EXISTS workspace_robot_settings
    ADD COLUMN IF NOT EXISTS max_pending_bets integer NOT NULL DEFAULT 50;
ALTER TABLE IF EXISTS workspace_robot_settings
    ADD COLUMN IF NOT EXISTS pause_reason varchar(240) NOT NULL DEFAULT '';

DO $$
BEGIN
    IF to_regclass('workspace_robot_settings') IS NOT NULL THEN
        UPDATE workspace_robot_settings
        SET enabled = false,
            interval_secs = GREATEST(30, LEAST(interval_secs, 3600)),
            bets_per_cycle = GREATEST(1, LEAST(bets_per_cycle, 20)),
            daily_bet_limit = CASE WHEN daily_bet_limit < 1 THEN 200 ELSE LEAST(daily_bet_limit, 10000) END,
            max_pending_bets = CASE WHEN max_pending_bets < 1 THEN 50 ELSE LEAST(max_pending_bets, 5000) END,
            pause_reason = CASE
                WHEN enabled THEN '升级后已安全暂停，请核对房间、彩种与限额后手动开启'
                ELSE pause_reason
            END,
            updated_at = now();
    END IF;
END $$;
