#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
# shellcheck source=production-recovery-evidence-check.sh
source "$ROOT_DIR/scripts/production-recovery-evidence-check.sh"

fixture="$(mktemp -d "${TMPDIR:-/tmp}/wangzhe-recovery-evidence-test.XXXXXX")"
cleanup_fixture() { rm -rf -- "$fixture"; }
trap cleanup_fixture EXIT INT TERM

for required in openssl sha256sum; do
  command -v "$required" >/dev/null 2>&1 || { echo "缺少测试命令：$required" >&2; exit 1; }
done
grep -q -- '-rawin' < <(openssl pkeyutl -help 2>&1 || true) || {
  echo "提示：本机 OpenSSL 不支持 Ed25519/-rawin，仅执行恢复证据源码门禁" >&2
  bash -n "$ROOT_DIR/scripts/production-recovery-evidence-check.sh"
  rg -Fq 'RECOVERY_EVIDENCE_ENV_FILE=/etc/wangzhe/recovery-evidence.env' "$ROOT_DIR/scripts/production-recovery-evidence-check.sh"
  rg -Fq 'RECOVERY_EVIDENCE_RCLONE_CONFIG_FILE=/etc/wangzhe/recovery-evidence-read-rclone.conf' "$ROOT_DIR/scripts/production-recovery-evidence-check.sh"
  exit 0
}

openssl genpkey -algorithm ED25519 -out "$fixture/logical-private.pem" >/dev/null 2>&1
openssl pkey -in "$fixture/logical-private.pem" -pubout -out "$fixture/logical-public.pem" >/dev/null
openssl genpkey -algorithm ED25519 -out "$fixture/pitr-private.pem" >/dev/null 2>&1
openssl pkey -in "$fixture/pitr-private.pem" -pubout -out "$fixture/pitr-public.pem" >/dev/null
require_distinct_status_key_domains "$fixture/logical-public.pem" "$fixture/pitr-public.pem"
if require_distinct_status_key_domains "$fixture/logical-public.pem" "$fixture/logical-public.pem" >/dev/null 2>&1; then
  echo "恢复证据门禁接受了相同签名域" >&2
  exit 1
fi

now="$(date +%s)"
source_epoch=$((now - 600))
target_epoch=$((now - 900))
export RECOVERY_EVIDENCE_EXPECTED_DATABASE_NAME=wangzhe
export RECOVERY_EVIDENCE_EXPECTED_DATABASE_REMOTE_SOURCE=remote:wangzhe-production
export RECOVERY_EVIDENCE_EXPECTED_UPLOAD_REMOTE_SOURCE=remote:wangzhe-production
export RECOVERY_EVIDENCE_EXPECTED_PITR_REMOTE_SOURCE=remote:wangzhe-production/pitr/1234567890123456789
export RECOVERY_EVIDENCE_PITR_CLUSTER_ID=1234567890123456789

logical="$fixture/last-success.status"
cat >"$logical" <<EOF
status_schema=wangzhe.restore-drill.v2
outcome=success
scope=logical_database_and_uploads
completed_at_epoch=$now
completed_at_utc=2026-08-30T00:00:00Z
isolation=offsite_download_loopback_database_and_fixed_targets
database_host=loopback
database_source_name=wangzhe
database_backup_name=wangzhe-20260830-000000-1.dump.age
database_artifact_bytes=100
database_sha256=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
database_provenance_sha256=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
database_offsite_source=remote:wangzhe-production/wangzhe-20260830-000000-1.dump.age
database_restore=verified
schema_migrations=1
negative_balances=0
orphan_bets=0
upload_backup_name=uploads-20260830-000000-1.tar.age
upload_artifact_bytes=200
upload_sha256=cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
upload_provenance_sha256=dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
upload_offsite_source=remote:wangzhe-production/uploads-20260830-000000-1.tar.age
upload_target_luks_mount=/var/lib/wangzhe-restore
restore_work_luks_mount=/var/lib/wangzhe-restore
database_data_luks_mount=/var/lib/wangzhe-recovery-postgresql
upload_restore=verified
upload_manifest_entries=2
upload_restored_files=2
upload_restored_bytes=50
pitr_restore=not_in_scope
EOF

