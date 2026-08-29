#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=lib/strict-env.sh
source "$ROOT_DIR/scripts/lib/strict-env.sh"
# shellcheck source=lib/encrypted-backup.sh
source "$ROOT_DIR/scripts/lib/encrypted-backup.sh"
fixture_dir="$(mktemp -d)"
fixture_dir="$(cd -P "$fixture_dir" && pwd -P)"
cleanup_fixture() {
  status=$?
  trap - EXIT INT TERM
  rm -rf -- "$fixture_dir"
  exit "$status"
}
trap cleanup_fixture EXIT INT TERM
command -v openssl >/dev/null 2>&1 || { echo "缺少 OpenSSL，无法检查恢复状态签名链" >&2; exit 1; }

for script in \
  postgres-backup.sh upload-backup.sh postgres-archive-wal.sh postgres-base-backup.sh \
  postgres-restore-wal.sh production-restore-drill.sh pitr-recovery-source-sync.sh production-pitr-restore-drill.sh \
  publish-pitr-drill-status.sh production-unit-failure-alert.sh redis-production-check.sh \
  redis-aof-restart-integration-test.sh production-monitor.sh production-backup-integrity.sh; do
  bash -n "$ROOT_DIR/scripts/$script"
done

injection_marker="$fixture_dir/injection-must-not-run"
ops_env="$fixture_dir/ops.env"
printf '%s\n' \
  'BACKUP_REQUIRE_OFFSITE=0' \
  'BACKUP_AGE_RECIPIENT=$(touch injection-must-not-run)' >"$ops_env"
chmod 600 "$ops_env"
(
  cd "$fixture_dir"
  load_strict_env "$ops_env" '^BACKUP_[A-Z0-9_]+$'
  [[ "$BACKUP_AGE_RECIPIENT" == '$(touch injection-must-not-run)' ]]
)
[[ ! -e "$injection_marker" ]] || { echo "严格环境解析器执行了命令替换" >&2; exit 1; }
printf '%s\n' 'OTHER_SECRET=not-allowed' >"$ops_env"
chmod 600 "$ops_env"
if load_strict_env "$ops_env" '^BACKUP_[A-Z0-9_]+$' >/dev/null 2>&1; then
  echo "严格环境解析器接受了越界变量" >&2
  exit 1
fi
if validate_remote_destination 'remote:../../etc' >/dev/null 2>&1; then
  echo "异机目的地接受了路径穿越" >&2
  exit 1
fi
if validate_remote_destination 'remote:path with space' >/dev/null 2>&1; then
  echo "异机目的地接受了空白字符" >&2
  exit 1
fi
if validate_age_recipient 'age1CHANGE_ME_PUBLIC_RECIPIENT' >/dev/null 2>&1; then
  echo "AGE 公钥接受了示例值" >&2
  exit 1
fi

# Linux mount/sysfs probes are wrapped so this policy test remains executable
# on macOS while still exercising every fail-closed decision.
(
  mock_mount="$fixture_dir/mock-direct-luks"
  mock_work="$mock_mount/work"
  mkdir -p "$mock_work"
  mock_dm_uuid=CRYPT-LUKS2-test
  mock_options=rw,nodev,nosuid,noexec
  mock_root_owner=0
  mock_work_mode=700
  direct_luks_findmnt_record() {
    case "$1" in
      mountpoint) printf '/dev/mapper/mock-recovery ext4 253:9 %s\n' "$mock_options" ;;
      target) printf '%s\n' "$mock_mount" ;;
      *) return 1 ;;
    esac
  }
  direct_luks_dm_uuid() { printf '%s\n' "$mock_dm_uuid"; }
  direct_luks_stat() {
    case "$1:$2" in
      owner:"$mock_mount") printf '%s\n' "$mock_root_owner" ;;
      owner:"$mock_work") printf '%s\n' "${EUID:-$(id -u)}" ;;
      mode:"$mock_mount") printf '750\n' ;;
      mode:"$mock_work") printf '%s\n' "$mock_work_mode" ;;
      device:*) printf '4242\n' ;;
      *) return 1 ;;
    esac
  }
  validate_direct_luks_mount "$mock_mount" "模拟恢复盘"
  validate_luks_service_directory "$mock_work" "$mock_mount" "模拟恢复工作目录"
  mock_dm_uuid=DM-LVM-test
  if validate_direct_luks_mount "$mock_mount" "模拟恢复盘" >/dev/null 2>&1; then
    echo "直接 LUKS 校验接受了非 CRYPT-LUKS dm 设备" >&2
    exit 1
  fi
  mock_dm_uuid=CRYPT-LUKS2-test
  mock_options=rw,nodev,nosuid
  if validate_direct_luks_mount "$mock_mount" "模拟恢复盘" >/dev/null 2>&1; then
    echo "直接 LUKS 校验接受了缺少 noexec 的挂载" >&2
    exit 1
  fi
  mock_options=rw,nodev,nosuid,noexec
  mock_root_owner=1000
  if validate_direct_luks_mount "$mock_mount" "模拟恢复盘" >/dev/null 2>&1; then
    echo "直接 LUKS 校验接受了非 root 所有的挂载根目录" >&2
    exit 1
  fi
  mock_root_owner=0
  mock_work_mode=750
  if validate_luks_service_directory "$mock_work" "$mock_mount" "模拟恢复工作目录" >/dev/null 2>&1; then
    echo "直接 LUKS 校验接受了非 0700 的服务工作目录" >&2
    exit 1
  fi
)

fake_bin="$fixture_dir/fake-bin"
mkdir "$fake_bin"
cat >"$fake_bin/redis-cli" <<'EOF'
#!/usr/bin/env bash
if [[ -z "${REDISCLI_AUTH:-}" ]]; then
  echo 'NOAUTH Authentication required.' >&2
  exit 1
fi
joined=" $* "
case "$joined" in
  *' PING '*) echo PONG ;;
  *' ACL WHOAMI '*) echo "${REDIS_USERNAME:-wangzhe-monitor}" ;;
  *' ACL GETUSER '*) printf 'flags\non\npasswords\naaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n' ;;
  *' ACL LIST '*)
    printf '%s\n' 'user default off sanitize-payload resetchannels -@all'
    printf '%s\n' "${FAKE_REDIS_APP_ACL:-user wangzhe-app on sanitize-payload #aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa ~wangzhe-production:* resetchannels &wangzhe-production:* -@all +@scripting -script +ping +hello +select +get +set +getdel +del +incr +pexpire +pttl +publish +subscribe +unsubscribe +xadd +xread +xtrim +xrevrange +zadd +zrem +zremrangebyscore +zcard +time}"
    printf '%s\n' 'user wangzhe-monitor on #bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb -@all +ping +info +config|get +acl|whoami +acl|list +acl|getuser'
    [[ -z "${FAKE_REDIS_EXTRA_ACL:-}" ]] || printf '%s\n' "$FAKE_REDIS_EXTRA_ACL"
    ;;
  *' INFO server '*) printf 'redis_version:%s\r\n' "${FAKE_REDIS_VERSION:-6.2.0}" ;;
  *' INFO persistence '*) printf 'aof_enabled:1\r\naof_last_write_status:ok\r\n' ;;
  *' INFO memory '*) printf 'maxmemory_policy:noeviction\r\n' ;;
  *' CONFIG GET appendfsync '*) printf 'appendfsync\neverysec\n' ;;
  *' CONFIG GET protected-mode '*) printf 'protected-mode\nyes\n' ;;
  *' CONFIG GET acl-pubsub-default '*) printf 'acl-pubsub-default\nresetchannels\n' ;;
  *) exit 1 ;;
