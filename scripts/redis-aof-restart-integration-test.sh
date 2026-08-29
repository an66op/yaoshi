#!/usr/bin/env bash
set -euo pipefail

readonly redis_image="redis:7.4.11-alpine3.21@sha256:ff02b58f971e7d7d156a1267e283fcbbeee91773b6aa36c49dac28ecfe28eadf"
readonly temp_root="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"
run_token="$(date -u +%s)-$$-${RANDOM}"
readonly run_token
readonly container_name="wangzhe-redis-aof-${run_token}"
readonly redis_password="RedisAofTest#${run_token}-persistence"
readonly test_key="wangzhe-aof-restart-test:${run_token}"
readonly test_value="persisted-${run_token}"

if [[ "${temp_root}" != /* || ! -d "${temp_root}" ]]; then
  printf 'temporary root must be an existing absolute directory: %s\n' "${temp_root}" >&2
  exit 1
fi

for command_name in cat date docker find grep id mktemp rmdir sleep tail tr; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    printf 'required command is unavailable: %s\n' "${command_name}" >&2
    exit 1
  fi
done

temp_dir="$(mktemp -d "${temp_root%/}/wangzhe-redis-aof.XXXXXXXX")"
readonly temp_dir

cleanup() {
  local cleanup_status=0
  set +e

  if [[ "${container_name}" =~ ^wangzhe-redis-aof-[0-9-]+$ ]]; then
    docker rm --force "${container_name}" >/dev/null 2>&1 || true
  fi

  case "${temp_dir}" in
    "${temp_root%/}"/wangzhe-redis-aof.*)
      if [[ -d "${temp_dir}" ]]; then
        find "${temp_dir}" -xdev -mindepth 1 -depth -delete || cleanup_status=1
        rmdir "${temp_dir}" || cleanup_status=1
      fi
      ;;
    *)
      printf 'refusing to clean unexpected temporary path: %s\n' "${temp_dir}" >&2
      cleanup_status=1
      ;;
  esac

  return "${cleanup_status}"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

umask 077
cat >"${temp_dir}/redis.conf" <<EOF
bind 127.0.0.1 -::1
protected-mode yes
port 6379
dir /data
appendonly yes
appenddirname appendonlydir
appendfsync everysec
aof-use-rdb-preamble yes
aof-load-truncated no
maxmemory-policy noeviction
stop-writes-on-bgsave-error yes
save ""
requirepass ${redis_password}
EOF

redis_cli() {
  docker exec \
    --env "REDISCLI_AUTH=${redis_password}" \
    "${container_name}" \
    redis-cli --no-auth-warning --raw "$@"
}

start_redis() {
  docker run --detach \
    --name "${container_name}" \
    --network none \
    --user "$(id -u):$(id -g)" \
    --volume "${temp_dir}:/data" \
    "${redis_image}" \
    redis-server /data/redis.conf >/dev/null

  local attempt
  for ((attempt = 1; attempt <= 30; attempt += 1)); do
    if [[ "$(redis_cli PING 2>/dev/null || true)" == "PONG" ]]; then
      return 0
    fi
    sleep 1
  done

  docker logs "${container_name}" >&2 || true
  printf 'Redis did not become ready after 30 seconds\n' >&2
  return 1
}

assert_persistence_ok() {
  local persistence_info
  persistence_info="$(redis_cli INFO persistence | tr -d '\r')"

  grep -Fqx 'aof_enabled:1' <<<"${persistence_info}"
  grep -Fqx 'aof_last_write_status:ok' <<<"${persistence_info}"
  grep -Fqx 'aof_last_bgrewrite_status:ok' <<<"${persistence_info}"
  [[ "$(redis_cli CONFIG GET appendonly | tail -n 1)" == "yes" ]]
  [[ "$(redis_cli CONFIG GET appendfsync | tail -n 1)" == "everysec" ]]
  [[ "$(redis_cli CONFIG GET maxmemory-policy | tail -n 1)" == "noeviction" ]]
}

docker pull "${redis_image}" >/dev/null
start_redis

[[ "$(docker inspect --format '{{.HostConfig.NetworkMode}}' "${container_name}")" == "none" ]]
[[ "$(redis_cli CONFIG GET bind | tail -n 1)" == "127.0.0.1 -::1" ]]
grep -Fqx 'redis_version:7.4.11' <<<"$(redis_cli INFO server | tr -d '\r')"

unauthenticated_ping="$(docker exec "${container_name}" redis-cli --raw PING 2>&1 || true)"
grep -Fq 'NOAUTH' <<<"${unauthenticated_ping}"

[[ "$(redis_cli SET "${test_key}" "${test_value}")" == "OK" ]]
redis_cli WAITAOF 1 0 10000 >/dev/null
assert_persistence_ok

if ! find "${temp_dir}/appendonlydir" -type f -size +0c -print -quit | grep -q .; then
  printf 'Redis did not create a non-empty AOF artifact\n' >&2
  exit 1
fi

docker stop --time 15 "${container_name}" >/dev/null
docker rm "${container_name}" >/dev/null

start_redis
[[ "$(redis_cli GET "${test_key}")" == "${test_value}" ]]
assert_persistence_ok

docker stop --time 15 "${container_name}" >/dev/null
docker rm "${container_name}" >/dev/null

printf 'Redis AOF restart persistence test passed with %s\n' "${redis_image}"
