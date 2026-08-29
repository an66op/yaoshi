#!/usr/bin/env bash
set -euo pipefail

ENV_SOURCE="${1:-/etc/wangzhe/redis-check.env}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/strict-env.sh
source "$SCRIPT_DIR/lib/strict-env.sh"
if [[ "$ENV_SOURCE" != "--current-env" ]]; then
  load_strict_env "$ENV_SOURCE" '^REDIS_[A-Z0-9_]+$'
fi

for command_name in awk grep redis-cli tail tr uname; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done
: "${REDIS_HOST:?缺少 REDIS_HOST}"
: "${REDIS_PORT:?缺少 REDIS_PORT}"
: "${REDIS_USERNAME:?缺少 REDIS_USERNAME}"
: "${REDIS_PASSWORD:?缺少 REDIS_PASSWORD}"
: "${REDIS_TLS:?缺少 REDIS_TLS}"
: "${REDIS_EXPECTED_APP_USERNAME:?缺少 REDIS_EXPECTED_APP_USERNAME}"
: "${REDIS_EXPECTED_APP_PREFIX:?缺少 REDIS_EXPECTED_APP_PREFIX}"
[[ "$REDIS_HOST" =~ ^[A-Za-z0-9_.:-]+$ ]] || { echo "REDIS_HOST 格式不安全" >&2; exit 1; }
[[ "$REDIS_USERNAME" =~ ^[A-Za-z0-9_.-]{1,64}$ && "$REDIS_USERNAME" != default ]] || { echo "REDIS_USERNAME 必须是非 default 的独立 ACL 用户" >&2; exit 1; }
[[ "$REDIS_EXPECTED_APP_USERNAME" =~ ^[A-Za-z0-9_.-]{1,64}$ && "$REDIS_EXPECTED_APP_USERNAME" != default && "$REDIS_EXPECTED_APP_USERNAME" != "$REDIS_USERNAME" ]] || {
  echo "REDIS_EXPECTED_APP_USERNAME 必须是与监控用户不同的非 default ACL 用户" >&2
  exit 1
}
[[ "$REDIS_EXPECTED_APP_PREFIX" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{2,95}$ ]] || {
  echo "REDIS_EXPECTED_APP_PREFIX 必须是不含通配符、冒号或空白的明确前缀" >&2
  exit 1
}
[[ "$REDIS_PORT" =~ ^[0-9]+$ ]] && (( REDIS_PORT >= 1 && REDIS_PORT <= 65535 )) || { echo "REDIS_PORT 无效" >&2; exit 1; }
[[ "$REDIS_TLS" == "true" || "$REDIS_TLS" == "false" ]] || { echo "REDIS_TLS 只能是 true 或 false" >&2; exit 1; }
(( ${#REDIS_PASSWORD} >= 24 )) || { echo "Redis 密码少于 24 位" >&2; exit 1; }
redis_password_lower="$(printf '%s' "$REDIS_PASSWORD" | tr '[:upper:]' '[:lower:]')"
case "$redis_password_lower" in
  *change*|*example*|*123456*) echo "Redis 密码仍是示例值或弱口令" >&2; exit 1 ;;
esac

# -h/-p are supported by the redis-cli shipped with Redis 6.2; its long
# --host/--port aliases were not yet available.
redis_args=(--no-auth-warning --user "$REDIS_USERNAME" -h "$REDIS_HOST" -p "$REDIS_PORT")
if [[ "$REDIS_TLS" == "true" ]]; then
  redis_args+=(--tls)
  if [[ -n "${REDIS_CA_FILE:-}" ]]; then
    [[ -f "$REDIS_CA_FILE" && ! -L "$REDIS_CA_FILE" ]] || { echo "Redis CA 文件无效" >&2; exit 1; }
    redis_args+=(--cacert "$REDIS_CA_FILE")
  fi
elif [[ "$REDIS_HOST" != "127.0.0.1" && "$REDIS_HOST" != "localhost" && "$REDIS_HOST" != "::1" ]]; then
  echo "远程 Redis 必须启用 TLS" >&2
  exit 1
fi

unauthenticated="$(env -u REDISCLI_AUTH redis-cli "${redis_args[@]}" PING 2>&1 || true)"
unauthenticated_upper="$(printf '%s' "$unauthenticated" | tr '[:lower:]' '[:upper:]')"
[[ "$unauthenticated_upper" == *NOAUTH* ]] || { echo "Redis 未强制认证" >&2; exit 1; }
export REDISCLI_AUTH="$REDIS_PASSWORD"
[[ "$(redis-cli "${redis_args[@]}" PING)" == "PONG" ]] || { echo "Redis PING 失败" >&2; exit 1; }
[[ "$(redis-cli "${redis_args[@]}" --raw ACL WHOAMI)" == "$REDIS_USERNAME" ]] || { echo "Redis 实际认证用户与监控配置不一致" >&2; exit 1; }
server_info="$(redis-cli "${redis_args[@]}" INFO server)"
version="$(printf '%s\n' "$server_info" | awk -F: '$1 == "redis_version" {gsub(/\r/, "", $2); print $2}')"
[[ "$version" =~ ^([0-9]+)\.([0-9]+)(\.[0-9]+)?$ ]] || { echo "无法识别 Redis 版本" >&2; exit 1; }
major="${BASH_REMATCH[1]}"
minor="${BASH_REMATCH[2]}"
(( major > 6 || (major == 6 && minor >= 2) )) || { echo "Redis 必须为 6.2+，当前 $version" >&2; exit 1; }

# ACL GETUSER/LIST only expose SHA-256 password hashes, never the application
# plaintext credential.  LIST is deliberately parsed as an exact allow-list:
# this catches both obvious broad grants (~*, &*, +@all) and less conspicuous
# additions such as CONFIG, ACL, FLUSH*, module commands or Redis 7 selectors.
app_acl_details="$(redis-cli "${redis_args[@]}" --raw ACL GETUSER "$REDIS_EXPECTED_APP_USERNAME")"
monitor_acl_details="$(redis-cli "${redis_args[@]}" --raw ACL GETUSER "$REDIS_USERNAME")"
[[ -n "$app_acl_details" && "$app_acl_details" != '(nil)' ]] || { echo "Redis 应用 ACL 用户不存在" >&2; exit 1; }
[[ -n "$monitor_acl_details" && "$monitor_acl_details" != '(nil)' ]] || { echo "Redis 监控 ACL 用户不存在" >&2; exit 1; }
acl_listing="$(redis-cli "${redis_args[@]}" --raw ACL LIST | tr -d '\r')"

acl_user_count=0
while IFS= read -r acl_user_line; do
  case "$acl_user_line" in
    "user default "*|"user $REDIS_EXPECTED_APP_USERNAME "*|"user $REDIS_USERNAME "*) ;;
    *) echo "Redis ACL 中存在未授权的额外用户" >&2; exit 1 ;;
  esac
  ((acl_user_count += 1))
