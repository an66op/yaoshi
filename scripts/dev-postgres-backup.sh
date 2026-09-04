#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
用法：
  scripts/dev-postgres-backup.sh --backup-dir /明确/且仅当前用户可访问的目录

仅用于本机 debug 数据库的业务重置前完整备份。调用进程必须显式提供
全部 BACKEND_DATABASE_*、BACKEND_SERVER_MODE，以及由本机密码管理器读取的
BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY。备份使用 age 收件人加密；脚本会在
发布最终文件前完成 pg_restore 明文校验、解密回读校验和 SHA-256 校验。
USAGE
}

backup_dir=""
while (($#)); do
  case "$1" in
    --backup-dir)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      backup_dir="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "未知参数：$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

: "${BACKEND_SERVER_MODE:?必须明确设置 BACKEND_SERVER_MODE}"
: "${BACKEND_DATABASE_HOST:?缺少 BACKEND_DATABASE_HOST}"
: "${BACKEND_DATABASE_PORT:?缺少 BACKEND_DATABASE_PORT}"
: "${BACKEND_DATABASE_USER:?缺少 BACKEND_DATABASE_USER}"
: "${BACKEND_DATABASE_PASSWORD:?缺少 BACKEND_DATABASE_PASSWORD}"
: "${BACKEND_DATABASE_DBNAME:?缺少 BACKEND_DATABASE_DBNAME}"
: "${BACKEND_DATABASE_SSLMODE:?缺少 BACKEND_DATABASE_SSLMODE}"
: "${BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY:?缺少 BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY}"

[[ "$BACKEND_SERVER_MODE" == "debug" ]] || { echo "开发备份仅允许 debug 环境" >&2; exit 1; }
case "$BACKEND_DATABASE_HOST" in
  127.0.0.1|localhost|::1) ;;
  *) echo "开发备份只允许本机 PostgreSQL" >&2; exit 1 ;;
