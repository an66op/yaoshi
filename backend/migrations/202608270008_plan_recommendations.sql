CREATE TABLE IF NOT EXISTS plan_recommendations (
    id bigserial PRIMARY KEY,
    workspace_id bigint NOT NULL REFERENCES workspaces(id),
    game_id varchar(40) NOT NULL REFERENCES lottery_games(id),
    issue varchar(64) NOT NULL,
    master_name varchar(60) NOT NULL,
    master_title varchar(80) NOT NULL DEFAULT '',
    master_color varchar(16) NOT NULL DEFAULT '#2aa9b3',
    numbers varchar(120) NOT NULL DEFAULT '',
    size varchar(4) NOT NULL DEFAULT '',
    parity varchar(4) NOT NULL DEFAULT '',
    result varchar(16) NOT NULL DEFAULT 'pending',
    note varchar(500) NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 100,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_plan_recommendations_workspace ON plan_recommendations(workspace_id);
CREATE INDEX IF NOT EXISTS idx_plan_recommendations_game ON plan_recommendations(game_id);
CREATE INDEX IF NOT EXISTS idx_plan_recommendations_issue ON plan_recommendations(issue);
CREATE INDEX IF NOT EXISTS idx_plan_recommendations_result ON plan_recommendations(result);
CREATE INDEX IF NOT EXISTS idx_plan_recommendations_enabled ON plan_recommendations(enabled);
CREATE INDEX IF NOT EXISTS idx_plan_recommendations_deleted_at ON plan_recommendations(deleted_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_plan_recommendation_identity
    ON plan_recommendations(workspace_id, game_id, issue, master_name)
    WHERE deleted_at IS NULL;

-- Give existing active rooms a small, editable persisted catalog. This is a
-- one-time migration; deleting or editing the rows is respected on restart.
-- Rows are only created for a real issue that is currently accepting bets and
-- remain pending until an operator records their actual outcome.
WITH templates(game_id, master_name, master_title, master_color, numbers, size, parity, sort_order) AS (
    VALUES
      ('speed-racing', '青云老师', '综合趋势', '#2aa9b3', '1,5,9', '大', '单', 10),
      ('speed-racing', '北斗数据师', '冷热分析', '#6e70df', '2,6,10', '小', '双', 20),
      ('speed-racing', '锦鲤计划师', '节奏追踪', '#e58b45', '3,4,8', '大', '双', 30),
      ('canada-28', '青云老师', '综合趋势', '#2aa9b3', '3,14,22', '大', '单', 10),
      ('canada-28', '北斗数据师', '冷热分析', '#6e70df', '6,11,19', '小', '双', 20),
      ('canada-28', '锦鲤计划师', '节奏追踪', '#e58b45', '8,17,25', '大', '双', 30),
      ('au-lucky-10', '青云老师', '综合趋势', '#2aa9b3', '1,4,8', '大', '单', 10),
      ('au-lucky-10', '北斗数据师', '冷热分析', '#6e70df', '2,5,9', '小', '双', 20),
      ('au-lucky-10', '锦鲤计划师', '节奏追踪', '#e58b45', '3,6,10', '大', '双', 30)
)
INSERT INTO plan_recommendations (
    workspace_id, game_id, issue, master_name, master_title, master_color,
    numbers, size, parity, result, note, enabled, sort_order
)
SELECT
    workspace.id,
    template.game_id,
    current_issue.issue,
    template.master_name, template.master_title, template.master_color,
    template.numbers, template.size, template.parity, 'pending',
    '房间人工计划，由后台维护', true, template.sort_order
FROM workspaces AS workspace
CROSS JOIN templates AS template
JOIN lottery_games AS game ON game.id = template.game_id
JOIN LATERAL (
    SELECT issue.issue
    FROM lottery_issues AS issue
    WHERE issue.game_id = template.game_id
      AND issue.status = 'accepting'
    ORDER BY issue.seal_at DESC, issue.id DESC
    LIMIT 1
) AS current_issue ON true
WHERE workspace.type IN ('tenant', 'agent')
  AND workspace.status = 1
  AND workspace.room_code <> ''
ON CONFLICT DO NOTHING;