done <<<"$acl_listing"
[[ "$acl_user_count" -eq 3 ]] || { echo "Redis ACL 必须且只能包含 default、应用和监控三个用户" >&2; exit 1; }
unset acl_user_line acl_user_count

find_acl_line() {
  local wanted_user="$1" line found=""
  while IFS= read -r line; do
    if [[ "$line" == "user $wanted_user "* ]]; then
      [[ -z "$found" ]] || return 1
      found="$line"
    fi
  done <<<"$acl_listing"
  [[ -n "$found" ]] || return 1
  printf '%s\n' "$found"
}

acl_rule_is_expected() {
  local wanted="$1" candidate
  shift
  for candidate in "$@"; do
    [[ "$candidate" == "$wanted" ]] && return 0
  done
  return 1
}

audit_acl_user() {
  local acl_line="$1" expected_user="$2" expected_key_pattern="$3" expected_channel_pattern="$4"
  local expected_rules_text="$5" token expected seen
  local seen_on=0 password_count=0 key_count=0 channel_count=0
  local -a tokens=() expected_rules=() command_rules=()
  read -r -a tokens <<<"$acl_line"
  read -r -a expected_rules <<<"$expected_rules_text"
  [[ "${tokens[0]:-}" == user && "${tokens[1]:-}" == "$expected_user" ]] || return 1

  for token in "${tokens[@]:2}"; do
    case "$token" in
      on) ((seen_on += 1)) ;;
      off|nopass|allkeys|allchannels|skip-sanitize-payload) return 1 ;;
      sanitize-payload|resetkeys|resetchannels) ;;
      \#[0-9a-fA-F]*)
        [[ "$token" =~ ^#[0-9a-fA-F]{64}$ ]] || return 1
        ((password_count += 1))
        ;;
      '~'*)
        [[ -n "$expected_key_pattern" && "$token" == "$expected_key_pattern" ]] || return 1
        ((key_count += 1))
        ;;
      '&'*)
        [[ -n "$expected_channel_pattern" && "$token" == "$expected_channel_pattern" ]] || return 1
        ((channel_count += 1))
        ;;
      +*|-*) command_rules+=("$(printf '%s' "$token" | tr '[:upper:]' '[:lower:]')") ;;
      *) return 1 ;;
    esac
  done

  [[ "$seen_on" -eq 1 && "$password_count" -eq 1 ]] || return 1
  if [[ -n "$expected_key_pattern" ]]; then
    [[ "$key_count" -eq 1 ]] || return 1
  else
    [[ "$key_count" -eq 0 ]] || return 1
  fi
  if [[ -n "$expected_channel_pattern" ]]; then
    [[ "$channel_count" -eq 1 ]] || return 1
  else
    [[ "$channel_count" -eq 0 ]] || return 1
  fi

  [[ "${#command_rules[@]}" -eq "${#expected_rules[@]}" ]] || return 1
  for token in "${command_rules[@]}"; do
    acl_rule_is_expected "$token" "${expected_rules[@]}" || return 1
  done
  for expected in "${expected_rules[@]}"; do
    seen=0
    for token in "${command_rules[@]}"; do
      [[ "$token" == "$expected" ]] && ((seen += 1))
    done
    [[ "$seen" -eq 1 ]] || return 1
  done
}

