#!/usr/bin/env bash
set -euo pipefail

ENV_SOURCE="${1:-/etc/wangzhe/monitor.env}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/strict-env.sh
source "$SCRIPT_DIR/lib/strict-env.sh"
# shellcheck source=lib/encrypted-backup.sh
source "$SCRIPT_DIR/lib/encrypted-backup.sh"
# shellcheck source=lib/safe-integer.sh
source "$SCRIPT_DIR/lib/safe-integer.sh"
if [[ "$ENV_SOURCE" != "--current-env" ]]; then
  load_strict_env "$ENV_SOURCE" '^MONITOR_[A-Z0-9_]+$'
fi
for command_name in awk basename curl date df dirname find grep jq mkdir mktemp mv openssl psql rm sha256sum sort stat tail timeout tr wc; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done
pkeyutl_help="$(openssl pkeyutl -help 2>&1 || true)"
grep -q -- '-rawin' <<<"$pkeyutl_help" || { echo "恢复状态 Ed25519 验签要求 OpenSSL 3.0+（缺少 pkeyutl -rawin）" >&2; exit 1; }
unset pkeyutl_help
required=(
  MONITOR_BACKEND_URL MONITOR_DATABASE_HOST MONITOR_DATABASE_PORT MONITOR_DATABASE_USER
  MONITOR_DATABASE_PASSWORD MONITOR_DATABASE_DBNAME MONITOR_DATABASE_SSLMODE MONITOR_REDIS_ENV_FILE
  MONITOR_DATABASE_BACKUP_DIR MONITOR_UPLOAD_BACKUP_DIR MONITOR_PITR_WAL_DIR
  MONITOR_PITR_BASEBACKUP_DIR MONITOR_RESTORE_STATUS_FILE MONITOR_PITR_RESTORE_STATUS_FILE MONITOR_TLS_HOSTS MONITOR_TLS_PORT
  MONITOR_RESTORE_STATUS_REMOTE_SOURCE MONITOR_PITR_RESTORE_STATUS_REMOTE_SOURCE MONITOR_RCLONE_CONFIG MONITOR_NGINX_ACCESS_LOG
  MONITOR_RESTORE_EXPECTED_DATABASE_NAME MONITOR_RESTORE_EXPECTED_DATABASE_REMOTE_SOURCE MONITOR_RESTORE_EXPECTED_UPLOAD_REMOTE_SOURCE
  MONITOR_BACKUP_INTEGRITY_STATUS_FILE
  MONITOR_RESTORE_STATUS_VERIFY_KEY_FILE MONITOR_PITR_RESTORE_STATUS_VERIFY_KEY_FILE
  MONITOR_BACKUP_PROVENANCE_VERIFY_KEY_FILE MONITOR_PITR_PROVENANCE_VERIFY_KEY_FILE MONITOR_PITR_RESTORE_EXPECTED_REMOTE_SOURCE
  MONITOR_STATE_DIR MONITOR_ALERT_REPEAT_MINUTES MONITOR_WEBHOOK_URL MONITOR_WEBHOOK_BEARER_TOKEN
)
for key in "${required[@]}"; do
  [[ -n "${!key:-}" ]] || { echo "监控配置缺少 $key" >&2; exit 1; }
