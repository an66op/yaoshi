#!/usr/bin/env bash

reset_sha256() {
  local value="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s' "$value" | sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    printf '%s' "$value" | shasum -a 256 | awk '{print $1}'
  else
    echo "缺少 sha256sum 或 shasum" >&2
    return 1
  fi
}

reset_file_sha256() {
  local file="$1"
  [[ -f "$file" && ! -L "$file" ]] || {
    echo "无法计算非普通文件的 SHA-256：$file" >&2
    return 1
  }
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
  else
    echo "缺少 sha256sum 或 shasum" >&2
    return 1
  fi
}

reset_stdin_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 | awk '{print $1}'
  else
    echo "缺少 sha256sum 或 shasum" >&2
    return 1
  fi
}

reset_psql() {
  PGPASSWORD="$BACKEND_DATABASE_PASSWORD" \
  PGSSLMODE="$BACKEND_DATABASE_SSLMODE" \
  psql \
    --host "$BACKEND_DATABASE_HOST" \
    --port "$BACKEND_DATABASE_PORT" \
    --username "$BACKEND_DATABASE_USER" \
    --dbname "$BACKEND_DATABASE_DBNAME" \
    --no-psqlrc --set ON_ERROR_STOP=1 "$@"
}

reset_assert_backend_port_stopped() {
  if lsof -nP -iTCP:"$BACKEND_SERVER_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "后端端口 $BACKEND_SERVER_PORT 仍有监听进程；请先停止后端" >&2
    return 1
  fi
}

# Emits one tab-separated line:
# system_identifier, server_address, server_port, user_count,
# total_balance_cents, balance_transaction_count.
reset_verified_identity() {
  local token_sha256="$1"
  local require_quiescent="${2:-true}"
  [[ "$token_sha256" =~ ^[0-9a-f]{64}$ ]] || { echo "开发 sentinel token 摘要不正确" >&2; return 1; }
  [[ "$require_quiescent" == "true" || "$require_quiescent" == "false" ]] || return 1

  PGAPPNAME="wangzhe-dev-reset-identity" reset_psql \
    --quiet --tuples-only --no-align --field-separator=$'\t' \
    --set expected_database="$BACKEND_DATABASE_DBNAME" \
    --set sentinel_token_sha256="$token_sha256" \
    --set require_quiescent="$require_quiescent" <<'SQL'
SET search_path = pg_catalog, public;
-- Store the guard values without emitting result rows.  This function's stdout
-- is a machine-readable single TSV row; ordinary SELECT set_config(...) calls
-- would prepend three lines and corrupt the caller's snapshot comparison.
SELECT pg_catalog.set_config('wangzhe.reset_expected_database', :'expected_database', false) AS reset_expected_database
\gset
SELECT pg_catalog.set_config('wangzhe.reset_expected_token_sha256', :'sentinel_token_sha256', false) AS reset_expected_token
\gset
SELECT pg_catalog.set_config('wangzhe.reset_require_quiescent', :'require_quiescent', false) AS reset_require_quiescent
\gset

DO $$
DECLARE
    live_system_identifier text;
    live_address text;
    live_port integer;
    other_sessions integer;
BEGIN
    IF pg_catalog.current_database() <> pg_catalog.current_setting('wangzhe.reset_expected_database') THEN
        RAISE EXCEPTION 'database identity does not match the authorized target';
    END IF;
    IF pg_catalog.to_regclass('wangzhe_meta.development_reset_sentinel') IS NULL THEN
        RAISE EXCEPTION 'development reset sentinel is not initialized';
    END IF;
    IF pg_catalog.to_regclass('public.schema_migrations') IS NULL OR NOT EXISTS (
        SELECT 1 FROM public.schema_migrations
        WHERE version = '202608270012_reset_identity_receipts.sql'
    ) THEN
        RAISE EXCEPTION 'latest development reset migrations are not applied';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM pg_catalog.pg_class relation
        JOIN pg_catalog.pg_namespace namespace ON namespace.oid = relation.relnamespace
        WHERE relation.relkind IN ('r', 'p')
          AND namespace.nspname NOT IN ('public', 'wangzhe_meta', 'pg_catalog', 'information_schema')
          AND namespace.nspname NOT LIKE 'pg_toast%'
          AND namespace.nspname NOT LIKE 'pg_temp%'
          AND (
              relation.relname ~ '^(lottery|workspace|member|chat|user|data|admin|agent|room|ops|special|entertainment|wallet|rebate|plan)_'
              OR relation.relname IN ('user', 'workspaces', 'system_settings', 'schema_migrations', 'activity_participations')
          )
    ) THEN
        RAISE EXCEPTION 'Wangzhe application tables exist outside public schema';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM pg_catalog.pg_class relation
        JOIN pg_catalog.pg_namespace namespace ON namespace.oid = relation.relnamespace
        WHERE namespace.nspname = 'wangzhe_meta'
          AND relation.relkind IN ('r', 'p')
          AND relation.relname <> 'development_reset_sentinel'
    ) THEN
        RAISE EXCEPTION 'wangzhe_meta contains an unapproved table';
    END IF;

    SELECT system_identifier::text INTO live_system_identifier
    FROM pg_catalog.pg_control_system();
    live_address := COALESCE(pg_catalog.inet_server_addr()::text, 'local-socket');
    live_port := COALESCE(pg_catalog.inet_server_port(), 0);
    IF NOT EXISTS (
        SELECT 1 FROM wangzhe_meta.development_reset_sentinel sentinel
        WHERE sentinel.singleton
          AND sentinel.database_name = pg_catalog.current_database()
          AND sentinel.system_identifier = live_system_identifier
          AND sentinel.server_address = live_address
          AND sentinel.server_port = live_port
          AND sentinel.token_sha256 = pg_catalog.current_setting('wangzhe.reset_expected_token_sha256')
    ) THEN
        RAISE EXCEPTION 'development reset sentinel does not match this physical database';
    END IF;

    IF pg_catalog.current_setting('wangzhe.reset_require_quiescent') = 'true' THEN
        SELECT COUNT(*) INTO other_sessions
        FROM pg_catalog.pg_stat_activity
        WHERE datname = pg_catalog.current_database()
          AND pid <> pg_catalog.pg_backend_pid()
          AND backend_type = 'client backend';
        IF other_sessions <> 0 THEN
            RAISE EXCEPTION 'database still has % other client session(s)', other_sessions;
        END IF;
    END IF;