audit_default_acl() {
  local acl_line="$1" token
  local seen_off=0 reset_count=0
  local -a tokens=()
  read -r -a tokens <<<"$acl_line"
  [[ "${tokens[0]:-}" == user && "${tokens[1]:-}" == default ]] || return 1
  for token in "${tokens[@]:2}"; do
    case "$token" in
      off) ((seen_off += 1)) ;;
      sanitize-payload|resetkeys|resetchannels) ;;
      -@all) ((reset_count += 1)) ;;
      *) return 1 ;;
    esac
  done
  [[ "$seen_off" -eq 1 && "$reset_count" -eq 1 ]]
}

app_acl_line="$(find_acl_line "$REDIS_EXPECTED_APP_USERNAME")" || { echo "Redis ACL LIST 中缺少唯一的应用用户" >&2; exit 1; }
monitor_acl_line="$(find_acl_line "$REDIS_USERNAME")" || { echo "Redis ACL LIST 中缺少唯一的监控用户" >&2; exit 1; }
default_acl_line="$(find_acl_line default)" || { echo "Redis ACL LIST 中缺少唯一的 default 用户" >&2; exit 1; }
app_command_rules='-@all +ping +hello +select +get +set +getdel +del +incr +pexpire +pttl +eval +evalsha +publish +subscribe +unsubscribe +xadd +xread +xtrim +xrevrange +zadd +zrem +zremrangebyscore +zcard +time'
if (( major == 6 )); then
  # Redis 6.2 canonicalizes the exact +eval/+evalsha pair as the equivalent
  # +@scripting -script. Do not accept that category form on newer majors,
  # where the scripting category contains additional commands.
  app_command_rules='-@all +ping +hello +select +get +set +getdel +del +incr +pexpire +pttl +@scripting -script +publish +subscribe +unsubscribe +xadd +xread +xtrim +xrevrange +zadd +zrem +zremrangebyscore +zcard +time'