esac
EOF
chmod 755 "$fake_bin/redis-cli"
redis_test_env=(
  REDIS_HOST=127.0.0.1 REDIS_PORT=6379
  REDIS_USERNAME=wangzhe-monitor
  REDIS_PASSWORD=redis-test-password-longer-than-24
  REDIS_EXPECTED_APP_USERNAME=wangzhe-app REDIS_EXPECTED_APP_PREFIX=wangzhe-production
  REDIS_TLS=false REDIS_CA_FILE=
)
env -i PATH="$fake_bin:$PATH" "${redis_test_env[@]}" \
  bash "$ROOT_DIR/scripts/redis-production-check.sh" --current-env >/dev/null
if env -i PATH="$fake_bin:$PATH" FAKE_REDIS_VERSION=6.0.20 "${redis_test_env[@]}" \
  bash "$ROOT_DIR/scripts/redis-production-check.sh" --current-env >/dev/null 2>&1; then
  echo "Redis 6.0 被生产检查错误接受" >&2
  exit 1
fi
good_app_acl='user wangzhe-app on sanitize-payload #aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa ~wangzhe-production:* resetchannels &wangzhe-production:* -@all +@scripting -script +ping +hello +select +get +set +getdel +del +incr +pexpire +pttl +publish +subscribe +unsubscribe +xadd +xread +xtrim +xrevrange +zadd +zrem +zremrangebyscore +zcard +time'
good_app_acl_redis7='user wangzhe-app on sanitize-payload #aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa ~wangzhe-production:* resetchannels &wangzhe-production:* -@all +ping +hello +select +get +set +getdel +del +incr +pexpire +pttl +eval +evalsha +publish +subscribe +unsubscribe +xadd +xread +xtrim +xrevrange +zadd +zrem +zremrangebyscore +zcard +time'
env -i PATH="$fake_bin:$PATH" FAKE_REDIS_VERSION=7.4.11 FAKE_REDIS_APP_ACL="$good_app_acl_redis7" "${redis_test_env[@]}" \
  bash "$ROOT_DIR/scripts/redis-production-check.sh" --current-env >/dev/null
bad_acl_variants=(
  "${good_app_acl/~wangzhe-production:\*/~*}"
  "${good_app_acl/&wangzhe-production:\*/&*}"
  "${good_app_acl/-@all/+@all}"
  "$good_app_acl +@admin"
  "$good_app_acl +config"
  "$good_app_acl +acl"
  "$good_app_acl +flushall"
)
for bad_acl in "${bad_acl_variants[@]}"; do
  if env -i PATH="$fake_bin:$PATH" FAKE_REDIS_APP_ACL="$bad_acl" "${redis_test_env[@]}" \
    bash "$ROOT_DIR/scripts/redis-production-check.sh" --current-env >/dev/null 2>&1; then
    echo "Redis ACL 漂移被生产检查错误接受" >&2
    exit 1
  fi
done
if env -i PATH="$fake_bin:$PATH" FAKE_REDIS_EXTRA_ACL='user unexpected on nopass ~* &* +@all' "${redis_test_env[@]}" \
  bash "$ROOT_DIR/scripts/redis-production-check.sh" --current-env >/dev/null 2>&1; then
  echo "Redis 额外 ACL 用户被生产检查错误接受" >&2
  exit 1
fi

system_test_workflow="$ROOT_DIR/.github/workflows/system-test.yml"
redis_aof_restart="$ROOT_DIR/scripts/redis-aof-restart-integration-test.sh"
postgres_image='postgres:16.15-alpine3.24@sha256:ab5c955e9e57ae9879d4411ab49a912be9d162455676f7bf56e951b11ac73785'
redis_image='redis:7.4.11-alpine3.21@sha256:ff02b58f971e7d7d156a1267e283fcbbeee91773b6aa36c49dac28ecfe28eadf'
grep -Fq "image: $postgres_image" "$system_test_workflow"
grep -Fq "image: $redis_image" "$system_test_workflow"
grep -Fq "readonly redis_image=\"$redis_image\"" "$redis_aof_restart"
grep -Fq -- "--command 'SHOW server_version'" "$system_test_workflow"
grep -Fq '[[ "${BASH_REMATCH[1]}" == "16" && "${BASH_REMATCH[2]}" == "15" ]]' "$system_test_workflow"

redis_config="$ROOT_DIR/deploy/redis/redis.conf.example"
redis_acl="$ROOT_DIR/deploy/redis/wangzhe-users.acl.example"
rg -q '^bind 127\.0\.0\.1 ::1$' "$redis_config"
rg -q '^protected-mode yes$' "$redis_config"
rg -q '^appendonly yes$' "$redis_config"
rg -q '^appendfsync everysec$' "$redis_config"
rg -q '^aof-load-truncated no$' "$redis_config"
rg -q '^maxmemory-policy noeviction$' "$redis_config"
rg -q '^acl-pubsub-default resetchannels$' "$redis_config"
rg -q '^user default reset off$' "$redis_acl"
app_acl_line="$(rg '^user wangzhe-app ' "$redis_acl")"
monitor_acl_line="$(rg '^user wangzhe-monitor ' "$redis_acl")"
for denied_command in flushall flushdb swapdb migrate; do
  ! rg -q -- "\+$denied_command([[:space:]]|$)" "$redis_acl" || {
    echo "Redis ACL 错误授予了 $denied_command" >&2
    exit 1
  }