END $$;

SELECT control.system_identifier::text,
       COALESCE(pg_catalog.inet_server_addr()::text, 'local-socket'),
       COALESCE(pg_catalog.inet_server_port(), 0),
       (SELECT COUNT(*) FROM public."user"),
       (SELECT COALESCE(SUM(balance_cents), 0) FROM public."user"),
       (SELECT COUNT(*) FROM public.user_balance_transactions)
FROM pg_catalog.pg_control_system() control;
SQL
}

reset_identity_matches() {
  local before="$1" after="$2"
  [[ -n "$before" && "$before" == "$after" ]] || {
    echo "备份期间数据库身份或关键账务计数发生变化，拒绝重置" >&2
    return 1
  }
}

reset_write_payment_qr_database_reference_manifest() {
  local output="$1" entry validation_error=""
  if ! reset_psql --quiet --tuples-only --no-align --command \
    "SELECT '.private/member-payment-qr/' || workspace_id::text || '/' || user_id::text || '/' || qr_code_file FROM public.member_payment_accounts WHERE qr_code_file IS NOT NULL ORDER BY workspace_id, user_id, qr_code_file, id" >"$output"; then
    echo "无法读取数据库二维码引用清单" >&2
    return 1
  fi
  while IFS= read -r entry; do
    [[ "$entry" =~ ^\.private/member-payment-qr/[1-9][0-9]*/[1-9][0-9]*/[0-9a-f]{32}\.png$ ]] || {
      validation_error="数据库包含不安全的二维码文件引用"
      break
    }
  done <"$output"
  if [[ -n "$validation_error" ]]; then
    echo "$validation_error" >&2
    return 1
  fi
}

# Before a reset snapshot is accepted, require a one-to-one match between all
# active/deleted-queue database references and the controlled files on disk.
# This fails closed on crash-orphans and missing files rather than producing a
# companion archive that cannot restore a consistent database state.
reset_validate_payment_qr_database_file_consistency() {
  local upload_root="$1"
  reset_validate_payment_qr_cleanup_target "$upload_root" || return 1
  (
    set -euo pipefail
    local database_manifest="" file_scan="" file_manifest="" database_sorted="" file_sorted=""
    local entry relative temporary
    cleanup_payment_qr_consistency() {
      for temporary in "$database_manifest" "$file_scan" "$file_manifest" "$database_sorted" "$file_sorted"; do
        [[ -z "$temporary" ]] || rm -f -- "$temporary"
      done
    }
    trap cleanup_payment_qr_consistency EXIT INT TERM
    database_manifest="$(mktemp "$upload_root/.member-payment-qr-db-refs.XXXXXXXX")"
    file_scan="$(mktemp "$upload_root/.member-payment-qr-file-scan.XXXXXXXX")"
    file_manifest="$(mktemp "$upload_root/.member-payment-qr-file-refs.XXXXXXXX")"
    database_sorted="$(mktemp "$upload_root/.member-payment-qr-db-sorted.XXXXXXXX")"
    file_sorted="$(mktemp "$upload_root/.member-payment-qr-file-sorted.XXXXXXXX")"
    chmod 600 "$database_manifest" "$file_scan" "$file_manifest" "$database_sorted" "$file_sorted"
    reset_write_payment_qr_database_reference_manifest "$database_manifest"
    if [[ -d "$RESET_PAYMENT_QR_DIRECTORY" ]]; then
      if ! find "$RESET_PAYMENT_QR_DIRECTORY" -xdev -type f -print0 >"$file_scan"; then
        echo "无法枚举二维码文件一致性清单" >&2
        exit 1
      fi
    fi
    while IFS= read -r -d '' entry; do
      relative="${entry#"$RESET_PAYMENT_QR_DIRECTORY"/}"
      [[ "$relative" =~ ^[1-9][0-9]*/[1-9][0-9]*/[0-9a-f]{32}\.png$ ]] || {
        echo "二维码文件一致性清单包含越界条目" >&2
        exit 1
      }
      printf '.private/member-payment-qr/%s\n' "$relative" >>"$file_manifest"
    done <"$file_scan"
    LC_ALL=C sort "$database_manifest" >"$database_sorted"
    LC_ALL=C sort "$file_manifest" >"$file_sorted"
    cmp -s "$database_sorted" "$file_sorted" || {
      echo "数据库二维码引用与受控文件不一致；请先运行启动清理/修复缺失文件" >&2
      exit 1
    }
  )
}

