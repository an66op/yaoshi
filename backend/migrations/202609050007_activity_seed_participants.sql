-- Early development defaults displayed fabricated participant totals. Real
-- participation incremented those bases, so recognize only a complete legacy
-- seed signature whose stored counter is exactly legacy_base + actual rows,
-- then replace it with the actual count. Any operator-edited signature or
-- counter is deliberately outside this reconciliation boundary.
WITH legacy_candidate AS (
    SELECT
        activity.id,
        CASE activity.type WHEN 'checkin' THEN 128 ELSE 56 END AS legacy_base,
        COUNT(participation.id)::bigint AS actual_participants
    FROM public.ops_activities AS activity
    LEFT JOIN public.activity_participations AS participation
      ON participation.activity_id = activity.id
    WHERE activity.deleted_at IS NULL
      AND (
          (
              activity.type = 'checkin'
              AND activity.title = '每日签到'
              AND activity.subtitle = '连续签到领取积分'
              AND activity.status = 'active'
              AND COALESCE(activity.cover, '') = ''
              AND activity.reward_cents = 100
              AND activity.pool_total_cents = 0
              AND activity.pool_remaining_cents = 0
              AND activity.config_json = '{"days":7}'
              AND activity.sort_order = 1
              AND activity.starts_at IS NULL
              AND activity.ends_at IS NULL
          )
          OR
          (
              activity.type = 'redpacket'
              AND activity.title = '幸运红包'
              AND activity.subtitle = '开奖聊天室随机红包'
              AND activity.status = 'active'
              AND COALESCE(activity.cover, '') = ''
              AND activity.reward_cents = 888
              AND activity.pool_total_cents = 8800
              AND activity.pool_remaining_cents = 8800
              AND activity.config_json = '{"pool":88,"min_amount":1,"max_amount":8.8}'
              AND activity.sort_order = 4
              AND activity.starts_at IS NULL
              AND activity.ends_at IS NULL
          )
      )
    GROUP BY activity.id, activity.type, activity.participants
    HAVING activity.participants =
        CASE activity.type WHEN 'checkin' THEN 128 ELSE 56 END
        + COUNT(participation.id)
)
UPDATE public.ops_activities AS activity
SET participants = legacy_candidate.actual_participants,
    updated_at = clock_timestamp()
FROM legacy_candidate
WHERE activity.id = legacy_candidate.id
  AND activity.participants =
      legacy_candidate.legacy_base + legacy_candidate.actual_participants;
