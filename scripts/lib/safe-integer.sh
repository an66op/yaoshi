#!/usr/bin/env bash

# Validate an untrusted database/API count before it enters Bash arithmetic.
# Bash recursively evaluates array subscripts in (( ... )), so merely placing a
# quoted string in an arithmetic expression can otherwise execute command
# substitutions supplied by a compromised upstream service.
require_decimal_count() {
  local label="$1"
  local value="${2-}"
  [[ "$value" =~ ^[0-9]+$ && ${#value} -le 18 ]] || {
    echo "$label 必须是最多 18 位的非负十进制整数" >&2
    return 1
  }
  printf '%s\n' "$value"
}