done
grep -Fq '~wangzhe-production:*' <<<"$app_acl_line"
grep -Fq '&wangzhe-production:*' <<<"$app_acl_line"
grep -Fq ' reset ' <<<"$app_acl_line"
grep -Fq ' -@all ' <<<"$app_acl_line"
grep -Fq '+publish' <<<"$app_acl_line"
grep -Fq '+config|get' <<<"$monitor_acl_line"
grep -Fq '+acl|whoami' <<<"$monitor_acl_line"
grep -Fq '+acl|list' <<<"$monitor_acl_line"
grep -Fq '+acl|getuser' <<<"$monitor_acl_line"
grep -Fq ' reset ' <<<"$monitor_acl_line"
grep -Fq ' -@all ' <<<"$monitor_acl_line"
[[ "$monitor_acl_line" != *'~'* && "$monitor_acl_line" != *'&'* ]] || {
  echo "Redis 监控用户不应拥有键或频道访问权限" >&2
  exit 1
}
! grep -Eq '(^|[[:space:]])~\*([[:space:]]|$)|(^|[[:space:]])&\*([[:space:]]|$)|\+@|\+(config|acl|flushall|flushdb)([[:space:]]|$)' <<<"$app_acl_line"
grep -Fq 'REDIS_EXPECTED_APP_USERNAME=wangzhe-app' "$ROOT_DIR/deploy/env/redis-check.env.example"
grep -Fq 'REDIS_EXPECTED_APP_PREFIX=wangzhe-production' "$ROOT_DIR/deploy/env/redis-check.env.example"
! rg -q 'REDIS_(EXPECTED_)?APP_PASSWORD|BACKEND_REDIS_PASSWORD' "$ROOT_DIR/deploy/env/redis-check.env.example"
rg -q 'ACL GETUSER.*REDIS_EXPECTED_APP_USERNAME' "$ROOT_DIR/scripts/redis-production-check.sh"
rg -q 'ACL LIST' "$ROOT_DIR/scripts/redis-production-check.sh"
rg -q 'load_strict_env.*REDIS_' "$ROOT_DIR/scripts/production-readiness.sh"
grep -Fq '[[ "$redis_expected_identity" == "$BACKEND_REDIS_USERNAME:$BACKEND_REDIS_PREFIX" ]]' "$ROOT_DIR/scripts/production-readiness.sh"

database_backup="$ROOT_DIR/scripts/postgres-backup.sh"
upload_backup="$ROOT_DIR/scripts/upload-backup.sh"
rg -q '\.dump\.age' "$database_backup"
rg -q 'encrypt_backup_file' "$database_backup"
rg -q 'sync_backup_offsite.*BACKUP_RCLONE_CONFIG' "$database_backup"
rg -q 'sha256sum --check' "$upload_backup"
rg -q 'archive_manifest=.*verify_dir' "$upload_backup"
rg -q 'cmp -s .*manifest_partial.*archive_manifest' "$upload_backup"
upload_extract_line="$(rg -n 'tar --extract --file \"\$plaintext_partial\"' "$upload_backup" | cut -d: -f1)"
upload_archive_manifest_line="$(rg -n '\) >\"\$archive_manifest\"' "$upload_backup" | cut -d: -f1)"
upload_exact_compare_line="$(rg -n 'cmp -s .*manifest_partial.*archive_manifest' "$upload_backup" | cut -d: -f1)"
upload_verify_line="$(rg -n 'sha256sum --check --strict' "$upload_backup" | cut -d: -f1)"
upload_encrypt_line="$(rg -n 'encrypt_backup_file' "$upload_backup" | tail -n1 | cut -d: -f1)"
[[ "$upload_extract_line" -lt "$upload_archive_manifest_line" && \
   "$upload_archive_manifest_line" -lt "$upload_exact_compare_line" && \
   "$upload_exact_compare_line" -lt "$upload_verify_line" && \
   "$upload_verify_line" -lt "$upload_encrypt_line" ]] || {
  echo "上传备份没有在加密发布前解包并精确核对完整文件清单" >&2
  exit 1
}
rg -q 'BACKUP_REQUIRE_OFFSITE' "$database_backup" "$upload_backup"
rg -Fq '[[ "$BACKUP_UPLOAD_SOURCE_DIR" == /var/lib/wangzhe/uploads ]]' "$upload_backup"
rg -q 'validate_no_symlink_path_components.*BACKUP_UPLOAD_SOURCE_DIR' "$upload_backup"
rg -q 'validate_encrypted_work_directory.*wangzhe-backup-work/database' "$database_backup"
rg -q 'validate_encrypted_work_directory.*wangzhe-backup-work/uploads' "$upload_backup"
rg -q 'validate_encrypted_work_directory.*wangzhe-backup-work/pitr' "$ROOT_DIR/scripts/postgres-base-backup.sh"
rg -q 'CRYPT-LUKS1.*CRYPT-LUKS2' "$ROOT_DIR/scripts/lib/encrypted-backup.sh"
rg -q 'findmnt --mountpoint.*mount_root' "$ROOT_DIR/scripts/lib/encrypted-backup.sh"
rg -q 'validate_direct_luks_mount' "$ROOT_DIR/scripts/lib/encrypted-backup.sh"
rg -q '/sys/dev/block/\$device_number/dm/uuid' "$ROOT_DIR/scripts/lib/encrypted-backup.sh"
rg -q 'root_owner.*== 0' "$ROOT_DIR/scripts/lib/encrypted-backup.sh"
rg -Fq 'for required_option in nodev nosuid noexec' "$ROOT_DIR/scripts/lib/encrypted-backup.sh"
rg -q '存在上次任务遗留，拒绝覆盖或自动删除' "$ROOT_DIR/scripts/lib/encrypted-backup.sh"
rg -q 'getent group.*monitor_group' "$ROOT_DIR/scripts/lib/encrypted-backup.sh"
rg -q 'local monitor_group=wangzhe-monitor' "$ROOT_DIR/scripts/lib/encrypted-backup.sh"
! rg -q 'BACKUP_MONITOR_GROUP|PITR_MONITOR_GROUP' "$ROOT_DIR/scripts" "$ROOT_DIR/deploy/env"
rg -q 'parent_mode_value.*02000.*0022' "$ROOT_DIR/scripts/lib/encrypted-backup.sh"
rg -q 'validate_backup_monitor_directory.*BACKUP_DIR' "$database_backup"
rg -q 'validate_backup_monitor_directory.*UPLOAD_BACKUP_DIR' "$upload_backup"
rg -q 'validate_backup_monitor_directory.*PITR_BASEBACKUP_DIR' "$ROOT_DIR/scripts/postgres-base-backup.sh"
backup_integrity="$ROOT_DIR/scripts/production-backup-integrity.sh"
rg -Fq "load_strict_env \"\$ENV_SOURCE\" '^MONITOR_[A-Z0-9_]+\$'" "$backup_integrity"
rg -Fq '[[ "$MONITOR_DATABASE_BACKUP_DIR" == /var/backups/wangzhe/database ]]' "$backup_integrity"
rg -Fq '[[ "$MONITOR_UPLOAD_BACKUP_DIR" == /var/backups/wangzhe/uploads ]]' "$backup_integrity"
rg -q 'validate_no_symlink_path_components.*directory' "$backup_integrity"
rg -Fq 'local_digest="$(sha256sum "$target" | awk' "$backup_integrity"
rg -Fq 'run_rclone hashsum sha256 "$remote_target" --download' "$backup_integrity"
rg -Fq 'verify_remote_evidence "$remote_target.sha256"' "$backup_integrity"
rg -Fq 'verify_remote_evidence "$remote_target.source.sha256"' "$backup_integrity"
rg -Fq 'MONITOR_BACKUP_INTEGRITY_STATUS_FILE' "$backup_integrity" "$ROOT_DIR/deploy/env/monitor.env.example"
rg -Fq "printf 'v2 %s %s %s %s %s %s %s %s" "$backup_integrity"
for backup_kind in database uploads basebackup; do
  rg -q "select_latest_published_artifact .* ${backup_kind} " "$backup_integrity"