pitr="$fixture/last-pitr-success.status"
cat >"$pitr" <<EOF
format_version=2
pitr_completed=1
target_reached=1
completed_at_epoch=$now
target_at_epoch=$target_epoch
target_at_utc=2026-08-29 23:45:00+00
duration_seconds=60
drill_luks_mount=/var/lib/wangzhe-pitr-drill
source_generation=20260830-000000-1
source_remote_destination=remote:wangzhe-production/pitr/1234567890123456789
source_snapshot_sha256=eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee
source_synced_at_epoch=$source_epoch
source_basebackup_count=1
source_wal_count=2
source_wal_segment_count=2
basebackup_file=basebackup-20260830-000000-1.tar.age
basebackup_sha256=ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff
postgres_major=16
system_identifier=1234567890123456789
timeline_id=1
replay_lsn=0/16B6C50
replay_timestamp=2026-08-29 23:45:00+00
restored_wal_count=2
restored_wal_segment_count=2
first_restored_wal=000000010000000000000001
last_restored_wal=000000010000000000000002
wal_audit_sha256=abababababababababababababababababababababababababababababababab
schema_migrations=1
negative_balances=0
orphan_bets=0
EOF

sign_bundle() {
  local file="$1" private_key="$2"
  printf '%s  %s\n' "$(sha256sum "$file" | awk '{print $1}')" "$(basename "$file")" >"$file.sha256"
  openssl pkeyutl -sign -rawin -inkey "$private_key" -in "$file" -out "$file.sig"
}
sign_bundle "$logical" "$fixture/logical-private.pem"
sign_bundle "$pitr" "$fixture/pitr-private.pem"
verify_status_bundle "$logical" "$logical.sha256" "$logical.sig" last-success.status "$fixture/logical-public.pem" logical
verify_status_bundle "$pitr" "$pitr.sha256" "$pitr.sig" last-pitr-success.status "$fixture/pitr-public.pem" pitr

phase_epoch=$((now - 1200))
phase_payload="$fixture/phase.payload"
phase_marker="$fixture/maintenance"
cat >"$phase_payload" <<EOF
schema=wangzhe.first-install-phase1.v2
status=awaiting-recovery
manifest_sha256=1212121212121212121212121212121212121212121212121212121212121212
release_id=bootstrap-1
backup_completed_at_epoch=$phase_epoch
database_artifact_name=wangzhe-20260830-000000-1.dump.age
database_cipher_sha256=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
upload_artifact_name=uploads-20260830-000000-1.tar.age
upload_cipher_sha256=cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
basebackup_artifact_name=basebackup-20260830-000000-1.tar.age
basebackup_cipher_sha256=ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff
wal_inventory_sha256=1313131313131313131313131313131313131313131313131313131313131313
EOF
phase_payload_sha="$(sha256sum "$phase_payload" | awk '{print $1}')"
{ printf 'wangzhe-first-install-phase1:v2:%s\n' "$phase_payload_sha"; cat "$phase_payload"; } >"$phase_marker"
chmod 0644 "$phase_marker"
strict_env_stat() {
  case "$1" in
    %u) printf '%s\n' 0 ;;
    %a) printf '%s\n' 644 ;;
    %h) printf '%s\n' 1 ;;
    %s) wc -c <"$3" | tr -d '[:space:]' ;;
    *) return 1 ;;
  esac
}
load_first_install_evidence_binding "$phase_marker"
# Assigned by the production function sourced above.
# shellcheck disable=SC2154
[[ "$first_install_evidence_required" == 1 && "$first_install_evidence_not_before_epoch" == "$phase_epoch" ]] || {
  echo "首次安装恢复证据绑定标记未被严格加载" >&2
  exit 1
}
validate_logical_recovery_evidence "$logical" "$now" 2592000
validate_pitr_recovery_evidence "$pitr" "$now" 2592000 172800

first_install_evidence_database_sha="$(printf '0%.0s' {1..64})"
if validate_logical_recovery_evidence "$logical" "$now" 2592000 >/dev/null 2>&1; then
  echo "逻辑恢复门禁接受了非阶段 1 数据库制品" >&2
  exit 1
fi
export first_install_evidence_database_sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
first_install_evidence_base_sha="$(printf '0%.0s' {1..64})"
if validate_pitr_recovery_evidence "$pitr" "$now" 2592000 172800 >/dev/null 2>&1; then
  echo "PITR 门禁接受了非阶段 1 基础备份" >&2
  exit 1
