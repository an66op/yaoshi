#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
reset_script="$script_dir/dev-reset-business-data.sh"
dev_backup_script="$script_dir/dev-postgres-backup.sh"
full_reset_script="$script_dir/dev-reset-database.sh"
sentinel_script="$script_dir/dev-reset-init-sentinel.sh"
verify_script="$script_dir/dev-reset-verify-bootstrap.sh"
complete_receipt_script="$script_dir/dev-reset-complete-receipt.sh"
restore_payment_qr_script="$script_dir/dev-reset-restore-payment-qr.sh"
migration="$script_dir/../backend/migrations/202608270010_dev_reset_guard.sql"
marker_migration="$script_dir/../backend/migrations/202608270011_reset_guard_workspace_marker.sql"
identity_migration="$script_dir/../backend/migrations/202608270012_reset_identity_receipts.sql"

bash -n "$reset_script" "$dev_backup_script" "$full_reset_script" "$sentinel_script" "$verify_script" "$complete_receipt_script" "$restore_payment_qr_script" "$script_dir/lib/dev-reset-safety.sh"
# shellcheck source=lib/backend-env.sh
source "$script_dir/lib/backend-env.sh"
# shellcheck source=lib/dev-reset-safety.sh
source "$script_dir/lib/dev-reset-safety.sh"
test_root="$(mktemp -d)"
test_root="$(cd "$test_root" && pwd -P)"
trap 'rm -rf -- "$test_root"' EXIT INT TERM

# Private payment QR cleanup is constrained to the generated
# workspace/user/random-name layout and deletes files individually. Exercise
# the helper without a database so this destructive boundary remains testable.
payment_upload_root="$test_root/payment-uploads"
mkdir -p "$payment_upload_root/.private/member-payment-qr/37/91"
payment_upload_root="$(cd "$payment_upload_root" && pwd -P)"
payment_qr_fixture="$payment_upload_root/.private/member-payment-qr/37/91/0123456789abcdef0123456789abcdef.png"
printf 'sanitized-png-fixture' >"$payment_qr_fixture"
reset_validate_payment_qr_cleanup_target "$payment_upload_root"
[[ "$RESET_PAYMENT_QR_DIRECTORY" == "$payment_upload_root/.private/member-payment-qr" && "$RESET_PAYMENT_QR_FILE_COUNT" == "1" ]] || {
  echo "二维码清理目标或文件计数不正确" >&2
  exit 1
}

# A reset backup is only complete when its private QR companion is encrypted,
# checksummed, independently readable and exact. Exercise creation, validation
# and the documented restore shape without requiring PostgreSQL.
payment_backup_dir="$test_root/payment-backups"
mkdir "$payment_backup_dir"
chmod 700 "$payment_backup_dir"
payment_database_backup="$payment_backup_dir/database.dump.age"
printf 'encrypted-database-placeholder' >"$payment_database_backup"
chmod 600 "$payment_database_backup"
payment_age_identity_file="$test_root/payment-backup-identity.txt"
age-keygen --output "$payment_age_identity_file" >/dev/null 2>&1
chmod 600 "$payment_age_identity_file"
payment_age_identity="$(awk '/^AGE-SECRET-KEY-/ { print }' "$payment_age_identity_file")"
reset_archive_payment_qr_files "$payment_upload_root" "$payment_database_backup" "$payment_age_identity"
payment_qr_archive="$RESET_PAYMENT_QR_ARCHIVE"
[[ "$RESET_PAYMENT_QR_ARCHIVE_FILE_COUNT" == "1" && "$payment_qr_archive" == "$payment_database_backup.member-payment-qr.tar.age" ]] || {
  echo "二维码配套归档路径或文件数不正确" >&2
  exit 1
}
[[ -f "$payment_qr_archive.sha256" && ! -L "$payment_qr_archive.sha256" ]] || { echo "二维码归档缺少校验文件" >&2; exit 1; }
reset_validate_payment_qr_archive "$payment_qr_archive" "$RESET_PAYMENT_QR_ARCHIVE_SHA256" 1 "$payment_age_identity"
if tar --list --file "$payment_qr_archive" >/dev/null 2>&1; then
  echo "二维码配套归档错误地发布成了明文 tar" >&2
  exit 1
fi
restored_payment_root="$test_root/restored-payment-uploads"
mkdir "$restored_payment_root"
restored_payment_root="$(cd "$restored_payment_root" && pwd -P)"
payment_restore_receipt="$payment_database_backup.reset-receipt"
{
  printf 'status=complete\n'
  printf 'database=wangzhe_test\n'
  printf 'backup=%s\n' "$(basename "$payment_database_backup")"
  printf 'backup_sha256=%s\n' "$(reset_file_sha256 "$payment_database_backup")"
  printf 'payment_qr_backup=%s\n' "$(basename "$payment_qr_archive")"
  printf 'payment_qr_backup_sha256=%s\n' "$RESET_PAYMENT_QR_ARCHIVE_SHA256"
  printf 'payment_qr_files_archived=1\n'
  printf 'payment_qr_files_expected=1\n'
  printf 'payment_qr_files_removed=1\n'
} >"$payment_restore_receipt"
chmod 600 "$payment_restore_receipt"
restore_fake_bin="$test_root/restore-fake-bin"
mkdir "$restore_fake_bin"
printf '%s\n' '#!/bin/sh' 'printf "%s\n" ".private/member-payment-qr/37/91/0123456789abcdef0123456789abcdef.png"' >"$restore_fake_bin/psql"
chmod 700 "$restore_fake_bin/psql"
PATH="$restore_fake_bin:$PATH" \
  BACKEND_SERVER_MODE=debug BACKEND_SERVER_PORT=65431 \
  BACKEND_DATABASE_HOST=127.0.0.1 BACKEND_DATABASE_PORT=5432 \
  BACKEND_DATABASE_USER=test BACKEND_DATABASE_PASSWORD=test BACKEND_DATABASE_DBNAME=wangzhe_test BACKEND_DATABASE_SSLMODE=disable \
  BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY="$payment_age_identity" \
  bash "$restore_payment_qr_script" --receipt "$payment_restore_receipt" --upload-dir "$restored_payment_root"
