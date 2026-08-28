#!/usr/bin/env bash

# Load the systemd-compatible BACKEND_* environment file without evaluating it
# as shell code. This is intentionally a parser, not `source`: a database
# password containing `$()` or backticks must remain data even when a readiness
# or backup command is run as root.

trim_backend_env_value() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

backend_env_stat() {
  local format_linux="$1"
  local format_bsd="$2"
  local file="$3"
  if stat -c "$format_linux" "$file" >/dev/null 2>&1; then
    stat -c "$format_linux" "$file"
  else
    stat -f "$format_bsd" "$file"
  fi
}

load_backend_env() {
  local env_file="$1"
  [[ -n "$env_file" && -f "$env_file" && ! -L "$env_file" ]] || {
    echo "环境文件不存在、不是普通文件或是符号链接：$env_file" >&2
    return 1
  }

  local mode owner mode_value expected_owner
  mode="$(backend_env_stat '%a' '%Lp' "$env_file")"
  owner="$(backend_env_stat '%u' '%u' "$env_file")"
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || { echo "无法确认环境文件权限：$env_file" >&2; return 1; }
  mode_value=$((8#$mode))
  (( (mode_value & 077) == 0 )) || {
    echo "环境文件不能允许组或其他用户访问（要求 600/400）：$env_file" >&2
    return 1
  }
  expected_owner="${EUID:-$(id -u)}"
  [[ "$owner" == "$expected_owner" ]] || {
    echo "环境文件必须属于执行检查的用户（当前 uid=$expected_owner，文件 uid=$owner）" >&2
    return 1
  }

  local line key value first last line_number=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    line_number=$((line_number + 1))
    line="${line%$'\r'}"
    line="$(trim_backend_env_value "$line")"
    [[ -z "$line" || "$line" == \#* ]] && continue
    [[ "$line" == *=* ]] || { echo "环境文件第 $line_number 行缺少 =" >&2; return 1; }
    key="$(trim_backend_env_value "${line%%=*}")"
    value="$(trim_backend_env_value "${line#*=}")"
    [[ "$key" =~ ^BACKEND_[A-Z0-9_]+$ ]] || {
      echo "环境文件第 $line_number 行包含不允许的变量名：$key" >&2
      return 1
    }
    if (( ${#value} >= 2 )); then
      first="${value:0:1}"
      last="${value: -1}"
      if [[ ( "$first" == '"' && "$last" == '"' ) || ( "$first" == "'" && "$last" == "'" ) ]]; then
        value="${value:1:${#value}-2}"
      fi
    fi
    export "$key=$value"
  done < "$env_file"
}