done
rg -q 'find .*MONITOR_PITR_WAL_DIR.*-name '\''\*\.age' "$backup_integrity"
rg -q 'cmp -s .*expected_remote_files.*actual_remote_files' "$backup_integrity"
rg -q 'wal_inventory_sha256=.*sha256sum' "$backup_integrity"
rg -q 'verify_ciphertext_and_evidence.*pitr-wal' "$backup_integrity"
rg -q 'remote_target\.provenance\.sig' "$backup_integrity"
! rg -q 'run_rclone (copy|copyto|sync|delete|purge|move|moveto)([[:space:]]|$)' "$backup_integrity"
rg -q 'production-backup-integrity\.sh' "$ROOT_DIR/Makefile" "$ROOT_DIR/scripts/production-deploy.sh" "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
deploy_script="$ROOT_DIR/scripts/production-deploy.sh"
rg -Fq 'BACKUP_INTEGRITY_ENV=/etc/wangzhe/monitor.env' "$deploy_script"
rg -Fq 'timeout 12h runuser --user wangzhe-monitor --' "$deploy_script"
rg -Fq '"$TRUSTED_SCRIPT_DIR/production-backup-integrity.sh" "$BACKUP_INTEGRITY_ENV"' "$deploy_script"
deploy_upload_backup_line="$(rg -n '"\$TRUSTED_SCRIPT_DIR/upload-backup\.sh"' "$deploy_script" | tail -n 1 | cut -d: -f1)"
deploy_remote_gate_line="$(rg -n '"\$TRUSTED_SCRIPT_DIR/production-backup-integrity\.sh" "\$BACKUP_INTEGRITY_ENV"' "$deploy_script" | tail -n 1 | cut -d: -f1)"
deploy_release_mutation_line="$(rg -n '^install -d -o root -g root -m 0755 /opt/wangzhe ' "$deploy_script" | cut -d: -f1)"
deploy_switch_line="$(rg -n '^mv -Tf "\$link_tmp" "\$CURRENT_LINK"$' "$deploy_script" | cut -d: -f1)"
[[ -n "$deploy_upload_backup_line" && -n "$deploy_remote_gate_line" && -n "$deploy_release_mutation_line" && -n "$deploy_switch_line" && \
   "$deploy_upload_backup_line" -lt "$deploy_remote_gate_line" && \
   "$deploy_remote_gate_line" -lt "$deploy_release_mutation_line" && \
   "$deploy_remote_gate_line" -lt "$deploy_switch_line" ]] || {
  echo "发布流程没有在新备份完成后、安装或切换版本前执行实时远端四类制品门禁" >&2
  exit 1
}
! rg -q 'production-backup-integrity\.sh.*\|\|[[:space:]]*true' "$deploy_script"
for unit_path in \
  'wangzhe-backup.service:/var/lib/wangzhe-backup-work/database' \
  'wangzhe-upload-backup.service:/var/lib/wangzhe-backup-work/uploads' \
  'wangzhe-base-backup.service:/var/lib/wangzhe-backup-work/pitr'; do
  unit="${unit_path%%:*}"
  work_path="${unit_path#*:}"
  rg -Fq "RequiresMountsFor=$work_path" "$ROOT_DIR/deploy/systemd/$unit"
  rg -q "^ReadWritePaths=.*${work_path}$" "$ROOT_DIR/deploy/systemd/$unit"
done

rg -q 'archive_mode = on' "$ROOT_DIR/deploy/postgresql/wangzhe-pitr.conf.example"
rg -q 'archive_command = ' "$ROOT_DIR/deploy/postgresql/wangzhe-pitr.conf.example"
rg -q '\[0-9A-F\].*\.history' "$ROOT_DIR/scripts/postgres-archive-wal.sh"
rg -q 'PITR_CLUSTER_ID' "$ROOT_DIR/scripts/postgres-archive-wal.sh" "$ROOT_DIR/scripts/postgres-base-backup.sh"
rg -q 'PITR_CLUSTER_ID_FILE' "$ROOT_DIR/scripts/postgres-archive-wal.sh" "$ROOT_DIR/scripts/postgres-base-backup.sh"
rg -q 'pg_controldata.*work_dir/data' "$ROOT_DIR/scripts/postgres-base-backup.sh"
rg -q 'source\.sha256' "$ROOT_DIR/scripts/postgres-archive-wal.sh" "$ROOT_DIR/scripts/postgres-restore-wal.sh"
rg -q 'pg_verifybackup' "$ROOT_DIR/scripts/postgres-base-backup.sh"
logical_drill="$ROOT_DIR/scripts/production-restore-drill.sh"
logical_drill_env="$ROOT_DIR/deploy/env/restore-drill.env.example"
logical_drill_unit="$ROOT_DIR/deploy/systemd/wangzhe-restore-drill.service"
rg -q 'WANGZHE_ISOLATED_RECOVERY_HOST' "$logical_drill"
rg -q 'schema_count.*-gt 0' "$logical_drill"
rg -q 'sha256sum --check' "$logical_drill"
for source_key in RESTORE_DRILL_DATABASE_REMOTE_SOURCE RESTORE_DRILL_UPLOAD_REMOTE_SOURCE; do
  rg -Fq "$source_key" "$logical_drill" "$logical_drill_env"