reset_validate_payment_qr_cleanup_target "$restored_payment_root"
[[ "$RESET_PAYMENT_QR_FILE_COUNT" == "1" ]] || { echo "二维码配套归档无法恢复精确文件集" >&2; exit 1; }
cmp -s "$payment_qr_fixture" "$restored_payment_root/.private/member-payment-qr/37/91/0123456789abcdef0123456789abcdef.png" || {
  echo "二维码配套归档恢复内容不一致" >&2
  exit 1
}
tampered_payment_archive="$payment_backup_dir/tampered.tar.age"
cp "$payment_qr_archive" "$tampered_payment_archive"
chmod 600 "$tampered_payment_archive"
printf 'tampered' >>"$tampered_payment_archive"
if reset_validate_payment_qr_archive "$tampered_payment_archive" "$RESET_PAYMENT_QR_ARCHIVE_SHA256" 1 "$payment_age_identity" >"$test_root/tampered-payment-archive.out" 2>&1; then
  echo "二维码归档校验错误地接受了篡改文件" >&2
  exit 1
fi
grep -Fq 'SHA-256 不匹配' "$test_root/tampered-payment-archive.out"

# A file arriving after the companion snapshot must never be silently deleted
# without a matching backup.
unarchived_payment_qr="$payment_upload_root/.private/member-payment-qr/37/91/abcdefabcdefabcdefabcdefabcdefab.png"
printf 'not-in-archive' >"$unarchived_payment_qr"
if reset_remove_payment_qr_files "$payment_upload_root" 1 >"$test_root/payment-archive-count-drift.out" 2>&1; then
  echo "二维码删除错误地接受了与归档不一致的文件集" >&2
  exit 1
fi
grep -Fq '与配套归档不一致' "$test_root/payment-archive-count-drift.out"
[[ -f "$payment_qr_fixture" && -f "$unarchived_payment_qr" ]] || { echo "归档数量漂移后文件被误删" >&2; exit 1; }
rm -- "$unarchived_payment_qr"
reset_remove_payment_qr_files "$payment_upload_root" 1
[[ "$RESET_PAYMENT_QR_REMOVED_COUNT" == "1" && ! -e "$payment_qr_fixture" ]] || {
  echo "二维码清理未精确删除目标文件" >&2
  exit 1
}
printf 'unrelated' >"$payment_upload_root/.private/member-payment-qr/37/91/customer.png"
if reset_validate_payment_qr_cleanup_target "$payment_upload_root" >"$test_root/invalid-payment-qr.out" 2>&1; then
  echo "二维码清理错误地接受了非应用生成文件" >&2
  exit 1
fi
grep -Fq '非应用生成文件' "$test_root/invalid-payment-qr.out"
rm -- "$payment_upload_root/.private/member-payment-qr/37/91/customer.png"
payment_qr_symlink="$payment_upload_root/.private/member-payment-qr/37/91/abcdefabcdefabcdefabcdefabcdefab.png"
ln -s "$test_root/never-delete-this-file" "$payment_qr_symlink"
if reset_validate_payment_qr_cleanup_target "$payment_upload_root" >"$test_root/symlink-payment-qr.out" 2>&1; then
  echo "二维码清理错误地接受了符号链接" >&2
  exit 1
fi
grep -Fq '包含符号链接' "$test_root/symlink-payment-qr.out"
rm -- "$payment_qr_symlink"
hardlink_source="$test_root/never-archive-this-file"
printf 'private-unrelated-data' >"$hardlink_source"
payment_qr_hardlink="$payment_upload_root/.private/member-payment-qr/37/91/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.png"
ln "$hardlink_source" "$payment_qr_hardlink"
if reset_validate_payment_qr_cleanup_target "$payment_upload_root" >"$test_root/hardlink-payment-qr.out" 2>&1; then
  echo "二维码归档边界错误地接受了硬链接" >&2
  exit 1
fi
grep -Fq '不是服务生成的独占普通文件' "$test_root/hardlink-payment-qr.out"
rm -- "$payment_qr_hardlink" "$hardlink_source"
payment_upload_link="$test_root/payment-uploads-link"
ln -s "$payment_upload_root" "$payment_upload_link"
if reset_validate_payment_qr_cleanup_target "$payment_upload_link" >"$test_root/symlink-payment-root.out" 2>&1; then
  echo "二维码清理错误地接受了符号链接上传根目录" >&2
  exit 1
