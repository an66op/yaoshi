#!/usr/bin/env bash

# Strictly parse one release's non-secret encrypted-field capability contract.
# Callers receive values only through the three encryption_cap_* variables;
# the metadata file is data and is never sourced or evaluated as shell code.
load_release_encryption_capabilities() {
  local release_dir="$1" metadata line key value line_count=0
  local format_version="" read_versions="" write_version="" previous_key_fallback=""
  local seen_format=0 seen_read=0 seen_write=0 seen_previous=0
  local -a versions=()
  local version seen_version_1=0 seen_version_2=0

  metadata="$release_dir/FIELD_ENCRYPTION_CAPABILITIES"
  [[ -f "$metadata" && ! -L "$metadata" ]] || {
    echo "发布版本缺少加密信封能力元数据：$release_dir" >&2
    return 1
  }
  while IFS= read -r line || [[ -n "$line" ]]; do
    ((line_count += 1))
    [[ "$line" != *$'\r'* && "$line" == *=* ]] || {
      echo "加密信封能力元数据格式无效：$release_dir" >&2
      return 1
    }
    key="${line%%=*}"
    value="${line#*=}"
    case "$key" in
      format_version)
        (( seen_format == 0 )) || { echo "加密信封能力元数据字段重复：$release_dir" >&2; return 1; }
        seen_format=1
        format_version="$value"
        ;;
      read_versions)
        (( seen_read == 0 )) || { echo "加密信封能力元数据字段重复：$release_dir" >&2; return 1; }
        seen_read=1
        read_versions="$value"
        ;;
      write_version)
        (( seen_write == 0 )) || { echo "加密信封能力元数据字段重复：$release_dir" >&2; return 1; }
        seen_write=1
        write_version="$value"
        ;;
      previous_key_fallback)
        (( seen_previous == 0 )) || { echo "加密信封能力元数据字段重复：$release_dir" >&2; return 1; }
        seen_previous=1
        previous_key_fallback="$value"
        ;;
      *)
        echo "加密信封能力元数据包含未知字段：$release_dir" >&2
        return 1
        ;;
    esac
  done <"$metadata"
  (( line_count == 4 && seen_format == 1 && seen_read == 1 && seen_write == 1 && seen_previous == 1 )) || {
    echo "加密信封能力元数据字段不完整：$release_dir" >&2
    return 1
  }
  [[ "$format_version" == "1" ]] || { echo "不支持的加密能力元数据版本：$release_dir" >&2; return 1; }
  [[ "$read_versions" =~ ^[12](,[12])?$ ]] || { echo "加密信封读取版本无效：$release_dir" >&2; return 1; }
  IFS=',' read -r -a versions <<<"$read_versions"
  for version in "${versions[@]}"; do
    case "$version" in
      1) (( seen_version_1 == 0 )) || { echo "加密信封读取版本重复：$release_dir" >&2; return 1; }; seen_version_1=1 ;;
      2) (( seen_version_2 == 0 )) || { echo "加密信封读取版本重复：$release_dir" >&2; return 1; }; seen_version_2=1 ;;
      *) echo "加密信封读取版本无效：$release_dir" >&2; return 1 ;;
    esac
  done
  [[ "$write_version" == "1" || "$write_version" == "2" ]] || {
    echo "加密信封写入版本无效：$release_dir" >&2
    return 1
  }
  encryption_version_supported "$read_versions" "$write_version" || {
    echo "发布版本无法读取自身写入的加密信封：$release_dir" >&2
    return 1
  }
  [[ "$previous_key_fallback" == "true" || "$previous_key_fallback" == "false" ]] || {
    echo "历史密钥读取能力无效：$release_dir" >&2
    return 1
  }

  # These are the deliberate cross-file outputs of this parser.
  # shellcheck disable=SC2034
  encryption_cap_read_versions="$read_versions"
  # shellcheck disable=SC2034
  encryption_cap_write_version="$write_version"
  # shellcheck disable=SC2034
  encryption_cap_previous_key_fallback="$previous_key_fallback"
}

encryption_version_supported() {
  local version_list="$1" wanted="$2" item
  local -a items=()
  IFS=',' read -r -a items <<<"$version_list"
  for item in "${items[@]}"; do
    [[ "$item" == "$wanted" ]] && return 0
  done
  return 1
}
