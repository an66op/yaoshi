#!/usr/bin/env bash
set -euo pipefail

ENV_SOURCE="${1:-/etc/wangzhe/backend.env}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/wangzhe}"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-14}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/backend-env.sh
source "$SCRIPT_DIR/lib/backend-env.sh"
if [[ "$ENV_SOURCE" == "--current-env" ]]; then
  : # The caller deliberately supplied every BACKEND_* value in this process.
else
  load_backend_env "$ENV_SOURCE"
fi

for command_name in pg_dump pg_restore sha256sum find stat awk basename; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done

: "${BACKEND_DATABASE_HOST:?缺少 BACKEND_DATABASE_HOST}"
: "${BACKEND_DATABASE_PORT:?缺少 BACKEND_DATABASE_PORT}"
: "${BACKEND_DATABASE_USER:?缺少 BACKEND_DATABASE_USER}"
: "${BACKEND_DATABASE_DBNAME:?缺少 BACKEND_DATABASE_DBNAME}"
: "${BACKEND_DATABASE_PASSWORD:?缺少 BACKEND_DATABASE_PASSWORD}"
: "${BACKEND_DATABASE_SSLMODE:?缺少 BACKEND_DATABASE_SSLMODE}"
case "$BACKEND_DATABASE_SSLMODE" in
  disable|allow|prefer|require|verify-ca|verify-full) ;;
  *) echo "BACKEND_DATABASE_SSLMODE 不正确" >&2; exit 1 ;;
esac

case "$BACKUP_DIR" in
  /|/home|/Users|"$HOME") echo "拒绝使用过宽的备份目录：$BACKUP_DIR" >&2; exit 1 ;;
esac
[[ "$BACKUP_DIR" == /* ]] || { echo "备份目录必须是绝对路径" >&2; exit 1; }
[[ "$BACKEND_DATABASE_DBNAME" =~ ^[A-Za-z0-9_.-]+$ ]] || { echo "数据库名不能用于安全的备份文件名" >&2; exit 1; }
[[ "$RETENTION_DAYS" =~ ^[0-9]+$ ]] && (( RETENTION_DAYS >= 1 && RETENTION_DAYS <= 3650 )) || {
  echo "BACKUP_RETENTION_DAYS 必须是 1-3650 的整数" >&2
  exit 1
}

umask 077
mkdir -p "$BACKUP_DIR"
[[ -d "$BACKUP_DIR" && ! -L "$BACKUP_DIR" ]] || { echo "备份目录不能是符号链接" >&2; exit 1; }
timestamp="$(date +%Y%m%d-%H%M%S)"
target="$BACKUP_DIR/${BACKEND_DATABASE_DBNAME}-${timestamp}.dump"
partial="$target.partial"
checksum_partial="$target.sha256.partial"
[[ ! -e "$target" && ! -e "$partial" && ! -e "$checksum_partial" ]] || { echo "同名备份文件已存在，拒绝覆盖" >&2; exit 1; }
cleanup_partial() {
  if [[ -n "${partial:-}" ]]; then
    rm -f -- "$partial"
  fi
  if [[ -n "${checksum_partial:-}" ]]; then
    rm -f -- "$checksum_partial"
  fi
  return 0
}
trap cleanup_partial EXIT INT TERM

export PGPASSWORD="$BACKEND_DATABASE_PASSWORD"
export PGSSLMODE="$BACKEND_DATABASE_SSLMODE"
pg_dump \
  --host "$BACKEND_DATABASE_HOST" \
  --port "$BACKEND_DATABASE_PORT" \
  --username "$BACKEND_DATABASE_USER" \
  --dbname "$BACKEND_DATABASE_DBNAME" \
  --format custom \
  --no-owner \
  --file "$partial"
pg_restore --list "$partial" >/dev/null
[[ -s "$partial" ]] || { echo "备份文件为空" >&2; exit 1; }
checksum="$(sha256sum "$partial" | awk '{print $1}')"
printf '%s  %s\n' "$checksum" "$(basename "$target")" >"$checksum_partial"
mv "$partial" "$target"
partial=""
mv "$checksum_partial" "$target.sha256"
checksum_partial=""

find "$BACKUP_DIR" -maxdepth 1 -type f \
  \( -name "${BACKEND_DATABASE_DBNAME}-*.dump" -o -name "${BACKEND_DATABASE_DBNAME}-*.dump.sha256" \) \
  -mtime "+$RETENTION_DAYS" -print -delete

echo "数据库备份完成：$target"
