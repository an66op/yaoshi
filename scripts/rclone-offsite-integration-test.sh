#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/encrypted-backup.sh
source "$SCRIPT_DIR/lib/encrypted-backup.sh"
for command_name in awk basename chmod date find grep mktemp mv openssl rclone sha256sum stat; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done

fixture_dir="$(mktemp -d "${TMPDIR:-/tmp}/wangzhe-rclone.XXXXXX")"
fixture_dir="$(cd -P "$fixture_dir" && pwd -P)"
cleanup() {
  case "$(basename "$fixture_dir")" in
    wangzhe-rclone.*) find "$fixture_dir" -depth -delete ;;
  esac
}
trap cleanup EXIT INT TERM
config_file="$fixture_dir/rclone.conf"
target="$fixture_dir/sample.dump.age"
checksum_file="$target.sha256"
marker_partial="$target.offsite-ok.partial"
provenance_file="$target.provenance"
signature_file="$target.provenance.sig"
signing_key="$fixture_dir/provenance-private.pem"
remote_destination="offsite:$fixture_dir/remote"
printf '[offsite]\ntype = local\n' >"$config_file"
chmod 0600 "$config_file"
printf 'encrypted-test-payload\n' >"$target"
write_backup_checksum "$target" "$checksum_file"
openssl genpkey -algorithm ED25519 -out "$signing_key"
chmod 0600 "$signing_key"
write_backup_provenance "$target" "$remote_destination" database wangzhe "$(date +%s)" \
  "$signing_key" "$provenance_file" "$signature_file"

sync_backup_offsite "$target" "$checksum_file" "$remote_destination" "$marker_partial" "$config_file" \
  "$provenance_file" "$signature_file"
mv "$marker_partial" "$target.offsite-ok"
validate_offsite_marker "$target" "$remote_destination"
[[ -s "$fixture_dir/remote/$(basename "$target")" && -s "$fixture_dir/remote/$(basename "$checksum_file")" ]]
[[ -s "$fixture_dir/remote/$(basename "$provenance_file")" && -s "$fixture_dir/remote/$(basename "$signature_file")" ]]
(
  cd "$fixture_dir/remote"
  sha256sum --check "$(basename "$checksum_file")" >/dev/null
)
echo "本地 rclone fixture 上传与全量 SHA-256 回读逻辑测试通过（不代表真实异机对象存储验收）"

# Reproduce the remote-only WAL restore staging shape. The random directory is
# private, while the file inside keeps the signed artifact basename. This is
# security-critical because provenance binds artifact_name as well as content.
wal_name=000000010000000000000001.age
wal_cluster=7399912345678901234
wal_target="$fixture_dir/$wal_name"
wal_checksum="$wal_target.sha256"
wal_marker_partial="$wal_target.offsite-ok.partial"
wal_remote_destination="offsite:$fixture_dir/remote-pitr/$wal_cluster"
printf 'encrypted-wal-test-payload\n' >"$wal_target"
write_backup_checksum "$wal_target" "$wal_checksum"
write_backup_provenance "$wal_target" "$wal_remote_destination" pitr-wal "$wal_cluster" "$(date +%s)" \
  "$signing_key" "$wal_target.provenance" "$wal_target.provenance.sig"
sync_backup_offsite "$wal_target" "$wal_checksum" "$wal_remote_destination" "$wal_marker_partial" "$config_file" \
  "$wal_target.provenance" "$wal_target.provenance.sig"
mv "$wal_marker_partial" "$wal_target.offsite-ok"

wal_staging_dir="$(mktemp -d "$fixture_dir/.restore-wal.XXXXXX")"
staged_wal="$wal_staging_dir/$wal_name"
for suffix in '' .sha256 .provenance .provenance.sig; do
  rclone --config "$config_file" copyto "${wal_remote_destination%/}/$wal_name$suffix" "$staged_wal$suffix" --no-traverse
done
validate_encrypted_backup_and_manifest "$staged_wal"
verify_backup_provenance "$staged_wal" pitr-wal "$wal_cluster" "${wal_remote_destination%/}/$wal_name" \
  "$signing_key" private
echo "本地 rclone fixture 的 WAL 随机暂存目录/原始文件名来源验签逻辑测试通过"
