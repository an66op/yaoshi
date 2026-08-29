#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

readonly EXPECTED_ENV_FILE=/etc/wangzhe/pitr-drill.env
readonly DRILL_MOUNT_ROOT=/var/lib/wangzhe-pitr-drill
readonly DRILL_ROOT="$DRILL_MOUNT_ROOT/work"
readonly DATA_DIR="$DRILL_ROOT/data"
readonly SOCKET_DIR="$DRILL_ROOT/socket"
readonly WAL_STAGE_ROOT="$DRILL_ROOT/wal-stage"
readonly DRILL_CONFIG="$DRILL_ROOT/postgresql-drill.conf"
readonly DRILL_HBA="$DRILL_ROOT/pg_hba-drill.conf"
readonly DRILL_IDENT="$DRILL_ROOT/pg_ident-drill.conf"
readonly DRILL_LOG="$DRILL_ROOT/last-postgres.log"
readonly WAL_AUDIT="$DRILL_ROOT/wal-restored.log"
readonly STATUS_FILE="$DRILL_ROOT/last-success.status"
readonly STATUS_CHECKSUM="$STATUS_FILE.sha256"
readonly STATUS_SIGNATURE="$STATUS_FILE.sig"
readonly EXPECTED_SOURCE_ROOT=/var/lib/wangzhe-pitr-source
readonly SOURCE_LOCK_FILE=/run/wangzhe-pitr-source/operation.lock

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=lib/strict-env.sh
source "$SCRIPT_DIR/lib/strict-env.sh"
# shellcheck source=lib/encrypted-backup.sh
source "$SCRIPT_DIR/lib/encrypted-backup.sh"

fail() {
  echo "$*" >&2
  exit 1
}