fi
grep -Fq '上传目录不存在、不是普通目录或是符号链接' "$test_root/symlink-payment-root.out"
rm -- "$payment_upload_link"
if reset_validate_payment_qr_cleanup_target / >"$test_root/broad-payment-qr.out" 2>&1; then
  echo "二维码清理错误地接受了根目录" >&2
  exit 1
fi
grep -Fq '拒绝使用过宽的上传目录' "$test_root/broad-payment-qr.out"

# A failed find must never be mistaken for an empty directory. Bash process
# substitution loses this status, so both the validation scan and the later
# deletion-manifest scan are exercised with deterministic command failures.
find() { return 73; }
if reset_validate_payment_qr_cleanup_target "$payment_upload_root" >"$test_root/payment-find-failure.out" 2>&1; then
  unset -f find
  echo "二维码校验错误地吞掉了枚举失败" >&2
  exit 1
fi
unset -f find
grep -Fq '无法完整枚举会员收款二维码目录' "$test_root/payment-find-failure.out"

payment_qr_fixture="$payment_upload_root/.private/member-payment-qr/37/91/11111111111111111111111111111111.png"
printf 'sanitized-png-fixture' >"$payment_qr_fixture"
payment_find_calls=0
find() {
  payment_find_calls=$((payment_find_calls + 1))
  if (( payment_find_calls == 1 )); then
    command find "$@"
    return
  fi
  return 74
}
if reset_remove_payment_qr_files "$payment_upload_root" >"$test_root/payment-remove-find-failure.out" 2>&1; then
  unset -f find
  echo "二维码删除错误地吞掉了枚举失败" >&2
  exit 1
fi
unset -f find
grep -Fq '无法完整枚举待删除的会员收款二维码文件' "$test_root/payment-remove-find-failure.out"
[[ -f "$payment_qr_fixture" ]] || { echo "枚举失败后二维码文件被误删" >&2; exit 1; }
rm -- "$payment_qr_fixture"

# Exercise the real filesystem permission failure where the current user is
# not root. The directory permission is always restored before asserting so
# test cleanup remains deterministic.
if (( EUID != 0 )); then
  permission_owner_directory="$payment_upload_root/.private/member-payment-qr/38/92"
  mkdir -p "$permission_owner_directory"
  permission_qr_fixture="$permission_owner_directory/22222222222222222222222222222222.png"
  printf 'sanitized-png-fixture' >"$permission_qr_fixture"
  chmod 000 "$permission_owner_directory"
  permission_scan_failed=false
  if ! reset_validate_payment_qr_cleanup_target "$payment_upload_root" >"$test_root/payment-permission-failure.out" 2>&1; then
    permission_scan_failed=true
  fi
  chmod 700 "$permission_owner_directory"
  [[ "$permission_scan_failed" == "true" ]] || { echo "二维码校验错误地接受了不可枚举目录" >&2; exit 1; }
  grep -Fq '无法完整枚举会员收款二维码目录' "$test_root/payment-permission-failure.out"
  [[ -f "$permission_qr_fixture" ]] || { echo "权限拒绝后二维码文件被误删" >&2; exit 1; }
  rm -- "$permission_qr_fixture"
fi

if grep -Eq '(^|[[:space:]])rm[[:space:]]+-[A-Za-z]*r|find .*-(delete|exec)' "$reset_script" "$full_reset_script" "$script_dir/lib/dev-reset-safety.sh"; then
  echo "二维码清理不得递归删除目录或使用 find 批量删除" >&2
  exit 1
fi

write_env() {
  local target="$1" mode="$2" host="$3"
  printf '%s\n' \
    "BACKEND_SERVER_MODE=$mode" \
    "BACKEND_SERVER_PORT=8080" \
    "BACKEND_DATABASE_HOST=$host" \
    "BACKEND_DATABASE_PORT=5432" \
    "BACKEND_DATABASE_USER=developer" \
    "BACKEND_DATABASE_PASSWORD=not-used-by-dry-run" \
    "BACKEND_DATABASE_DBNAME=wangzhe_dev" \
    "BACKEND_DATABASE_SSLMODE=disable" \
    >"$target"
  chmod 600 "$target"
}

write_env "$test_root/debug.env" debug 127.0.0.1
printf '%s\n' 'BACKEND_SEED_EXPERIENCE_ACCOUNTS=true' >>"$test_root/debug.env"
dry_run_output="$(bash "$reset_script" --dry-run "$test_root/debug.env")"
grep -Fq '仅预览：没有连接数据库、没有备份、没有修改任何数据。' <<<"$dry_run_output"
grep -Fq 'schema_migrations' <<<"$dry_run_output"
grep -Fq 'development_reset_receipts' <<<"$dry_run_output"
grep -Fq 'workspace_migration_markers' <<<"$dry_run_output"

write_env "$test_root/debug-without-experience-seed.env" debug 127.0.0.1
if bash "$full_reset_script" --dry-run "$test_root/debug-without-experience-seed.env" >"$test_root/full-missing-experience-seed.out" 2>&1; then
  echo "完整重建错误地允许未显式启用体验账号夹具" >&2
  exit 1