done
rg -Fq 'RESTORE_DRILL_SOURCE_DATABASE_NAME=wangzhe' "$logical_drill_env"
rg -Fq '"${RESTORE_DRILL_SOURCE_DATABASE_NAME}-*.dump.age"' "$logical_drill"
rg -q 'database_name_regex=.*RESTORE_DRILL_SOURCE_DATABASE_NAME.*dump' "$logical_drill"
rg -Fq 'run_source_rclone lsf "$remote_source" --files-only --max-depth 1 --format p --include "$include_pattern"' "$logical_drill"
rg -Fq "'uploads-*.tar.age'" "$logical_drill"
rg -Fq 'run_source_rclone copyto "$database_offsite_source" "$database_backup" --no-traverse' "$logical_drill"
rg -Fq 'run_source_rclone copyto "$database_offsite_source.sha256" "$database_backup.sha256" --no-traverse' "$logical_drill"
rg -Fq 'run_source_rclone copyto "$upload_offsite_source" "$upload_backup" --no-traverse' "$logical_drill"
rg -Fq 'run_source_rclone copyto "$upload_offsite_source.sha256" "$upload_backup.sha256" --no-traverse' "$logical_drill"
rg -q 'validate_encrypted_backup_and_manifest.*database_backup' "$logical_drill"
rg -q 'validate_encrypted_backup_and_manifest.*upload_backup' "$logical_drill"
rg -q 'verify_backup_provenance.*database_backup.*database' "$logical_drill"
rg -q 'verify_backup_provenance.*upload_backup.*uploads' "$logical_drill"
rg -q 'tar --verbose --list.*upload_plain' "$logical_drill"
rg -q 'duplicate_archive_path=.*uniq -d' "$logical_drill"
rg -q '上传备份包含多个清单' "$logical_drill"
rg -Fq 'RESTORE_DRILL_UPLOAD_TARGET_DIR" == /var/lib/wangzhe-restore/work/uploads' "$logical_drill"
rg -Fq 'RESTORE_DRILL_STATUS_FILE" == /var/lib/wangzhe-restore/work/last-success.status' "$logical_drill"
rg -q 'validate_direct_luks_mount.*restore_mount_root' "$logical_drill"
rg -q 'SHOW data_directory' "$logical_drill"
rg -q 'direct_luks_mount_for_directory.*postgres_data_directory' "$logical_drill"
rg -q 'direct_luks_mount_for_directory.*RESTORE_DRILL_UPLOAD_TARGET_DIR' "$logical_drill"
rg -q 'database_data_luks_mount=%s' "$logical_drill"
rg -q 'upload_target_luks_mount=%s' "$logical_drill"
rg -Fq 'status_schema=wangzhe.restore-drill.v2' "$logical_drill"
rg -q '逻辑恢复 LUKS 工作目录存在未完成任务遗留' "$logical_drill"
rg -Fq 'RequiresMountsFor=/var/lib/wangzhe-restore' "$logical_drill_unit"
rg -Fq 'ReadWritePaths=/var/lib/wangzhe-restore/work' "$logical_drill_unit"
rg -Fq 'RESTORE_DRILL_UPLOAD_TARGET_DIR=/var/lib/wangzhe-restore/work/uploads' "$logical_drill_env"
logical_mount_check_line="$(rg -n 'validate_direct_luks_mount.*restore_mount_root' "$logical_drill" | cut -d: -f1)"
logical_database_decrypt_line="$(rg -n 'age --decrypt.*database_plain.*database_backup' "$logical_drill" | cut -d: -f1)"
logical_drop_line="$(rg -n '^dropdb ' "$logical_drill" | cut -d: -f1)"
logical_pgdata_check_line="$(rg -n 'direct_luks_mount_for_directory.*postgres_data_directory' "$logical_drill" | cut -d: -f1)"
[[ "$logical_mount_check_line" -lt "$logical_database_decrypt_line" && "$logical_pgdata_check_line" -lt "$logical_drop_line" ]] || {
  echo "逻辑恢复未在解密/替换数据库前验证 LUKS 工作盘和 PostgreSQL data_directory" >&2
  exit 1
}
database_verify_line="$(rg -n 'validate_encrypted_backup_and_manifest.*database_backup' "$logical_drill" | cut -d: -f1)"
database_decrypt_line="$(rg -n 'age --decrypt.*database_plain.*database_backup' "$logical_drill" | cut -d: -f1)"
upload_verify_line="$(rg -n 'validate_encrypted_backup_and_manifest.*upload_backup' "$logical_drill" | cut -d: -f1)"
upload_decrypt_line="$(rg -n 'age --decrypt.*upload_plain.*upload_backup' "$logical_drill" | cut -d: -f1)"
[[ "$database_verify_line" -lt "$database_decrypt_line" && "$upload_verify_line" -lt "$upload_decrypt_line" ]] || { echo "逻辑恢复在完整 SHA-256 校验前解密了异机制品" >&2; exit 1; }
! rg -q 'RESTORE_DRILL_(DATABASE|UPLOAD)_BACKUP_DIR|validate_offsite_marker' "$logical_drill" "$logical_drill_env"
rg -q 'database_offsite_source' "$logical_drill" "$ROOT_DIR/scripts/production-monitor.sh"
rg -q 'database_source_name.*status_field' "$ROOT_DIR/scripts/production-monitor.sh"
rg -Fq 'database_source_name=%s' "$logical_drill"
rg -q 'STATUS_REMOTE_DESTINATION' "$logical_drill"
rg -q 'pitr_restore=not_in_scope' "$logical_drill"
rg -Fq 'isolation=offsite_download_loopback_database_and_fixed_targets' "$logical_drill" "$ROOT_DIR/scripts/production-monitor.sh"
for config_path in logical-restore-source-read-rclone.conf logical-restore-status-write-rclone.conf; do
  rg -Fq "$config_path" "$logical_drill" "$logical_drill_env" "$logical_drill_unit"