# Validate the one private subtree owned by member payment QR uploads. The
# caller supplies an explicit absolute upload root; no working-directory
# fallback is allowed for a destructive reset. Every descendant must match the
# server-generated workspace/user/random-filename layout before any file can be
# removed.
reset_validate_payment_qr_cleanup_target() {
  local upload_root="$1"
  local physical_upload_root payment_qr_directory physical_payment_qr_directory
  local target_device entry entry_device entry_links relative manifest validation_error=""

  [[ -n "$upload_root" && "$upload_root" == /* && "$upload_root" != *$'\n'* && "$upload_root" != *$'\r'* ]] || {
    echo "BACKEND_UPLOAD_DIR 必须是不含换行的绝对路径" >&2
    return 1
  }
  upload_root="${upload_root%/}"
  case "$upload_root" in
    ""|/|/home|/Users|"$HOME")
      echo "拒绝使用过宽的上传目录：$upload_root" >&2
      return 1
      ;;
  esac
  [[ -d "$upload_root" && ! -L "$upload_root" ]] || {
    echo "上传目录不存在、不是普通目录或是符号链接：$upload_root" >&2
    return 1
  }
  physical_upload_root="$(cd "$upload_root" && pwd -P)"
  [[ "$physical_upload_root" == "$upload_root" ]] || {
    echo "上传目录路径包含符号链接，拒绝清理：$upload_root" >&2
    return 1
  }

  payment_qr_directory="$upload_root/.private/member-payment-qr"
  RESET_PAYMENT_QR_DIRECTORY="$payment_qr_directory"
  RESET_PAYMENT_QR_FILE_COUNT=0
  export RESET_PAYMENT_QR_DIRECTORY RESET_PAYMENT_QR_FILE_COUNT
  if [[ ! -e "$payment_qr_directory" && ! -L "$payment_qr_directory" ]]; then
    return 0
  fi
  [[ -d "$payment_qr_directory" && ! -L "$payment_qr_directory" ]] || {
    echo "会员收款二维码目录不是普通目录：$payment_qr_directory" >&2
    return 1
  }
  physical_payment_qr_directory="$(cd "$payment_qr_directory" && pwd -P)"
  [[ "$physical_payment_qr_directory" == "$payment_qr_directory" ]] || {
    echo "会员收款二维码路径包含符号链接，拒绝清理" >&2
    return 1
  }
  target_device="$(backend_env_stat '%d' '%d' "$payment_qr_directory")"
  [[ "$target_device" =~ ^[0-9]+$ ]] || {
    echo "无法确认会员收款二维码目录的文件系统" >&2
    return 1
  }

  manifest="$(mktemp "$upload_root/.member-payment-qr-scan.XXXXXXXX")" || {
    echo "无法创建会员收款二维码安全枚举清单" >&2
    return 1
  }
  if ! chmod 600 "$manifest"; then
    rm -- "$manifest" || true
    echo "无法限制会员收款二维码安全枚举清单权限" >&2
    return 1
  fi
  # Process substitution does not propagate find's exit status. Materialize a
  # private manifest first and check find directly, otherwise a permission
  # error could look like an empty tree and falsely authorize completion.
  if ! find "$payment_qr_directory" -xdev -mindepth 1 -print0 >"$manifest"; then
    rm -- "$manifest" || true
    echo "无法完整枚举会员收款二维码目录，拒绝清理" >&2
    return 1
  fi
  while IFS= read -r -d '' entry; do
    relative="${entry#"$payment_qr_directory"/}"
    [[ "$relative" != "$entry" && "$relative" != *$'\n'* && "$relative" != *$'\r'* ]] || {
      validation_error="会员收款二维码目录包含不安全路径"
      break
    }
    [[ ! -L "$entry" ]] || {
      validation_error="会员收款二维码目录包含符号链接：$relative"
      break
    }
    if ! entry_device="$(backend_env_stat '%d' '%d' "$entry")"; then
      validation_error="无法确认会员收款二维码条目的文件系统：$relative"
      break
    fi
    [[ "$entry_device" == "$target_device" ]] || {
      validation_error="会员收款二维码目录包含其他文件系统：$relative"
      break
    }
    if [[ -d "$entry" ]]; then
      [[ "$relative" =~ ^[1-9][0-9]*$ || "$relative" =~ ^[1-9][0-9]*/[1-9][0-9]*$ ]] || {
        validation_error="会员收款二维码目录包含越界子目录：$relative"
        break
      }
    elif [[ -f "$entry" ]]; then
      [[ "$relative" =~ ^[1-9][0-9]*/[1-9][0-9]*/[0-9a-f]{32}\.png$ ]] || {
        validation_error="会员收款二维码目录包含非应用生成文件：$relative"
        break
      }
      if ! entry_links="$(backend_env_stat '%h' '%l' "$entry")" || [[ "$entry_links" != "1" ]]; then
        validation_error="会员收款二维码文件不是服务生成的独占普通文件：$relative"
        break
      fi
      RESET_PAYMENT_QR_FILE_COUNT=$((RESET_PAYMENT_QR_FILE_COUNT + 1))
    else
      validation_error="会员收款二维码目录包含特殊文件：$relative"
      break
    fi
  done <"$manifest"
  if ! rm -- "$manifest"; then
    echo "无法删除会员收款二维码安全枚举清单" >&2
    return 1
  fi
  if [[ -n "$validation_error" ]]; then
    echo "$validation_error" >&2
    return 1
  fi
  export RESET_PAYMENT_QR_FILE_COUNT
}