fi
export first_install_evidence_base_sha=ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff
first_install_evidence_not_before_epoch="$now"
if validate_logical_recovery_evidence "$logical" "$now" 2592000 >/dev/null 2>&1 || \
   validate_pitr_recovery_evidence "$pitr" "$now" 2592000 172800 >/dev/null 2>&1; then
  echo "恢复门禁接受了阶段 1 备份完成前或同时完成的旧证据" >&2
  exit 1
fi
first_install_evidence_not_before_epoch="$phase_epoch"

if verify_status_bundle "$logical" "$logical.sha256" "$logical.sig" last-success.status "$fixture/pitr-public.pem" logical >/dev/null 2>&1; then
  echo "PITR 签名域错误接受了逻辑恢复证据" >&2
  exit 1
fi
if validate_logical_recovery_evidence "$logical" "$((now + 2592001))" 2592000 >/dev/null 2>&1; then
  echo "逻辑恢复证据新鲜度门禁接受了过期状态" >&2
  exit 1
fi
export RECOVERY_EVIDENCE_EXPECTED_DATABASE_REMOTE_SOURCE=remote:staging
if validate_logical_recovery_evidence "$logical" "$now" 2592000 >/dev/null 2>&1; then
  echo "逻辑恢复门禁接受了错误生产 remote" >&2
  exit 1
fi
export RECOVERY_EVIDENCE_EXPECTED_DATABASE_REMOTE_SOURCE=remote:wangzhe-production
export RECOVERY_EVIDENCE_PITR_CLUSTER_ID=9999999999999999999
if validate_pitr_recovery_evidence "$pitr" "$now" 2592000 172800 >/dev/null 2>&1; then
  echo "PITR 门禁接受了错误生产集群" >&2
  exit 1
fi

bash -n "$ROOT_DIR/scripts/production-recovery-evidence-check.sh"
rg -Fq '此门禁不接受命令行覆盖' "$ROOT_DIR/scripts/production-recovery-evidence-check.sh"
rg -Fq 'RECOVERY_EVIDENCE_ENV_FILE=/etc/wangzhe/recovery-evidence.env' "$ROOT_DIR/scripts/production-recovery-evidence-check.sh"
rg -Fq 'RECOVERY_EVIDENCE_RCLONE_CONFIG_FILE=/etc/wangzhe/recovery-evidence-read-rclone.conf' "$ROOT_DIR/scripts/production-recovery-evidence-check.sh"
rg -Fq 'logical_fingerprint" != "$pitr_fingerprint' "$ROOT_DIR/scripts/production-recovery-evidence-check.sh"
rg -Fq 'RECOVERY_EVIDENCE_LOGICAL_MAX_AGE_SECONDS <= 3024000' "$ROOT_DIR/scripts/production-recovery-evidence-check.sh"
rg -Fq 'RECOVERY_EVIDENCE_PITR_MAX_AGE_SECONDS <= 3024000' "$ROOT_DIR/scripts/production-recovery-evidence-check.sh"

deploy_script="$ROOT_DIR/scripts/production-deploy.sh"
makefile="$ROOT_DIR/Makefile"
rg -Fq 'production-backup-integrity.sh production-recovery-evidence-check.sh' "$deploy_script"
rg -Fq 'scripts/production-recovery-evidence-check.sh' "$deploy_script" "$makefile"
rg -Fq 'timeout 10m "$TRUSTED_SCRIPT_DIR/production-recovery-evidence-check.sh"' "$deploy_script"
rg -Fq 'bash scripts/recovery-evidence-check-static-test.sh' "$makefile"
evidence_line="$(rg -n 'timeout 10m \"\$TRUSTED_SCRIPT_DIR/production-recovery-evidence-check.sh\"' "$deploy_script" | cut -d: -f1)"
release_mutation_line="$(rg -n '^install -d -o root -g root -m 0755 /opt/wangzhe \"\$RELEASE_ROOT\"$' "$deploy_script" | cut -d: -f1)"
current_switch_line="$(rg -n '^mv -Tf \"\$link_tmp\" \"\$CURRENT_LINK\"$' "$deploy_script" | cut -d: -f1)"
[[ "$evidence_line" =~ ^[0-9]+$ && "$release_mutation_line" =~ ^[0-9]+$ && "$current_switch_line" =~ ^[0-9]+$ && \
   "$evidence_line" -lt "$release_mutation_line" && "$evidence_line" -lt "$current_switch_line" ]] || {
  echo "恢复演练证据必须在创建 release 或切换 current 之前 fail closed" >&2
  exit 1
}
echo "生产恢复证据门禁静态与篡改测试通过"