fi
grep -Fq '完整重建必须显式设置 BACKEND_SEED_EXPERIENCE_ACCOUNTS=true' "$test_root/full-missing-experience-seed.out"
if bash "$verify_script" "$test_root/debug-without-experience-seed.env" >"$test_root/verify-missing-experience-seed.out" 2>&1; then
  echo "只读验收错误地允许未显式启用体验账号夹具" >&2
  exit 1
fi
grep -Fq '只读重建验收必须显式设置 BACKEND_SEED_EXPERIENCE_ACCOUNTS=true' "$test_root/verify-missing-experience-seed.out"
printf '%s\n' 'BACKEND_SEED_EXPERIENCE_ACCOUNTS=false' >>"$test_root/debug-without-experience-seed.env"
if bash "$verify_script" "$test_root/debug-without-experience-seed.env" >"$test_root/verify-false-experience-seed.out" 2>&1; then
  echo "只读验收错误地允许禁用的体验账号夹具" >&2
  exit 1
fi
grep -Fq '只读重建验收必须显式设置 BACKEND_SEED_EXPERIENCE_ACCOUNTS=true' "$test_root/verify-false-experience-seed.out"
if grep -Fq 'BACKEND_SEED_EXPERIENCE_ACCOUNTS' "$script_dir/local-dev.sh"; then
  echo "普通 local-dev.sh 不得默认设置体验账号夹具开关" >&2
  exit 1
fi

# Purely static: prove the human preview, audited SQL manifest and explicit
# truncate statement describe the same exact tables, then compare that union
# against the canonical migration inventory. New tables require an explicit
# preserve/clear decision instead of silently widening a destructive command.
preview_preserved="$(sed -n 's/^保留表（[0-9]*）：//p' <<<"$dry_run_output" | tr ' ' '\n' | LC_ALL=C sort)"
preview_cleared="$(sed -n 's/^清理表（[0-9]*）：//p' <<<"$dry_run_output" | tr ' ' '\n' | LC_ALL=C sort)"
manifest_preserved="$(grep -Eo "\('[a-z_][a-z0-9_]*','preserve'\)" "$reset_script" | cut -d "'" -f2 | LC_ALL=C sort)"
manifest_cleared="$(grep -Eo "\('[a-z_][a-z0-9_]*','clear'\)" "$reset_script" | cut -d "'" -f2 | LC_ALL=C sort)"
truncate_statement="$(sed -n '/^TRUNCATE TABLE$/,/^RESTART IDENTITY;$/p' "$reset_script")"
truncate_tables="$(grep -Eo 'public\.[a-z_][a-z0-9_]*' <<<"$truncate_statement" | cut -d. -f2 | LC_ALL=C sort)"
[[ "$(grep -c . <<<"$manifest_preserved")" == "25" && "$(grep -c . <<<"$manifest_cleared")" == "33" ]] || {
  echo "业务重置必须保持当前 25 张保留表、33 张清理表的审定边界" >&2
  exit 1
}
[[ -n "$preview_preserved" && "$preview_preserved" == "$manifest_preserved" ]] || { echo "保留表预览与 SQL manifest 不一致" >&2; exit 1; }
[[ -n "$preview_cleared" && "$preview_cleared" == "$manifest_cleared" && "$manifest_cleared" == "$truncate_tables" ]] || {
  echo "清理表预览、SQL manifest 与明确 TRUNCATE 范围不一致" >&2
  exit 1
}
if grep -Eiq '(^|[[:space:]])CASCADE([[:space:];]|$)' <<<"$truncate_statement"; then
  echo "业务重置不得通过 CASCADE 隐式清除保留表" >&2
  exit 1
fi
manifest_tables="$(printf '%s\n' "$manifest_preserved" "$manifest_cleared" | LC_ALL=C sort)"
[[ -z "$(uniq -d <<<"$manifest_tables")" ]] || { echo "重置清单重复分类同一张表" >&2; exit 1; }
schema_declarations="$(grep -Eih '^[[:space:]]*CREATE[[:space:]]+TABLE([[:space:]]|$)' "$script_dir/../backend/migrations"/*.sql)"
schema_tables="$(sed -nE 's/^[[:space:]]*CREATE TABLE( IF NOT EXISTS)? ("?public"?\.)?"?([a-z_][a-z0-9_]*)"?[[:space:]]*\(.*/\3/p' <<<"$schema_declarations")"
[[ "$(wc -l <<<"$schema_declarations" | tr -d ' ')" == "$(wc -l <<<"$schema_tables" | tr -d ' ')" ]] || {
  echo "存在未识别的 SQL 建表语法，不能据此扩大重置范围" >&2
  exit 1
}
schema_tables="$(printf '%s\n' schema_migrations "$schema_tables" | LC_ALL=C sort -u)"
[[ "$manifest_tables" == "$schema_tables" ]] || {
  echo "开发重置清单没有精确覆盖当前 SQL 迁移表；必须审定新增表的保留/清理范围" >&2
  diff <(printf '%s\n' "$schema_tables") <(printf '%s\n' "$manifest_tables") >&2 || true
  exit 1
}
for preserved in schema_migrations development_reset_receipts workspace_migration_markers workspace_robot_reset_receipts ws_session_revocation_outbox lottery_games ops_activities special_number_campaigns plan_automations; do
  grep -Fxq "$preserved" <<<"$manifest_preserved" || { echo "不可变凭证、安全撤销意图或配置表未保留：$preserved" >&2; exit 1; }