# Validate the standalone archive paired with a reset database dump. Archive
# entries are deliberately restricted to the same server-generated QR layout
# as deletion. In particular, absolute paths, traversal, directories, links,
# devices and extra payloads are all rejected before the archive is trusted.
reset_validate_payment_qr_archive() {
  local archive="$1" expected_sha256="$2" expected_count="$3" age_identity="$4"
  local archive_parent physical_archive_parent archive_owner archive_mode archive_mode_value
  local actual_sha256 list_manifest verbose_manifest identity_file decrypted_archive archive_entry verbose_entry
  local entry_count=0 verbose_count=0 validation_error=""

  [[ "$archive" == /* && "$archive" != *$'\n'* && "$archive" != *$'\r'* ]] || {
    echo "二维码归档必须是不含换行的绝对路径" >&2
    return 1
  }
  [[ -f "$archive" && ! -L "$archive" ]] || {
    echo "二维码归档不存在、不是普通文件或是符号链接：$archive" >&2
    return 1
  }
  archive_parent="$(dirname "$archive")"
  [[ -d "$archive_parent" && ! -L "$archive_parent" ]] || {
    echo "二维码归档父目录不存在或是符号链接" >&2
    return 1
  }
  physical_archive_parent="$(cd "$archive_parent" && pwd -P)"
  [[ "$physical_archive_parent" == "$archive_parent" ]] || {
    echo "二维码归档路径包含符号链接" >&2
    return 1
  }
  archive_owner="$(backend_env_stat '%u' '%u' "$archive")"
  archive_mode="$(backend_env_stat '%a' '%Lp' "$archive")"
  [[ "$archive_owner" == "${EUID:-$(id -u)}" && "$archive_mode" =~ ^[0-7]{3,4}$ ]] || {
    echo "二维码归档 owner 或权限无法确认" >&2
    return 1
  }
  archive_mode_value=$((8#$archive_mode))
  (( (archive_mode_value & 077) == 0 )) || {
    echo "二维码归档必须禁止 group/other 访问（当前权限 $archive_mode）" >&2
    return 1
  }
  [[ "$expected_sha256" =~ ^[0-9a-f]{64}$ ]] || {
    echo "二维码归档 SHA-256 格式不正确" >&2
    return 1
  }
  [[ "$expected_count" =~ ^[0-9]+$ ]] || {
    echo "二维码归档文件数格式不正确" >&2
    return 1
  }
  actual_sha256="$(reset_file_sha256 "$archive")"
  [[ "$actual_sha256" == "$expected_sha256" ]] || {
    echo "二维码归档 SHA-256 不匹配" >&2
    return 1
  }

  [[ "$age_identity" == *"AGE-SECRET-KEY-"* ]] || {
    echo "二维码归档 age identity 格式不正确" >&2
    return 1
  }
  list_manifest="$(mktemp "$archive_parent/.member-payment-qr-archive-list.XXXXXXXX")" || {
    echo "无法创建二维码归档校验清单" >&2
    return 1
  }
  verbose_manifest="$(mktemp "$archive_parent/.member-payment-qr-archive-types.XXXXXXXX")" || {
    rm -- "$list_manifest" || true
    echo "无法创建二维码归档类型清单" >&2
    return 1
  }
  identity_file="$(mktemp "$archive_parent/.member-payment-qr-age-identity.XXXXXXXX")" || {
    rm -- "$list_manifest" "$verbose_manifest" || true
    echo "无法创建二维码归档解密 identity 临时文件" >&2
    return 1
  }
  decrypted_archive="$(mktemp "$archive_parent/.member-payment-qr-decrypted.XXXXXXXX")" || {
    rm -- "$list_manifest" "$verbose_manifest" "$identity_file" || true
    echo "无法创建二维码归档解密回读文件" >&2
    return 1
  }
  if ! chmod 600 "$list_manifest" "$verbose_manifest" "$identity_file" "$decrypted_archive"; then
    rm -- "$list_manifest" "$verbose_manifest" "$identity_file" "$decrypted_archive" || true
    echo "无法限制二维码归档校验清单权限" >&2
    return 1
  fi
  printf '%s\n' "$age_identity" >"$identity_file"
  if ! age --decrypt --identity "$identity_file" --output "$decrypted_archive" "$archive"; then
    rm -- "$list_manifest" "$verbose_manifest" "$identity_file" "$decrypted_archive" || true
    echo "二维码归档无法使用指定 identity 解密" >&2
    return 1
  fi
  if ! tar --list --file "$decrypted_archive" >"$list_manifest"; then
    rm -- "$list_manifest" "$verbose_manifest" "$identity_file" "$decrypted_archive" || true
    echo "无法完整枚举二维码归档" >&2
    return 1
  fi
  if ! tar --verbose --list --file "$decrypted_archive" >"$verbose_manifest"; then
    rm -- "$list_manifest" "$verbose_manifest" "$identity_file" "$decrypted_archive" || true
    echo "无法确认二维码归档条目类型" >&2
    return 1
  fi
  while IFS= read -r archive_entry; do
    [[ "$archive_entry" =~ ^\.private/member-payment-qr/[1-9][0-9]*/[1-9][0-9]*/[0-9a-f]{32}\.png$ ]] || {
      validation_error="二维码归档包含越界路径或非应用文件：$archive_entry"
      break
    }
    entry_count=$((entry_count + 1))
  done <"$list_manifest"
  if [[ -z "$validation_error" ]]; then
    while IFS= read -r verbose_entry; do
      [[ -n "$verbose_entry" && "${verbose_entry:0:1}" == "-" ]] || {
        validation_error="二维码归档包含目录、链接或特殊条目"
        break
      }
      verbose_count=$((verbose_count + 1))
    done <"$verbose_manifest"
  fi
  if ! rm -- "$list_manifest" "$verbose_manifest" "$identity_file" "$decrypted_archive"; then
    echo "无法删除二维码归档校验清单" >&2
    return 1
  fi
  if [[ -n "$validation_error" ]]; then
    echo "$validation_error" >&2
    return 1
  fi
  [[ "$entry_count" == "$verbose_count" && "$entry_count" == "$expected_count" ]] || {
    echo "二维码归档文件数或条目类型不一致" >&2
    return 1
  }
  RESET_PAYMENT_QR_ARCHIVE_FILE_COUNT="$entry_count"
  export RESET_PAYMENT_QR_ARCHIVE_FILE_COUNT
}

