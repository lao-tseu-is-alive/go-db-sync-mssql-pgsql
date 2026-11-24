#!/usr/bin/env bash
# Exercise the daily wrapper without connecting to either database.
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
daily_script="${script_dir}/run_daily_transit_replication.sh"
temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/go-db-sync-daily-test.XXXXXX")"
trap 'rm -rf -- "${temp_dir}"' EXIT

fail() {
    echo "daily-script-test: $*" >&2
    exit 1
}

run_expect_failure() {
    local expected="$1"
    shift
    local output status

    set +e
    output="$("$@" 2>&1)"
    status=$?
    set -e
    [[ "${status}" -ne 0 ]] || fail "command unexpectedly succeeded"
    [[ "${output}" == *"${expected}"* ]] || fail "missing expected error: ${expected}"
}

no_argument_help="$("${daily_script}")"
[[ "${no_argument_help}" == *'Purpose:'* ]] || fail "no-argument help lacks its purpose"
[[ "${no_argument_help}" == *'This script never drops or recreates'* ]] || fail "help lacks the no-recreation boundary"
[[ "${no_argument_help}" == *'does not need'*'CREATEDB'* ]] || fail "help lacks daily role guidance"
[[ "${no_argument_help}" == *'downstream imports must not start'* ]] || fail "help lacks downstream safety guidance"

run_expect_failure '--workers must be an integer greater than zero' \
    "${daily_script}" --database go_poc_replica --owner go_db_sync_goeland --workers 0

run_expect_failure 'refusing PostgreSQL system database: postgres' \
    "${daily_script}" --database postgres --owner go_db_sync_goeland

fake_bin_dir="${temp_dir}/fake-bin"
mkdir -p "${fake_bin_dir}"
fake_binary="${fake_bin_dir}/go-db-sync-mssql-pgsql"
cat >"${fake_binary}" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1-}" == "--version" ]]; then
    echo 'go-db-sync-mssql-pgsql v0.test.0 (revision test, built test)'
    exit 0
fi
printf '%s\n' "$@" >"${DAILY_ARGS_FILE:?}"
echo 'phase=discovering-tables'
echo 'table=dbo.Hidden phase=discovering'
echo 'mode=plan requested_mode=apply-preflight selected=2 collisions=0 invalid=0 no_PostgreSQL_writes=true'
echo 'table=dbo.Slow phase=copying rows=0/42 percent=0.0 elapsed=0s rows_per_second=0.0'
echo 'table=dbo.Slow phase=copying rows=21/42 percent=50.0 elapsed=30s rows_per_second=0.7'
echo 'table=dbo.Slow phase=copying rows=42/42 percent=100.0 elapsed=1m0s rows_per_second=0.7'
if [[ "${DAILY_FAKE_FAILURE:-0}" == "1" ]]; then
    echo 'mode=applied all_tables=true sync_mode=replace workers=2 succeeded=0 failed=1 cancelled=0 rows=0 downstream_ready=false'
    exit 1
fi
echo 'table source=dbo.Hidden target=public.hidden status=succeeded rows=42 duration=1s'
echo 'mode=applied all_tables=true sync_mode=replace workers=2 succeeded=2 failed=0 cancelled=0 rows=42 downstream_ready=true'
EOF

cat >"${fake_bin_dir}/psql" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '127.0.0.1\t5432\t%s\tgo_db_sync_goeland\tgo_db_sync_goeland\n' \
    "${DAILY_FAKE_DATABASE:-go_poc_replica}"
EOF
chmod 0700 "${fake_binary}" "${fake_bin_dir}/psql"

tables_config="${temp_dir}/tables.yaml"
printf 'version: 1\nexclude: []\n' >"${tables_config}"
log_dir="${temp_dir}/logs"
lock_file="${temp_dir}/daily.lock"
args_file="${temp_dir}/args"
database="go_poc_replica"
owner="go_db_sync_goeland"
mssql_dsn="sqlserver://example.invalid/source"
pgsql_dsn="postgres://example.invalid/go_poc_replica"