done
for cleared in member_payment_accounts special_number_grants member_chat_read_cursors lottery_issue_windows plan_generation_receipts plan_streams plan_stream_cycles plan_stream_periods plan_publication_views lottery_sgssc_backfill_attempts lottery_sgssc_backfill_items system_event_logs; do
  grep -Fxq "$cleared" <<<"$truncate_tables" || { echo "运行记录未随关联业务一起清理：$cleared" >&2; exit 1; }
done
grep -Fq 'public.lottery_sgssc_backfill_attempts, public.lottery_sgssc_backfill_items,' <<<"$truncate_statement" || {
  echo "SG 补采尝试与父队列必须按明确顺序纳入同一条受控 TRUNCATE" >&2
  exit 1
}
empty_table_assertions="$(sed -n '/FOREACH table_name IN ARRAY ARRAY\[/,/] LOOP/p' "$verify_script")"
for empty_table in lottery_sgssc_backfill_attempts lottery_sgssc_backfill_items system_event_logs; do
  grep -Fq "'$empty_table'" <<<"$empty_table_assertions" || { echo "新库验收遗漏 SG 补采空历史表：$empty_table" >&2; exit 1; }
done

runtime_reset_sql="$(sed -n '/^-- Preserved catalogue\/configuration rows also contain derived runtime fields\./,/^-- Fail closed before writing the immutable receipt\./p' "$reset_script")"
for fragment in \
  'UPDATE public.lottery_games' "next_issue = ''" 'next_draw_at = NULL' \
  "THEN 'pending'" "THEN 'stale'" 'last_sync_at = NULL' "last_sync_error = ''" \
  'UPDATE public.ops_activities' 'participants = 0' 'pool_remaining_cents = pool_total_cents' \
  'UPDATE public.special_number_campaigns' 'granted_count = 0' \
  'UPDATE public.plan_automations' 'last_run_at = NULL' 'last_created_count = 0'; do
  grep -Fq "$fragment" <<<"$runtime_reset_sql" || { echo "业务重置缺少运行态复位：$fragment" >&2; exit 1; }
done

lottery_runtime_reset="$(sed -n '/^UPDATE public.lottery_games$/,/^    updated_at = now();$/p' "$reset_script")"
for forbidden in 'code =' 'name =' 'category =' 'lobby_category =' 'lobby_sort_order =' 'badge =' 'badge_color =' 'enabled =' 'sort_order =' 'draw_interval =' 'source_kind =' 'source_name =' 'source_url =' 'odds_config_revision ='; do
  if grep -Fq "$forbidden" <<<"$lottery_runtime_reset"; then
    echo "彩票运行态复位错误地改写保留配置：$forbidden" >&2
    exit 1
  fi
done
plan_runtime_reset="$(sed -n '/^UPDATE public.plan_automations$/,/^    updated_at = now();$/p' "$reset_script")"
for forbidden in 'enabled =' 'mode =' 'game_ids_json =' 'positions_json =' 'plan_keys_json ='; do
  if grep -Fq "$forbidden" <<<"$plan_runtime_reset"; then
    echo "计划运行态复位错误地改写保留配置：$forbidden" >&2
    exit 1
  fi
done
activity_runtime_reset="$(sed -n '/^UPDATE public.ops_activities$/,/^    updated_at = now();$/p' "$reset_script")"
for forbidden in 'workspace_id =' 'type =' 'title =' 'subtitle =' 'status =' 'cover =' 'reward_cents =' 'pool_total_cents =' 'config_json =' 'sort_order =' 'starts_at =' 'ends_at ='; do
  if grep -Fq "$forbidden" <<<"$activity_runtime_reset"; then
    echo "活动运行态复位错误地改写保留配置：$forbidden" >&2
    exit 1
  fi
done
special_campaign_runtime_reset="$(sed -n '/^UPDATE public.special_number_campaigns$/,/^    updated_at = now();$/p' "$reset_script")"
for forbidden in 'title =' 'status =' 'rule_text =' 'starts_at =' 'ends_at ='; do
  if grep -Fq "$forbidden" <<<"$special_campaign_runtime_reset"; then
    echo "特殊号码计数复位错误地改写保留配置：$forbidden" >&2
    exit 1
  fi
done

post_reset_assertions="$(sed -n '/^-- Fail closed before writing the immutable receipt\./,/^INSERT INTO public.development_reset_receipts (/p' "$reset_script")"
for fragment in \
  'FROM dev_reset_manifest' "WHERE disposition = 'clear'" \
  "format('SELECT count(*) FROM public.%I', cleared_table)" \
  'cleared table % still contains % row(s) after reset' \
  'FROM public.lottery_games' 'FROM public.ops_activities' \
  'FROM public.special_number_campaigns' 'FROM public.plan_automations'; do
  grep -Fq "$fragment" <<<"$post_reset_assertions" || { echo "业务重置缺少事务内结果断言：$fragment" >&2; exit 1; }
done

full_dry_run_output="$(bash "$full_reset_script" --dry-run "$test_root/debug.env")"
grep -Fq '仅预览：没有连接数据库、没有备份、没有修改任何 schema 或数据。' <<<"$full_dry_run_output"
sentinel_dry_run_output="$(bash "$sentinel_script" --dry-run "$test_root/debug.env")"
grep -Fq '仅预览：没有连接数据库、没有写入 sentinel。' <<<"$sentinel_dry_run_output"