reset_validate_payment_qr_archive_database_consistency() {
  local archive="$1" expected_sha256="$2" expected_count="$3" age_identity="$4"
  local archive_parent
  reset_validate_payment_qr_archive "$archive" "$expected_sha256" "$expected_count" "$age_identity" || return 1
  archive_parent="$(dirname "$archive")"
  (
    set -euo pipefail
    local identity_file="" decrypted_archive="" archive_manifest="" database_manifest="" archive_sorted="" database_sorted=""
    local temporary
    cleanup_payment_qr_archive_database_check() {
      for temporary in "$identity_file" "$decrypted_archive" "$archive_manifest" "$database_manifest" "$archive_sorted" "$database_sorted"; do
        [[ -z "$temporary" ]] || rm -f -- "$temporary"
      done
    }
    trap cleanup_payment_qr_archive_database_check EXIT INT TERM
    identity_file="$(mktemp "$archive_parent/.member-payment-qr-db-check-identity.XXXXXXXX")"
    decrypted_archive="$(mktemp "$archive_parent/.member-payment-qr-db-check-tar.XXXXXXXX")"
    archive_manifest="$(mktemp "$archive_parent/.member-payment-qr-archive-refs.XXXXXXXX")"
    database_manifest="$(mktemp "$archive_parent/.member-payment-qr-current-db-refs.XXXXXXXX")"
    archive_sorted="$(mktemp "$archive_parent/.member-payment-qr-archive-sorted.XXXXXXXX")"
    database_sorted="$(mktemp "$archive_parent/.member-payment-qr-current-db-sorted.XXXXXXXX")"
    chmod 600 "$identity_file" "$decrypted_archive" "$archive_manifest" "$database_manifest" "$archive_sorted" "$database_sorted"
    printf '%s\n' "$age_identity" >"$identity_file"
    age --decrypt --identity "$identity_file" --output "$decrypted_archive" "$archive"
    tar --list --file "$decrypted_archive" >"$archive_manifest"
    reset_write_payment_qr_database_reference_manifest "$database_manifest"
    LC_ALL=C sort "$archive_manifest" >"$archive_sorted"
    LC_ALL=C sort "$database_manifest" >"$database_sorted"
    cmp -s "$archive_sorted" "$database_sorted" || {
      echo "当前数据库二维码引用与配套归档不一致，拒绝继续" >&2
      exit 1
    }
  )
}

