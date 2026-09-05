-- Check-in credits are member points. Reconcile the former cash-oriented copy
-- without touching any amounts, balances or non-check-in ledger records.
UPDATE public.user_balance_transactions
SET remark = replace(remark, '签到奖励', '签到积分')
WHERE type = 'checkin'
  AND remark LIKE '签到奖励 · 连续%天';

UPDATE public.member_notifications
SET content = regexp_replace(
        replace(replace(content, '签到奖励', '签到积分'), '，到账 ', '，获得 '),
        ' 元$',
        ' 积分'
    )
WHERE title = '签到成功'
  AND content LIKE '签到奖励 · 连续%天，到账 % 元';

UPDATE public.ops_activities
SET title = '连续签到七天送积分',
    subtitle = '连续签到，天天领取积分好礼',
    updated_at = clock_timestamp()
WHERE type = 'promotion'
  AND title = '连续签到七天送彩金';
