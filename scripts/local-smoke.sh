#!/usr/bin/env bash
set -euo pipefail

API="${BACKEND_URL:-http://127.0.0.1:8080}/api"

member_token="$(curl -fsS -H 'Content-Type: application/json' -d '{"username":"wangzhe88","password":"Wz888888"}' "$API/member/login" | jq -r '.data.token')"
curl -fsS -X POST -H 'Content-Type: application/json' -H "Authorization: Bearer $member_token" -d '{"room_code":"8801"}' "$API/member/room/join" >/dev/null
member="$(curl -fsS -H "Authorization: Bearer $member_token" "$API/member/me")"
[[ "$(printf '%s' "$member" | jq -r '.data.room_code')" == "8801" ]]

# Member histories are cursor based. When there is another page, assert that
# the second request continues strictly before the last id from page one.
first_bets="$(curl -fsS -H "Authorization: Bearer $member_token" "$API/member/bets?page_size=2")"
first_last_id="$(printf '%s' "$first_bets" | jq -r '.data.next_before_id // 0')"
if [[ "$(printf '%s' "$first_bets" | jq -r '.data.has_more')" == "true" ]]; then
	second_bets="$(curl -fsS -H "Authorization: Bearer $member_token" "$API/member/bets?page_size=2&before_id=$first_last_id")"
	second_first_id="$(printf '%s' "$second_bets" | jq -r '.data.items[0].id // 0')"
	[[ "$second_first_id" -gt 0 && "$second_first_id" -lt "$first_last_id" ]]
fi

admin_token="$(curl -fsS -H 'Content-Type: application/json' -d '{"username":"admin","password":"123456"}' "$API/login" | jq -r '.data.token')"
reconciliation="$(curl -fsS -H "Authorization: Bearer $admin_token" "$API/admin/reconciliation")"
[[ "$(printf '%s' "$reconciliation" | jq -r '.code')" == "200" ]]

agent_token="$(curl -fsS -H 'Content-Type: application/json' -d '{"username":"suyang","password":"Room8801"}' "$API/login" | jq -r '.data.token')"
agent_me="$(curl -fsS -H "Authorization: Bearer $agent_token" "$API/agent/me")"
agent_room="$(printf '%s' "$agent_me" | jq -r '.data.room_code')"
agent_id="$(printf '%s' "$agent_me" | jq -r '.data.id')"
[[ "$agent_room" == "8801" ]]
agent_report_code="$(curl -fsS -H "Authorization: Bearer $agent_token" "$API/agent/reports/operating" | jq -r '.code')"
[[ "$agent_report_code" == "200" ]]
agent_share_code="$(curl -fsS -H "Authorization: Bearer $agent_token" "$API/agent/reports/profit-shares" | jq -r '.code')"
[[ "$agent_share_code" == "200" ]]
admin_status="$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $agent_token" "$API/admin/dashboard")"
[[ "$admin_status" == "403" ]]

# Every management role has a separate route boundary. A token cannot gain a
# wider role by changing a frontend route or the role value in localStorage.
tenant_token="$(curl -fsS -H 'Content-Type: application/json' -d '{"username":"wangzhetenant","password":"WzTenant8801"}' "$API/login" | jq -r '.data.token')"
[[ "$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $tenant_token" "$API/tenant/dashboard")" == "200" ]]
[[ "$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $tenant_token" "$API/agent/dashboard")" == "403" ]]
[[ "$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $agent_token" "$API/tenant/dashboard")" == "403" ]]
[[ "$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $member_token" "$API/admin/dashboard")" == "403" ]]
[[ "$(curl -sS -o /dev/null -w '%{http_code}' "$API/admin/dashboard")" == "401" ]]

# Agent list queries are row-scoped. The fixture includes lobby users that do
# not belong to room 8801; none may appear in the agent workspace.
agent_users="$(curl -fsS -H "Authorization: Bearer $agent_token" "$API/agent/users?page_size=100")"
[[ "$(printf '%s' "$agent_users" | jq --argjson id "$agent_id" '[.data.items[] | select(.parent_agent_id != $id)] | length')" == "0" ]]
foreign_user_id="$(curl -fsS -H "Authorization: Bearer $admin_token" "$API/admin/users?page_size=100&role=member" | jq -r --argjson id "$agent_id" '.data.items[] | select(.parent_agent_id != $id) | .id' | head -n 1)"
if [[ -n "$foreign_user_id" ]]; then
	[[ "$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $agent_token" "$API/agent/bets?user_id=$foreign_user_id")" == "403" ]]
	[[ "$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $agent_token" "$API/agent/chat/messages?scope=user%3A$foreign_user_id&room_type=service&game_id=service")" == "403" ]]
fi

echo "烟雾测试通过：会员房间与注单游标、管理员对账、角色路由与代理行级隔离均正常"
