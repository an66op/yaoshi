#!/usr/bin/env bash
set -euo pipefail

usage() { echo "用法：scripts/dev-reset-complete-receipt.sh --receipt /绝对路径/*.full-reset-receipt [ENV_FILE]" >&2; }
receipt_file=""
env_file=""
while (($#)); do
  case "$1" in
    --receipt) [[ $# -ge 2 ]] || { usage; exit 2; }; receipt_file="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    --*) echo "未知参数：$1" >&2; usage; exit 2 ;;
    *) [[ -z "$env_file" ]] || { echo "只能指定一个环境文件" >&2; exit 2; }; env_file="$1"; shift ;;
  esac
done

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/backend-env.sh
source "$script_dir/lib/backend-env.sh"
# shellcheck source=lib/dev-reset-safety.sh
source "$script_dir/lib/dev-reset-safety.sh"
if [[ -n "$env_file" ]]; then
  unset BACKEND_SERVER_MODE BACKEND_SERVER_PORT BACKEND_DATABASE_HOST BACKEND_DATABASE_PORT
  unset BACKEND_DATABASE_USER BACKEND_DATABASE_PASSWORD BACKEND_DATABASE_DBNAME BACKEND_DATABASE_SSLMODE
  unset BACKEND_DEVELOPMENT_RESET_SENTINEL_TOKEN BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY
  load_backend_env "$env_file"
fi
for name in BACKEND_SERVER_MODE BACKEND_SERVER_PORT BACKEND_DATABASE_HOST BACKEND_DATABASE_PORT BACKEND_DATABASE_USER BACKEND_DATABASE_PASSWORD BACKEND_DATABASE_DBNAME BACKEND_DATABASE_SSLMODE; do
  [[ -n "${!name:-}" ]] || { echo "缺少 $name" >&2; exit 1; }
done
[[ "$BACKEND_SERVER_MODE" == "debug" ]] || { echo "凭证完成仅允许 debug 环境" >&2; exit 1; }
case "$BACKEND_DATABASE_HOST" in 127.0.0.1|localhost|::1) ;; *) echo "凭证完成只允许本机 PostgreSQL" >&2; exit 1 ;; esac
[[ "$BACKEND_SERVER_PORT" =~ ^[0-9]+$ ]] && (( BACKEND_SERVER_PORT >= 1 && BACKEND_SERVER_PORT <= 65535 )) || { echo "后端端口不正确" >&2; exit 1; }
[[ "$receipt_file" == /* && -f "$receipt_file" && ! -L "$receipt_file" ]] || { echo "--receipt 必须是明确的绝对普通文件" >&2; exit 1; }
sentinel_token="${BACKEND_DEVELOPMENT_RESET_SENTINEL_TOKEN:-}"
(( ${#sentinel_token} >= 32 && ${#sentinel_token} <= 256 )) || { echo "sentinel token 必须为 32-256 个字符" >&2; exit 1; }
for command_name in age psql curl awk chmod id mktemp mv rm basename dirname mkdir rmdir stat tar; do command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }; done

receipt_value() { awk -F= -v key="$1" '$1 == key { sub(/^[^=]*=/, ""); print }' "$receipt_file"; }
receipt_unique() { [[ "$(awk -F= -v key="$1" '$1 == key {n++} END {print n+0}' "$receipt_file")" == "1" ]]; }
for key in status database backup backup_sha256 scope server_system_identifier server_address server_port sentinel_token_sha256 payment_qr_backup payment_qr_backup_sha256 payment_qr_files_archived payment_qr_files_expected payment_qr_files_removed; do
  receipt_unique "$key" || { echo "凭证字段 $key 缺失或不唯一" >&2; exit 1; }
done
[[ "$(receipt_value status)" == "bootstrap_pending" ]] || { echo "凭证不是 bootstrap_pending，拒绝改写" >&2; exit 1; }
[[ "$(receipt_value database)" == "$BACKEND_DATABASE_DBNAME" && "$(receipt_value scope)" == "public_schema_rebuild" ]] || { echo "凭证目标或范围不匹配" >&2; exit 1; }
backup_name="$(receipt_value backup)"
[[ -n "$backup_name" && "$backup_name" == "$(basename "$backup_name")" ]] || { echo "凭证备份文件名不安全" >&2; exit 1; }
backup_file="$(dirname "$receipt_file")/$backup_name"
[[ -f "$backup_file" && ! -L "$backup_file" ]] || { echo "凭证对应备份不存在" >&2; exit 1; }
expected_backup_sha256="$(receipt_value backup_sha256)"
[[ "$expected_backup_sha256" =~ ^[0-9a-f]{64}$ ]] || { echo "凭证备份摘要格式错误" >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then actual_backup_sha256="$(sha256sum "$backup_file" | awk '{print $1}')"; else actual_backup_sha256="$(shasum -a 256 "$backup_file" | awk '{print $1}')"; fi
[[ "$actual_backup_sha256" == "$expected_backup_sha256" ]] || { echo "备份文件摘要与凭证不一致" >&2; exit 1; }

payment_qr_backup_name="$(receipt_value payment_qr_backup)"
[[ -n "$payment_qr_backup_name" && "$payment_qr_backup_name" == "$(basename "$payment_qr_backup_name")" ]] || { echo "凭证二维码归档文件名不安全" >&2; exit 1; }
payment_qr_backup="$(dirname "$receipt_file")/$payment_qr_backup_name"
payment_qr_backup_sha256="$(receipt_value payment_qr_backup_sha256)"
payment_qr_files_archived="$(receipt_value payment_qr_files_archived)"
payment_qr_files_expected="$(receipt_value payment_qr_files_expected)"
payment_qr_files_removed="$(receipt_value payment_qr_files_removed)"
[[ "$payment_qr_files_archived" =~ ^[0-9]+$ && "$payment_qr_files_archived" == "$payment_qr_files_expected" && "$payment_qr_files_archived" == "$payment_qr_files_removed" ]] || {
  echo "凭证中的二维码归档、预期和删除数量不一致" >&2
  exit 1
}
[[ -f "$payment_qr_backup.sha256" && ! -L "$payment_qr_backup.sha256" ]] || { echo "二维码归档校验文件不存在" >&2; exit 1; }
read -r payment_qr_manifest_sha256 payment_qr_manifest_name payment_qr_manifest_extra <"$payment_qr_backup.sha256" || { echo "二维码归档校验文件不可读" >&2; exit 1; }
[[ "$payment_qr_manifest_sha256" == "$payment_qr_backup_sha256" && "$payment_qr_manifest_name" == "$payment_qr_backup_name" && -z "${payment_qr_manifest_extra:-}" ]] || {
  echo "二维码归档校验文件与凭证不一致" >&2
  exit 1
}
reset_validate_payment_qr_archive "$payment_qr_backup" "$payment_qr_backup_sha256" "$payment_qr_files_archived" \
  "${BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY:?缺少 BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY}"
unset BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY

token_sha256="$(reset_sha256 "$sentinel_token")"
unset sentinel_token BACKEND_DEVELOPMENT_RESET_SENTINEL_TOKEN
[[ "$(receipt_value sentinel_token_sha256)" == "$token_sha256" ]] || { echo "sentinel 授权与凭证不一致" >&2; exit 1; }
identity_now="$(reset_verified_identity "$token_sha256" false)"
IFS=$'\t' read -r system_identifier server_address server_port _ _ _ <<<"$identity_now"
[[ "$(receipt_value server_system_identifier)" == "$system_identifier" && "$(receipt_value server_address)" == "$server_address" && "$(receipt_value server_port)" == "$server_port" ]] || { echo "物理数据库身份与凭证不一致" >&2; exit 1; }

ready_json="$(curl --fail --silent --show-error --max-time 5 "http://127.0.0.1:${BACKEND_SERVER_PORT}/ready")"
grep -Eq '"status"[[:space:]]*:[[:space:]]*"ready"' <<<"$ready_json" || { echo "/ready 未返回 ready" >&2; exit 1; }
if [[ -n "$env_file" ]]; then bash "$script_dir/dev-reset-verify-bootstrap.sh" "$env_file"; else bash "$script_dir/dev-reset-verify-bootstrap.sh"; fi

lock_dir="$receipt_file.lock"
mkdir "$lock_dir" 2>/dev/null || { echo "凭证正由其他验收进程处理" >&2; exit 1; }
receipt_partial="$receipt_file.partial"
cleanup_receipt() { rm -f -- "$receipt_partial"; rmdir "$lock_dir" 2>/dev/null || true; }
trap cleanup_receipt EXIT INT TERM
[[ "$(receipt_value status)" == "bootstrap_pending" ]] || { echo "凭证状态已变化，拒绝覆盖" >&2; exit 1; }
[[ ! -e "$receipt_partial" ]] || { echo "凭证临时文件已存在" >&2; exit 1; }
umask 077
awk -F= '$1 != "status" && $1 != "validated_at_utc"' "$receipt_file" >"$receipt_partial"
printf 'status=complete\nvalidated_at_utc=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >>"$receipt_partial"
mv "$receipt_partial" "$receipt_file"
rmdir "$lock_dir"
trap - EXIT INT TERM
echo "严格只读 bootstrap 验收通过；指定凭证已原子更新为 complete：$receipt_file"
