#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
用法：
  scripts/dev-reset-restore-payment-qr.sh \
    --receipt /绝对路径/<数据库备份>.reset-receipt \
    --upload-dir /绝对路径/backend/uploads [ENV_FILE]

必须先停止后端并恢复同一凭证对应的数据库备份。本工具只把该凭证中
经过 age 解密、SHA-256、条目类型和路径校验的配套二维码归档恢复到一个
当前不含二维码文件的明确上传根目录；不会合并或覆盖现有二维码。
USAGE
}

receipt_file=""
upload_root=""
env_file=""
while (($#)); do
  case "$1" in
    --receipt) [[ $# -ge 2 ]] || { usage; exit 2; }; receipt_file="$2"; shift 2 ;;
    --upload-dir) [[ $# -ge 2 ]] || { usage; exit 2; }; upload_root="$2"; shift 2 ;;
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
  unset BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY
  load_backend_env "$env_file"
fi
for name in BACKEND_SERVER_MODE BACKEND_SERVER_PORT BACKEND_DATABASE_HOST BACKEND_DATABASE_PORT BACKEND_DATABASE_USER BACKEND_DATABASE_PASSWORD BACKEND_DATABASE_DBNAME BACKEND_DATABASE_SSLMODE; do
  [[ -n "${!name:-}" ]] || { echo "缺少 $name" >&2; exit 1; }
done
[[ "$BACKEND_SERVER_MODE" == "debug" ]] || { echo "二维码恢复仅允许 debug 环境" >&2; exit 1; }
case "$BACKEND_DATABASE_HOST" in 127.0.0.1|localhost|::1) ;; *) echo "二维码恢复只允许本机 PostgreSQL" >&2; exit 1 ;; esac
for command_name in age awk basename chmod cmp dirname find id lsof mktemp psql rm sort stat tar; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done
[[ "$receipt_file" == /* && -f "$receipt_file" && ! -L "$receipt_file" ]] || {
  echo "--receipt 必须是明确的绝对普通文件" >&2
  exit 1
}
receipt_parent="$(dirname "$receipt_file")"
[[ "$(cd "$receipt_parent" && pwd -P)" == "$receipt_parent" ]] || {
  echo "凭证路径包含符号链接" >&2
  exit 1
}
[[ "$upload_root" == /* ]] || { echo "--upload-dir 必须是明确的绝对路径" >&2; exit 1; }
age_identity="${BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY:?缺少 BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY}"

receipt_value() { awk -F= -v key="$1" '$1 == key { sub(/^[^=]*=/, ""); print }' "$receipt_file"; }
receipt_unique() { [[ "$(awk -F= -v key="$1" '$1 == key {n++} END {print n+0}' "$receipt_file")" == "1" ]]; }
for key in status database backup backup_sha256 payment_qr_backup payment_qr_backup_sha256 payment_qr_files_archived payment_qr_files_expected payment_qr_files_removed; do
  receipt_unique "$key" || { echo "凭证字段 $key 缺失或不唯一" >&2; exit 1; }
done
[[ "$(receipt_value database)" == "$BACKEND_DATABASE_DBNAME" ]] || { echo "凭证数据库与当前目标不一致" >&2; exit 1; }
case "$(receipt_value status)" in
  complete|bootstrap_pending) ;;
  *) echo "凭证尚未到达可恢复状态" >&2; exit 1 ;;
esac

backup_name="$(receipt_value backup)"
qr_archive_name="$(receipt_value payment_qr_backup)"
[[ -n "$backup_name" && "$backup_name" == "$(basename "$backup_name")" ]] || { echo "数据库备份文件名不安全" >&2; exit 1; }
[[ "$qr_archive_name" == "$backup_name.member-payment-qr.tar.age" && "$qr_archive_name" == "$(basename "$qr_archive_name")" ]] || {
  echo "二维码归档没有与凭证数据库备份精确配对" >&2
  exit 1
}
database_backup="$receipt_parent/$backup_name"
qr_archive="$receipt_parent/$qr_archive_name"
database_sha256="$(receipt_value backup_sha256)"
qr_sha256="$(receipt_value payment_qr_backup_sha256)"
archived_count="$(receipt_value payment_qr_files_archived)"
expected_count="$(receipt_value payment_qr_files_expected)"
removed_count="$(receipt_value payment_qr_files_removed)"
[[ "$database_sha256" =~ ^[0-9a-f]{64}$ && "$qr_sha256" =~ ^[0-9a-f]{64}$ ]] || { echo "凭证摘要格式错误" >&2; exit 1; }
[[ "$archived_count" =~ ^[0-9]+$ && "$archived_count" == "$expected_count" && "$archived_count" == "$removed_count" ]] || {
  echo "凭证二维码归档、预期和删除数量不一致" >&2
  exit 1
}
[[ -f "$database_backup" && ! -L "$database_backup" && "$(reset_file_sha256 "$database_backup")" == "$database_sha256" ]] || {
  echo "配对数据库备份不存在或摘要不匹配" >&2
  exit 1
}
[[ -f "$qr_archive.sha256" && ! -L "$qr_archive.sha256" ]] || { echo "二维码归档校验文件不存在" >&2; exit 1; }
read -r manifest_sha256 manifest_name manifest_extra <"$qr_archive.sha256" || { echo "二维码归档校验文件不可读" >&2; exit 1; }
[[ "$manifest_sha256" == "$qr_sha256" && "$manifest_name" == "$qr_archive_name" && -z "${manifest_extra:-}" ]] || {
  echo "二维码归档校验文件与凭证不一致" >&2
  exit 1
}
reset_validate_payment_qr_archive "$qr_archive" "$qr_sha256" "$archived_count" "$age_identity"
reset_assert_backend_port_stopped
reset_validate_payment_qr_archive_database_consistency "$qr_archive" "$qr_sha256" "$archived_count" "$age_identity"
reset_validate_payment_qr_cleanup_target "$upload_root"
(( RESET_PAYMENT_QR_FILE_COUNT == 0 )) || { echo "目标上传目录已有二维码文件，拒绝合并或覆盖" >&2; exit 1; }

umask 077
decrypted_archive=""
restore_identity_file=""
cleanup_restore_archive() {
  [[ -z "$decrypted_archive" ]] || rm -f -- "$decrypted_archive"
  [[ -z "$restore_identity_file" ]] || rm -f -- "$restore_identity_file"
}
trap cleanup_restore_archive EXIT INT TERM
decrypted_archive="$(mktemp "$upload_root/.member-payment-qr-restore.XXXXXXXX")"
restore_identity_file="$(mktemp "$upload_root/.member-payment-qr-identity.XXXXXXXX")"
chmod 600 "$decrypted_archive" "$restore_identity_file"
printf '%s\n' "$age_identity" >"$restore_identity_file"
age --decrypt --identity "$restore_identity_file" --output "$decrypted_archive" "$qr_archive"
[[ "$(reset_file_sha256 "$qr_archive")" == "$qr_sha256" ]] || { echo "二维码归档在恢复期间发生变化" >&2; exit 1; }
tar --extract --file "$decrypted_archive" --directory "$upload_root" --no-same-owner --no-same-permissions
reset_validate_payment_qr_cleanup_target "$upload_root"
[[ "$RESET_PAYMENT_QR_FILE_COUNT" == "$archived_count" ]] || { echo "二维码恢复后的文件数不一致" >&2; exit 1; }
rm -- "$decrypted_archive" "$restore_identity_file"
trap - EXIT INT TERM
unset age_identity BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY
echo "配套会员收款二维码已恢复：$archived_count 个文件"