fi
monitor_command_rules='-@all +ping +info +config|get +acl|whoami +acl|list +acl|getuser'
audit_acl_user "$app_acl_line" "$REDIS_EXPECTED_APP_USERNAME" "~$REDIS_EXPECTED_APP_PREFIX:*" "&$REDIS_EXPECTED_APP_PREFIX:*" "$app_command_rules" || {
  echo "Redis 应用 ACL 发生漂移：必须只有预期 production 键/频道和命令权限" >&2
  exit 1
}
audit_acl_user "$monitor_acl_line" "$REDIS_USERNAME" '' '' "$monitor_command_rules" || {
  echo "Redis 监控 ACL 发生漂移：必须保持无键/频道访问的只读审计权限" >&2
  exit 1
}
audit_default_acl "$default_acl_line" || {
  echo "Redis default ACL 必须已 reset/off，且不得保留密码、键、频道或命令权限" >&2
  exit 1
}
unset app_acl_details monitor_acl_details acl_listing app_acl_line monitor_acl_line default_acl_line

persistence_info="$(redis-cli "${redis_args[@]}" INFO persistence)"
memory_info="$(redis-cli "${redis_args[@]}" INFO memory)"
appendfsync="$(redis-cli "${redis_args[@]}" --raw CONFIG GET appendfsync | tail -n 1 | tr -d '\r')"
protected_mode="$(redis-cli "${redis_args[@]}" --raw CONFIG GET protected-mode | tail -n 1 | tr -d '\r')"
acl_pubsub_default="$(redis-cli "${redis_args[@]}" --raw CONFIG GET acl-pubsub-default | tail -n 1 | tr -d '\r')"
grep -q $'^aof_enabled:1\r\{0,1\}$' <<<"$persistence_info" || { echo "Redis AOF 未启用" >&2; exit 1; }
grep -q $'^aof_last_write_status:ok\r\{0,1\}$' <<<"$persistence_info" || { echo "Redis AOF 最近写入失败" >&2; exit 1; }
grep -q $'^maxmemory_policy:noeviction\r\{0,1\}$' <<<"$memory_info" || { echo "Redis 必须使用 noeviction" >&2; exit 1; }
[[ "$appendfsync" == everysec ]] || { echo "Redis appendfsync 必须为 everysec" >&2; exit 1; }
[[ "$protected_mode" == yes ]] || { echo "Redis protected-mode 必须启用" >&2; exit 1; }
[[ "$acl_pubsub_default" == resetchannels ]] || { echo "Redis acl-pubsub-default 必须为 resetchannels" >&2; exit 1; }
if [[ "$(uname -s)" == Linux && ( "$REDIS_HOST" == 127.0.0.1 || "$REDIS_HOST" == localhost || "$REDIS_HOST" == ::1 ) ]]; then
  command -v ss >/dev/null 2>&1 || { echo "本机 Redis 监听检查需要 ss" >&2; exit 1; }
  listeners="$(ss -H -ltn "sport = :$REDIS_PORT" | awk '{print $4}')"
  [[ -n "$listeners" ]] || { echo "没有找到 Redis 监听端口" >&2; exit 1; }
  while IFS= read -r listener; do
    case "$listener" in
      127.0.0.1:"$REDIS_PORT"|'[::1]':"$REDIS_PORT"|::1:"$REDIS_PORT") ;;
      *) echo "Redis 监听了非回环地址：$listener" >&2; exit 1 ;;
    esac
  done <<<"$listeners"
fi

echo "Redis 生产检查通过：版本 ${version}，认证/AOF/noeviction/ACL 无漂移"