# Create a permission-restricted, checksummed QR archive next to one already
# verified database dump. The backend is stopped by callers, but every source
# entry is still revalidated before and after tar reads it. All temporary
# manifests are private files with explicit exit cleanup; no recursive cleanup
# or unbounded path is used.
reset_archive_payment_qr_files() {
  local upload_root="$1" database_backup="$2" age_identity="$3"
  local backup_parent physical_backup_parent backup_owner backup_mode backup_mode_value archive archive_sha256 archive_count

  reset_validate_payment_qr_cleanup_target "$upload_root" || return 1
  archive_count="$RESET_PAYMENT_QR_FILE_COUNT"
  [[ "$age_identity" == *"AGE-SECRET-KEY-"* ]] || {
    echo "二维码归档 age identity 格式不正确" >&2
    return 1
  }
  [[ "$database_backup" == /* && -f "$database_backup" && ! -L "$database_backup" ]] || {
    echo "数据库备份必须是明确的绝对普通文件" >&2
    return 1
  }
  backup_parent="$(dirname "$database_backup")"
  [[ -d "$backup_parent" && ! -L "$backup_parent" ]] || {
    echo "数据库备份父目录不存在或是符号链接" >&2
    return 1
  }
  physical_backup_parent="$(cd "$backup_parent" && pwd -P)"
  [[ "$physical_backup_parent" == "$backup_parent" ]] || {
    echo "数据库备份路径包含符号链接" >&2
    return 1
  }
  backup_owner="$(backend_env_stat '%u' '%u' "$database_backup")"
  backup_mode="$(backend_env_stat '%a' '%Lp' "$database_backup")"
  [[ "$backup_owner" == "${EUID:-$(id -u)}" && "$backup_mode" =~ ^[0-7]{3,4}$ ]] || {
    echo "数据库备份 owner 或权限无法确认" >&2
    return 1
  }
  backup_mode_value=$((8#$backup_mode))
  (( (backup_mode_value & 077) == 0 )) || {
    echo "数据库备份必须禁止 group/other 访问，拒绝创建配套二维码归档" >&2
    return 1
  }
  archive="$database_backup.member-payment-qr.tar.age"
  for candidate in "$archive" "$archive.sha256" "$archive.partial" "$archive.sha256.partial" "$archive.plaintext.partial"; do
    [[ ! -e "$candidate" && ! -L "$candidate" ]] || {
      echo "同名二维码归档制品已存在，拒绝覆盖：$candidate" >&2
      return 1
    }
  done

  (
    set -euo pipefail
    local source_manifest="" tar_manifest="" expected_manifest="" actual_manifest="" verbose_manifest="" identity_file=""
    local entry entry_links relative archive_entry source_count=0 source_hash archived_hash
    local plaintext_partial="$archive.plaintext.partial" archive_partial="$archive.partial" checksum_partial="$archive.sha256.partial"
    local age_recipient published=false
    umask 077
    cleanup_payment_qr_archive() {
      local temporary
      for temporary in "$source_manifest" "$tar_manifest" "$expected_manifest" "$actual_manifest" "$verbose_manifest" \
        "$identity_file" "$plaintext_partial" "$archive_partial" "$checksum_partial"; do
        [[ -z "$temporary" ]] || rm -f -- "$temporary"
      done
      if [[ "$published" != "true" ]]; then
        rm -f -- "$archive" "$archive.sha256"
      fi
    }
    trap cleanup_payment_qr_archive EXIT INT TERM
    source_manifest="$(mktemp "$backup_parent/.member-payment-qr-source.XXXXXXXX")"
    tar_manifest="$(mktemp "$backup_parent/.member-payment-qr-tar.XXXXXXXX")"
    expected_manifest="$(mktemp "$backup_parent/.member-payment-qr-expected.XXXXXXXX")"
    actual_manifest="$(mktemp "$backup_parent/.member-payment-qr-actual.XXXXXXXX")"
    verbose_manifest="$(mktemp "$backup_parent/.member-payment-qr-verbose.XXXXXXXX")"
    identity_file="$(mktemp "$backup_parent/.member-payment-qr-encryption-identity.XXXXXXXX")"
    chmod 600 "$source_manifest" "$tar_manifest" "$expected_manifest" "$actual_manifest" "$verbose_manifest" "$identity_file"
    printf '%s\n' "$age_identity" >"$identity_file"
    age_recipient="$(age-keygen -y "$identity_file")"
    [[ "$age_recipient" =~ ^age1[0-9a-z]+$ ]] || {
      echo "无法从二维码归档 identity 派生收件人" >&2
      exit 1
    }

    if [[ -d "$RESET_PAYMENT_QR_DIRECTORY" ]]; then
      if ! find "$RESET_PAYMENT_QR_DIRECTORY" -xdev -type f -print0 >"$source_manifest"; then
        echo "无法完整枚举待归档的会员收款二维码文件" >&2
        exit 1
      fi
    fi
    while IFS= read -r -d '' entry; do
      relative="${entry#"$RESET_PAYMENT_QR_DIRECTORY"/}"
      [[ ! -L "$entry" && -f "$entry" && "$relative" =~ ^[1-9][0-9]*/[1-9][0-9]*/[0-9a-f]{32}\.png$ ]] || {
        echo "二维码文件在归档前发生了变化，拒绝继续：$relative" >&2
        exit 1
      }
      entry_links="$(backend_env_stat '%h' '%l' "$entry")"
      [[ "$entry_links" == "1" ]] || { echo "二维码文件在归档前不是独占普通文件：$relative" >&2; exit 1; }
      archive_entry=".private/member-payment-qr/$relative"
      printf '%s\0' "$archive_entry" >>"$tar_manifest"
      printf '%s\n' "$archive_entry" >>"$expected_manifest"
      source_count=$((source_count + 1))
    done <"$source_manifest"
    [[ "$source_count" == "$archive_count" ]] || {
      echo "二维码文件数在归档期间发生了变化" >&2
      exit 1
    }

    tar --create --file "$plaintext_partial" --directory "$upload_root" \
      --no-recursion --null --files-from "$tar_manifest"
    chmod 600 "$plaintext_partial"
    if ! tar --list --file "$plaintext_partial" >"$actual_manifest"; then
      echo "无法回读二维码归档清单" >&2
      exit 1
    fi
    cmp -s "$expected_manifest" "$actual_manifest" || {
      echo "二维码归档条目与精确源清单不一致" >&2
      exit 1
    }
    if ! tar --verbose --list --file "$plaintext_partial" >"$verbose_manifest"; then
      echo "无法回读二维码归档条目类型" >&2
      exit 1
    fi
    while IFS= read -r verbose_entry; do
      [[ -n "$verbose_entry" && "${verbose_entry:0:1}" == "-" ]] || {
        echo "二维码归档包含目录、链接或特殊条目" >&2
        exit 1
      }
    done <"$verbose_manifest"

    while IFS= read -r -d '' entry; do
      relative="${entry#"$RESET_PAYMENT_QR_DIRECTORY"/}"
      archive_entry=".private/member-payment-qr/$relative"
      [[ ! -L "$entry" && -f "$entry" ]] || {
        echo "二维码文件在归档回读时发生了变化：$relative" >&2
        exit 1
      }
      entry_links="$(backend_env_stat '%h' '%l' "$entry")"
      [[ "$entry_links" == "1" ]] || { echo "二维码文件在归档回读时不是独占普通文件：$relative" >&2; exit 1; }
      source_hash="$(reset_file_sha256 "$entry")"
      archived_hash="$(tar --extract --to-stdout --file "$plaintext_partial" "$archive_entry" | reset_stdin_sha256)"
      [[ "$source_hash" == "$archived_hash" ]] || {
        echo "二维码归档内容回读不一致：$relative" >&2
        exit 1
      }
    done <"$source_manifest"
    reset_validate_payment_qr_cleanup_target "$upload_root"
    [[ "$RESET_PAYMENT_QR_FILE_COUNT" == "$archive_count" ]] || {
      echo "二维码文件数在归档完成前发生了变化" >&2
      exit 1
    }

    age --recipient "$age_recipient" --output "$archive_partial" "$plaintext_partial"
    chmod 600 "$archive_partial"
    rm -- "$plaintext_partial"
    archive_sha256="$(reset_file_sha256 "$archive_partial")"
    reset_validate_payment_qr_archive "$archive_partial" "$archive_sha256" "$archive_count" "$age_identity"
    mv "$archive_partial" "$archive"
    printf '%s  %s\n' "$archive_sha256" "$(basename "$archive")" >"$checksum_partial"
    chmod 600 "$checksum_partial"
    mv "$checksum_partial" "$archive.sha256"
    published=true
  ) || return 1

  archive_sha256="$(reset_file_sha256 "$archive")"
  if ! reset_validate_payment_qr_archive "$archive" "$archive_sha256" "$archive_count" "$age_identity"; then
    rm -f -- "$archive" "$archive.sha256"
    return 1
  fi
  RESET_PAYMENT_QR_ARCHIVE="$archive"
  RESET_PAYMENT_QR_ARCHIVE_SHA256="$archive_sha256"
  RESET_PAYMENT_QR_ARCHIVE_FILE_COUNT="$archive_count"
  export RESET_PAYMENT_QR_ARCHIVE RESET_PAYMENT_QR_ARCHIVE_SHA256 RESET_PAYMENT_QR_ARCHIVE_FILE_COUNT
}