current_env_output="$(
  BACKEND_SERVER_MODE=debug \
  BACKEND_SERVER_PORT=8080 \
  BACKEND_DATABASE_HOST=localhost \
  BACKEND_DATABASE_PORT=5432 \
  BACKEND_DATABASE_USER=developer \
  BACKEND_DATABASE_PASSWORD=not-used-by-dry-run \
  BACKEND_DATABASE_DBNAME=wangzhe_dev \
  BACKEND_DATABASE_SSLMODE=disable \
  bash "$reset_script" --dry-run
)"
grep -Fq '仅预览：没有连接数据库、没有备份、没有修改任何数据。' <<<"$current_env_output"
current_env_full_output="$(
  BACKEND_SERVER_MODE=debug \
  BACKEND_SEED_EXPERIENCE_ACCOUNTS=true \
  BACKEND_SERVER_PORT=8080 \
  BACKEND_DATABASE_HOST=::1 \
  BACKEND_DATABASE_PORT=5432 \
  BACKEND_DATABASE_USER=developer \
  BACKEND_DATABASE_PASSWORD=not-used-by-dry-run \
  BACKEND_DATABASE_DBNAME=wangzhe_dev \
  BACKEND_DATABASE_SSLMODE=disable \
  bash "$full_reset_script" --dry-run
)"
grep -Fq '仅预览：没有连接数据库、没有备份、没有修改任何 schema 或数据。' <<<"$current_env_full_output"
grep -Fq -- '--current-env' "$script_dir/postgres-backup.sh"
grep -Fq '"$script_dir/dev-postgres-backup.sh" --backup-dir "$backup_dir"' "$reset_script"
grep -Fq 'age --recipient "$backup_recipient"' "$dev_backup_script"
grep -Fq 'age --decrypt --identity "$identity_file"' "$dev_backup_script"
[[ "$(grep -Fc '"$pg_restore_bin" --list' "$dev_backup_script")" == "2" ]] || {
  echo "本机开发备份必须同时验证原始 pg_dump 与加密回读内容" >&2
  exit 1
}
if grep -Eq '(^|[[:space:]])(find .*-delete|rm -rf)([[:space:]]|$)' "$dev_backup_script"; then
  echo "本机开发备份不得自动删除历史备份或递归删除目录" >&2
  exit 1
fi

# File mode cannot inherit a dangerous authorization that the file omitted.
if BACKEND_ALLOW_DEVELOPMENT_RESET=YES \
  BACKEND_DEVELOPMENT_RESET_DATABASE=wangzhe_dev \
  bash "$reset_script" --execute \
    --confirm 'RESET:wangzhe_dev:BUSINESS-DATA' \
    --backup-dir "$test_root/backups" "$test_root/debug.env" \
    >"$test_root/inherited-auth.out" 2>&1; then
  echo "环境文件错误地继承了 shell 中的开发重置授权" >&2
  exit 1
fi
grep -Fq '必须显式设置 BACKEND_ALLOW_DEVELOPMENT_RESET=YES' "$test_root/inherited-auth.out"
if BACKEND_ALLOW_DEVELOPMENT_RESET=YES \
  BACKEND_DEVELOPMENT_RESET_DATABASE=wangzhe_dev \
  bash "$full_reset_script" --execute \
    --confirm 'DROP:wangzhe_dev:REBUILD-PUBLIC-SCHEMA' \
    --backup-dir "$test_root/backups" "$test_root/debug.env" \
    >"$test_root/full-inherited-auth.out" 2>&1; then
  echo "环境文件错误地继承了 shell 中的完整重置授权" >&2
  exit 1
fi
grep -Fq '必须显式设置 BACKEND_ALLOW_DEVELOPMENT_RESET=YES' "$test_root/full-inherited-auth.out"