validate_root_owned_marker() {
  local marker="$1" marker_mode marker_owner
  [[ "$marker" == /etc/wangzhe/recovery-host ]] || fail "PITR 演练隔离标记必须是 /etc/wangzhe/recovery-host"
  [[ -f "$marker" && ! -L "$marker" ]] || fail "缺少隔离恢复主机确认标记"
  validate_no_symlink_path_components "$marker"
  marker_mode="$(strict_env_stat '%a' '%Lp' "$marker")"
  marker_owner="$(strict_env_stat '%u' '%u' "$marker")"
  [[ "$marker_owner" == 0 && "$marker_mode" =~ ^[0-7]{3,4}$ ]] || fail "隔离恢复主机标记必须由 root 所有"
  (( (8#$marker_mode & 022) == 0 )) || fail "隔离恢复主机标记不能被非 root 修改"
  grep -qx 'WANGZHE_ISOLATED_RECOVERY_HOST' "$marker" || fail "隔离恢复主机标记内容无效"
}

validate_pitr_rclone_credential_separation() {
  local wal_config="$1" status_config="$2"
  local wal_parent status_parent wal_canonical status_canonical
  local wal_file_identity status_file_identity wal_sha256 status_sha256

  [[ "$wal_config" == /etc/wangzhe/pitr-wal-read-rclone.conf ]] || fail "PITR 制品读取凭据必须使用固定路径"
  [[ "$status_config" == /etc/wangzhe/pitr-status-write-rclone.conf ]] || fail "PITR 状态发布凭据必须使用固定路径"
  [[ "$wal_config" != "$status_config" ]] || fail "PITR 制品读取与状态发布凭据路径不能相同"
  validate_rclone_config "$wal_config"
  validate_rclone_config "$status_config"

  wal_parent="$(cd -P -- "$(dirname -- "$wal_config")" && pwd -P)" || fail "无法规范化 PITR 制品读取凭据目录"
  status_parent="$(cd -P -- "$(dirname -- "$status_config")" && pwd -P)" || fail "无法规范化 PITR 状态发布凭据目录"
  wal_canonical="$wal_parent/$(basename -- "$wal_config")"
  status_canonical="$status_parent/$(basename -- "$status_config")"
  [[ "$wal_config" == "$wal_canonical" && "$status_config" == "$status_canonical" ]] || fail "PITR rclone 配置必须使用规范绝对路径"
  [[ "$wal_canonical" != "$status_canonical" ]] || fail "PITR 制品读取与状态发布凭据规范路径不能相同"
  [[ ! "$wal_config" -ef "$status_config" ]] || fail "PITR 制品读取与状态发布凭据不能是同一文件或硬链接"

  wal_file_identity="$(strict_env_stat '%d:%i' '%d:%i' "$wal_config")"
  status_file_identity="$(strict_env_stat '%d:%i' '%d:%i' "$status_config")"
  [[ -n "$wal_file_identity" && -n "$status_file_identity" && "$wal_file_identity" != "$status_file_identity" ]] || fail "PITR rclone 配置的设备/inode 身份必须不同"
  wal_sha256="$(sha256sum "$wal_config" | awk '{print $1}')"
  status_sha256="$(sha256sum "$status_config" | awk '{print $1}')"
  [[ "$wal_sha256" =~ ^[0-9a-f]{64}$ && "$status_sha256" =~ ^[0-9a-f]{64}$ && "$wal_sha256" != "$status_sha256" ]] || fail "PITR 读取与状态写入必须使用内容不同的凭据"
}

validate_fixed_drill_root() {
  validate_direct_luks_mount "$DRILL_MOUNT_ROOT" "PITR 恢复演练 LUKS 挂载点" || fail "PITR 演练盘未安全挂载"
  validate_luks_service_directory "$DRILL_ROOT" "$DRILL_MOUNT_ROOT" "PITR 恢复演练私有工作目录" || fail "PITR 演练工作目录不安全"
}

load_and_validate_environment() {
  local env_source="$1" command_path key
  [[ "$env_source" == "$EXPECTED_ENV_FILE" ]] || fail "PITR 演练只接受固定环境文件 $EXPECTED_ENV_FILE"
  validate_no_symlink_path_components "$env_source"
  load_strict_env "$env_source" '^PITR_DRILL_[A-Z0-9_]+$'

  for key in \
    PITR_DRILL_ISOLATION_MARKER PITR_DRILL_AGE_IDENTITY_FILE \
    PITR_DRILL_CLUSTER_ID PITR_DRILL_SOURCE_ROOT PITR_DRILL_SOURCE_REMOTE_DESTINATION \
    PITR_DRILL_POSTGRES_BIN_DIR PITR_DRILL_DATABASE_NAME \
    PITR_DRILL_DATABASE_USER PITR_DRILL_PORT \
    PITR_DRILL_RECOVERY_LAG_MINUTES PITR_DRILL_MIN_BASE_AGE_MINUTES \
    PITR_DRILL_START_TIMEOUT_SECONDS PITR_DRILL_MAX_SOURCE_AGE_SECONDS \
    PITR_DRILL_STATUS_SIGNING_KEY_FILE PITR_DRILL_PROVENANCE_VERIFY_KEY_FILE \
    PITR_DRILL_WAL_READ_RCLONE_CONFIG PITR_DRILL_STATUS_WRITE_RCLONE_CONFIG; do
    [[ -n "${!key:-}" ]] || fail "缺少 $key"
  done

  [[ "$PITR_DRILL_CLUSTER_ID" =~ ^[0-9]{10,30}$ ]] || fail "PITR_DRILL_CLUSTER_ID 必须是 PostgreSQL system identifier"
  [[ "$PITR_DRILL_SOURCE_ROOT" == "$EXPECTED_SOURCE_ROOT" ]] || fail "PITR 演练源目录必须精确为 $EXPECTED_SOURCE_ROOT"
  [[ -d "$PITR_DRILL_SOURCE_ROOT" && ! -L "$PITR_DRILL_SOURCE_ROOT" ]] || fail "PITR 异机同步源目录不存在"
  validate_no_symlink_path_components "$PITR_DRILL_SOURCE_ROOT"
  validate_remote_destination "$PITR_DRILL_SOURCE_REMOTE_DESTINATION"
  [[ "$(basename "${PITR_DRILL_SOURCE_REMOTE_DESTINATION#*:}")" == "$PITR_DRILL_CLUSTER_ID" ]] || fail "PITR 演练远端来源末级必须等于集群标识"

  [[ "$PITR_DRILL_POSTGRES_BIN_DIR" =~ ^/usr/lib/postgresql/[0-9]+/bin$ ]] || fail "PostgreSQL 二进制目录必须是版本化的 /usr/lib/postgresql/N/bin"
  [[ -d "$PITR_DRILL_POSTGRES_BIN_DIR" && ! -L "$PITR_DRILL_POSTGRES_BIN_DIR" ]] || fail "PostgreSQL 二进制目录无效"
  validate_no_symlink_path_components "$PITR_DRILL_POSTGRES_BIN_DIR"
  for command_path in age awk basename chmod cp date dirname find findmnt flock grep head id kill mkdir mktemp mountpoint mv openssl rm sha256sum sort stat tail tar tr wc; do
    command -v "$command_path" >/dev/null 2>&1 || fail "缺少命令：$command_path"
  done
  pkeyutl_help="$(openssl pkeyutl -help 2>&1 || true)"
  grep -q -- '-rawin' <<<"$pkeyutl_help" || fail "PITR 状态 Ed25519 签名要求 OpenSSL 3.0+（缺少 pkeyutl -rawin）"
  unset pkeyutl_help
  for command_path in pg_ctl pg_controldata pg_verifybackup postgres psql; do
    [[ -x "$PITR_DRILL_POSTGRES_BIN_DIR/$command_path" && ! -L "$PITR_DRILL_POSTGRES_BIN_DIR/$command_path" ]] || fail "缺少固定 PostgreSQL 命令：$command_path"
  done
  [[ -x "$SCRIPT_DIR/postgres-restore-wal.sh" && ! -L "$SCRIPT_DIR/postgres-restore-wal.sh" ]] || fail "缺少现有 WAL 恢复脚本"
  [[ "$SCRIPT_DIR" =~ ^/opt/wangzhe/(current|releases/[A-Za-z0-9][A-Za-z0-9._-]{0,63})/scripts$ ]] || fail "PITR 演练脚本必须从受控的 /opt/wangzhe 发布目录运行"

  validate_root_owned_marker "$PITR_DRILL_ISOLATION_MARKER"
  validate_private_file "$PITR_DRILL_AGE_IDENTITY_FILE" "PITR AGE 恢复身份文件"
  [[ "$PITR_DRILL_PROVENANCE_VERIFY_KEY_FILE" == /etc/wangzhe/pitr-provenance-ed25519-public.pem ]] || fail "PITR 来源验签公钥必须使用固定路径"
  validate_ed25519_public_key "$PITR_DRILL_PROVENANCE_VERIFY_KEY_FILE" "PITR 备份来源 Ed25519 公钥"
  [[ "$PITR_DRILL_STATUS_SIGNING_KEY_FILE" == /etc/wangzhe/pitr-restore-status-ed25519-private.pem ]] || fail "PITR 状态签名私钥必须使用固定独立路径"
  validate_private_file "$PITR_DRILL_STATUS_SIGNING_KEY_FILE" "PITR 恢复状态 Ed25519 私钥"
  openssl pkey -in "$PITR_DRILL_STATUS_SIGNING_KEY_FILE" -noout -text 2>/dev/null | grep -q '^ED25519 Private-Key:' || fail "PITR 状态签名私钥不是有效 Ed25519 私钥"
  validate_pitr_rclone_credential_separation "$PITR_DRILL_WAL_READ_RCLONE_CONFIG" "$PITR_DRILL_STATUS_WRITE_RCLONE_CONFIG"
  [[ "$PITR_DRILL_DATABASE_NAME" =~ ^[A-Za-z_][A-Za-z0-9_]{0,62}$ ]] || fail "PITR_DRILL_DATABASE_NAME 无效"
  [[ "$PITR_DRILL_DATABASE_USER" =~ ^[A-Za-z_][A-Za-z0-9_]{0,62}$ ]] || fail "PITR_DRILL_DATABASE_USER 无效"
  [[ "$PITR_DRILL_PORT" =~ ^[1-9][0-9]*$ ]] && (( PITR_DRILL_PORT >= 55432 && PITR_DRILL_PORT <= 55499 )) || fail "PITR 演练端口只允许 55432-55499"
  [[ "$PITR_DRILL_RECOVERY_LAG_MINUTES" =~ ^[1-9][0-9]*$ ]] && (( PITR_DRILL_RECOVERY_LAG_MINUTES >= 5 && PITR_DRILL_RECOVERY_LAG_MINUTES <= 1440 )) || fail "PITR_DRILL_RECOVERY_LAG_MINUTES 必须是 5-1440"
  [[ "$PITR_DRILL_MIN_BASE_AGE_MINUTES" =~ ^[1-9][0-9]*$ ]] && (( PITR_DRILL_MIN_BASE_AGE_MINUTES >= 30 && PITR_DRILL_MIN_BASE_AGE_MINUTES <= 10080 )) || fail "PITR_DRILL_MIN_BASE_AGE_MINUTES 必须是 30-10080"
  [[ "$PITR_DRILL_START_TIMEOUT_SECONDS" =~ ^[1-9][0-9]*$ ]] && (( PITR_DRILL_START_TIMEOUT_SECONDS >= 60 && PITR_DRILL_START_TIMEOUT_SECONDS <= 1800 )) || fail "PITR_DRILL_START_TIMEOUT_SECONDS 必须是 60-1800"
  [[ "$PITR_DRILL_MAX_SOURCE_AGE_SECONDS" =~ ^[1-9][0-9]*$ ]] && (( PITR_DRILL_MAX_SOURCE_AGE_SECONDS >= 3600 && PITR_DRILL_MAX_SOURCE_AGE_SECONDS <= 172800 )) || fail "PITR_DRILL_MAX_SOURCE_AGE_SECONDS 必须是 3600-172800"

  [[ -z "${PITR_DRILL_WAL_REMOTE_DESTINATION:-}" && -z "${PITR_DRILL_RCLONE_CONFIG:-}" ]] || fail "计划任务使用隔离网络，只允许预先同步到本机并校验过的加密 WAL"
}

source_status_field() {
  local file="$1" key="$2"
  awk -F= -v key="$key" '$1 == key { count++; value=substr($0, length(key) + 2) } END { if (count == 1) print value; else exit 1 }' "$file"
}

lock_and_resolve_source() {
  local lock_parent generation generation_extra generation_root status_file command_path
  local format cluster remote snapshot_sha base_count wal_count segment_count synced_epoch now_epoch
  local actual_base_count actual_wal_count actual_segment_count unexpected

  lock_parent="$(dirname "$SOURCE_LOCK_FILE")"
  [[ -d "$lock_parent" && ! -L "$lock_parent" ]] || fail "systemd 未创建 PITR 异机源锁目录"
  validate_no_symlink_path_components "$lock_parent"
  if [[ -e "$SOURCE_LOCK_FILE" || -L "$SOURCE_LOCK_FILE" ]]; then
    [[ -f "$SOURCE_LOCK_FILE" && ! -L "$SOURCE_LOCK_FILE" && "$(strict_env_stat '%h' '%l' "$SOURCE_LOCK_FILE")" == 1 ]] || fail "PITR 异机源锁文件无效"
  fi
  exec 8>>"$SOURCE_LOCK_FILE"
  flock -s -w 30 8 || fail "PITR 异机源正在被同步替换"

  [[ -f "$PITR_DRILL_SOURCE_ROOT/active-generation" && ! -L "$PITR_DRILL_SOURCE_ROOT/active-generation" ]] || fail "PITR 异机源缺少 active-generation"
  read -r generation generation_extra <"$PITR_DRILL_SOURCE_ROOT/active-generation" || fail "PITR active-generation 不可读"
  [[ "$generation" =~ ^[0-9]{8}-[0-9]{6}-[0-9]+$ && -z "${generation_extra:-}" ]] || fail "PITR active-generation 内容无效"
  generation_root="$PITR_DRILL_SOURCE_ROOT/generations/$generation"
  [[ -d "$generation_root" && ! -L "$generation_root" ]] || fail "PITR active generation 不存在"
  validate_no_symlink_path_components "$generation_root"

  PITR_DRILL_BASEBACKUP_DIR="$generation_root/base/$PITR_DRILL_CLUSTER_ID"
  PITR_DRILL_WAL_ARCHIVE_DIR="$generation_root/wal/$PITR_DRILL_CLUSTER_ID"
  for command_path in "$PITR_DRILL_BASEBACKUP_DIR" "$PITR_DRILL_WAL_ARCHIVE_DIR"; do
    [[ -d "$command_path" && ! -L "$command_path" ]] || fail "PITR generation 输入目录无效：$command_path"
    validate_no_symlink_path_components "$command_path"
  done
  unexpected="$(find "$generation_root" -xdev -mindepth 1 -maxdepth 1 ! -name base ! -name wal ! -name source.status -print -quit)"
  [[ -z "$unexpected" ]] || fail "PITR generation 包含意外顶层条目：$unexpected"
  unexpected="$(find "$generation_root" -xdev \( -type l -o \( ! -type f -a ! -type d \) \) -print -quit)"
  [[ -z "$unexpected" ]] || fail "PITR generation 包含符号链接或特殊文件：$unexpected"

  status_file="$generation_root/source.status"
  [[ -f "$status_file" && ! -L "$status_file" ]] || fail "PITR generation 缺少异机同步状态"
  format="$(source_status_field "$status_file" format_version)" || fail "PITR 同步状态缺少版本"
  cluster="$(source_status_field "$status_file" cluster_id)" || fail "PITR 同步状态缺少集群标识"
  remote="$(source_status_field "$status_file" remote_destination)" || fail "PITR 同步状态缺少远端来源"
  snapshot_sha="$(source_status_field "$status_file" remote_snapshot_sha256)" || fail "PITR 同步状态缺少远端快照摘要"
  base_count="$(source_status_field "$status_file" basebackup_count)" || fail "PITR 同步状态缺少基础备份数"
  wal_count="$(source_status_field "$status_file" wal_count)" || fail "PITR 同步状态缺少 WAL 数"
  segment_count="$(source_status_field "$status_file" wal_segment_count)" || fail "PITR 同步状态缺少 WAL 段数"
  synced_epoch="$(source_status_field "$status_file" synced_at_epoch)" || fail "PITR 同步状态缺少完成时间"
  [[ "$format" == 1 && "$cluster" == "$PITR_DRILL_CLUSTER_ID" && "$remote" == "$PITR_DRILL_SOURCE_REMOTE_DESTINATION" ]] || fail "PITR 同步状态与演练来源不一致"
  [[ "$snapshot_sha" =~ ^[0-9a-f]{64}$ && "$base_count" =~ ^[1-9][0-9]*$ && "$wal_count" =~ ^[1-9][0-9]*$ && "$segment_count" =~ ^[1-9][0-9]*$ ]] || fail "PITR 同步状态计数或摘要无效"
  [[ "$synced_epoch" =~ ^[0-9]+$ ]] || fail "PITR 同步完成时间无效"
  now_epoch="$(date +%s)"
  (( synced_epoch <= now_epoch + 300 && now_epoch - synced_epoch <= PITR_DRILL_MAX_SOURCE_AGE_SECONDS )) || fail "PITR 异机同步源太旧或位于未来"

  actual_base_count="$(find "$PITR_DRILL_BASEBACKUP_DIR" -xdev -maxdepth 1 -type f -name 'basebackup-*.tar.age' -printf '.' | wc -c | tr -d '[:space:]')"
  actual_wal_count="$(find "$PITR_DRILL_WAL_ARCHIVE_DIR" -xdev -maxdepth 1 -type f -name '*.age' -printf '.' | wc -c | tr -d '[:space:]')"
  actual_segment_count="$(find "$PITR_DRILL_WAL_ARCHIVE_DIR" -xdev -maxdepth 1 -type f -regextype posix-extended -regex '.*/[0-9A-F]{24}\.age' -printf '.' | wc -c | tr -d '[:space:]')"
  [[ "$actual_base_count" == "$base_count" && "$actual_wal_count" == "$wal_count" && "$actual_segment_count" == "$segment_count" ]] || fail "PITR generation 文件数与异机同步状态不一致"

  PITR_SOURCE_GENERATION="$generation"
  PITR_SOURCE_REMOTE="$remote"
  PITR_SOURCE_SNAPSHOT_SHA256="$snapshot_sha"
  PITR_SOURCE_SYNCED_EPOCH="$synced_epoch"
  PITR_SOURCE_BASEBACKUP_COUNT="$base_count"
  PITR_SOURCE_WAL_COUNT="$wal_count"
  PITR_SOURCE_WAL_SEGMENT_COUNT="$segment_count"
}

safe_remove_tree() {
  local target="$1"
  case "$target" in
    "$DATA_DIR"|"$SOCKET_DIR"|"$WAL_STAGE_ROOT"|"$DRILL_ROOT"/.work.*) ;;
    *) fail "拒绝清理不属于固定 PITR 演练目录的路径：$target" ;;
  esac
  [[ ! -L "$target" ]] || fail "拒绝清理符号链接：$target"
  if [[ -e "$target" ]] && mountpoint --quiet "$target"; then
    fail "拒绝清理挂载点：$target"
  fi
  [[ ! -e "$target" ]] || rm -rf -- "$target"
}

restore_one_wal() {
  local env_source="$1" wal_name="$2" restore_target="$3"
  load_and_validate_environment "$env_source"
  validate_fixed_drill_root
  lock_and_resolve_source

  [[ "$wal_name" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)$ ]] || fail "PITR drill WAL 文件名无效"
  [[ "$restore_target" != *..* ]] || fail "PostgreSQL WAL 恢复目标包含路径穿越"
  local destination_parent destination stage_dir stage_file partial
  case "$restore_target" in
    pg_wal/*) destination="$DATA_DIR/$restore_target" ;;
    "$DATA_DIR"/pg_wal/*) destination="$restore_target" ;;
    *) fail "PostgreSQL WAL 恢复目标必须位于演练实例 pg_wal" ;;
  esac
  destination_parent="$(dirname "$destination")"
  [[ -d "$destination_parent" && ! -L "$destination_parent" ]] || fail "演练 WAL 恢复目标父目录无效"
  destination_parent="$(cd "$destination_parent" && pwd -P)"
  [[ "$destination_parent" == "$DATA_DIR/pg_wal" ]] || fail "演练 WAL 恢复目标越界"
  destination="$destination_parent/$(basename "$destination")"
  [[ ! -e "$destination" && ! -L "$destination" ]] || fail "演练 WAL 恢复目标已存在"

  [[ -d "$WAL_STAGE_ROOT" && ! -L "$WAL_STAGE_ROOT" ]] || fail "WAL 暂存根目录无效"
  validate_no_symlink_path_components "$WAL_STAGE_ROOT"
  stage_dir="$WAL_STAGE_ROOT/$$"
  [[ ! -e "$stage_dir" && ! -L "$stage_dir" ]] || fail "WAL 暂存目录已存在"
  mkdir -m 0700 -- "$stage_dir"
  stage_file="$stage_dir/$wal_name"
  partial="$destination.partial.$$"
  cleanup_wal_stage() {
    rm -f -- "$stage_file" "$partial"
    rmdir -- "$stage_dir" 2>/dev/null || true
  }
  trap cleanup_wal_stage EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM

  export PITR_RESTORE_AGE_IDENTITY_FILE="$PITR_DRILL_AGE_IDENTITY_FILE"
  export PITR_RESTORE_CLUSTER_ID="$PITR_DRILL_CLUSTER_ID"
  export PITR_RESTORE_LOCAL_ARCHIVE_DIR="$PITR_DRILL_WAL_ARCHIVE_DIR"
  export PITR_RESTORE_PROVENANCE_VERIFY_KEY_FILE="$PITR_DRILL_PROVENANCE_VERIFY_KEY_FILE"
  unset PITR_RESTORE_REMOTE_DESTINATION PITR_RESTORE_RCLONE_CONFIG || true
  local encrypted_wal="$PITR_DRILL_WAL_ARCHIVE_DIR/$wal_name.age"
  validate_offsite_marker "$encrypted_wal" "$PITR_DRILL_SOURCE_REMOTE_DESTINATION" || fail "WAL 缺少精确匹配远端来源的回读凭证"
  "$SCRIPT_DIR/postgres-restore-wal.sh" --current-env "$wal_name" "$stage_file"
  [[ -s "$stage_file" && ! -L "$stage_file" ]] || fail "现有 WAL 恢复脚本未产生有效文件"
  cp -- "$stage_file" "$partial"
  chmod 0600 -- "$partial"
  mv -- "$partial" "$destination"
  partial=""
  [[ -f "$WAL_AUDIT" && ! -L "$WAL_AUDIT" && "$(strict_env_stat '%h' '%l' "$WAL_AUDIT")" == 1 ]] || fail "WAL 恢复审计文件无效"
  printf '%s\n' "$wal_name" >>"$WAL_AUDIT"
  rm -f -- "$stage_file"
  rmdir -- "$stage_dir"
  trap - EXIT INT TERM
}

validate_basebackup_tar() {
  local archive="$1" entry first_type
  tar --list --file "$archive" >/dev/null
  while IFS= read -r entry; do
    [[ -n "$entry" ]] || continue
    [[ "$entry" != /* && "$entry" != *$'\n'* && "$entry" != *$'\r'* ]] || fail "基础备份包含无效路径"
    case "/$entry/" in *'/../'*|*'/./'*) fail "基础备份包含路径穿越：$entry" ;; esac
    case "$entry" in data|data/*) ;; *) fail "基础备份包含越界路径：$entry" ;; esac
  done < <(tar --list --file "$archive")

  while IFS= read -r first_type; do
    case "${first_type:0:1}" in -|d) ;; *) fail "基础备份包含符号链接、硬链接或特殊文件" ;; esac
  done < <(tar --verbose --list --file "$archive" | awk '{print $1}')
}

select_basebackup() {
  local cutoff_epoch="$1" candidate candidate_mtime
  while IFS= read -r candidate; do
    [[ -n "$candidate" ]] || continue
    [[ "$(basename "$candidate")" =~ ^basebackup-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age$ ]] || continue
    candidate_mtime="$(strict_env_stat '%Y' '%m' "$candidate")"
    [[ "$candidate_mtime" =~ ^[0-9]+$ ]] || continue
    (( candidate_mtime <= cutoff_epoch )) || continue
    if validate_encrypted_backup_and_manifest "$candidate" && \
      validate_offsite_marker "$candidate" "$PITR_DRILL_SOURCE_REMOTE_DESTINATION" && \
      verify_backup_provenance "$candidate" pitr-basebackup "$PITR_DRILL_CLUSTER_ID" \
        "${PITR_DRILL_SOURCE_REMOTE_DESTINATION%/}/$(basename "$candidate")" "$PITR_DRILL_PROVENANCE_VERIFY_KEY_FILE"; then
      printf '%s' "$candidate"
      return 0
    fi
  done < <(find "$PITR_DRILL_BASEBACKUP_DIR" -maxdepth 1 -type f -name 'basebackup-*.tar.age' -print | LC_ALL=C sort -r)
  return 1
}

run_drill() {
  local env_source="$1"
  load_and_validate_environment "$env_source"
  [[ "$(id -un)" == postgres ]] || fail "PITR 恢复演练必须以 postgres 服务用户运行"
  validate_fixed_drill_root
  lock_and_resolve_source
  umask 077

  local lock_file="$DRILL_ROOT/.pitr-drill.lock"
  [[ ! -L "$lock_file" && ( ! -e "$lock_file" || -f "$lock_file" ) ]] || fail "PITR 演练锁文件无效"
  [[ ! -e "$lock_file" || "$(strict_env_stat '%h' '%l' "$lock_file")" == 1 ]] || fail "PITR 演练锁文件不能是硬链接"
  exec 9>"$lock_file"
  flock -w 1 9 || fail "另一个 PITR 恢复演练仍在运行"
  local residual_work
  residual_work="$(find "$DRILL_ROOT" -xdev -mindepth 1 -maxdepth 1 -name '.work.*' -print -quit)"
  [[ -z "$residual_work" ]] || fail "PITR LUKS 工作目录存在未完成解密任务遗留，拒绝覆盖或自动删除：$residual_work"

  if [[ -f "$DATA_DIR/postmaster.pid" && ! -L "$DATA_DIR/postmaster.pid" ]]; then
    local previous_pid
    previous_pid="$(head -n 1 "$DATA_DIR/postmaster.pid" 2>/dev/null || true)"
    if [[ "$previous_pid" =~ ^[0-9]+$ ]] && kill -0 "$previous_pid" 2>/dev/null; then
      fail "发现仍在运行的旧演练 PostgreSQL，拒绝覆盖"
    fi
  elif [[ -e "$DATA_DIR/postmaster.pid" || -L "$DATA_DIR/postmaster.pid" ]]; then
    fail "旧演练 postmaster.pid 无效，拒绝自动清理"
  fi

  safe_remove_tree "$DATA_DIR"
  safe_remove_tree "$SOCKET_DIR"
  safe_remove_tree "$WAL_STAGE_ROOT"
  mkdir -m 0700 -- "$SOCKET_DIR" "$WAL_STAGE_ROOT"

  local started=0 work_dir base_plain extracted_data selected_base data_mount
  local now_epoch target_epoch base_cutoff_epoch target_utc start_epoch
  local postgres_major backup_major base_checksum backup_system_identifier
  local schema_count negative_balances orphan_bets recovery_state target_reached
  local replay_lsn replay_timestamp system_identifier timeline_id completed_epoch duration_seconds
  local wal_restore_count wal_segment_restore_count first_restored_wal last_restored_wal wal_audit_checksum
  work_dir="$(mktemp -d "$DRILL_ROOT/.work.XXXXXX")"
  base_plain="$work_dir/basebackup.tar"
  extracted_data="$work_dir/data"
  start_epoch="$(date +%s)"

  cleanup_drill() {
    local rc=$?
    trap - EXIT INT TERM
    if (( started == 1 )); then
      if "$PITR_DRILL_POSTGRES_BIN_DIR/pg_ctl" --pgdata "$DATA_DIR" status >/dev/null 2>&1; then
        if ! "$PITR_DRILL_POSTGRES_BIN_DIR/pg_ctl" --pgdata "$DATA_DIR" --mode fast --wait --timeout 60 stop >/dev/null 2>&1; then
          echo "无法干净停止 PITR 演练 PostgreSQL；保留固定演练目录供人工处置" >&2
          exit 1
        fi
      fi
      started=0
    fi
    if [[ -f "$DATA_DIR/postmaster.pid" && ! -L "$DATA_DIR/postmaster.pid" ]]; then
      local cleanup_pid
      cleanup_pid="$(head -n 1 "$DATA_DIR/postmaster.pid" 2>/dev/null || true)"
      if [[ "$cleanup_pid" =~ ^[0-9]+$ ]] && kill -0 "$cleanup_pid" 2>/dev/null; then
        echo "PITR 演练进程仍存活；拒绝删除其数据目录" >&2
        exit 1
      fi
    fi
    if [[ -n "${work_dir:-}" && "$work_dir" == "$DRILL_ROOT"/.work.* && ! -L "$work_dir" ]]; then
      safe_remove_tree "$work_dir"
    fi
    if [[ -d "$DATA_DIR" && ! -L "$DATA_DIR" ]]; then
      safe_remove_tree "$DATA_DIR"
    fi
    safe_remove_tree "$SOCKET_DIR"
    safe_remove_tree "$WAL_STAGE_ROOT"
    case "${status_partial:-}" in "$STATUS_FILE".partial.*) rm -f -- "$status_partial" ;; esac
    case "${checksum_partial:-}" in "$STATUS_CHECKSUM".partial.*) rm -f -- "$checksum_partial" ;; esac
    case "${signature_partial:-}" in "$STATUS_SIGNATURE".partial.*) rm -f -- "$signature_partial" ;; esac
    exit "$rc"
  }
  trap cleanup_drill EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM

  now_epoch="$(date +%s)"
  target_epoch=$((now_epoch - PITR_DRILL_RECOVERY_LAG_MINUTES * 60))
  base_cutoff_epoch=$((now_epoch - PITR_DRILL_MIN_BASE_AGE_MINUTES * 60))
  (( target_epoch < base_cutoff_epoch )) && base_cutoff_epoch="$target_epoch"
  selected_base="$(select_basebackup "$base_cutoff_epoch")" || fail "没有完成时间早于恢复目标且校验有效的加密基础备份"
  validate_encrypted_backup_and_manifest "$selected_base" || fail "选中的基础备份校验失败"
  base_checksum="$(sha256sum "$selected_base" | awk '{print $1}')"

  age --decrypt --identity "$PITR_DRILL_AGE_IDENTITY_FILE" --output "$base_plain" "$selected_base"
  [[ -s "$base_plain" && ! -L "$base_plain" ]] || fail "解密后的基础备份无效"
  validate_basebackup_tar "$base_plain"
  tar --extract --file "$base_plain" --directory "$work_dir" --no-same-owner --no-same-permissions --numeric-owner
  rm -f -- "$base_plain"
  base_plain=""
  [[ -d "$extracted_data" && ! -L "$extracted_data" ]] || fail "基础备份缺少 data 目录"
  validate_no_symlink_path_components "$extracted_data"
  [[ -f "$extracted_data/PG_VERSION" && ! -L "$extracted_data/PG_VERSION" ]] || fail "基础备份缺少 PG_VERSION"
  [[ -f "$extracted_data/global/pg_control" && ! -L "$extracted_data/global/pg_control" ]] || fail "基础备份缺少 pg_control"
  "$PITR_DRILL_POSTGRES_BIN_DIR/pg_verifybackup" "$extracted_data"
  backup_system_identifier="$("$PITR_DRILL_POSTGRES_BIN_DIR/pg_controldata" "$extracted_data" | awk -F: '/Database system identifier/ {gsub(/[[:space:]]/, "", $2); print $2}')"
  [[ "$backup_system_identifier" == "$PITR_DRILL_CLUSTER_ID" ]] || fail "基础备份不属于配置的 PostgreSQL 集群"

  backup_major="$(tr -d '[:space:]' <"$extracted_data/PG_VERSION")"
  postgres_major="$("$PITR_DRILL_POSTGRES_BIN_DIR/postgres" --version | awk '{print $3}' | awk -F. '{print $1}')"
  [[ "$backup_major" =~ ^[0-9]+$ && "$postgres_major" == "$backup_major" ]] || fail "基础备份 PostgreSQL 主版本与演练二进制不一致"

  mv -- "$extracted_data" "$DATA_DIR"
  chmod 0700 -- "$DATA_DIR"
  data_mount="$(direct_luks_mount_for_directory "$DATA_DIR" "PITR PostgreSQL data_directory")" || fail "PITR 数据目录未落在直接 LUKS dm-crypt 文件系统"
  [[ "$data_mount" == "$DRILL_MOUNT_ROOT" ]] || fail "PITR 数据目录不在固定演练 LUKS 挂载点"
  : >"$DATA_DIR/postgresql.auto.conf"
  chmod 0600 -- "$DATA_DIR/postgresql.auto.conf"
  rm -f -- "$DATA_DIR/postmaster.pid" "$DATA_DIR/postmaster.opts" "$DATA_DIR/standby.signal" "$DATA_DIR/recovery.signal"
  : >"$DATA_DIR/recovery.signal"
  chmod 0600 -- "$DATA_DIR/recovery.signal"

  target_utc="$(date -u -d "@$target_epoch" '+%Y-%m-%d %H:%M:%S+00')"
  local fixed_output
  for fixed_output in "$DRILL_CONFIG" "$DRILL_HBA" "$DRILL_IDENT" "$DRILL_LOG" "$WAL_AUDIT"; do
    [[ ! -L "$fixed_output" && ( ! -e "$fixed_output" || -f "$fixed_output" ) ]] || fail "PITR 演练固定输出无效：$fixed_output"
    rm -f -- "$fixed_output"
  done
  printf '%s\n' \
    "data_directory = '$DATA_DIR'" \
    "hba_file = '$DRILL_HBA'" \
    "ident_file = '$DRILL_IDENT'" \
    "listen_addresses = '127.0.0.1'" \
    "port = $PITR_DRILL_PORT" \
    "unix_socket_directories = '$SOCKET_DIR'" \
    "unix_socket_permissions = 0700" \
    "ssl = off" \
    "archive_mode = off" \
    "archive_command = ''" \
    "primary_conninfo = ''" \
    "primary_slot_name = ''" \
    "shared_preload_libraries = ''" \
    "session_preload_libraries = ''" \
    "local_preload_libraries = ''" \
    "max_connections = 20" \
    "max_wal_senders = 0" \
    "max_worker_processes = 0" \
    "max_parallel_workers = 0" \
    "max_logical_replication_workers = 0" \
    "max_sync_workers_per_subscription = 0" \
    "shared_buffers = '128MB'" \
    "dynamic_shared_memory_type = mmap" \
    "huge_pages = off" \
    "autovacuum = off" \
    "jit = off" \
    "restart_after_crash = off" \
    "hot_standby = off" \
    "cluster_name = 'wangzhe-pitr-drill'" \
    "logging_collector = off" \
    "log_destination = 'stderr'" \
    "log_min_messages = notice" \
    "restore_command = '$SCRIPT_DIR/production-pitr-restore-drill.sh --restore-wal /etc/wangzhe/pitr-drill.env \"%f\" \"%p\"'" \
    "recovery_target_timeline = 'latest'" \
    "recovery_target_time = '$target_utc'" \
    "recovery_target_inclusive = on" \
    "recovery_target_action = 'promote'" >"$DRILL_CONFIG"
  printf '%s\n' \
    'local all all trust' \
    'host all all 127.0.0.1/32 reject' \
    'host all all ::1/128 reject' >"$DRILL_HBA"
  : >"$DRILL_IDENT"
  chmod 0600 -- "$DRILL_CONFIG" "$DRILL_HBA" "$DRILL_IDENT"
  : >"$DRILL_LOG"
  : >"$WAL_AUDIT"
  chmod 0600 -- "$DRILL_LOG" "$WAL_AUDIT"

  started=1
  "$PITR_DRILL_POSTGRES_BIN_DIR/pg_ctl" --pgdata "$DATA_DIR" \
    --options="-c config_file=$DRILL_CONFIG" --log "$DRILL_LOG" \
    --wait --timeout "$PITR_DRILL_START_TIMEOUT_SECONDS" start

  local psql_args
  psql_args=(--host "$SOCKET_DIR" --port "$PITR_DRILL_PORT" --username "$PITR_DRILL_DATABASE_USER" --dbname "$PITR_DRILL_DATABASE_NAME" --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1)
  recovery_state="$("$PITR_DRILL_POSTGRES_BIN_DIR/psql" "${psql_args[@]}" --command 'SELECT pg_is_in_recovery();')"
  # A time target includes commits at or before the requested instant and
  # promotes only after PostgreSQL has encountered the recovery target.  The
  # last replayed commit must therefore not be newer than the target.  A clean
  # promoted startup proves that PostgreSQL reached the configured target;
  # otherwise startup fails with "recovery ended before configured target".
  target_reached="$("$PITR_DRILL_POSTGRES_BIN_DIR/psql" "${psql_args[@]}" --command "SELECT COALESCE(pg_last_xact_replay_timestamp() <= to_timestamp($target_epoch), false);")"
  schema_count="$("$PITR_DRILL_POSTGRES_BIN_DIR/psql" "${psql_args[@]}" --command 'SELECT count(*) FROM schema_migrations;')"
  negative_balances="$("$PITR_DRILL_POSTGRES_BIN_DIR/psql" "${psql_args[@]}" --command 'SELECT count(*) FROM "user" WHERE balance_cents < 0 AND deleted_at IS NULL;')"
  orphan_bets="$("$PITR_DRILL_POSTGRES_BIN_DIR/psql" "${psql_args[@]}" --command 'SELECT count(*) FROM lottery_bets WHERE workspace_id = 0;')"
  replay_lsn="$("$PITR_DRILL_POSTGRES_BIN_DIR/psql" "${psql_args[@]}" --command "SELECT COALESCE(pg_last_wal_replay_lsn()::text, '');")"
  replay_timestamp="$("$PITR_DRILL_POSTGRES_BIN_DIR/psql" "${psql_args[@]}" --command "SELECT COALESCE(pg_last_xact_replay_timestamp()::text, '');")"
  system_identifier="$("$PITR_DRILL_POSTGRES_BIN_DIR/psql" "${psql_args[@]}" --command 'SELECT system_identifier FROM pg_control_system();')"
  timeline_id="$("$PITR_DRILL_POSTGRES_BIN_DIR/psql" "${psql_args[@]}" --command 'SELECT timeline_id FROM pg_control_checkpoint();')"
  wal_restore_count="$(wc -l <"$WAL_AUDIT" | tr -d '[:space:]')"
  wal_segment_restore_count="$(grep -Ec '^[0-9A-F]{24}$' "$WAL_AUDIT" || true)"
  first_restored_wal="$(head -n 1 "$WAL_AUDIT")"
  last_restored_wal="$(tail -n 1 "$WAL_AUDIT")"
  wal_audit_checksum="$(sha256sum "$WAL_AUDIT" | awk '{print $1}')"
  [[ "$recovery_state" == f && "$target_reached" == t ]] || fail "PITR 未到达指定时间或尚未完成提升"
  [[ "$schema_count" =~ ^[0-9]+$ && "$schema_count" -gt 0 ]] || fail "恢复库缺少有效 schema_migrations"
  [[ "$negative_balances" == 0 && "$orphan_bets" == 0 ]] || fail "PITR 业务一致性校验失败"
  [[ "$replay_lsn" =~ ^[0-9A-F]+/[0-9A-F]+$ && -n "$replay_timestamp" ]] || fail "PITR 缺少 WAL 重放证据"
  [[ "$system_identifier" == "$PITR_DRILL_CLUSTER_ID" && "$timeline_id" =~ ^[0-9]+$ ]] || fail "PITR 控制信息与目标集群不匹配"
  [[ "$wal_restore_count" =~ ^[1-9][0-9]*$ ]] || fail "PITR 未实际调用加密 WAL 恢复链路"
  [[ "$wal_segment_restore_count" =~ ^[1-9][0-9]*$ ]] || fail "PITR 未实际恢复任何归档 WAL 段"
  [[ "$first_restored_wal" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)$ ]] || fail "首个 WAL 恢复审计记录无效"
  [[ "$last_restored_wal" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)$ ]] || fail "末个 WAL 恢复审计记录无效"
  [[ "$wal_audit_checksum" =~ ^[0-9a-f]{64}$ ]] || fail "WAL 恢复审计摘要无效"

  "$PITR_DRILL_POSTGRES_BIN_DIR/pg_ctl" --pgdata "$DATA_DIR" --mode fast --wait --timeout 60 stop
  started=0
  safe_remove_tree "$DATA_DIR"
  safe_remove_tree "$SOCKET_DIR"
  safe_remove_tree "$WAL_STAGE_ROOT"
  safe_remove_tree "$work_dir"
  work_dir=""

  completed_epoch="$(date +%s)"
  duration_seconds=$((completed_epoch - start_epoch))
  local status_partial checksum_partial signature_partial status_digest status_bytes signature_size
  status_partial="$STATUS_FILE.partial.$$"
  checksum_partial="$STATUS_CHECKSUM.partial.$$"
  signature_partial="$STATUS_SIGNATURE.partial.$$"
  for candidate in "$STATUS_FILE" "$STATUS_CHECKSUM" "$STATUS_SIGNATURE"; do
    [[ ! -L "$candidate" && ( ! -e "$candidate" || -f "$candidate" ) ]] || fail "PITR 最终状态文件无效"
  done
  for candidate in "$status_partial" "$checksum_partial" "$signature_partial"; do
    [[ ! -e "$candidate" && ! -L "$candidate" ]] || fail "PITR 状态临时文件已存在"
  done
  printf 'format_version=2\npitr_completed=1\ntarget_reached=1\ncompleted_at_epoch=%s\ntarget_at_epoch=%s\ntarget_at_utc=%s\nduration_seconds=%s\ndrill_luks_mount=%s\nsource_generation=%s\nsource_remote_destination=%s\nsource_snapshot_sha256=%s\nsource_synced_at_epoch=%s\nsource_basebackup_count=%s\nsource_wal_count=%s\nsource_wal_segment_count=%s\nbasebackup_file=%s\nbasebackup_sha256=%s\npostgres_major=%s\nsystem_identifier=%s\ntimeline_id=%s\nreplay_lsn=%s\nreplay_timestamp=%s\nrestored_wal_count=%s\nrestored_wal_segment_count=%s\nfirst_restored_wal=%s\nlast_restored_wal=%s\nwal_audit_sha256=%s\nschema_migrations=%s\nnegative_balances=%s\norphan_bets=%s\n' \
    "$completed_epoch" "$target_epoch" "$target_utc" "$duration_seconds" \
    "$DRILL_MOUNT_ROOT" \
    "$PITR_SOURCE_GENERATION" "$PITR_SOURCE_REMOTE" "$PITR_SOURCE_SNAPSHOT_SHA256" "$PITR_SOURCE_SYNCED_EPOCH" \
    "$PITR_SOURCE_BASEBACKUP_COUNT" "$PITR_SOURCE_WAL_COUNT" "$PITR_SOURCE_WAL_SEGMENT_COUNT" \
    "$(basename "$selected_base")" "$base_checksum" \
    "$postgres_major" "$system_identifier" "$timeline_id" "$replay_lsn" "$replay_timestamp" \
    "$wal_restore_count" "$wal_segment_restore_count" "$first_restored_wal" "$last_restored_wal" "$wal_audit_checksum" \
    "$schema_count" "$negative_balances" "$orphan_bets" >"$status_partial"
  chmod 0600 -- "$status_partial"
  status_bytes="$(strict_env_stat '%s' '%z' "$status_partial")"
  [[ "$status_bytes" =~ ^[0-9]+$ && "$status_bytes" -ge 1 && "$status_bytes" -le 16384 ]] || fail "PITR 状态大小无效"
  status_digest="$(sha256sum "$status_partial" | awk '{print $1}')"
  printf '%s  %s\n' "$status_digest" "$(basename "$STATUS_FILE")" >"$checksum_partial"
  chmod 0600 -- "$checksum_partial"
  openssl pkeyutl -sign -rawin -inkey "$PITR_DRILL_STATUS_SIGNING_KEY_FILE" -in "$status_partial" -out "$signature_partial"
  signature_size="$(strict_env_stat '%s' '%z' "$signature_partial")"
  [[ "$signature_size" == 64 ]] || fail "PITR 状态 Ed25519 签名长度无效"
  openssl pkeyutl -verify -rawin -inkey "$PITR_DRILL_STATUS_SIGNING_KEY_FILE" -in "$status_partial" -sigfile "$signature_partial" >/dev/null || fail "PITR 状态本地签名自检失败"
  chmod 0600 -- "$signature_partial"
  mv -- "$signature_partial" "$STATUS_SIGNATURE"
  mv -- "$checksum_partial" "$STATUS_CHECKSUM"
  mv -- "$status_partial" "$STATUS_FILE"
  (cd "$DRILL_ROOT" && sha256sum --check "$(basename "$STATUS_CHECKSUM")") >/dev/null
  trap - EXIT INT TERM
  echo "隔离 PostgreSQL PITR 演练通过：target=$target_utc base=$(basename "$selected_base")"
}

case "${1:-}" in
  --restore-wal)
    [[ $# == 4 ]] || fail "内部 WAL 恢复参数无效"
    restore_one_wal "$2" "$3" "$4"
    ;;
  '')
    run_drill "$EXPECTED_ENV_FILE"
    ;;
  *)
    [[ $# == 1 ]] || fail "PITR 演练参数无效"
    run_drill "$1"
    ;;
esac