done
rg -q 'SOURCE_RCLONE_CONFIG.*-ef.*STATUS_RCLONE_CONFIG' "$logical_drill"
rg -q 'source_config_sha256.*!=.*status_config_sha256' "$logical_drill"
trap_line="$(rg -n '^trap cleanup_drill EXIT INT TERM$' "$logical_drill" | cut -d: -f1)"
first_source_line="$(rg -n 'select_latest_remote_backup .*RESTORE_DRILL_DATABASE_REMOTE_SOURCE' "$logical_drill" | cut -d: -f1)"
[[ "$trap_line" -lt "$first_source_line" ]] || { echo "逻辑恢复在安装清理 trap 前访问异机源" >&2; exit 1; }
! rg -q 'run_source_rclone .*RESTORE_DRILL_STATUS_REMOTE_DESTINATION|run_status_rclone lsf|run_status_rclone copyto .*database_(backup|offsite)|run_status_rclone copyto .*upload_(backup|offsite)' "$logical_drill"
! rg -Fq '/var/backups/wangzhe' "$logical_drill_unit"
rg -Fq 'release/deploy/env/restore-drill.env.example' "$ROOT_DIR/Makefile"
rg -q 'recovery_target_time' "$ROOT_DIR/scripts/production-pitr-restore-drill.sh"
rg -q 'pg_verifybackup' "$ROOT_DIR/scripts/production-pitr-restore-drill.sh"
rg -q 'pitr_completed=1' "$ROOT_DIR/scripts/production-pitr-restore-drill.sh"
rg -q 'target_reached=1' "$ROOT_DIR/scripts/production-pitr-restore-drill.sh"
rg -Fq 'readonly DRILL_MOUNT_ROOT=/var/lib/wangzhe-pitr-drill' "$ROOT_DIR/scripts/production-pitr-restore-drill.sh"
rg -Fq 'readonly DRILL_ROOT="$DRILL_MOUNT_ROOT/work"' "$ROOT_DIR/scripts/production-pitr-restore-drill.sh"
rg -q 'validate_direct_luks_mount.*DRILL_MOUNT_ROOT' "$ROOT_DIR/scripts/production-pitr-restore-drill.sh"
rg -q 'direct_luks_mount_for_directory.*DATA_DIR' "$ROOT_DIR/scripts/production-pitr-restore-drill.sh"
rg -q 'drill_luks_mount=%s' "$ROOT_DIR/scripts/production-pitr-restore-drill.sh"
rg -q 'PITR LUKS 工作目录存在未完成解密任务遗留' "$ROOT_DIR/scripts/production-pitr-restore-drill.sh"
pitr_source_sync="$ROOT_DIR/scripts/pitr-recovery-source-sync.sh"
rg -Fq 'readonly EXPECTED_SOURCE_ROOT=/var/lib/wangzhe-pitr-source' "$pitr_source_sync"
rg -Fq 'PITR_SOURCE_SYNC_RCLONE_CONFIG" == /etc/wangzhe/pitr-wal-read-rclone.conf' "$pitr_source_sync"
rg -q 'run_rclone copy .*PITR_SOURCE_SYNC_REMOTE_DESTINATION' "$pitr_source_sync"
rg -q 'remote_snapshot.*remote-before|remote_snapshot.*remote-after' "$pitr_source_sync"
rg -q 'cmp -s .*before\.canonical.*after\.canonical' "$pitr_source_sync"
rg -q 'sha256sum.*artifact' "$pitr_source_sync"
rg -q 'source\.sha256' "$pitr_source_sync"
rg -q 'verify_backup_provenance.*artifact.*artifact_class' "$pitr_source_sync"
rg -q 'PITR_SOURCE_SYNC_PROVENANCE_VERIFY_KEY_FILE' "$pitr_source_sync" "$ROOT_DIR/deploy/env/pitr-source-sync.env.example"
rg -q 'validate_offsite_marker.*PITR_SOURCE_SYNC_REMOTE_DESTINATION' "$pitr_source_sync"
rg -q 'active-generation' "$pitr_source_sync" "$ROOT_DIR/scripts/production-pitr-restore-drill.sh"
rg -q 'WANGZHE_ISOLATED_RECOVERY_HOST' "$pitr_source_sync"
pitr_source_unit="$ROOT_DIR/deploy/systemd/wangzhe-pitr-source-sync.service"
pitr_drill_unit="$ROOT_DIR/deploy/systemd/wangzhe-pitr-restore-drill.service"
pitr_status_unit="$ROOT_DIR/deploy/systemd/wangzhe-pitr-status-publish.service"
pitr_drill="$ROOT_DIR/scripts/production-pitr-restore-drill.sh"
pitr_drill_env="$ROOT_DIR/deploy/env/pitr-drill.env.example"
pitr_restore="$ROOT_DIR/scripts/postgres-restore-wal.sh"
pitr_restore_env="$ROOT_DIR/deploy/env/pitr-restore.env.example"
pitr_status="$ROOT_DIR/scripts/publish-pitr-drill-status.sh"
pitr_status_env="$ROOT_DIR/deploy/env/pitr-status.env.example"
rg -Fq 'temporary_directory="$(mktemp -d "$PITR_RESTORE_LOCAL_ARCHIVE_DIR/.restore-$WAL_NAME.XXXXXX")"' "$pitr_restore"
rg -Fq 'temporary_remote="$temporary_directory/$WAL_NAME.age"' "$pitr_restore"
rg -q 'trap cleanup_restore EXIT INT TERM' "$pitr_restore"
rg -q '^Requires=wangzhe-pitr-source-sync\.service$' "$pitr_drill_unit"
rg -q '^After=.*wangzhe-pitr-source-sync\.service$' "$pitr_drill_unit"
rg -Fq 'RequiresMountsFor=/var/lib/wangzhe-pitr-drill' "$pitr_drill_unit"
rg -Fq 'ReadWritePaths=/var/lib/wangzhe-pitr-drill/work /run/wangzhe-pitr-source' "$pitr_drill_unit"
! rg -q '^StateDirectory=wangzhe-pitr-drill$' "$pitr_drill_unit"
rg -q '^PrivateNetwork=true$' "$pitr_drill_unit"
rg -Fq 'ReadOnlyPaths=/var/lib/wangzhe-pitr-source ' "$pitr_drill_unit"
for target in "$pitr_source_sync" "$pitr_source_unit" "$ROOT_DIR/deploy/env/pitr-source-sync.env.example" "$pitr_restore" "$pitr_restore_env"; do
  rg -Fq 'pitr-wal-read-rclone.conf' "$target"
done
for target in "$pitr_status" "$pitr_status_env" "$pitr_status_unit"; do
  rg -Fq 'pitr-status-write-rclone.conf' "$target"
done
rg -Fq 'PITR_DRILL_WAL_READ_RCLONE_CONFIG=/etc/wangzhe/pitr-wal-read-rclone.conf' "$pitr_drill_env"
rg -Fq 'PITR_DRILL_STATUS_WRITE_RCLONE_CONFIG=/etc/wangzhe/pitr-status-write-rclone.conf' "$pitr_drill_env"
for target in "$pitr_drill" "$pitr_drill_unit"; do
  rg -Fq '/etc/wangzhe/pitr-wal-read-rclone.conf' "$target"
  rg -Fq '/etc/wangzhe/pitr-status-write-rclone.conf' "$target"
done
rg -q 'wal_canonical.*!=.*status_canonical' "$pitr_drill"
rg -q 'wal_config.*-ef.*status_config' "$pitr_drill"
rg -q 'wal_file_identity.*!=.*status_file_identity' "$pitr_drill"
rg -q 'wal_sha256.*!=.*status_sha256' "$pitr_drill"
if rg -Fq 'pitr-status-write-rclone.conf' "$pitr_source_sync" "$pitr_source_unit" "$pitr_restore" "$pitr_restore_env"; then
  echo "PITR 读取流程暴露了状态写入凭据" >&2
  exit 1
fi
if rg -Fq 'pitr-wal-read-rclone.conf' "$pitr_status" "$pitr_status_env" "$pitr_status_unit"; then
  echo "PITR 状态发布流程暴露了备份读取凭据" >&2
  exit 1
fi
! rg -q '^ConditionPath' "$pitr_source_unit" "$pitr_drill_unit"
rg -q 'pitr-recovery-source-sync\.sh' "$ROOT_DIR/Makefile" "$ROOT_DIR/scripts/production-deploy.sh"
rg -q 'last-pitr-success.status' "$pitr_status"
rg -Fq 'EXPECTED_STATUS_FILE=/var/lib/wangzhe-pitr-drill/work/last-success.status' "$pitr_status"
rg -q 'drill_luks_mount.*EXPECTED_DRILL_LUKS_MOUNT' "$pitr_status"
rg -q 'restore_work_luks_mount.*var/lib/wangzhe-restore' "$ROOT_DIR/scripts/production-monitor.sh"
rg -q 'drill_luks_mount.*var/lib/wangzhe-pitr-drill' "$ROOT_DIR/scripts/production-monitor.sh"
rg -q 'pkeyutl -sign -rawin.*RESTORE_DRILL_STATUS_SIGNING_KEY_FILE' "$ROOT_DIR/scripts/production-restore-drill.sh"
rg -q 'pkeyutl -sign -rawin.*PITR_DRILL_STATUS_SIGNING_KEY_FILE' "$ROOT_DIR/scripts/production-pitr-restore-drill.sh"
rg -q "format_version=2.*source_generation=.*source_remote_destination=.*source_snapshot_sha256" "$ROOT_DIR/scripts/production-pitr-restore-drill.sh"
rg -q 'PITR_STATUS_EXPECTED_BACKUP_REMOTE_SOURCE' "$pitr_status" "$pitr_status_env"
rg -q 'PITR_STATUS_REMOTE_DESTINATION\.sig' "$pitr_status"
rg -q 'RESTORE_DRILL_STATUS_REMOTE_DESTINATION\.sig' "$ROOT_DIR/scripts/production-restore-drill.sh"
rg -q 'logical-restore-status-write-rclone\.conf' "$ROOT_DIR/deploy/env/restore-drill.env.example" "$ROOT_DIR/scripts/production-restore-drill.sh"
rg -Fq 'read-only' "$pitr_restore_env"
rg -Fq 'write-only' "$pitr_status_env"
rg -Fq 'wangzhe-production/pitr/<system_identifier>' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -Fq 'wangzhe-production/restore-status/last-pitr-success.status' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"