esac
[[ "$BACKEND_DATABASE_PORT" =~ ^[0-9]+$ ]] &&
  (( BACKEND_DATABASE_PORT >= 1 && BACKEND_DATABASE_PORT <= 65535 )) || {
  echo "数据库端口不正确" >&2
  exit 1
}
[[ "$BACKEND_DATABASE_DBNAME" =~ ^[A-Za-z0-9_.-]+$ ]] || {
  echo "数据库名格式不安全" >&2
  exit 1
}
[[ "$BACKEND_DATABASE_SSLMODE" == "disable" ]] || {
  echo "本机开发备份要求 BACKEND_DATABASE_SSLMODE=disable" >&2
  exit 1
}
[[ -n "$backup_dir" && "$backup_dir" == /* ]] || {
  echo "--backup-dir 必须是明确的绝对路径" >&2
  exit 1
}
case "$backup_dir" in
  /|/Users|/home|"$HOME")
    echo "拒绝使用过宽的备份目录：$backup_dir" >&2
    exit 1
    ;;
esac

backup_identity="$BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY"
unset BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY
[[ "$backup_identity" == *"AGE-SECRET-KEY-"* ]] || {
  echo "开发备份 age identity 格式不正确" >&2
  exit 1
}

for command_name in age age-keygen awk basename chmod date dirname id mkdir mktemp mv psql rmdir rm sed stat uname; do
  command -v "$command_name" >/dev/null 2>&1 || {
    echo "缺少命令：$command_name" >&2
    exit 1
  }
done
if command -v sha256sum >/dev/null 2>&1; then
  sha256_file() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
  sha256_file() { shasum -a 256 "$1" | awk '{print $1}'; }
else
  echo "缺少 sha256sum 或 shasum" >&2
  exit 1
fi

export PGPASSWORD="$BACKEND_DATABASE_PASSWORD"
export PGSSLMODE="$BACKEND_DATABASE_SSLMODE"
export PGAPPNAME="wangzhe-dev-backup-version-check"
server_version_num="$(psql \
  --host "$BACKEND_DATABASE_HOST" \
  --port "$BACKEND_DATABASE_PORT" \
  --username "$BACKEND_DATABASE_USER" \
  --dbname "$BACKEND_DATABASE_DBNAME" \
  --no-psqlrc --tuples-only --no-align --quiet \
  --command 'SHOW server_version_num')"
unset PGPASSWORD PGSSLMODE PGAPPNAME
[[ "$server_version_num" =~ ^[0-9]+$ ]] || {
  echo "无法读取 PostgreSQL 服务端版本" >&2
  exit 1
}
server_major="$((server_version_num / 10000))"

pg_bin_candidates=()
if [[ -n "${BACKEND_DEVELOPMENT_PG_BIN_DIR:-}" ]]; then
  [[ "$BACKEND_DEVELOPMENT_PG_BIN_DIR" == /* ]] || {
    echo "BACKEND_DEVELOPMENT_PG_BIN_DIR 必须是绝对路径" >&2
    exit 1
  }
  pg_bin_candidates+=("$BACKEND_DEVELOPMENT_PG_BIN_DIR")
fi
pg_bin_candidates+=(
  "/Library/PostgreSQL/$server_major/bin"
  "/opt/homebrew/opt/postgresql@$server_major/bin"
  "/usr/local/opt/postgresql@$server_major/bin"
  "$(dirname "$(command -v pg_dump 2>/dev/null || printf '/missing/pg_dump')")"
)
pg_dump_bin=""
pg_restore_bin=""
for candidate_dir in "${pg_bin_candidates[@]}"; do
  [[ -x "$candidate_dir/pg_dump" && -x "$candidate_dir/pg_restore" ]] || continue
  candidate_major="$("$candidate_dir/pg_dump" --version | sed -nE 's/.*\) ([0-9]+)(\..*)?$/\1/p')"
  if [[ "$candidate_major" == "$server_major" ]]; then
    pg_dump_bin="$candidate_dir/pg_dump"
    pg_restore_bin="$candidate_dir/pg_restore"
    break
  fi
done
[[ -n "$pg_dump_bin" && -n "$pg_restore_bin" ]] || {
  echo "找不到与 PostgreSQL $server_major 服务端匹配的 pg_dump/pg_restore；可显式设置 BACKEND_DEVELOPMENT_PG_BIN_DIR" >&2
  exit 1
}

umask 077
backup_parent="$(dirname "$backup_dir")"
[[ -d "$backup_parent" && ! -L "$backup_parent" ]] || {
  echo "备份父目录不存在或是符号链接：$backup_parent" >&2
  exit 1
}
if [[ ! -e "$backup_dir" ]]; then
  mkdir -m 700 "$backup_dir"
fi
[[ -d "$backup_dir" && ! -L "$backup_dir" ]] || {
  echo "备份目录必须是普通目录且不能是符号链接：$backup_dir" >&2
  exit 1
}
physical_backup_dir="$(cd "$backup_dir" && pwd -P)"
[[ "$physical_backup_dir" == "$backup_dir" ]] || {
  echo "备份目录路径包含符号链接，拒绝继续：$backup_dir" >&2
  exit 1
}

if [[ "$(uname -s)" == "Darwin" ]]; then
  backup_uid="$(stat -f '%u' "$backup_dir")"
  backup_mode="$(stat -f '%Lp' "$backup_dir")"
else
  backup_uid="$(stat -c '%u' "$backup_dir")"
  backup_mode="$(stat -c '%a' "$backup_dir")"
fi
[[ "$backup_uid" == "$(id -u)" ]] || {
  echo "备份目录不属于当前用户" >&2
  exit 1
}
mode_tail="${backup_mode: -2}"
[[ "$mode_tail" == "00" ]] || {
  echo "备份目录必须禁止 group/other 访问（当前权限 $backup_mode）" >&2
  exit 1
}

lock_dir="$backup_dir/.dev-backup.lock"
mkdir "$lock_dir" 2>/dev/null || {
  echo "另一个开发数据库备份仍在运行" >&2
  exit 1
}

temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/wangzhe-dev-pg-backup.XXXXXX")"
chmod 700 "$temporary_dir"
plain_dump="$temporary_dir/database.dump"
verified_dump="$temporary_dir/database.verify.dump"
identity_file="$temporary_dir/age-identity.txt"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)-$$"
target="$backup_dir/${BACKEND_DATABASE_DBNAME}-${timestamp}.dump.age"
encrypted_partial="$target.partial"
checksum_partial="$target.sha256.partial"

cleanup() {
  rm -f -- "$plain_dump" "$verified_dump" "$identity_file" "$encrypted_partial" "$checksum_partial"
  rmdir "$temporary_dir" 2>/dev/null || true
  rmdir "$lock_dir" 2>/dev/null || true
  unset backup_identity backup_recipient PGPASSWORD PGSSLMODE PGAPPNAME
}
trap cleanup EXIT INT TERM

for candidate in "$target" "$target.sha256" "$encrypted_partial" "$checksum_partial"; do
  [[ ! -e "$candidate" && ! -L "$candidate" ]] || {
    echo "同名备份文件已存在，拒绝覆盖：$candidate" >&2
    exit 1
  }
done

export PGPASSWORD="$BACKEND_DATABASE_PASSWORD"
export PGSSLMODE="$BACKEND_DATABASE_SSLMODE"
export PGAPPNAME="wangzhe-dev-business-reset-backup"
"$pg_dump_bin" \
  --host "$BACKEND_DATABASE_HOST" \
  --port "$BACKEND_DATABASE_PORT" \
  --username "$BACKEND_DATABASE_USER" \
  --dbname "$BACKEND_DATABASE_DBNAME" \
  --format custom \
  --no-owner \
  --file "$plain_dump"
unset PGPASSWORD PGSSLMODE PGAPPNAME
[[ -s "$plain_dump" ]] || { echo "数据库备份文件为空" >&2; exit 1; }
"$pg_restore_bin" --list "$plain_dump" >/dev/null

printf '%s\n' "$backup_identity" >"$identity_file"
chmod 600 "$identity_file"
backup_recipient="$(age-keygen -y "$identity_file")"
[[ "$backup_recipient" =~ ^age1[0-9a-z]+$ ]] || {
  echo "无法从开发备份 identity 派生收件人" >&2
  exit 1
}
age --recipient "$backup_recipient" --output "$encrypted_partial" "$plain_dump"
chmod 600 "$encrypted_partial"
rm -f -- "$plain_dump"

age --decrypt --identity "$identity_file" --output "$verified_dump" "$encrypted_partial"
[[ -s "$verified_dump" ]] || { echo "加密备份解密回读为空" >&2; exit 1; }
"$pg_restore_bin" --list "$verified_dump" >/dev/null
rm -f -- "$verified_dump"
rm -f -- "$identity_file"
unset backup_identity backup_recipient

mv "$encrypted_partial" "$target"
target_sha256="$(sha256_file "$target")"
[[ "$target_sha256" =~ ^[0-9a-f]{64}$ ]] || {
  echo "加密备份 SHA-256 不正确" >&2
  exit 1
}
printf '%s  %s\n' "$target_sha256" "$(basename "$target")" >"$checksum_partial"
chmod 600 "$checksum_partial"
mv "$checksum_partial" "$target.sha256"

echo "数据库备份完成：$target"
