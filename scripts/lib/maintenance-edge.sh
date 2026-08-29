#!/usr/bin/env bash

# Marker ownership is scoped to one deploy/rollback process. A pre-existing
# marker belongs to an operator or an earlier failed operation and must never
# be replaced or removed by the current command.
maintenance_was_active=0
maintenance_marker_created=0
maintenance_marker_token=""

maintenance_marker_exists() {
  [[ $# -eq 1 ]] || return 1
  [[ -e "$1" || -L "$1" ]]
}

capture_maintenance_marker_state() {
  [[ $# -eq 1 ]] || return 1
  maintenance_was_active=0
  maintenance_marker_created=0
  maintenance_marker_token=""
  if maintenance_marker_exists "$1"; then
    maintenance_was_active=1
  fi
}

maintenance_marker_owned_by() {
  [[ $# -eq 2 && -n "$2" && -f "$1" && ! -L "$1" ]] || return 1
  local recorded_token=""
  IFS= read -r recorded_token <"$1" || return 1
  [[ "$recorded_token" == "$2" ]]
}

create_maintenance_marker_atomic() {
  [[ $# -eq 1 ]] || return 1
  local marker="$1" marker_dir marker_name temporary token
  [[ "$marker" == /* && "$marker" != "/" ]] || return 1
  marker_dir="${marker%/*}"
  marker_name="${marker##*/}"
  [[ -n "$marker_dir" ]] || marker_dir=/
  [[ -n "$marker_name" && "$marker_name" != "." && "$marker_name" != ".." ]] || return 1
  [[ -d "$marker_dir" && ! -L "$marker_dir" ]] || return 1
  maintenance_marker_exists "$marker" && return 2

  temporary="$(mktemp "$marker_dir/.${marker_name}.tmp.XXXXXX")" || return 1
  token="wangzhe-maintenance:${marker_name}:$$:${temporary##*/}"
  if ! printf '%s\n' "$token" >"$temporary" || ! chmod 0644 "$temporary"; then
    rm -f -- "$temporary"
    return 1
  fi

  # -n is essential: an operator may activate maintenance after the initial
  # state check, and their marker must not be overwritten. Moving a prepared
  # file from the same directory makes the successful creation atomic.
  if ! mv -n -- "$temporary" "$marker"; then
    rm -f -- "$temporary"
    return 1
  fi
  if [[ -e "$temporary" || -L "$temporary" ]]; then
    rm -f -- "$temporary"
    return 2
  fi
  if ! maintenance_marker_owned_by "$marker" "$token"; then
    return 1
  fi
  maintenance_marker_token="$token"
}

ensure_maintenance_marker() {
  [[ $# -eq 1 ]] || return 1
  local marker="$1"
  if (( maintenance_was_active == 1 )); then
    # Never recreate a marker that existed when the operation began. Its
    # disappearance is an external state change and must stop the operation.
    maintenance_marker_exists "$marker"
    return
  fi
  if maintenance_marker_exists "$marker"; then
    # Another operator activated maintenance after capture. It is not ours.
    return 0
  fi
  if create_maintenance_marker_atomic "$marker"; then
    maintenance_marker_created=1
    return 0
  fi
  # A no-clobber move may lose a race to an external marker. Accept that
  # fail-closed state, but never claim ownership of it.
  if maintenance_marker_exists "$marker"; then
    maintenance_marker_token=""
    return 0
  fi
  return 1
}

finish_maintenance_marker() {
  [[ $# -eq 1 ]] || return 1
  local marker="$1"
  (( maintenance_marker_created == 1 )) || return 0
  maintenance_marker_owned_by "$marker" "$maintenance_marker_token" || return 1
  rm -f -- "$marker"
  maintenance_marker_created=0
  maintenance_marker_token=""
}

# Prove that the currently running Nginx has already placed both public
# origins behind the persistent maintenance marker. This must run before any
# release symlink is switched.
verify_maintenance_headers() {
  local label="$1" headers="$2" lower_headers status_code
  lower_headers="$(printf '%s' "$headers" | tr -d '\r' | tr '[:upper:]' '[:lower:]')"
  status_code="$(printf '%s\n' "$lower_headers" | awk '/^http\// { status=$2 } END { print status }')"
  [[ "$status_code" == "503" ]] || {
    echo "$label 未进入维护模式：期望 503，实际 ${status_code:-未知}" >&2
    return 1
  }
  grep -q '^strict-transport-security:.*max-age=' <<<"$lower_headers" || { echo "$label 的维护响应缺少 HSTS" >&2; return 1; }
  grep -q '^content-security-policy:' <<<"$lower_headers" || { echo "$label 的维护响应缺少 CSP" >&2; return 1; }
  grep -q '^x-content-type-options:[[:space:]]*nosniff' <<<"$lower_headers" || { echo "$label 的维护响应缺少 nosniff" >&2; return 1; }
}

verify_maintenance_origin() {
  local url="$1" headers authority host port local_headers
  [[ "$url" =~ ^https://[A-Za-z0-9.-]+(:[0-9]+)?$ ]] || {
    echo "维护模式检查只接受 HTTPS Origin：$url" >&2
    return 1
  }
  headers="$(curl --noproxy '*' -sSI --max-time 10 "$url/")" || {
    echo "维护模式检查无法连接：$url" >&2
    return 1
  }
  verify_maintenance_headers "$url 公网入口" "$headers" || return 1

  # Also bypass DNS/CDN and hit the local TLS listener. The public check alone
  # could otherwise mistake an unrelated upstream 503 for proof that this
  # host's currently loaded Nginx configuration honors the marker.
  authority="${url#https://}"
  host="${authority%%:*}"
  port=443
  [[ "$authority" == *:* ]] && port="${authority##*:}"
  local_headers="$(curl --noproxy '*' -sSI --max-time 10 --resolve "$host:$port:127.0.0.1" "$url/")" || {
    echo "无法直接验证本机 Nginx 维护模式：$url" >&2
    return 1
  }
  verify_maintenance_headers "$url 本机 Nginx" "$local_headers" || return 1
}

verify_maintenance_edge() {
  [[ $# -eq 2 ]] || { echo "维护模式检查需要用户端和管理端两个 Origin" >&2; return 1; }
  verify_maintenance_origin "$1" || return 1
  verify_maintenance_origin "$2" || return 1
}