monitor="$ROOT_DIR/scripts/production-monitor.sh"
for evidence in \
  source_error_game_count error_issue_count stale_pending_issue_count abnormal_bet_count \
  '账务末值不一致会员' '账本算术错误流水' '孤儿账本流水' '账本重复reference组' '账本链断裂流水' \
  redis-production-check pg_stat_archiver '\.dump\.age' \
  'TLS证书将在21天内过期' 'Nginx新发生5xx' MONITOR_RESTORE_STATUS_REMOTE_SOURCE \
  MONITOR_PITR_RESTORE_STATUS_REMOTE_SOURCE 'PITR恢复演练状态证据无效' 'inode使用率' \
  MONITOR_BACKUP_INTEGRITY_STATUS_FILE '每日备份完整性全量校验已过期' \
  MONITOR_RESTORE_EXPECTED_DATABASE_NAME MONITOR_RESTORE_EXPECTED_DATABASE_REMOTE_SOURCE \
  MONITOR_RESTORE_EXPECTED_UPLOAD_REMOTE_SOURCE MONITOR_ALERT_REPEAT_MINUTES \
  MONITOR_BACKUP_PROVENANCE_VERIFY_KEY_FILE MONITOR_PITR_PROVENANCE_VERIFY_KEY_FILE \
  MONITOR_PITR_RESTORE_EXPECTED_REMOTE_SOURCE source_snapshot_sha256 \
  alert.signature '状态未确认，下次将重试'; do
  rg -q "$evidence" "$monitor" || { echo "主动监控缺少覆盖：$evidence" >&2; exit 1; }
done
! rg -q '^RemoveIPC=true$' "$ROOT_DIR/deploy/systemd/wangzhe-base-backup.service" "$ROOT_DIR/deploy/systemd/wangzhe-pitr-status-publish.service"
! rg -q "created_at < now\(\) - interval '1 hour'" "$monitor"
rg -q 'openssl s_client.*-verify_hostname' "$monitor"
rg -q 'SELECT DISTINCT ON \(user_id\).*user_balance_transactions.*UNION ALL.*SELECT DISTINCT ON \(user_id\).*user_balance_transaction_archives' "$monitor"
rg -q 'MONITOR_ACCOUNTING_AUDIT_INTERVAL_MINUTES' "$monitor" "$ROOT_DIR/deploy/env/monitor.env.example"
rg -q 'accounting-audit\.status' "$monitor"
rg -q 'statement_timeout=20000' "$monitor"
rg -q 'LAG\(after_cents\) OVER \(PARTITION BY user_id ORDER BY id\)' "$monitor"
rg -q 'HAVING COUNT\(\*\) > 1' "$monitor"
rg -q 'idx_balance_ledger_monitor_chain' "$ROOT_DIR/backend/migrations/202608300001_monitor_ledger_index.sql"
rg -q 'idx_balance_ledger_monitor_arithmetic_error' "$ROOT_DIR/backend/migrations/202608300001_monitor_ledger_index.sql"
rg -q 'validate_backup_manifest_metadata' "$monitor"
rg -q 'run_rclone size --json' "$monitor"
rg -q 'remote_small_object_is_valid' "$monitor"
rg -q 'remote_bytes >= 1 && remote_bytes <= 4096' "$monitor"
rg -q 'validate_remote_checksum_manifest.*remote_target\.sha256' "$monitor"
rg -q 'run_rclone copyto "\$remote"' "$monitor"
rg -q 'pkeyutl -verify -pubin -inkey "\$verify_key" -rawin' "$monitor"
rg -q 'remote_source\.sig.*signature_partial' "$monitor"
rg -q 'MONITOR_RESTORE_STATUS_VERIFY_KEY_FILE' "$monitor" "$ROOT_DIR/deploy/env/monitor.env.example"
rg -q 'MONITOR_PITR_RESTORE_STATUS_VERIFY_KEY_FILE' "$monitor" "$ROOT_DIR/deploy/env/monitor.env.example"
rg -q 'logical_verify_key_fingerprint.*!=.*pitr_verify_key_fingerprint' "$monitor"
verify_line="$(rg -n 'pkeyutl -verify -pubin -inkey "\$verify_key" -rawin' "$monitor" | cut -d: -f1)"
accept_line="$(rg -n 'mv "\$status_partial" "\$local_target"' "$monitor" | cut -d: -f1)"
[[ "$verify_line" -lt "$accept_line" ]] || { echo "监控在验签前接受了恢复状态" >&2; exit 1; }
rg -q 'database_offsite_source.*status_field' "$monitor"
rg -q 'validate_remote_destination.*database_offsite_source' "$monitor"
rg -Fq 'database_source_name" == "$MONITOR_RESTORE_EXPECTED_DATABASE_NAME' "$monitor"
rg -Fq 'MONITOR_RESTORE_EXPECTED_DATABASE_REMOTE_SOURCE%/}/$database_backup_name' "$monitor"
rg -Fq 'MONITOR_RESTORE_EXPECTED_UPLOAD_REMOTE_SOURCE%/}/$upload_backup_name' "$monitor"
rg -Fq '[[ "$version" == v2' "$monitor"
rg -Fq '[[ "$format" == 2' "$monitor"
rg -q 'repeat_seconds=.*ALERT_REPEAT_MINUTES' "$monitor"
rg -q "printf 'v2 %s %s" "$monitor"
rg -q 'now_epoch - previous_sent_epoch >= repeat_seconds' "$monitor"
rg -q 'cursor_version.*previous_inode.*previous_line' "$monitor"
rg -q 'rotated_log="\$MONITOR_NGINX_ACCESS_LOG\.1"' "$monitor"
rg -q 'rotated_inode.*previous_inode' "$monitor"
rg -q "printf 'v1 %s %s" "$monitor"
! rg -q 'validate_encrypted_backup_and_manifest.*latest' "$monitor"
! rg -q 'sha256sum.*\$latest|sha256sum.*\$local_base' "$monitor"
for key_path in \
  logical-restore-status-ed25519-public.pem \
  pitr-restore-status-ed25519-public.pem; do
  rg -Fq "$key_path" "$ROOT_DIR/deploy/systemd/wangzhe-monitor.service"
done
rg -Fq 'logical-restore-status-ed25519-private.pem' "$ROOT_DIR/deploy/systemd/wangzhe-restore-drill.service"
rg -Fq 'pitr-restore-status-ed25519-private.pem' "$ROOT_DIR/deploy/systemd/wangzhe-pitr-restore-drill.service"
! rg -q 'ed25519-private' "$ROOT_DIR/deploy/systemd/wangzhe-monitor.service" "$ROOT_DIR/deploy/env/monitor.env.example"