grep -Fq 'SET LOCAL search_path = pg_catalog, public;' "$reset_script"
grep -Fq 'FROM public.schema_migrations' "$reset_script"
grep -Fq 'FROM pg_catalog.pg_stat_activity' "$reset_script"
grep -Fq 'public.workspace_robot_settings' "$reset_script"
grep -Fq 'UPDATE public."user"' "$reset_script"
grep -Fq '彩票期号/同步状态、活动参与/剩余奖池、特殊号码发放计数和计划运行统计将复位。' <<<"$dry_run_output"
grep -Fq 'INSERT INTO public.development_reset_receipts' "$reset_script"
grep -Fq 'public.user_balance_transactions' "$reset_script"
grep -Fq 'SET LOCAL search_path = pg_catalog, public;' "$full_reset_script"
grep -Fq 'FROM public.schema_migrations' "$full_reset_script"
grep -Fq 'FROM pg_catalog.pg_stat_activity' "$full_reset_script"
grep -Fq 'DROP SCHEMA public CASCADE' "$full_reset_script"
grep -Fq 'CREATE SCHEMA public AUTHORIZATION CURRENT_USER' "$full_reset_script"
grep -Fq 'unset BACKEND_UPLOAD_DIR' "$full_reset_script"
grep -Fq ': "${BACKEND_UPLOAD_DIR:?执行完整重建时必须显式设置 BACKEND_UPLOAD_DIR}"' "$full_reset_script"
grep -Fq 'reset_validate_payment_qr_cleanup_target "$BACKEND_UPLOAD_DIR"' "$full_reset_script"
grep -Fq '"备份目录不能位于待清理的二维码目录内"' "$full_reset_script"
grep -Fq 'reset_archive_payment_qr_files "$BACKEND_UPLOAD_DIR" "$backup_file"' "$full_reset_script"
grep -Fq '"$script_dir/dev-postgres-backup.sh" --backup-dir "$backup_dir"' "$full_reset_script"
grep -Fq 'age --decrypt --identity "$restore_identity_file" --output "$restore_database_file" "$backup_file"' "$full_reset_script"
grep -Fq '"$restore_database_file"' "$full_reset_script"
grep -Fq "printf 'payment_qr_backup=%s\\n'" "$full_reset_script"
grep -Fq "printf 'payment_qr_backup_sha256=%s\\n'" "$full_reset_script"
grep -Fq "printf 'payment_qr_files_archived=%s\\n'" "$full_reset_script"
grep -Fq 'write_external_receipt "schema_rebuilt_qr_cleanup_pending"' "$full_reset_script"
grep -Fq 'reset_remove_payment_qr_files "$BACKEND_UPLOAD_DIR" "$payment_qr_archived_count"' "$full_reset_script"
grep -Fq "printf 'payment_qr_files_removed=%s\\n'" "$full_reset_script"
[[ "$(grep -Fc 'chmod 600 "$manifest"' "$script_dir/lib/dev-reset-safety.sh")" == "2" ]] || {
  echo "二维码扫描和删除 manifest 都必须显式限制为 0600" >&2
  exit 1
}

# A successful schema COMMIT must be recorded as cleanup-pending before any QR
# deletion. Only successful exact cleanup may advance the receipt to bootstrap.
full_commit_line="$(grep -n '^COMMIT;$' "$full_reset_script" | tail -n 1 | cut -d: -f1)"
full_qr_archive_line="$(grep -n 'reset_archive_payment_qr_files "$BACKEND_UPLOAD_DIR" "$backup_file"' "$full_reset_script" | cut -d: -f1)"
full_cleanup_pending_line="$(grep -n 'write_external_receipt "schema_rebuilt_qr_cleanup_pending"' "$full_reset_script" | cut -d: -f1)"
full_qr_remove_line="$(grep -n 'reset_remove_payment_qr_files "$BACKEND_UPLOAD_DIR" "$payment_qr_archived_count"' "$full_reset_script" | cut -d: -f1)"
full_bootstrap_pending_line="$(grep -n 'write_external_receipt "bootstrap_pending"' "$full_reset_script" | cut -d: -f1)"
[[ -n "$full_qr_archive_line" && -n "$full_commit_line" && -n "$full_cleanup_pending_line" && -n "$full_qr_remove_line" && -n "$full_bootstrap_pending_line" ]] &&
  (( full_qr_archive_line < full_commit_line &&
     full_commit_line < full_cleanup_pending_line &&
     full_cleanup_pending_line < full_qr_remove_line &&
     full_qr_remove_line < full_bootstrap_pending_line )) || {
  echo "完整重建必须在 schema 提交后先记录二维码清理 pending，仅清理成功后记录 bootstrap pending" >&2
  exit 1
}
grep -Fq 'reset_archive_payment_qr_files "$BACKEND_UPLOAD_DIR" "$backup_file"' "$reset_script"
grep -Fq 'reset_remove_payment_qr_files "$BACKEND_UPLOAD_DIR" "$payment_qr_archived_count"' "$reset_script"
grep -Fq "printf 'payment_qr_backup=%s\\n'" "$reset_script"
grep -Fq "printf 'payment_qr_backup_sha256=%s\\n'" "$reset_script"
grep -Fq "printf 'payment_qr_files_archived=%s\\n'" "$reset_script"
business_qr_archive_line="$(grep -n 'reset_archive_payment_qr_files "$BACKEND_UPLOAD_DIR" "$backup_file"' "$reset_script" | cut -d: -f1)"
business_commit_line="$(grep -n '^COMMIT;$' "$reset_script" | tail -n 1 | cut -d: -f1)"
business_qr_remove_line="$(grep -n 'reset_remove_payment_qr_files "$BACKEND_UPLOAD_DIR" "$payment_qr_archived_count"' "$reset_script" | cut -d: -f1)"
[[ -n "$business_qr_archive_line" && -n "$business_commit_line" && -n "$business_qr_remove_line" ]] &&
  (( business_qr_archive_line < business_commit_line && business_commit_line < business_qr_remove_line )) || {
  echo "业务重置必须先发布配套二维码归档，再提交数据库并精确删除文件" >&2
  exit 1
}
grep -Fq 'reset_validate_payment_qr_archive "$payment_qr_backup"' "$complete_receipt_script"
if grep -Eiq 'DROP[[:space:]]+DATABASE' "$full_reset_script"; then
  echo "完整开发重置不得删除数据库本身" >&2
  exit 1
fi

write_env "$test_root/release.env" release 127.0.0.1
if bash "$reset_script" --dry-run "$test_root/release.env" >"$test_root/release.out" 2>&1; then
  echo "release 环境错误地通过了开发重置保护" >&2
  exit 1