success_output="$(
    env DAILY_ARGS_FILE="${args_file}" MSSQL_DSN="${mssql_dsn}" PGSQL_DSN="${pgsql_dsn}" \
        PATH="${fake_bin_dir}:${PATH}" "${daily_script}" \
        --database "${database}" \
        --owner "${owner}" \
        --binary "${fake_binary}" \
        --tables-config "${tables_config}" \
        --workers 2 \
        --progress-interval 45s \
        --log-dir "${log_dir}" \
        --lock-file "${lock_file}"
)"
[[ "${success_output}" == *'daily_replication_status=succeeded downstream_ready=true'* ]] || \
    fail "successful aggregate was not accepted"
[[ "${success_output}" == *'target_identity=127.0.0.1:5432/go_poc_replica user=go_db_sync_goeland owner=go_db_sync_goeland'* ]] || \
    fail "compact console lacks the verified target identity"
[[ "${success_output}" == *'percent=50.0'* ]] || fail "compact console lacks periodic progress"
[[ "${success_output}" != *'table=dbo.Hidden phase=discovering'* ]] || fail "compact console exposed verbose discovery detail"
[[ "${success_output}" != *'status=succeeded rows=42'* ]] || fail "compact console exposed per-table success detail"

for expected_argument in \
    '--all-tables' \
    "--tables-config=${tables_config}" \
    '--sync-mode=replace' \
    '--apply' \
    '--workers=2' \
    '--progress-interval=45s'; do
    grep -Fxq -- "${expected_argument}" "${args_file}" || \
        fail "daily command lacks argument: ${expected_argument}"
done

log_count="$(find "${log_dir}" -maxdepth 1 -type f -name 'daily-*.log' | wc -l)"
[[ "${log_count}" -eq 1 ]] || fail "expected one private daily log, found ${log_count}"
log_file="$(find "${log_dir}" -maxdepth 1 -type f -name 'daily-*.log')"
grep -Fq 'table=dbo.Hidden phase=discovering' "${log_file}" || fail "full log lost discovery detail"
grep -Fq 'status=succeeded rows=42' "${log_file}" || fail "full log lost per-table success detail"

run_expect_failure 'PGSQL_DSN targets database another_database, expected go_poc_replica' \
    env DAILY_ARGS_FILE="${args_file}" DAILY_FAKE_DATABASE=another_database \
    MSSQL_DSN="${mssql_dsn}" PGSQL_DSN="${pgsql_dsn}" PATH="${fake_bin_dir}:${PATH}" \
    "${daily_script}" \
    --database "${database}" \
    --owner "${owner}" \
    --binary "${fake_binary}" \
    --tables-config "${tables_config}" \
    --log-dir "${log_dir}" \
    --lock-file "${lock_file}"

exec 8>"${lock_file}"
flock -n 8 || fail "test could not acquire the daily lock"
run_expect_failure 'another daily replication is already running' \
    env DAILY_ARGS_FILE="${args_file}" MSSQL_DSN="${mssql_dsn}" PGSQL_DSN="${pgsql_dsn}" \
    PATH="${fake_bin_dir}:${PATH}" \
    "${daily_script}" \
    --database "${database}" \
    --owner "${owner}" \
    --binary "${fake_binary}" \
    --tables-config "${tables_config}" \
    --log-dir "${log_dir}" \
    --lock-file "${lock_file}"
flock -u 8

run_expect_failure 'daily_replication_status=failed downstream_ready=false' \
    env DAILY_ARGS_FILE="${args_file}" DAILY_FAKE_FAILURE=1 \
    MSSQL_DSN="${mssql_dsn}" PGSQL_DSN="${pgsql_dsn}" PATH="${fake_bin_dir}:${PATH}" \
    "${daily_script}" \
    --database "${database}" \
    --owner "${owner}" \
    --binary "${fake_binary}" \
    --tables-config "${tables_config}" \
    --workers 2 \
    --log-dir "${log_dir}" \
    --lock-file "${lock_file}"

echo 'daily-script-test: OK (operator script: scripts/run_daily_transit_replication.sh)'