# macOS ships an old LibreSSL without Ed25519/-rawin. CI and production use
# OpenSSL 3.x; exercise the exact detached-signature commands whenever the
# local implementation advertises both required capabilities.
signature_fixture="$fixture_dir/status-signature"
mkdir "$signature_fixture"
test_pkeyutl_help="$(openssl pkeyutl -help 2>&1 || true)"
if openssl genpkey -algorithm ED25519 -out "$signature_fixture/logical-private.pem" >/dev/null 2>&1 && \
   grep -q -- '-rawin' <<<"$test_pkeyutl_help"; then
  openssl pkey -in "$signature_fixture/logical-private.pem" -pubout -out "$signature_fixture/logical-public.pem"
  openssl genpkey -algorithm ED25519 -out "$signature_fixture/pitr-private.pem"
  openssl pkey -in "$signature_fixture/pitr-private.pem" -pubout -out "$signature_fixture/pitr-public.pem"
  printf 'encrypted-artifact-fixture\n' >"$signature_fixture/artifact.age"
  write_backup_provenance "$signature_fixture/artifact.age" remote:production/database database wangzhe 1700000000 \
    "$signature_fixture/logical-private.pem" "$signature_fixture/artifact.age.provenance" "$signature_fixture/artifact.age.provenance.sig"
  verify_backup_provenance "$signature_fixture/artifact.age" database wangzhe remote:production/database/artifact.age \
    "$signature_fixture/logical-private.pem" private
  if verify_backup_provenance "$signature_fixture/artifact.age" database wangzhe remote:production/database/artifact.age \
    "$signature_fixture/pitr-private.pem" private >/dev/null 2>&1; then
    echo "独立 PITR 来源私钥错误接受了数据库来源签名" >&2
    exit 1
  fi
  printf 'status_schema=signature-test.v1\noutcome=success\n' >"$signature_fixture/status"
  openssl pkeyutl -sign -rawin -inkey "$signature_fixture/logical-private.pem" -in "$signature_fixture/status" -out "$signature_fixture/status.sig"
  [[ "$(strict_env_stat '%s' '%z' "$signature_fixture/status.sig")" == 64 ]]
  openssl pkeyutl -verify -pubin -inkey "$signature_fixture/logical-public.pem" -rawin -in "$signature_fixture/status" -sigfile "$signature_fixture/status.sig" >/dev/null
  if openssl pkeyutl -verify -pubin -inkey "$signature_fixture/pitr-public.pem" -rawin -in "$signature_fixture/status" -sigfile "$signature_fixture/status.sig" >/dev/null 2>&1; then
    echo "独立 PITR 公钥错误接受了逻辑恢复签名" >&2
    exit 1
  fi
  printf 'forged=1\n' >>"$signature_fixture/status"
  if openssl pkeyutl -verify -pubin -inkey "$signature_fixture/logical-public.pem" -rawin -in "$signature_fixture/status" -sigfile "$signature_fixture/status.sig" >/dev/null 2>&1; then
    echo "Ed25519 验签错误接受了篡改状态" >&2
    exit 1
  fi
else
  echo "提示：本机 OpenSSL 不支持 Ed25519/-rawin，已执行源码门禁；生产要求 OpenSSL 3.0+" >&2
fi
unset test_pkeyutl_help
if rg -n -g '!ops-resilience-static-test.sh' '^[[:space:]]*rclone[[:space:]]+(--config[^\n]*[[:space:]])?sha256sum([[:space:]]|$)' "$ROOT_DIR/scripts"; then
  echo "发现不存在的 rclone sha256sum 子命令" >&2
  exit 1
fi

for unit in \
  wangzhe-backend.service wangzhe-backup.service wangzhe-upload-backup.service wangzhe-base-backup.service \
  wangzhe-monitor.service wangzhe-backup-integrity.service wangzhe-restore-drill.service wangzhe-pitr-source-sync.service wangzhe-pitr-restore-drill.service \
  wangzhe-pitr-status-publish.service; do
  rg -q '^NoNewPrivileges=true$' "$ROOT_DIR/deploy/systemd/$unit"
  rg -q '^ProtectSystem=strict$' "$ROOT_DIR/deploy/systemd/$unit"
  rg -q '^CapabilityBoundingSet=$' "$ROOT_DIR/deploy/systemd/$unit"
  rg -q '^OnFailure=wangzhe-ops-failure-alert@%n\.service$' "$ROOT_DIR/deploy/systemd/$unit"
done
! rg -q '^ConditionPath' "$ROOT_DIR/deploy/systemd"
failure_unit="$ROOT_DIR/deploy/systemd/wangzhe-ops-failure-alert@.service"
rg -q 'production-unit-failure-alert\.sh' "$failure_unit" "$ROOT_DIR/Makefile" "$ROOT_DIR/scripts/production-deploy.sh"
rg -q '^User=wangzhe-monitor$' "$failure_unit"
rg -q '^NoNewPrivileges=true$' "$failure_unit"
rg -q '^CapabilityBoundingSet=$' "$failure_unit"
rg -q '^SuccessExitStatus=2$' "$ROOT_DIR/deploy/systemd/wangzhe-monitor.service"
! rg -q '^SupplementaryGroups=adm$' "$ROOT_DIR/deploy/systemd/wangzhe-monitor.service"
backup_integrity_unit="$ROOT_DIR/deploy/systemd/wangzhe-backup-integrity.service"
rg -q '^User=wangzhe-monitor$' "$backup_integrity_unit"
rg -q '^Group=wangzhe-monitor$' "$backup_integrity_unit"
rg -q '^ReadOnlyPaths=/var/backups/wangzhe .*monitor-rclone\.conf ' "$backup_integrity_unit"
rg -q 'backup-provenance-ed25519-public\.pem.*pitr-provenance-ed25519-public\.pem' "$backup_integrity_unit"
rg -q '^StateDirectory=wangzhe-monitor$' "$backup_integrity_unit"
rg -q '^ReadWritePaths=/var/lib/wangzhe-monitor$' "$backup_integrity_unit"
! rg -q '^ReadWritePaths=.*var/backups/wangzhe' "$backup_integrity_unit"
rg -q '^    create 0640 www-data wangzhe-monitor$' "$ROOT_DIR/deploy/logrotate/wangzhe-nginx"
for timer in wangzhe-backup.timer wangzhe-upload-backup.timer wangzhe-base-backup.timer wangzhe-monitor.timer wangzhe-backup-integrity.timer wangzhe-restore-drill.timer wangzhe-pitr-restore-drill.timer; do
  rg -q '^Persistent=true$' "$ROOT_DIR/deploy/systemd/$timer"
done

if rg -n '(^|[;&|()][[:space:]]*)[[:space:]]*rclone[[:space:]]+(copyto|hashsum|copy|sync|cat|size)' "$ROOT_DIR/scripts"; then
  echo "发现未显式提供 --config 的 rclone 数据操作" >&2
  exit 1
fi

echo "生产运维韧性静态检查通过"