done
[[ "$MONITOR_BACKEND_URL" =~ ^http://(127\.0\.0\.1|localhost|\[::1\]):[0-9]+$ ]] || { echo "监控后端地址必须是本机 HTTP origin" >&2; exit 1; }
[[ "$MONITOR_DATABASE_PORT" =~ ^[0-9]+$ ]] && (( MONITOR_DATABASE_PORT >= 1 && MONITOR_DATABASE_PORT <= 65535 )) || { echo "数据库端口无效" >&2; exit 1; }
case "$MONITOR_DATABASE_SSLMODE" in disable|verify-ca|verify-full) ;; *) echo "监控数据库 SSLMODE 无效" >&2; exit 1;; esac
case "$MONITOR_DATABASE_HOST" in localhost|127.0.0.1|::1) ;; *) [[ "$MONITOR_DATABASE_SSLMODE" == verify-ca || "$MONITOR_DATABASE_SSLMODE" == verify-full ]] || { echo "远程监控数据库必须校验证书" >&2; exit 1; };; esac
[[ "$MONITOR_DATABASE_DBNAME" =~ ^[A-Za-z0-9_.-]+$ ]] || { echo "监控数据库名无效" >&2; exit 1; }
webhook_url_pattern='^https://[^[:space:]"\\]+$'
[[ "$MONITOR_WEBHOOK_URL" =~ $webhook_url_pattern ]] || { echo "告警 Webhook 必须是 HTTPS" >&2; exit 1; }
[[ "$MONITOR_WEBHOOK_URL" != *CHANGE_ME* && "$MONITOR_WEBHOOK_BEARER_TOKEN" != *CHANGE_ME* && ${#MONITOR_WEBHOOK_BEARER_TOKEN} -ge 20 ]] || { echo "告警 Webhook/令牌仍是示例值或过短" >&2; exit 1; }
case "$MONITOR_WEBHOOK_BEARER_TOKEN" in
  *'"'*|*'\'*|*$'\r'*|*$'\n'*) echo "告警令牌包含不安全字符" >&2; exit 1 ;;
esac
for path in "$MONITOR_STATE_DIR" "$MONITOR_DATABASE_BACKUP_DIR" "$MONITOR_UPLOAD_BACKUP_DIR" "$MONITOR_PITR_WAL_DIR" "$MONITOR_PITR_BASEBACKUP_DIR"; do
  [[ "$path" == /* && "$path" != / ]] || { echo "监控路径必须是非根绝对路径：$path" >&2; exit 1; }
done
for path in "$MONITOR_REDIS_ENV_FILE" "$MONITOR_NGINX_ACCESS_LOG"; do
  [[ "$path" == /* ]] || { echo "监控文件必须是绝对路径：$path" >&2; exit 1; }
done
[[ "$MONITOR_TLS_PORT" =~ ^[0-9]+$ ]] && (( MONITOR_TLS_PORT >= 1 && MONITOR_TLS_PORT <= 65535 )) || { echo "TLS 监控端口无效" >&2; exit 1; }
[[ "$MONITOR_RESTORE_STATUS_FILE" == "$MONITOR_STATE_DIR/last-restore-drill.status" ]] || { echo "恢复演练状态必须固定在监控状态目录" >&2; exit 1; }
[[ "$MONITOR_PITR_RESTORE_STATUS_FILE" == "$MONITOR_STATE_DIR/last-pitr-drill.status" ]] || { echo "PITR 演练状态必须固定在监控状态目录" >&2; exit 1; }
[[ "$MONITOR_BACKUP_INTEGRITY_STATUS_FILE" == "$MONITOR_STATE_DIR/last-backup-integrity.status" ]] || { echo "备份完整性状态必须固定在监控状态目录" >&2; exit 1; }
[[ "$MONITOR_RESTORE_EXPECTED_DATABASE_NAME" =~ ^[A-Za-z][A-Za-z0-9_]{0,62}$ ]] || { echo "逻辑恢复的预期源数据库名无效" >&2; exit 1; }
[[ "$MONITOR_RESTORE_STATUS_VERIFY_KEY_FILE" == /etc/wangzhe/logical-restore-status-ed25519-public.pem ]] || { echo "逻辑恢复状态验签公钥必须使用固定路径" >&2; exit 1; }
[[ "$MONITOR_PITR_RESTORE_STATUS_VERIFY_KEY_FILE" == /etc/wangzhe/pitr-restore-status-ed25519-public.pem ]] || { echo "PITR 状态验签公钥必须使用固定独立路径" >&2; exit 1; }
[[ "$MONITOR_BACKUP_PROVENANCE_VERIFY_KEY_FILE" == /etc/wangzhe/backup-provenance-ed25519-public.pem ]] || { echo "数据库/上传来源验签公钥必须使用固定路径" >&2; exit 1; }
[[ "$MONITOR_PITR_PROVENANCE_VERIFY_KEY_FILE" == /etc/wangzhe/pitr-provenance-ed25519-public.pem ]] || { echo "PITR 来源验签公钥必须使用固定路径" >&2; exit 1; }

validate_status_verify_key() {
  local key_file="$1" label="$2" owner mode mode_value
  [[ -f "$key_file" && ! -L "$key_file" && -r "$key_file" ]] || { echo "$label 必须是监控用户可读的普通文件" >&2; return 1; }
  validate_no_symlink_path_components "$key_file"
  owner="$(strict_env_stat '%u' '%u' "$key_file")"
  mode="$(strict_env_stat '%a' '%Lp' "$key_file")"
  [[ "$owner" == 0 && "$mode" =~ ^[0-7]{3,4}$ ]] || { echo "$label 必须由 root 所有" >&2; return 1; }
  mode_value=$((8#$mode))
  (( (mode_value & 022) == 0 )) || { echo "$label 不能被非 root 修改" >&2; return 1; }
  openssl pkey -pubin -in "$key_file" -noout -text 2>/dev/null | grep -q '^ED25519 Public-Key:' || {
    echo "$label 不是有效 Ed25519 公钥" >&2
    return 1
  }
}
validate_status_verify_key "$MONITOR_RESTORE_STATUS_VERIFY_KEY_FILE" "逻辑恢复状态验签公钥"
validate_status_verify_key "$MONITOR_PITR_RESTORE_STATUS_VERIFY_KEY_FILE" "PITR 恢复状态验签公钥"
validate_ed25519_public_key "$MONITOR_BACKUP_PROVENANCE_VERIFY_KEY_FILE" "数据库/上传来源验签公钥"
validate_ed25519_public_key "$MONITOR_PITR_PROVENANCE_VERIFY_KEY_FILE" "PITR 来源验签公钥"
logical_verify_key_fingerprint="$(openssl pkey -pubin -in "$MONITOR_RESTORE_STATUS_VERIFY_KEY_FILE" -pubout -outform DER 2>/dev/null | sha256sum | awk '{print $1}')"
pitr_verify_key_fingerprint="$(openssl pkey -pubin -in "$MONITOR_PITR_RESTORE_STATUS_VERIFY_KEY_FILE" -pubout -outform DER 2>/dev/null | sha256sum | awk '{print $1}')"
[[ "$logical_verify_key_fingerprint" =~ ^[0-9a-f]{64}$ && "$pitr_verify_key_fingerprint" =~ ^[0-9a-f]{64}$ ]] || {
  echo "无法计算恢复状态验签公钥指纹" >&2
  exit 1
}
[[ "$logical_verify_key_fingerprint" != "$pitr_verify_key_fingerprint" ]] || {
  echo "逻辑恢复与 PITR 恢复必须使用不同的 Ed25519 验签密钥" >&2
  exit 1
}
unset logical_verify_key_fingerprint pitr_verify_key_fingerprint
backup_provenance_fingerprint="$(openssl pkey -pubin -in "$MONITOR_BACKUP_PROVENANCE_VERIFY_KEY_FILE" -outform DER 2>/dev/null | sha256sum | awk '{print $1}')"
pitr_provenance_fingerprint="$(openssl pkey -pubin -in "$MONITOR_PITR_PROVENANCE_VERIFY_KEY_FILE" -outform DER 2>/dev/null | sha256sum | awk '{print $1}')"
[[ "$backup_provenance_fingerprint" =~ ^[0-9a-f]{64}$ && "$pitr_provenance_fingerprint" =~ ^[0-9a-f]{64}$ && "$backup_provenance_fingerprint" != "$pitr_provenance_fingerprint" ]] || {
  echo "数据库/上传与 PITR 必须使用不同的来源验签密钥" >&2
  exit 1
}
unset backup_provenance_fingerprint pitr_provenance_fingerprint
IFS=',' read -r -a tls_hosts <<<"$MONITOR_TLS_HOSTS"
(( ${#tls_hosts[@]} > 0 )) || { echo "至少配置一个 TLS 监控域名" >&2; exit 1; }
for tls_host in "${tls_hosts[@]}"; do
  [[ "$tls_host" =~ ^([A-Za-z0-9-]+\.)+[A-Za-z]{2,63}$ ]] || { echo "TLS 监控域名无效：$tls_host" >&2; exit 1; }
done
command -v rclone >/dev/null 2>&1 || { echo "主动监控恢复演练状态需要 rclone" >&2; exit 1; }
validate_remote_destination "$MONITOR_RESTORE_STATUS_REMOTE_SOURCE"
validate_remote_destination "$MONITOR_PITR_RESTORE_STATUS_REMOTE_SOURCE"
validate_remote_destination "$MONITOR_RESTORE_EXPECTED_DATABASE_REMOTE_SOURCE"
validate_remote_destination "$MONITOR_RESTORE_EXPECTED_UPLOAD_REMOTE_SOURCE"
validate_remote_destination "$MONITOR_PITR_RESTORE_EXPECTED_REMOTE_SOURCE"
[[ "$(basename "${MONITOR_PITR_RESTORE_EXPECTED_REMOTE_SOURCE#*:}")" == "$(basename "$MONITOR_PITR_BASEBACKUP_DIR")" ]] || { echo "PITR 演练预期远端末级与集群标识不一致" >&2; exit 1; }
validate_rclone_config "$MONITOR_RCLONE_CONFIG"
rclone_args=(--config "$MONITOR_RCLONE_CONFIG" --contimeout 3s --timeout 8s --retries 1 --low-level-retries 1)
run_rclone() { timeout 15 rclone "${rclone_args[@]}" "$@"; }

MAX_BACKUP_AGE="$(require_decimal_count MONITOR_MAX_BACKUP_AGE_MINUTES "${MONITOR_MAX_BACKUP_AGE_MINUTES:-1560}")"
MAX_WAL_AGE="$(require_decimal_count MONITOR_MAX_WAL_ARCHIVE_AGE_MINUTES "${MONITOR_MAX_WAL_ARCHIVE_AGE_MINUTES:-15}")"
MAX_BASEBACKUP_AGE="$(require_decimal_count MONITOR_MAX_BASEBACKUP_AGE_MINUTES "${MONITOR_MAX_BASEBACKUP_AGE_MINUTES:-11520}")"
MAX_RESTORE_AGE="$(require_decimal_count MONITOR_MAX_RESTORE_DRILL_AGE_MINUTES "${MONITOR_MAX_RESTORE_DRILL_AGE_MINUTES:-50400}")"
MAX_PITR_RESTORE_AGE="$(require_decimal_count MONITOR_MAX_PITR_RESTORE_DRILL_AGE_MINUTES "${MONITOR_MAX_PITR_RESTORE_DRILL_AGE_MINUTES:-50400}")"
MAX_BACKUP_INTEGRITY_AGE="$(require_decimal_count MONITOR_MAX_BACKUP_INTEGRITY_AGE_MINUTES "${MONITOR_MAX_BACKUP_INTEGRITY_AGE_MINUTES:-1800}")"
MAX_5XX="$(require_decimal_count MONITOR_MAX_5XX_PER_INTERVAL "${MONITOR_MAX_5XX_PER_INTERVAL:-0}")"
MAX_CONNECTION_PERCENT="$(require_decimal_count MONITOR_MAX_DATABASE_CONNECTION_PERCENT "${MONITOR_MAX_DATABASE_CONNECTION_PERCENT:-80}")"
MAX_BACKUP_DISK_PERCENT="$(require_decimal_count MONITOR_MAX_BACKUP_DISK_PERCENT "${MONITOR_MAX_BACKUP_DISK_PERCENT:-80}")"
MAX_BACKUP_INODE_PERCENT="$(require_decimal_count MONITOR_MAX_BACKUP_INODE_PERCENT "${MONITOR_MAX_BACKUP_INODE_PERCENT:-80}")"
ACCOUNTING_AUDIT_INTERVAL_MINUTES="$(require_decimal_count MONITOR_ACCOUNTING_AUDIT_INTERVAL_MINUTES "${MONITOR_ACCOUNTING_AUDIT_INTERVAL_MINUTES:-60}")"
ALERT_REPEAT_MINUTES="$(require_decimal_count MONITOR_ALERT_REPEAT_MINUTES "$MONITOR_ALERT_REPEAT_MINUTES")"
(( MAX_BACKUP_AGE > 0 && MAX_WAL_AGE > 0 && MAX_BASEBACKUP_AGE > 0 && MAX_RESTORE_AGE > 0 && MAX_PITR_RESTORE_AGE > 0 && MAX_BACKUP_INTEGRITY_AGE > 0 )) || { echo "监控时间阈值必须大于 0" >&2; exit 1; }
(( MAX_CONNECTION_PERCENT >= 1 && MAX_CONNECTION_PERCENT <= 100 )) || { echo "数据库连接阈值必须是 1-100" >&2; exit 1; }
(( MAX_BACKUP_DISK_PERCENT >= 1 && MAX_BACKUP_DISK_PERCENT <= 100 )) || { echo "备份磁盘阈值必须是 1-100" >&2; exit 1; }
(( MAX_BACKUP_INODE_PERCENT >= 1 && MAX_BACKUP_INODE_PERCENT <= 100 )) || { echo "备份 inode 阈值必须是 1-100" >&2; exit 1; }
(( ACCOUNTING_AUDIT_INTERVAL_MINUTES >= 15 && ACCOUNTING_AUDIT_INTERVAL_MINUTES <= 1440 )) || { echo "账务深度巡检间隔必须是 15-1440 分钟" >&2; exit 1; }
(( ALERT_REPEAT_MINUTES >= 1 && ALERT_REPEAT_MINUTES <= 1440 )) || { echo "相同主动告警重发间隔必须是 1-1440 分钟" >&2; exit 1; }

umask 077
mkdir -p -- "$MONITOR_STATE_DIR"
[[ -d "$MONITOR_STATE_DIR" && ! -L "$MONITOR_STATE_DIR" ]] || { echo "监控状态目录不能是符号链接" >&2; exit 1; }
alerts=()
add_alert() { alerts+=("$1"); }
now_epoch="$(date +%s)"

# Chain continuity and orphan/reference scans are deliberately not run every
# minute. Keep the most recent successful result in the private state directory
# and include it in every alert signature until the next bounded deep audit.
accounting_state_file="$MONITOR_STATE_DIR/accounting-audit.status"
accounting_state_valid=0
accounting_last_success=0
accounting_arithmetic_errors=0
accounting_orphan_ledgers=0
accounting_duplicate_references=0
accounting_chain_gaps=0
if [[ -f "$accounting_state_file" && ! -L "$accounting_state_file" ]]; then
  accounting_state_version=""
  accounting_state_extra=""
  read -r accounting_state_version accounting_last_success accounting_arithmetic_errors \
    accounting_orphan_ledgers accounting_duplicate_references accounting_chain_gaps \
    accounting_state_extra <"$accounting_state_file" || true
  if [[ "$accounting_state_version" == v1 && "$accounting_last_success" =~ ^[1-9][0-9]{0,11}$ && \
        "$accounting_arithmetic_errors" =~ ^[0-9]+$ && "$accounting_orphan_ledgers" =~ ^[0-9]+$ && \
        "$accounting_duplicate_references" =~ ^[0-9]+$ && "$accounting_chain_gaps" =~ ^[0-9]+$ && \
        -z "$accounting_state_extra" ]]; then
    accounting_state_valid=1
  fi
fi
accounting_interval_seconds=$((ACCOUNTING_AUDIT_INTERVAL_MINUTES * 60))
accounting_audit_due=0
if (( accounting_state_valid == 0 || now_epoch < accounting_last_success || now_epoch - accounting_last_success >= accounting_interval_seconds )); then
  accounting_audit_due=1
fi

if ! curl -fsS --max-time 8 "$MONITOR_BACKEND_URL/health" >/dev/null; then
  add_alert "后端 health 检查失败"
fi
if ! curl -fsS --max-time 8 "$MONITOR_BACKEND_URL/ready" >/dev/null; then
  add_alert "后端 ready 检查失败"
fi
if lottery_payload="$(curl -fsS --max-time 8 "$MONITOR_BACKEND_URL/api/public/lottery/status" 2>/dev/null)"; then
  for spec in \
    'source_error_game_count:开奖源异常' \
    'error_issue_count:开奖错误期号' \
    'stale_pending_issue_count:开奖超时待处理期号' \
    'unrecoverable_bet_count:不可恢复注单' \
    'abnormal_bet_count:异常注单'; do
    field="${spec%%:*}"
    label="${spec#*:}"
    value="$(printf '%s' "$lottery_payload" | jq -er ".data.health.$field | numbers" 2>/dev/null || printf invalid)"
    if [[ ! "$value" =~ ^[0-9]+$ ]]; then
      add_alert "$label 指标格式无效"
    elif (( 10#$value > 0 )); then
      add_alert "$label=$value"
    fi
  done
else
  add_alert "无法读取开奖健康状态"
fi

export PGPASSWORD="$MONITOR_DATABASE_PASSWORD"
export PGSSLMODE="$MONITOR_DATABASE_SSLMODE"
export PGCONNECT_TIMEOUT=5
psql_base=(psql --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1 --host "$MONITOR_DATABASE_HOST" --port "$MONITOR_DATABASE_PORT" --username "$MONITOR_DATABASE_USER" --dbname "$MONITOR_DATABASE_DBNAME")
db_query() { timeout 8 "${psql_base[@]}" --command "$1" 2>/dev/null | tr -d '[:space:]'; }
db_accounting_query() {
  timeout 25 env PGOPTIONS='-c statement_timeout=20000' "${psql_base[@]}" --command "$1" 2>/dev/null | tr -d '[:space:]'
}
if db_ping="$(db_query 'SELECT 1;')" && [[ "$db_ping" == 1 ]]; then
  declare -a database_checks=(
    "SELECT count(*) FROM lottery_bets WHERE reconciliation_status = 'abnormal';|异常注单"
    "SELECT count(*) FROM \"user\" WHERE balance_cents < 0 AND deleted_at IS NULL;|负余额会员"
    "WITH candidates AS ((SELECT DISTINCT ON (user_id) id,user_id,after_cents FROM user_balance_transactions ORDER BY user_id,id DESC) UNION ALL (SELECT DISTINCT ON (user_id) id,user_id,after_cents FROM user_balance_transaction_archives ORDER BY user_id,id DESC)), latest AS (SELECT DISTINCT ON (user_id) user_id,after_cents FROM candidates ORDER BY user_id,id DESC) SELECT count(*) FROM \"user\" u LEFT JOIN latest l ON l.user_id=u.user_id WHERE u.deleted_at IS NULL AND COALESCE(l.after_cents,0) <> u.balance_cents;|账务末值不一致会员"
  )
  for check in "${database_checks[@]}"; do
    sql="${check%%|*}"
    label="${check#*|}"
    value="$(db_query "$sql" || printf invalid)"
    if [[ ! "$value" =~ ^[0-9]+$ ]]; then
      add_alert "$label 查询失败"
    elif (( 10#$value > 0 )); then
      add_alert "$label=$value"
    fi
  done
  if (( accounting_audit_due == 1 )); then
    accounting_sql="WITH arithmetic_errors AS (
      SELECT COUNT(*) AS total
      FROM user_balance_transactions
      WHERE after_cents <> before_cents + amount_cents
         OR before_cents < 0 OR after_cents < 0
    ), orphan_ledgers AS (
      SELECT COUNT(*) AS total
      FROM user_balance_transactions AS ledger
      WHERE NOT EXISTS (SELECT 1 FROM \"user\" AS account WHERE account.user_id = ledger.user_id)
    ), duplicate_references AS (
      SELECT COUNT(*) AS total
      FROM (
        SELECT user_id, reference
        FROM user_balance_transactions
        WHERE reference <> ''
        GROUP BY user_id, reference
        HAVING COUNT(*) > 1
      ) AS duplicate_groups
    ), chain_gaps AS (
      SELECT COUNT(*) AS total
      FROM (
        SELECT before_cents,
               LAG(after_cents) OVER (PARTITION BY user_id ORDER BY id) AS prior_after_cents
        FROM user_balance_transactions
      ) AS ordered
      WHERE prior_after_cents IS NOT NULL AND before_cents <> prior_after_cents
    )
    SELECT arithmetic_errors.total || ':' || orphan_ledgers.total || ':' ||
           duplicate_references.total || ':' || chain_gaps.total
    FROM arithmetic_errors, orphan_ledgers, duplicate_references, chain_gaps;"
    accounting_metrics="$(db_accounting_query "$accounting_sql" || true)"
    if [[ "$accounting_metrics" =~ ^([0-9]+):([0-9]+):([0-9]+):([0-9]+)$ ]]; then
      accounting_last_success="$now_epoch"
      accounting_arithmetic_errors="${BASH_REMATCH[1]}"
      accounting_orphan_ledgers="${BASH_REMATCH[2]}"
      accounting_duplicate_references="${BASH_REMATCH[3]}"
      accounting_chain_gaps="${BASH_REMATCH[4]}"
      printf 'v1 %s %s %s %s %s\n' "$accounting_last_success" "$accounting_arithmetic_errors" \
        "$accounting_orphan_ledgers" "$accounting_duplicate_references" "$accounting_chain_gaps" \
        >"$accounting_state_file.partial"
      mv "$accounting_state_file.partial" "$accounting_state_file"
      accounting_state_valid=1
    else
      add_alert "账务深度巡检查询失败或超过20秒"
    fi
  fi
  connection_metrics="$(db_query "SELECT count(*) || ':' || current_setting('max_connections') FROM pg_stat_activity;" || true)"
  if [[ "$connection_metrics" =~ ^([0-9]+):([0-9]+)$ && ${BASH_REMATCH[2]} -gt 0 ]]; then
    connection_percent=$((BASH_REMATCH[1] * 100 / BASH_REMATCH[2]))
    (( connection_percent < MAX_CONNECTION_PERCENT )) || add_alert "PostgreSQL连接使用率=${connection_percent}%"
  else
    add_alert "PostgreSQL连接指标查询失败"
  fi
  archiver_failed="$(db_query "SELECT CASE WHEN failed_count > 0 AND (last_archived_time IS NULL OR last_failed_time > last_archived_time) THEN 1 ELSE 0 END FROM pg_stat_archiver;" || printf invalid)"
  [[ "$archiver_failed" == 0 ]] || add_alert "PostgreSQL WAL归档器存在未恢复失败"
  archive_mode="$(db_query "SELECT current_setting('archive_mode');" || printf invalid)"
  [[ "$archive_mode" == on || "$archive_mode" == always ]] || add_alert "PostgreSQL archive_mode未启用"
else
  add_alert "PostgreSQL 连接失败"
fi
if (( accounting_state_valid == 1 )); then
  (( 10#$accounting_arithmetic_errors == 0 )) || add_alert "账本算术错误流水=$accounting_arithmetic_errors"
  (( 10#$accounting_orphan_ledgers == 0 )) || add_alert "孤儿账本流水=$accounting_orphan_ledgers"
  (( 10#$accounting_duplicate_references == 0 )) || add_alert "账本重复reference组=$accounting_duplicate_references"
  (( 10#$accounting_chain_gaps == 0 )) || add_alert "账本链断裂流水=$accounting_chain_gaps"
  if (( now_epoch < accounting_last_success || now_epoch - accounting_last_success > accounting_interval_seconds * 2 )); then
    add_alert "账务深度巡检成功状态已过期"
  fi
else
  add_alert "缺少有效账务深度巡检成功状态"
fi
unset PGPASSWORD PGSSLMODE PGCONNECT_TIMEOUT

if ! timeout 10 "$SCRIPT_DIR/redis-production-check.sh" "$MONITOR_REDIS_ENV_FILE" >/dev/null 2>&1; then
  add_alert "Redis认证/版本/AOF/noeviction/ACL漂移检查失败"
fi

backup_manifest_checksum=""
backup_manifest_name=""
marker_remote=""

validate_backup_manifest_metadata() {
  local target="$1" recorded_checksum recorded_name extra manifest_bytes
  backup_manifest_checksum=""
  backup_manifest_name=""
  [[ -f "$target" && ! -L "$target" && -s "$target" && -f "$target.sha256" && ! -L "$target.sha256" ]] || return 1
  manifest_bytes="$(stat -c %s "$target.sha256" 2>/dev/null || stat -f %z "$target.sha256")"
  (( manifest_bytes >= 1 && manifest_bytes <= 4096 )) || return 1
  read -r recorded_checksum recorded_name extra <"$target.sha256" || return 1
  [[ "$recorded_checksum" =~ ^[0-9a-f]{64}$ && "$recorded_name" == "$(basename "$target")" && -z "${extra:-}" ]] || return 1
  backup_manifest_checksum="$recorded_checksum"
  backup_manifest_name="$recorded_name"
}

validate_offsite_marker_metadata() {
  local target="$1" expected_checksum="$2" marker_checksum remote extra marker_bytes
  marker_remote=""
  [[ -f "$target.offsite-ok" && ! -L "$target.offsite-ok" ]] || return 1
  marker_bytes="$(stat -c %s "$target.offsite-ok" 2>/dev/null || stat -f %z "$target.offsite-ok")"
  (( marker_bytes >= 1 && marker_bytes <= 4096 )) || return 1
  read -r marker_checksum remote extra <"$target.offsite-ok" || return 1
  [[ "$marker_checksum" == "$expected_checksum" && -z "${extra:-}" ]] || return 1
  validate_remote_destination "$remote" >/dev/null 2>&1 || return 1
  marker_remote="$remote"
}

validate_remote_object_size() {
  local remote="$1" local_target="$2" label="$3" size_json remote_count remote_bytes local_bytes
  if ! size_json="$(run_rclone size --json "$remote" 2>/dev/null)"; then
    add_alert "$label 异机对象不存在或大小不可读"
    return 1
  fi
  remote_count="$(printf '%s' "$size_json" | jq -er '.count | numbers' 2>/dev/null || true)"
  remote_bytes="$(printf '%s' "$size_json" | jq -er '.bytes | numbers' 2>/dev/null || true)"
  local_bytes="$(stat -c %s "$local_target" 2>/dev/null || stat -f %z "$local_target")"
  if [[ ! "$remote_count" =~ ^[0-9]+$ || ! "$remote_bytes" =~ ^[0-9]+$ || "$remote_count" != 1 || "$remote_bytes" != "$local_bytes" ]]; then
    add_alert "$label 异机对象数量或大小不一致"
    return 1
  fi
}

remote_small_object_is_valid() {
  local remote="$1" size_json remote_count remote_bytes
  size_json="$(run_rclone size --json "$remote" 2>/dev/null)" || return 1
  remote_count="$(printf '%s' "$size_json" | jq -er '.count | numbers' 2>/dev/null || true)"
  remote_bytes="$(printf '%s' "$size_json" | jq -er '.bytes | numbers' 2>/dev/null || true)"
  [[ "$remote_count" == 1 && "$remote_bytes" =~ ^[0-9]+$ ]] || return 1
  (( remote_bytes >= 1 && remote_bytes <= 4096 ))
}

validate_remote_checksum_manifest() {
  local remote="$1" expected_checksum="$2" expected_name="$3" label="$4"
  local partial remote_checksum remote_name remote_extra manifest_bytes
  if ! remote_small_object_is_valid "$remote"; then
    add_alert "$label 异机校验清单缺失或大小无效"
    return 1
  fi
  partial="$(mktemp "$MONITOR_STATE_DIR/.remote-backup-manifest.XXXXXX")"
  if ! run_rclone copyto "$remote" "$partial" --no-traverse >/dev/null 2>&1; then
    rm -f -- "$partial"
    add_alert "$label 异机校验清单缺失"
    return 1
  fi
  manifest_bytes="$(stat -c %s "$partial" 2>/dev/null || stat -f %z "$partial")"
  read -r remote_checksum remote_name remote_extra <"$partial" || true
  rm -f -- "$partial"
  if (( manifest_bytes < 1 || manifest_bytes > 4096 )) || [[ "$remote_checksum" != "$expected_checksum" || "$remote_name" != "$expected_name" || -n "${remote_extra:-}" ]]; then
    add_alert "$label 异机校验清单无效"
    return 1
  fi
}

validate_remote_source_manifest() {
  local remote="$1" expected_checksum="$2" expected_wal_name="$3" expected_cluster="$4" label="$5"
  local partial source_checksum source_wal_name source_cluster source_extra manifest_bytes
  if ! remote_small_object_is_valid "$remote"; then
    add_alert "$label 异机源WAL凭证缺失或大小无效"
    return 1
  fi
  partial="$(mktemp "$MONITOR_STATE_DIR/.remote-source-manifest.XXXXXX")"
  if ! run_rclone copyto "$remote" "$partial" --no-traverse >/dev/null 2>&1; then
    rm -f -- "$partial"
    add_alert "$label 异机源WAL凭证缺失"
    return 1
  fi
  manifest_bytes="$(stat -c %s "$partial" 2>/dev/null || stat -f %z "$partial")"
  read -r source_checksum source_wal_name source_cluster source_extra <"$partial" || true
  rm -f -- "$partial"
  if (( manifest_bytes < 1 || manifest_bytes > 4096 )) || [[ "$source_checksum" != "$expected_checksum" || "$source_wal_name" != "$expected_wal_name" || "$source_cluster" != "$expected_cluster" || -n "${source_extra:-}" ]]; then
    add_alert "$label 异机源WAL凭证无效"
    return 1
  fi
}

validate_remote_evidence_digest() {
  local remote="$1" local_file="$2" label="$3" expected_bytes="${4:-0}"
  local local_bytes local_digest output remote_digest remote_name remote_extra
  [[ -f "$local_file" && ! -L "$local_file" && -s "$local_file" ]] || { add_alert "$label 本地证据缺失"; return 1; }
  local_bytes="$(strict_env_stat '%s' '%z' "$local_file")"
  [[ "$local_bytes" =~ ^[0-9]+$ ]] || { add_alert "$label 本地证据大小无效"; return 1; }
  if (( expected_bytes > 0 )) && [[ "$local_bytes" != "$expected_bytes" ]]; then
    add_alert "$label 本地证据大小无效"
    return 1
  fi
  (( local_bytes >= 1 && local_bytes <= 4096 )) || { add_alert "$label 本地证据超过大小上限"; return 1; }
  local_digest="$(sha256sum "$local_file" | awk '{print $1}')"
  output="$(run_rclone hashsum sha256 "$remote" --download 2>/dev/null)" || { add_alert "$label 异机证据无法完整回读"; return 1; }
  [[ -n "$output" && "$output" != *$'\n'* ]] || { add_alert "$label 异机证据摘要输出不唯一"; return 1; }
  read -r remote_digest remote_name remote_extra <<<"$output"
  [[ "$remote_digest" == "$local_digest" && -n "$remote_name" && -z "${remote_extra:-}" ]] || {
    add_alert "$label 异机证据与本地不一致"
    return 1
  }
}

check_backup_kind() {
  local directory="$1" pattern="$2" max_age_minutes="$3" label="$4" require_offsite="$5" require_wal_source="${6:-0}"
  local artifact_class="$7" source_id="$8" provenance_key="$9"
  local latest mtime age_minutes recorded_checksum recorded_name remote_target
  local source_manifest source_checksum source_wal_name source_cluster source_extra
  if [[ ! -d "$directory" || -L "$directory" ]]; then
    add_alert "$label 目录无效"
    return
  fi
  if ! latest="$(find "$directory" -maxdepth 1 -type f -name "$pattern" -print 2>/dev/null | LC_ALL=C sort | tail -n 1)"; then
    add_alert "$label 目录不可读"
    return
  fi
  if [[ -z "$latest" ]] || ! validate_backup_manifest_metadata "$latest"; then
    add_alert "$label 缺失或本地校验清单无效"
    return
  fi
  recorded_checksum="$backup_manifest_checksum"
  recorded_name="$backup_manifest_name"
  mtime="$(stat -c %Y "$latest" 2>/dev/null || stat -f %m "$latest")"
  age_minutes=$(((now_epoch - mtime) / 60))
  (( age_minutes >= -5 )) || add_alert "$label 时间戳位于未来"
  (( age_minutes <= max_age_minutes )) || add_alert "$label 已过期(${age_minutes}分钟)"
  if [[ "$require_offsite" == 1 ]]; then
    if ! validate_offsite_marker_metadata "$latest" "$recorded_checksum"; then
      add_alert "$label 缺少或损坏异机上传凭证"
      return
    fi
    remote_target="$marker_remote"
    validate_remote_object_size "$remote_target" "$latest" "$label" || true
    validate_remote_checksum_manifest "$remote_target.sha256" "$recorded_checksum" "$recorded_name" "$label" || true
    if ! verify_backup_provenance "$latest" "$artifact_class" "$source_id" "$remote_target" "$provenance_key"; then
      add_alert "$label 来源 Ed25519 签名或绑定字段无效"
      return
    fi
    validate_remote_evidence_digest "$remote_target.provenance" "$latest.provenance" "$label 来源凭证" || true
    validate_remote_evidence_digest "$remote_target.provenance.sig" "$latest.provenance.sig" "$label 来源签名" 64 || true
  fi
  if [[ "$require_wal_source" == 1 ]]; then
    source_manifest="$latest.source.sha256"
    if [[ ! -f "$source_manifest" || -L "$source_manifest" ]]; then
      add_alert "$label 缺少源WAL与集群绑定凭证"
      return
    fi
    read -r source_checksum source_wal_name source_cluster source_extra <"$source_manifest" || true
    if [[ ! "$source_checksum" =~ ^[0-9a-f]{64}$ || "$source_wal_name.age" != "$(basename "$latest")" || "$source_cluster" != "$(basename "$directory")" || -n "${source_extra:-}" ]]; then
      add_alert "$label 源WAL与集群绑定凭证无效"
      return
    fi
    validate_remote_source_manifest "$remote_target.source.sha256" "$source_checksum" "$source_wal_name" "$source_cluster" "$label" || true
  fi
}
check_backup_kind "$MONITOR_DATABASE_BACKUP_DIR" '*.dump.age' "$MAX_BACKUP_AGE" "数据库加密备份" 1 0 database "$MONITOR_DATABASE_DBNAME" "$MONITOR_BACKUP_PROVENANCE_VERIFY_KEY_FILE"
check_backup_kind "$MONITOR_UPLOAD_BACKUP_DIR" 'uploads-*.tar.age' "$MAX_BACKUP_AGE" "上传目录加密备份" 1 0 uploads /var/lib/wangzhe/uploads "$MONITOR_BACKUP_PROVENANCE_VERIFY_KEY_FILE"
check_backup_kind "$MONITOR_PITR_WAL_DIR" '*.age' "$MAX_WAL_AGE" "PITR WAL归档" 1 1 pitr-wal "$(basename "$MONITOR_PITR_WAL_DIR")" "$MONITOR_PITR_PROVENANCE_VERIFY_KEY_FILE"
check_backup_kind "$MONITOR_PITR_BASEBACKUP_DIR" 'basebackup-*.tar.age' "$MAX_BASEBACKUP_AGE" "PITR基础备份" 1 0 pitr-basebackup "$(basename "$MONITOR_PITR_BASEBACKUP_DIR")" "$MONITOR_PITR_PROVENANCE_VERIFY_KEY_FILE"

validate_backup_integrity_status() {
  local file="$1" version completed_epoch database_name upload_name basebackup_name wal_count first_wal last_wal inventory_sha extra
  local database_suffix status_mode status_owner status_bytes integrity_age
  [[ -f "$file" && ! -L "$file" && -r "$file" ]] || return 1
  validate_no_symlink_path_components "$file" || return 1
  status_mode="$(strict_env_stat '%a' '%Lp' "$file")"
  status_owner="$(strict_env_stat '%u' '%u' "$file")"
  status_bytes="$(strict_env_stat '%s' '%z' "$file")"
  [[ "$status_mode" == 600 && "$status_owner" == "$EUID" && "$status_bytes" =~ ^[0-9]+$ ]] || return 1
  (( status_bytes >= 1 && status_bytes <= 4096 )) || return 1
  read -r version completed_epoch database_name upload_name basebackup_name wal_count first_wal last_wal inventory_sha extra <"$file" || return 1
  [[ "$version" == v2 && "$completed_epoch" =~ ^[1-9][0-9]{0,11}$ && -z "${extra:-}" ]] || return 1
  [[ "$database_name" == "$MONITOR_DATABASE_DBNAME"-* ]] || return 1
  database_suffix="${database_name#"$MONITOR_DATABASE_DBNAME"-}"
  [[ "$database_suffix" =~ ^[0-9]{8}-[0-9]{6}-[0-9]+\.dump\.age$ ]] || return 1
  [[ "$upload_name" =~ ^uploads-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age$ ]] || return 1
  [[ "$basebackup_name" =~ ^basebackup-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age$ ]] || return 1
  [[ "$wal_count" =~ ^[1-9][0-9]*$ && "$inventory_sha" =~ ^[0-9a-f]{64}$ ]] || return 1
  [[ "$first_wal" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)\.age$ ]] || return 1
  [[ "$last_wal" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)\.age$ ]] || return 1
  [[ "$first_wal" == "$last_wal" || "$first_wal" < "$last_wal" ]] || return 1
  (( completed_epoch <= now_epoch + 300 )) || return 1
  integrity_age=$(((now_epoch - completed_epoch) / 60))
  if (( integrity_age > MAX_BACKUP_INTEGRITY_AGE )); then
    add_alert "每日备份完整性全量校验已过期(${integrity_age}分钟)"
  fi
}
if ! validate_backup_integrity_status "$MONITOR_BACKUP_INTEGRITY_STATUS_FILE"; then
  add_alert "缺少或损坏每日备份完整性全量校验状态"
fi

check_backup_filesystem() {
  local directory="$1" label="$2" disk_percent inode_percent
  if [[ ! -d "$directory" || -L "$directory" ]]; then
    return
  fi
  disk_percent="$(df -P "$directory" 2>/dev/null | awk 'NR == 2 {gsub(/%/, "", $5); print $5}')"
  inode_percent="$(df -Pi "$directory" 2>/dev/null | awk 'NR == 2 {gsub(/%/, "", $5); print $5}')"
  if [[ "$disk_percent" =~ ^[0-9]+$ ]]; then
    (( disk_percent < MAX_BACKUP_DISK_PERCENT )) || add_alert "$label 磁盘使用率=${disk_percent}%"
  else
    add_alert "$label 磁盘容量指标读取失败"
  fi
  if [[ "$inode_percent" =~ ^[0-9]+$ ]]; then
    (( inode_percent < MAX_BACKUP_INODE_PERCENT )) || add_alert "$label inode使用率=${inode_percent}%"
  else
    add_alert "$label inode指标读取失败"
  fi
}
check_backup_filesystem "$MONITOR_DATABASE_BACKUP_DIR" "数据库备份盘"
check_backup_filesystem "$MONITOR_UPLOAD_BACKUP_DIR" "上传备份盘"
check_backup_filesystem "$MONITOR_PITR_WAL_DIR" "PITR WAL盘"
check_backup_filesystem "$MONITOR_PITR_BASEBACKUP_DIR" "PITR基础备份盘"

sync_remote_status() {
  local remote_source="$1" local_target="$2" expected_name="$3" label="$4" verify_key="$5"
  local status_partial checksum_partial signature_partial remote_checksum remote_name remote_extra downloaded_checksum
  local status_bytes checksum_bytes signature_bytes
  remote_checksum=""
  remote_name=""
  remote_extra=""
  status_partial="$(mktemp "$MONITOR_STATE_DIR/.remote-status.XXXXXX")"
  checksum_partial="$(mktemp "$MONITOR_STATE_DIR/.remote-checksum.XXXXXX")"
  signature_partial="$(mktemp "$MONITOR_STATE_DIR/.remote-signature.XXXXXX")"
  if run_rclone copyto "$remote_source" "$status_partial" --no-traverse >/dev/null 2>&1 && \
     run_rclone copyto "$remote_source.sha256" "$checksum_partial" --no-traverse >/dev/null 2>&1 && \
     run_rclone copyto "$remote_source.sig" "$signature_partial" --no-traverse >/dev/null 2>&1; then
    read -r remote_checksum remote_name remote_extra <"$checksum_partial" || true
    downloaded_checksum="$(sha256sum "$status_partial" | awk '{print $1}')"
    status_bytes="$(strict_env_stat '%s' '%z' "$status_partial")"
    checksum_bytes="$(strict_env_stat '%s' '%z' "$checksum_partial")"
    signature_bytes="$(strict_env_stat '%s' '%z' "$signature_partial")"
    if [[ ! "$status_bytes" =~ ^[0-9]+$ || ! "$checksum_bytes" =~ ^[0-9]+$ || "$status_bytes" -lt 1 || "$status_bytes" -gt 16384 || "$checksum_bytes" -lt 1 || "$checksum_bytes" -gt 4096 ]]; then
      add_alert "$label 远端状态或摘要大小无效"
    elif [[ "$remote_checksum" != "$downloaded_checksum" || "$remote_name" != "$expected_name" || -n "${remote_extra:-}" ]]; then
      add_alert "$label 远端状态摘要校验失败"
    elif [[ "$signature_bytes" != 64 ]] || ! openssl pkeyutl -verify -pubin -inkey "$verify_key" -rawin -in "$status_partial" -sigfile "$signature_partial" >/dev/null 2>&1; then
      add_alert "$label 远端状态 Ed25519 签名验证失败"
    elif [[ -L "$local_target" || ( -e "$local_target" && ! -f "$local_target" ) || -L "$local_target.sig" || ( -e "$local_target.sig" && ! -f "$local_target.sig" ) ]]; then
      add_alert "$label 本地状态目标不安全"
    else
      mv "$signature_partial" "$local_target.sig"
      signature_partial=""
      mv "$status_partial" "$local_target"
      status_partial=""
    fi
  else
    add_alert "无法同步$label 状态"
  fi
  [[ -z "$status_partial" ]] || rm -f -- "$status_partial"
  [[ -z "$signature_partial" ]] || rm -f -- "$signature_partial"
  rm -f -- "$checksum_partial"
}
sync_remote_status "$MONITOR_RESTORE_STATUS_REMOTE_SOURCE" "$MONITOR_RESTORE_STATUS_FILE" last-success.status "逻辑与上传恢复演练" "$MONITOR_RESTORE_STATUS_VERIFY_KEY_FILE"
sync_remote_status "$MONITOR_PITR_RESTORE_STATUS_REMOTE_SOURCE" "$MONITOR_PITR_RESTORE_STATUS_FILE" last-pitr-success.status "PITR恢复演练" "$MONITOR_PITR_RESTORE_STATUS_VERIFY_KEY_FILE"
status_field() {
  local file="$1" key="$2"
  awk -F= -v key="$key" '$1 == key { count++; value=substr($0, length(key) + 2) } END { if (count == 1) print value; else exit 1 }' "$file"
}
validate_logical_restore_status() {
  local file="$1" schema outcome scope isolation database_host database_restore upload_restore pitr_restore
  local epoch database_sha upload_sha database_provenance_sha upload_provenance_sha database_offsite_source upload_offsite_source
  local database_source_name database_backup_name upload_backup_name
  local migrations negative orphan manifest_entries restored_files database_bytes upload_bytes completed_utc
  local restore_work_luks_mount database_data_luks_mount upload_target_luks_mount
  schema="$(status_field "$file" status_schema)" || return 1
  outcome="$(status_field "$file" outcome)" || return 1
  scope="$(status_field "$file" scope)" || return 1
  isolation="$(status_field "$file" isolation)" || return 1
  database_host="$(status_field "$file" database_host)" || return 1
  database_restore="$(status_field "$file" database_restore)" || return 1
  upload_restore="$(status_field "$file" upload_restore)" || return 1
  pitr_restore="$(status_field "$file" pitr_restore)" || return 1
  epoch="$(status_field "$file" completed_at_epoch)" || return 1
  completed_utc="$(status_field "$file" completed_at_utc)" || return 1
  database_sha="$(status_field "$file" database_sha256)" || return 1
  upload_sha="$(status_field "$file" upload_sha256)" || return 1
  database_provenance_sha="$(status_field "$file" database_provenance_sha256)" || return 1
  upload_provenance_sha="$(status_field "$file" upload_provenance_sha256)" || return 1
  database_source_name="$(status_field "$file" database_source_name)" || return 1
  database_backup_name="$(status_field "$file" database_backup_name)" || return 1
  upload_backup_name="$(status_field "$file" upload_backup_name)" || return 1
  database_offsite_source="$(status_field "$file" database_offsite_source)" || return 1
  upload_offsite_source="$(status_field "$file" upload_offsite_source)" || return 1
  restore_work_luks_mount="$(status_field "$file" restore_work_luks_mount)" || return 1
  database_data_luks_mount="$(status_field "$file" database_data_luks_mount)" || return 1
  upload_target_luks_mount="$(status_field "$file" upload_target_luks_mount)" || return 1
  migrations="$(status_field "$file" schema_migrations)" || return 1
  negative="$(status_field "$file" negative_balances)" || return 1
  orphan="$(status_field "$file" orphan_bets)" || return 1
  manifest_entries="$(status_field "$file" upload_manifest_entries)" || return 1
  restored_files="$(status_field "$file" upload_restored_files)" || return 1
  database_bytes="$(status_field "$file" database_artifact_bytes)" || return 1
  upload_bytes="$(status_field "$file" upload_artifact_bytes)" || return 1
  [[ "$schema" == wangzhe.restore-drill.v2 && "$outcome" == success && "$scope" == logical_database_and_uploads ]] || return 1
  [[ "$isolation" == offsite_download_loopback_database_and_fixed_targets && "$database_host" == loopback ]] || return 1
  [[ "$database_restore" == verified && "$upload_restore" == verified && "$pitr_restore" == not_in_scope ]] || return 1
  [[ "$epoch" =~ ^[0-9]+$ && "$completed_utc" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]] || return 1
  [[ "$database_sha" =~ ^[0-9a-f]{64}$ && "$upload_sha" =~ ^[0-9a-f]{64}$ ]] || return 1
  [[ "$database_provenance_sha" =~ ^[0-9a-f]{64}$ && "$upload_provenance_sha" =~ ^[0-9a-f]{64}$ ]] || return 1
  [[ "$database_source_name" == "$MONITOR_RESTORE_EXPECTED_DATABASE_NAME" ]] || return 1
  [[ "$database_backup_name" =~ ^${database_source_name}-[0-9]{8}-[0-9]{6}-[0-9]+\.dump\.age$ ]] || return 1
  [[ "$upload_backup_name" =~ ^uploads-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age$ ]] || return 1
  validate_remote_destination "$database_offsite_source" >/dev/null 2>&1 || return 1
  validate_remote_destination "$upload_offsite_source" >/dev/null 2>&1 || return 1
  [[ "$database_offsite_source" == "${MONITOR_RESTORE_EXPECTED_DATABASE_REMOTE_SOURCE%/}/$database_backup_name" ]] || return 1
  [[ "$upload_offsite_source" == "${MONITOR_RESTORE_EXPECTED_UPLOAD_REMOTE_SOURCE%/}/$upload_backup_name" ]] || return 1
  [[ "$restore_work_luks_mount" == /var/lib/wangzhe-restore && "$upload_target_luks_mount" == "$restore_work_luks_mount" ]] || return 1
  [[ "$database_data_luks_mount" =~ ^/[A-Za-z0-9._/-]+$ && "$database_data_luks_mount" != / && "$database_data_luks_mount" != *..* ]] || return 1
  [[ "$migrations" =~ ^[0-9]+$ && "$negative" == 0 && "$orphan" == 0 ]] || return 1
  (( 10#$migrations > 0 )) || return 1
  [[ "$manifest_entries" =~ ^[0-9]+$ && "$restored_files" =~ ^[0-9]+$ && "$manifest_entries" == "$restored_files" ]] || return 1
  [[ "$database_bytes" =~ ^[0-9]+$ && "$upload_bytes" =~ ^[0-9]+$ ]] || return 1
  (( 10#$database_bytes > 0 && 10#$upload_bytes > 0 )) || return 1
  validated_restore_epoch="$epoch"
}
if [[ -f "$MONITOR_RESTORE_STATUS_FILE" && ! -L "$MONITOR_RESTORE_STATUS_FILE" ]]; then
  validated_restore_epoch=""
  if validate_logical_restore_status "$MONITOR_RESTORE_STATUS_FILE"; then
    if (( validated_restore_epoch > now_epoch + 300 )); then
      add_alert "隔离恢复演练状态时间位于未来"
    else
      restore_age=$(((now_epoch - validated_restore_epoch) / 60))
      (( restore_age <= MAX_RESTORE_AGE )) || add_alert "隔离恢复演练已过期(${restore_age}分钟)"
    fi
  else
    add_alert "隔离恢复演练状态证据无效"
  fi
else
  add_alert "缺少隔离恢复演练成功状态"
fi

validate_pitr_restore_status() {
  local file="$1" format completed target_reached epoch target_epoch target_utc base_name base_sha postgres_major system_identifier
  local timeline replay_lsn replay_timestamp wal_count segment_count first_wal last_wal wal_audit migrations negative orphan local_base
  local source_generation source_remote source_snapshot_sha source_synced_epoch source_base_count source_wal_count source_segment_count drill_luks_mount
  format="$(status_field "$file" format_version)" || return 1
  completed="$(status_field "$file" pitr_completed)" || return 1
  target_reached="$(status_field "$file" target_reached)" || return 1
  epoch="$(status_field "$file" completed_at_epoch)" || return 1
  target_epoch="$(status_field "$file" target_at_epoch)" || return 1
  target_utc="$(status_field "$file" target_at_utc)" || return 1
  drill_luks_mount="$(status_field "$file" drill_luks_mount)" || return 1
  source_generation="$(status_field "$file" source_generation)" || return 1
  source_remote="$(status_field "$file" source_remote_destination)" || return 1
  source_snapshot_sha="$(status_field "$file" source_snapshot_sha256)" || return 1
  source_synced_epoch="$(status_field "$file" source_synced_at_epoch)" || return 1
  source_base_count="$(status_field "$file" source_basebackup_count)" || return 1
  source_wal_count="$(status_field "$file" source_wal_count)" || return 1
  source_segment_count="$(status_field "$file" source_wal_segment_count)" || return 1
  base_name="$(status_field "$file" basebackup_file)" || return 1
  base_sha="$(status_field "$file" basebackup_sha256)" || return 1
  postgres_major="$(status_field "$file" postgres_major)" || return 1
  system_identifier="$(status_field "$file" system_identifier)" || return 1
  timeline="$(status_field "$file" timeline_id)" || return 1
  replay_lsn="$(status_field "$file" replay_lsn)" || return 1
  replay_timestamp="$(status_field "$file" replay_timestamp)" || return 1
  wal_count="$(status_field "$file" restored_wal_count)" || return 1
  segment_count="$(status_field "$file" restored_wal_segment_count)" || return 1
  first_wal="$(status_field "$file" first_restored_wal)" || return 1
  last_wal="$(status_field "$file" last_restored_wal)" || return 1
  wal_audit="$(status_field "$file" wal_audit_sha256)" || return 1
  migrations="$(status_field "$file" schema_migrations)" || return 1
  negative="$(status_field "$file" negative_balances)" || return 1
  orphan="$(status_field "$file" orphan_bets)" || return 1
  [[ "$format" == 2 && "$completed" == 1 && "$target_reached" == 1 ]] || return 1
  [[ "$drill_luks_mount" == /var/lib/wangzhe-pitr-drill ]] || return 1
  [[ "$epoch" =~ ^[0-9]+$ && "$target_epoch" =~ ^[0-9]+$ && "$target_epoch" -lt "$epoch" ]] || return 1
  [[ "$source_generation" =~ ^[0-9]{8}-[0-9]{6}-[0-9]+$ && "$source_remote" == "$MONITOR_PITR_RESTORE_EXPECTED_REMOTE_SOURCE" ]] || return 1
  [[ "$source_snapshot_sha" =~ ^[0-9a-f]{64}$ && "$source_synced_epoch" =~ ^[0-9]+$ ]] || return 1
  (( source_synced_epoch <= epoch && source_synced_epoch <= now_epoch + 300 )) || return 1
  [[ "$source_base_count" =~ ^[1-9][0-9]*$ && "$source_wal_count" =~ ^[1-9][0-9]*$ && "$source_segment_count" =~ ^[1-9][0-9]*$ ]] || return 1
  (( 10#$source_segment_count <= 10#$source_wal_count )) || return 1
  [[ "$target_utc" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}[[:space:]][0-9]{2}:[0-9]{2}:[0-9]{2}\+00$ ]] || return 1
  [[ "$base_name" =~ ^basebackup-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age$ && "$base_sha" =~ ^[0-9a-f]{64}$ ]] || return 1
  [[ "$postgres_major" =~ ^[0-9]+$ && "$system_identifier" == "$(basename "$MONITOR_PITR_BASEBACKUP_DIR")" ]] || return 1
  [[ "$timeline" =~ ^[0-9]+$ && "$replay_lsn" =~ ^[0-9A-F]+/[0-9A-F]+$ && -n "$replay_timestamp" ]] || return 1
  [[ "$wal_count" =~ ^[1-9][0-9]*$ && "$segment_count" =~ ^[1-9][0-9]*$ ]] || return 1
  [[ "$first_wal" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)$ ]] || return 1
  [[ "$last_wal" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)$ && "$wal_audit" =~ ^[0-9a-f]{64}$ ]] || return 1
  [[ "$migrations" =~ ^[0-9]+$ && "$negative" == 0 && "$orphan" == 0 ]] || return 1
  (( 10#$migrations > 0 )) || return 1
  local_base="$MONITOR_PITR_BASEBACKUP_DIR/$base_name"
  validate_backup_manifest_metadata "$local_base" || return 1
  [[ "$backup_manifest_checksum" == "$base_sha" ]] || return 1
  verify_backup_provenance "$local_base" pitr-basebackup "$system_identifier" "${source_remote%/}/$base_name" "$MONITOR_PITR_PROVENANCE_VERIFY_KEY_FILE" || return 1
  validated_pitr_restore_epoch="$epoch"
}
if [[ -f "$MONITOR_PITR_RESTORE_STATUS_FILE" && ! -L "$MONITOR_PITR_RESTORE_STATUS_FILE" ]]; then
  validated_pitr_restore_epoch=""
  if validate_pitr_restore_status "$MONITOR_PITR_RESTORE_STATUS_FILE"; then
    if (( validated_pitr_restore_epoch > now_epoch + 300 )); then
      add_alert "PITR恢复演练状态时间位于未来"
    else
      pitr_restore_age=$(((now_epoch - validated_pitr_restore_epoch) / 60))
      (( pitr_restore_age <= MAX_PITR_RESTORE_AGE )) || add_alert "PITR恢复演练已过期(${pitr_restore_age}分钟)"
    fi
  else
    add_alert "PITR恢复演练状态证据无效"
  fi
else
  add_alert "缺少PITR恢复演练成功状态"
fi

for tls_host in "${tls_hosts[@]}"; do
  if served_certificate="$(timeout 8 openssl s_client -connect "$tls_host:$MONITOR_TLS_PORT" -servername "$tls_host" -verify_hostname "$tls_host" -verify_return_error -showcerts </dev/null 2>/dev/null | timeout 4 openssl x509 -outform PEM 2>/dev/null)"; then
    if ! openssl x509 -checkend 1814400 -noout <<<"$served_certificate" >/dev/null 2>&1; then
      add_alert "$tls_host TLS证书将在21天内过期或无效"
    fi
  else
    add_alert "$tls_host TLS握手、证书链或主机名验证失败"
  fi
done

cursor_file="$MONITOR_STATE_DIR/nginx.cursor"
cursor_version=""
previous_inode=0
previous_line=0
cursor_extra=""
cursor_valid=0
write_nginx_cursor=0
if [[ -f "$cursor_file" && ! -L "$cursor_file" ]]; then
  read -r cursor_version previous_inode previous_line cursor_extra <"$cursor_file" || true
  if [[ "$cursor_version" == v1 && "$previous_inode" =~ ^[1-9][0-9]*$ && "$previous_line" =~ ^[0-9]+$ && -z "${cursor_extra:-}" ]]; then
    cursor_valid=1
  else
    add_alert "Nginx 5xx游标无效，已重新建立基线"
  fi
elif [[ -e "$cursor_file" || -L "$cursor_file" ]]; then
  add_alert "Nginx 5xx游标不是普通文件"
fi

nginx_file_inode() {
  stat -c %i "$1" 2>/dev/null || stat -f %i "$1"
}

count_nginx_5xx_from() {
  local log_file="$1" from_line="$2"
  tail -n +"$from_line" "$log_file" |
    jq -Rr 'fromjson? | select(((.status | tonumber?) // 0) >= 500 and ((.status | tonumber?) // 0) < 600) | 1' |
    wc -l | tr -d '[:space:]'
}

if [[ -f "$MONITOR_NGINX_ACCESS_LOG" && ! -L "$MONITOR_NGINX_ACCESS_LOG" && -r "$MONITOR_NGINX_ACCESS_LOG" ]]; then
  current_inode="$(nginx_file_inode "$MONITOR_NGINX_ACCESS_LOG")"
  current_line="$(wc -l <"$MONITOR_NGINX_ACCESS_LOG" | tr -d '[:space:]')"
  [[ "$current_inode" =~ ^[1-9][0-9]*$ && "$current_line" =~ ^[0-9]+$ ]] || { echo "Nginx访问日志元数据无效" >&2; exit 1; }
  write_nginx_cursor=1
  rotated_log="$MONITOR_NGINX_ACCESS_LOG.1"
  rotated_line=0
  rotated_new_count=0
  current_new_count=0
  scan_rotated_from=1
  scan_current_from=$((current_line + 1))
  scan_rotated=0

  if (( cursor_valid == 1 )); then
    if [[ "$current_inode" == "$previous_inode" ]]; then
      if (( current_line >= previous_line )); then
        current_new_count=$((current_line - previous_line))
        scan_current_from=$((previous_line + 1))
      else
        add_alert "Nginx日志同一 inode 被截断，5xx统计存在轮转缺口"
        current_new_count="$current_line"
        scan_current_from=1
      fi
    else
      rotated_inode=""
      if [[ -f "$rotated_log" && ! -L "$rotated_log" && -r "$rotated_log" ]]; then
        rotated_inode="$(nginx_file_inode "$rotated_log")"
      fi
      if [[ "$rotated_inode" == "$previous_inode" ]]; then
        rotated_line="$(wc -l <"$rotated_log" | tr -d '[:space:]')"
        if [[ "$rotated_line" =~ ^[0-9]+$ ]] && (( rotated_line >= previous_line )); then
          scan_rotated=1
          rotated_new_count=$((rotated_line - previous_line))
          scan_rotated_from=$((previous_line + 1))
        else
          add_alert "Nginx轮转日志短于旧游标，5xx统计存在轮转缺口"
        fi
      else
        add_alert "Nginx轮转后未找到匹配旧 inode 的 .1，5xx统计存在轮转缺口"
      fi
      current_new_count="$current_line"
      scan_current_from=1
    fi
  fi

  new_line_count=$((rotated_new_count + current_new_count))
  if (( new_line_count > 100000 )); then
    add_alert "Nginx日志积压超过100000行，5xx统计不完整"
    remaining_lines=100000
    current_take="$current_new_count"
    (( current_take <= remaining_lines )) || current_take="$remaining_lines"
    if (( current_take > 0 )); then
      scan_current_from=$((current_line - current_take + 1))
      remaining_lines=$((remaining_lines - current_take))
    else
      scan_current_from=$((current_line + 1))
    fi
    rotated_take="$rotated_new_count"
    (( rotated_take <= remaining_lines )) || rotated_take="$remaining_lines"
    if (( rotated_take > 0 )); then
      scan_rotated_from=$((rotated_line - rotated_take + 1))
    else
      scan_rotated=0
    fi
  fi

  five_xx=0
  if (( scan_rotated == 1 && scan_rotated_from <= rotated_line )); then
    rotated_five_xx="$(count_nginx_5xx_from "$rotated_log" "$scan_rotated_from")"
    five_xx=$((five_xx + rotated_five_xx))
  fi
  if (( scan_current_from <= current_line )); then
    current_five_xx="$(count_nginx_5xx_from "$MONITOR_NGINX_ACCESS_LOG" "$scan_current_from")"
    five_xx=$((five_xx + current_five_xx))
  fi
  (( five_xx <= MAX_5XX )) || add_alert "Nginx新发生5xx=$five_xx"
else
  add_alert "Nginx访问日志不可读"
fi

status_file="$MONITOR_STATE_DIR/alert.signature"
previous_signature=""
previous_sent_epoch=0
if [[ -f "$status_file" && ! -L "$status_file" ]]; then
  state_version=""
  state_signature=""
  state_sent_epoch=""
  state_extra=""
  read -r state_version state_signature state_sent_epoch state_extra <"$status_file" || true
  if [[ "$state_version" == v2 && "$state_signature" =~ ^[0-9a-f]{64}$ && "$state_sent_epoch" =~ ^(0|[1-9][0-9]{0,11})$ && -z "$state_extra" ]]; then
    previous_signature="$state_signature"
    previous_sent_epoch="$state_sent_epoch"
  elif [[ "$state_version" =~ ^[0-9a-f]{64}$ && -z "${state_signature:-}" ]]; then
    # One-time migration from the original signature-only state.  An active
    # alert is resent immediately because the old file carried no send time.
    previous_signature="$state_version"
  fi
fi

alerts_text=""
if (( ${#alerts[@]} > 0 )); then
  printf -v alerts_text '%s\n' "${alerts[@]}"
fi
signature="$(printf '%s' "$alerts_text" | sha256sum | awk '{print $1}')"
empty_signature="$(printf '' | sha256sum | awk '{print $1}')"
repeat_seconds=$((ALERT_REPEAT_MINUTES * 60))
send_status=""
if (( ${#alerts[@]} > 0 )); then
  if [[ "$signature" != "$previous_signature" ]] || (( previous_sent_epoch == 0 || now_epoch < previous_sent_epoch || now_epoch - previous_sent_epoch >= repeat_seconds )); then
    send_status=firing
  fi
elif [[ -n "$previous_signature" && "$previous_signature" != "$empty_signature" ]]; then
  send_status=resolved
fi

if [[ -n "$send_status" ]]; then
  payload_file="$(mktemp "$MONITOR_STATE_DIR/.payload.XXXXXX")"
  curl_config="$(mktemp "$MONITOR_STATE_DIR/.curl.XXXXXX")"
  cleanup_monitor_tmp() { rm -f -- "$payload_file" "$curl_config"; }
  trap cleanup_monitor_tmp EXIT INT TERM
  printf '%s' "$alerts_text" | jq -R -s --arg status "$send_status" --argjson timestamp "$now_epoch" \
    '{service:"wangzhe",status:$status,timestamp:$timestamp,alerts:(split("\n")|map(select(length>0)))}' >"$payload_file"
  {
    printf 'url = "%s"\n' "$MONITOR_WEBHOOK_URL"
    printf 'request = "POST"\nheader = "Content-Type: application/json"\n'
    printf 'header = "Authorization: Bearer %s"\n' "$MONITOR_WEBHOOK_BEARER_TOKEN"
    printf 'fail\nsilent\nshow-error\nmax-time = 10\n'
  } >"$curl_config"
  curl --config "$curl_config" --data-binary "@$payload_file" >/dev/null || {
    echo "主动告警发送失败；状态未确认，下次将重试" >&2
    exit 1
  }
  rm -f -- "$payload_file" "$curl_config"
  trap - EXIT INT TERM
fi

sent_epoch="$previous_sent_epoch"
if [[ -n "$send_status" ]]; then
  sent_epoch="$now_epoch"
fi
printf 'v2 %s %s\n' "$signature" "$sent_epoch" >"$status_file.partial"
mv "$status_file.partial" "$status_file"
if (( write_nginx_cursor == 1 )); then
  printf 'v1 %s %s\n' "$current_inode" "$current_line" >"$cursor_file.partial"
  mv "$cursor_file.partial" "$cursor_file"
fi
if (( ${#alerts[@]} > 0 )); then
  printf '监控发现 %d 个问题：%s\n' "${#alerts[@]}" "${alerts[*]}" >&2
  exit 2
fi
echo "生产主动监控通过"