reset_remove_payment_qr_files() {
  local upload_root="$1" archived_count="${2:-}"
  local expected_count removed_count=0 manifest_count=0 entry entry_device entry_links target_device relative manifest validation_error="" deletion_error=""
  reset_validate_payment_qr_cleanup_target "$upload_root" || return 1
  expected_count="$RESET_PAYMENT_QR_FILE_COUNT"
  if [[ -n "$archived_count" ]]; then
    [[ "$archived_count" =~ ^[0-9]+$ && "$expected_count" == "$archived_count" ]] || {
      echo "当前二维码文件数与配套归档不一致，拒绝删除" >&2
      return 1
    }
  fi
  if (( expected_count == 0 )); then
    RESET_PAYMENT_QR_REMOVED_COUNT=0
    export RESET_PAYMENT_QR_REMOVED_COUNT
    return 0
  fi

  # Delete only the individually revalidated server-generated PNG paths. Empty
  # workspace/user directories are intentionally retained; there is no
  # recursive directory deletion anywhere in this cleanup.
  manifest="$(mktemp "$upload_root/.member-payment-qr-remove.XXXXXXXX")" || {
    echo "无法创建会员收款二维码删除清单" >&2
    return 1
  }
  if ! chmod 600 "$manifest"; then
    rm -- "$manifest" || true
    echo "无法限制会员收款二维码删除清单权限" >&2
    return 1
  fi
  if ! find "$RESET_PAYMENT_QR_DIRECTORY" -xdev -type f -print0 >"$manifest"; then
    rm -- "$manifest" || true
    echo "无法完整枚举待删除的会员收款二维码文件" >&2
    return 1
  fi
  if ! target_device="$(backend_env_stat '%d' '%d' "$RESET_PAYMENT_QR_DIRECTORY")"; then
    rm -- "$manifest" || true
    echo "无法确认待删除二维码目录的文件系统" >&2
    return 1
  fi
  # Validate the complete deletion manifest before unlinking its first file.
  # This catches files or permissions changing between the initial tree scan
  # and the exact-file scan without partially deleting a known-stale list.
  while IFS= read -r -d '' entry; do
    relative="${entry#"$RESET_PAYMENT_QR_DIRECTORY"/}"
    [[ ! -L "$entry" && -f "$entry" && "$relative" =~ ^[1-9][0-9]*/[1-9][0-9]*/[0-9a-f]{32}\.png$ ]] || {
      validation_error="二维码文件在清理前发生了变化，拒绝继续：$relative"
      break
    }
    if ! entry_links="$(backend_env_stat '%h' '%l' "$entry")" || [[ "$entry_links" != "1" ]]; then
      validation_error="二维码文件在清理前不再是独占普通文件：$relative"
      break
    fi
    if ! entry_device="$(backend_env_stat '%d' '%d' "$entry")" || [[ "$entry_device" != "$target_device" ]]; then
      validation_error="二维码文件在清理前跨越了文件系统：$relative"
      break
    fi
    manifest_count=$((manifest_count + 1))
  done <"$manifest"
  if [[ -z "$validation_error" && "$manifest_count" != "$expected_count" ]]; then
    validation_error="二维码文件数在清理期间发生了变化"
  fi
  if [[ -n "$validation_error" ]]; then
    rm -- "$manifest" || true
    echo "$validation_error" >&2
    return 1
  fi

  while IFS= read -r -d '' entry; do
    relative="${entry#"$RESET_PAYMENT_QR_DIRECTORY"/}"
    [[ ! -L "$entry" && -f "$entry" && "$relative" =~ ^[1-9][0-9]*/[1-9][0-9]*/[0-9a-f]{32}\.png$ ]] || {
      deletion_error="二维码文件在删除前发生了变化，拒绝继续：$relative"
      break
    }
    if ! entry_links="$(backend_env_stat '%h' '%l' "$entry")" || [[ "$entry_links" != "1" ]]; then
      deletion_error="二维码文件在删除前不再是独占普通文件：$relative"
      break
    fi
    if ! entry_device="$(backend_env_stat '%d' '%d' "$entry")" || [[ "$entry_device" != "$target_device" ]]; then
      deletion_error="二维码文件在删除前跨越了文件系统：$relative"
      break
    fi
    if ! rm -- "$entry"; then
      deletion_error="删除会员收款二维码失败：$relative"
      break
    fi
    removed_count=$((removed_count + 1))
  done <"$manifest"
  if ! rm -- "$manifest"; then
    echo "无法删除会员收款二维码删除清单" >&2
    return 1
  fi
  if [[ -n "$deletion_error" ]]; then
    echo "$deletion_error" >&2
    return 1
  fi
  [[ "$removed_count" == "$expected_count" ]] || {
    echo "二维码文件数在清理期间发生了变化" >&2
    return 1
  }
  reset_validate_payment_qr_cleanup_target "$upload_root" || return 1
  (( RESET_PAYMENT_QR_FILE_COUNT == 0 )) || {
    echo "二维码目录在清理后仍有文件" >&2
    return 1
  }
  RESET_PAYMENT_QR_REMOVED_COUNT="$removed_count"
  export RESET_PAYMENT_QR_REMOVED_COUNT
}
