#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: scripts/release-integrity.sh generate|verify RELEASE_DIR"
}
[[ $# -eq 2 ]] || { usage >&2; exit 2; }
mode="$1"
release_dir="$(cd "$2" && pwd -P)"
[[ "$release_dir" != "/" ]] || { echo "拒绝对根目录生成清单" >&2; exit 1; }
manifest="$release_dir/SHA256SUMS"

reject_non_regular_entries() {
  local unsafe entry
  unsafe="$(cd "$release_dir" && find . -mindepth 1 ! -type d ! -type f -print -quit)"
  [[ -z "$unsafe" ]] || { echo "发布包包含符号链接或特殊文件：$unsafe" >&2; exit 1; }
  while IFS= read -r -d '' entry; do
    [[ "$entry" != *$'\n'* && "$entry" != *$'\r'* ]] || {
      echo "发布包文件名包含换行符，无法写入安全清单" >&2
      exit 1
    }
  done < <(cd "$release_dir" && find . -mindepth 1 -print0)
}

if command -v sha256sum >/dev/null 2>&1; then
  hash_file() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
  hash_file() { shasum -a 256 "$1" | awk '{print $1}'; }
else
  echo "缺少 sha256sum 或 shasum" >&2
  exit 1
fi

case "$mode" in
  generate)
    reject_non_regular_entries
    tmp_manifest="$release_dir/.SHA256SUMS.$$"
    cleanup_generate() { rm -f -- "$tmp_manifest"; }
    trap cleanup_generate EXIT INT TERM
    : >"$tmp_manifest"
    while IFS= read -r path; do
      relative="./${path#./}"
      checksum="$(hash_file "$release_dir/${path#./}")"
      printf '%s  %s\n' "$checksum" "$relative" >>"$tmp_manifest"
    done < <(cd "$release_dir" && find . -type f ! -path './SHA256SUMS' ! -name '.SHA256SUMS.*' -print | LC_ALL=C sort)
    [[ -s "$tmp_manifest" ]] || { echo "发布目录为空" >&2; exit 1; }
    mv "$tmp_manifest" "$manifest"
    ;;
  verify)
    reject_non_regular_entries
    [[ -f "$manifest" && ! -L "$manifest" ]] || { echo "发布包缺少 SHA256SUMS" >&2; exit 1; }
    fixture_dir="$(mktemp -d)"
    cleanup_verify() { rm -rf -- "$fixture_dir"; }
    trap cleanup_verify EXIT INT TERM
    manifest_paths="$fixture_dir/manifest-paths"
    actual_paths="$fixture_dir/actual-paths"
    : >"$manifest_paths"
    while IFS= read -r line || [[ -n "$line" ]]; do
      checksum="${line%%  *}"
      path="${line#*  }"
      [[ "$checksum" =~ ^[0-9a-f]{64}$ && "$path" == ./* && "$path" != *'/../'* && "$path" != './..' ]] || {
        echo "SHA256SUMS 包含不安全或无效条目" >&2
        exit 1
      }
      printf '%s\n' "$path" >>"$manifest_paths"
      actual="$(hash_file "$release_dir/${path#./}")"
      [[ "$actual" == "$checksum" ]] || { echo "发布文件校验失败：$path" >&2; exit 1; }
    done <"$manifest"
    [[ -s "$manifest_paths" ]] || { echo "SHA256SUMS 为空" >&2; exit 1; }
    LC_ALL=C sort "$manifest_paths" -o "$manifest_paths"
    [[ "$(uniq -d "$manifest_paths" | wc -l | tr -d ' ')" == "0" ]] || { echo "SHA256SUMS 存在重复路径" >&2; exit 1; }
    (cd "$release_dir" && find . -type f ! -path './SHA256SUMS' -print | LC_ALL=C sort) >"$actual_paths"
    cmp -s "$manifest_paths" "$actual_paths" || { echo "发布包文件清单与 SHA256SUMS 不一致" >&2; exit 1; }
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

echo "发布包完整性检查通过：$mode"