fi
grep -Fq '业务数据重置仅允许 debug 环境' "$test_root/release.out"
if bash "$full_reset_script" --dry-run "$test_root/release.env" >"$test_root/full-release.out" 2>&1; then
  echo "release 环境错误地通过了完整开发重置保护" >&2
  exit 1
fi
grep -Fq '完整重建仅允许 debug 环境' "$test_root/full-release.out"
if bash "$sentinel_script" --dry-run "$test_root/release.env" >"$test_root/sentinel-release.out" 2>&1; then
  echo "release 环境错误地通过了 sentinel 初始化保护" >&2
  exit 1
fi
grep -Fq 'sentinel 初始化仅允许 debug 环境' "$test_root/sentinel-release.out"

write_env "$test_root/test.env" test 127.0.0.1
if bash "$reset_script" --dry-run "$test_root/test.env" >"$test_root/partial-test.out" 2>&1; then
  echo "test 环境错误地通过了业务数据重置保护" >&2
  exit 1
fi
grep -Fq '业务数据重置仅允许 debug 环境' "$test_root/partial-test.out"
if bash "$full_reset_script" --dry-run "$test_root/test.env" >"$test_root/full-test.out" 2>&1; then
  echo "test 环境错误地通过了完整开发重置保护" >&2
  exit 1
fi
grep -Fq '完整重建仅允许 debug 环境' "$test_root/full-test.out"

write_env "$test_root/remote.env" debug 192.0.2.10
if bash "$reset_script" --dry-run "$test_root/remote.env" >"$test_root/remote.out" 2>&1; then
  echo "远程数据库错误地通过了开发重置保护" >&2
  exit 1
fi
grep -Fq '只允许连接本机 PostgreSQL' "$test_root/remote.out"
if bash "$full_reset_script" --dry-run "$test_root/remote.env" >"$test_root/full-remote.out" 2>&1; then
  echo "远程数据库错误地通过了完整开发重置保护" >&2
  exit 1
fi
grep -Fq '只允许连接本机 PostgreSQL' "$test_root/full-remote.out"

grep -Fq 'reject_unapproved_application_truncate' "$migration"
grep -Fq 'development reset receipts are immutable' "$migration"
grep -Fq "relation.relname <> 'development_reset_receipts'" "$migration"
grep -Fq 'CREATE TABLE IF NOT EXISTS public.workspace_migration_markers' "$marker_migration"
grep -Fq 'SELECT public.install_application_truncate_guards()' "$marker_migration"
grep -Fq 'server_system_identifier' "$identity_migration"
grep -Fq "version = '202608270012_reset_identity_receipts.sql'" "$reset_script"
grep -Fq "version = '202608270012_reset_identity_receipts.sql'" "$full_reset_script"
grep -Fq 'wangzhe_meta.development_reset_sentinel' "$reset_script"
grep -Fq 'wangzhe_meta.development_reset_sentinel' "$full_reset_script"
grep -Fq 'createdb --host' "$full_reset_script"
grep -Fq 'pg_restore --exit-on-error' "$full_reset_script"
grep -Fq 'dropdb --host' "$full_reset_script"
grep -Fq 'scratch_restore_verified=true' "$full_reset_script"
grep -Fq 'status=complete' "$complete_receipt_script"
grep -Fq '/ready' "$complete_receipt_script"
grep -Fq 'dev-reset-verify-bootstrap.sh' "$complete_receipt_script"
grep -Fq 'BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;' "$verify_script"
grep -Fq 'for migration_file in "$migration_dir"/*.sql' "$verify_script"
grep -Fq 'jsonb_to_recordset' "$verify_script"
grep -Fq 'sha256sum "$migration_file"' "$verify_script"
grep -Fq 'trg_reject_unapproved_application_truncate' "$verify_script"
if grep -Eq 'COUNT\(\*\) FROM schema_migrations\) <> 12|30 个彩种应各有 9 组默认赔率' "$verify_script"; then
  echo "只读重建验收不得保留旧迁移清单或自动默认赔率基线" >&2
  exit 1
fi
if bash "$verify_script" "$test_root/release.env" >"$test_root/verify-release.out" 2>&1; then
  echo "release 环境错误地通过了只读重建验收保护" >&2
  exit 1
fi
grep -Fq '只读重建验收仅允许 debug 环境' "$test_root/verify-release.out"
if bash "$verify_script" "$test_root/remote.env" >"$test_root/verify-remote.out" 2>&1; then
  echo "远程数据库错误地通过了只读重建验收保护" >&2
  exit 1
fi
grep -Fq '只读重建验收只允许连接本机 PostgreSQL' "$test_root/verify-remote.out"
grep -Fq 'BACKEND_DATABASE_SSLMODE' "$script_dir/postgres-backup.sh"
grep -Fq 'export PGSSLMODE' "$script_dir/postgres-backup.sh"
if grep -Eiq 'DROP[[:space:]]+(DATABASE|SCHEMA)|TRUNCATE[[:space:]]+TABLE[[:space:]]+schema_migrations' "$reset_script"; then
  echo "开发重置脚本不得删除数据库、schema 或迁移记录" >&2
  exit 1
fi

echo "开发数据重置静态保护检查通过"
