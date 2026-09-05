#!/usr/bin/env bash
set -euo pipefail

ENV_SOURCE="${1:-/etc/wangzhe/ops-alert.env}"
FAILED_UNIT="${2:-}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/strict-env.sh
source "$SCRIPT_DIR/lib/strict-env.sh"
load_strict_env "$ENV_SOURCE" '^OPS_ALERT_[A-Z0-9_]+$'

for command_name in curl date jq mktemp rm; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done
: "${OPS_ALERT_WEBHOOK_URL:?缺少 OPS_ALERT_WEBHOOK_URL}"
: "${OPS_ALERT_WEBHOOK_BEARER_TOKEN:?缺少 OPS_ALERT_WEBHOOK_BEARER_TOKEN}"
[[ "$FAILED_UNIT" =~ ^wangzhe-[A-Za-z0-9@_.:-]+\.service$ ]] || {
  echo "拒绝发送格式无效的失败单元名：$FAILED_UNIT" >&2
  exit 1
}
webhook_url_pattern='^https://[^[:space:]"\\]+$'
[[ "$OPS_ALERT_WEBHOOK_URL" =~ $webhook_url_pattern ]] || { echo "失败告警 Webhook 必须是 HTTPS" >&2; exit 1; }
[[ "$OPS_ALERT_WEBHOOK_URL" != *CHANGE_ME* && "$OPS_ALERT_WEBHOOK_BEARER_TOKEN" != *CHANGE_ME* && ${#OPS_ALERT_WEBHOOK_BEARER_TOKEN} -ge 20 ]] || {
  echo "失败告警 Webhook/令牌仍是示例值或过短" >&2
  exit 1
}
case "$OPS_ALERT_WEBHOOK_BEARER_TOKEN" in
  *'"'*|*'\'*|*$'\r'*|*$'\n'*) echo "失败告警令牌包含不安全字符" >&2; exit 1 ;;
esac

umask 077
payload_file="$(mktemp)"
curl_config="$(mktemp)"
cleanup() { rm -f -- "$payload_file" "$curl_config"; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
now_epoch="$(date +%s)"
jq -n --arg unit "$FAILED_UNIT" --argjson timestamp "$now_epoch" \
  '{service:"wangzhe",status:"firing",timestamp:$timestamp,alerts:[("systemd任务失败：" + $unit)]}' >"$payload_file"
{
  printf 'url = "%s"\n' "$OPS_ALERT_WEBHOOK_URL"
  printf 'request = "POST"\nheader = "Content-Type: application/json"\n'
  printf 'header = "Authorization: Bearer %s"\n' "$OPS_ALERT_WEBHOOK_BEARER_TOKEN"
  printf 'fail\nsilent\nshow-error\nmax-time = 10\n'
} >"$curl_config"
curl --config "$curl_config" --data-binary "@$payload_file" >/dev/null
echo "已发送 systemd 失败告警：$FAILED_UNIT"
