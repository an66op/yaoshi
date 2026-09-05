#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/backend-env.sh
source "$ROOT_DIR/scripts/lib/backend-env.sh"

if (( $# > 1 )); then
  echo "用法：scripts/local-smoke.sh [ENV_FILE]" >&2
  exit 1
fi

load_optional_backend_env "${1:-}"
apply_local_backend_defaults
require_local_backend_target

for command_name in go jq; do
  command -v "$command_name" >/dev/null 2>&1 || {
    echo "缺少命令：$command_name" >&2
    exit 1
  }
done

# This command is intentionally independent of running HTTP services. The
# database transaction is forced read-only by dev-bootstrap, so an audit can
# never rerun migrations, seed accounts, reopen games or rewrite odds.
bootstrap_report="$(
  cd "$ROOT_DIR/backend"
  go run ./cmd/dev-bootstrap --confirm-local-development --audit-only
)"
jq -e '
  .profile_version == "development-acceptance-odds-v1" and
  .human_accounts >= 4 and
  .robot_accounts >= 30 and
  .workspaces >= 3 and
  .active_accounts >= 34 and
  .active_memberships == .active_accounts and
  .configured_games == 22 and
  .configured_play_quotes == 1437 and
  .agent_room_code == "88001" and
  .agent_room_open_games == 22 and
  .agent_room_robot_quota == 10 and
  .agent_room_robots == 10
' <<<"$bootstrap_report" >/dev/null

echo "本地只读验收通过：四账号层级、工作区关系、账务链、1437 项赔率、88001 房间及 10 个机器人名额均正常"
