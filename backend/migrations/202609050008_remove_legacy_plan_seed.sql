-- The original plan-recommendations migration inserted nine fixed editorial
-- examples into every active room. Once outcomes became draw-derived, those
-- examples could be mistaken for real publications and enter hit statistics.
-- Remove only the complete legacy template signature. Any row whose operator
-- changed even one editorial field remains genuine room-owned history.
DROP TRIGGER IF EXISTS trg_reject_viewed_plan_recommendation_change ON plan_recommendations;

WITH legacy_templates(game_id, master_name, master_title, master_color, numbers, size, parity, sort_order) AS (
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
DELETE FROM plan_recommendations AS recommendation
USING legacy_templates AS template
WHERE recommendation.game_id = template.game_id
  AND recommendation.master_name = template.master_name
  AND recommendation.master_title = template.master_title
  AND recommendation.master_color = template.master_color
  AND recommendation.numbers = template.numbers
  AND recommendation.size = template.size
  AND recommendation.parity = template.parity
  AND recommendation.sort_order = template.sort_order
  AND recommendation.result = 'pending'
  AND recommendation.source = 'manual'
  AND recommendation.note = '房间人工计划，由后台维护'
  AND recommendation.enabled = true
  AND recommendation.deleted_at IS NULL;

-- Reinstall the latest guard function from 006. Keeping the trigger disabled
-- beyond this migration would make viewed and sealed publications editable.
CREATE TRIGGER trg_reject_viewed_plan_recommendation_change
BEFORE UPDATE OR DELETE ON plan_recommendations
FOR EACH ROW EXECUTE FUNCTION reject_viewed_plan_recommendation_change();
