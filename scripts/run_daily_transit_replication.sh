#!/usr/bin/env bash
# Run one guarded daily full-snapshot replication into the transit database.
set -euo pipefail

umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "${script_dir}/.." && pwd)"
cd "${repo_root}"

source "${script_dir}/lib/dotenv.sh"

binary="${repo_root}/bin/go-db-sync-mssql-pgsql"
tables_config="${repo_root}/config/tables.yaml"
workers=2
progress_interval="30s"
log_dir="/var/tmp/go-db-sync-mssql-pgsql-${UID}"
lock_file=""
database=""
owner=""
env_file="${repo_root}/.env"
env_file_explicit=0

usage() {
    cat <<'EOF'
Purpose:
  Run one production full-snapshot replication from the restored SQL Server
  snapshot into the PostgreSQL transit database. The command performs the
  complete all-table preflight, then replaces every selected table once.

  This script never drops or recreates the PostgreSQL database. It is intended
  for a daily scheduler after the nightly MSSQL restore has completed.

Usage:
  scripts/run_daily_transit_replication.sh \
    --database <target-database> --owner <target-role> \
    [--tables-config <path>] [--workers <n>] \
    [--progress-interval <duration>] [--binary <path>] \
    [--log-dir <directory>] [--lock-file <path>] [--env-file <path>]

Required options:
  --database NAME       Exact PostgreSQL database PGSQL_DSN must reach.
  --owner ROLE          Exact connected role and database owner expected from
                        PGSQL_DSN.

Options:
  --tables-config PATH
                        Exact-name table exclusion configuration.
                        Default: config/tables.yaml
  --workers N           Maximum concurrent table jobs. Default: 2.
  --progress-interval D COPY progress interval accepted by the CLI.
                        Default: 30s. Use 0 to disable periodic progress.
  --binary PATH         CLI binary. Default: bin/go-db-sync-mssql-pgsql.
  --log-dir PATH        Private directory for timestamped run logs.
                        Default: /var/tmp/go-db-sync-mssql-pgsql-<uid>
  --lock-file PATH      Exclusive non-blocking process lock.
                        Default: <log-dir>/daily-replication.lock
  --env-file PATH       Dotenv file used for missing connection values.
                        Default: optional ./.env.
  -h, --help            Display this help and exit.

Connections:
  Export MSSQL_DSN and PGSQL_DSN, or place missing values in the repository
  ./.env file. Exported values take precedence. Dotenv values are parsed as
  data and are never expanded, executed or printed.

  The daily run does not create or drop databases and therefore does not need
  PostgreSQL CREATEDB. The target role still needs the schema and table rights
  required to create a missing target, TRUNCATE an existing target and COPY its
  replacement data.

Exact workflow:
  1. Acquire an exclusive non-blocking lock; overlapping runs fail immediately.
  2. Connect through PGSQL_DSN and verify the exact database, connected role,
     database owner and resolved server identity before synchronization.
  3. Record the binary version, verified identity, configuration and start time
     in a private log.
  4. Execute exactly one --all-tables --sync-mode=replace --apply operation.
     The CLI validates the complete plan before starting table writes.
  5. Require the final aggregate to report failed=0, cancelled=0 and
     downstream_ready=true.
  6. Exit zero only after complete aggregate success. On failure, already
     committed tables remain committed because the run is not database-wide
     atomic; downstream imports must not start.

Console output is compact: identity, aggregate plans, periodic in-progress
COPY lines, failures, final aggregate and key resource measurements. The
private log preserves the complete original output, including every table.

Example:
  scripts/run_daily_transit_replication.sh \
    --database go_poc_replica \
    --owner go_db_sync_goeland \
    --tables-config config/tables_poc.yaml \
    --workers 2 \
    --progress-interval 30s \
    --log-dir log/go-db-sync-mssql-pgsql

Schedule this command only after the nightly source restore is complete. The
scheduler must treat a nonzero exit as a failed refresh and must start any
downstream import only after this script exits successfully. Logs are never
deleted automatically; configure retention outside this script.
EOF
}

die() {
    echo "error: $*" >&2
    exit 1
}

need_value() {
    local option="$1"
    local value="${2-}"
    [[ -n "${value}" ]] || die "${option} requires a value"
}

if [[ $# -eq 0 ]]; then
    usage
    exit 0
fi

while [[ $# -gt 0 ]]; do
    case "$1" in
        --database)
            need_value "$1" "${2-}"
            database="$2"
            shift 2
            ;;
        --owner)
            need_value "$1" "${2-}"
            owner="$2"
            shift 2
            ;;
        --tables-config)
            need_value "$1" "${2-}"
            tables_config="$2"
            shift 2
            ;;
        --workers)
            need_value "$1" "${2-}"
            workers="$2"
            shift 2
            ;;
        --progress-interval)
            need_value "$1" "${2-}"
            progress_interval="$2"
            shift 2
            ;;
        --binary)
            need_value "$1" "${2-}"
            binary="$2"
            shift 2
            ;;
        --log-dir)
            need_value "$1" "${2-}"
            log_dir="$2"
            shift 2
            ;;
        --lock-file)
            need_value "$1" "${2-}"
            lock_file="$2"
            shift 2
            ;;
        --env-file)
            need_value "$1" "${2-}"
            env_file="$2"
            env_file_explicit=1
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            die "unknown argument: $1"
            ;;
    esac
done

require_command() {
    command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

require_connection() {
    local name="$1"
    [[ -n "${!name-}" ]] || die "${name} must be set in the environment or dotenv file"
}

compact_console() {
    local line
    while IFS= read -r line || [[ -n "${line}" ]]; do
        case "${line}" in
            daily_replication_started_at=*|binary_version=*|target_identity=*|tables_config=*|phase=discovering-tables|mode=plan\ *|mode=applied\ *|error:*|warning:*)
                printf '%s\n' "${line}"
                ;;
            table\ source=*status=failed*|table\ source=*status=cancelled*)
                printf '%s\n' "${line}"
                ;;
            table=*phase=copying*|table=*phase=staging*)
                if [[ "${line}" != *"percent=0.0"* && "${line}" != *"percent=100.0"* ]]; then
                    printf '%s\n' "${line}"
                fi
                ;;
            *"User time (seconds):"*|*"System time (seconds):"*|*"Percent of CPU this job got:"*|*"Elapsed (wall clock) time"*|*"Maximum resident set size (kbytes):"*|*"Exit status:"*)
                printf '%s\n' "${line}"
                ;;
        esac
    done
}

last_summary() {
    local file="$1"
    awk 'index($0, "mode=applied all_tables=true ") == 1 { line=$0 } END { print line }' "${file}"
}

assert_apply_success() {
    local summary
    summary="$(last_summary "$1")"
    [[ -n "${summary}" ]] || { echo "error: daily apply produced no aggregate result" >&2; return 1; }
    [[ "${summary}" == *"sync_mode=replace"* ]] || { echo "error: daily apply did not use replace mode" >&2; return 1; }
    [[ "${summary}" == *"failed=0"* ]] || { echo "error: daily apply reported a failed table" >&2; return 1; }
    [[ "${summary}" == *"cancelled=0"* ]] || { echo "error: daily apply reported a cancelled table" >&2; return 1; }
    [[ "${summary}" == *"downstream_ready=true"* ]] || { echo "error: daily apply is not ready for downstream use" >&2; return 1; }
}

[[ -n "${database}" ]] || die "--database is required"
[[ -n "${owner}" ]] || die "--owner is required"
[[ "${database}" =~ ^[a-z][a-z0-9_]{0,62}$ ]] || die "database must match ^[a-z][a-z0-9_]{0,62}$"
[[ "${owner}" =~ ^[a-z][a-z0-9_]{0,62}$ ]] || die "owner must match ^[a-z][a-z0-9_]{0,62}$"
case "${database}" in
    postgres|template0|template1)
        die "refusing PostgreSQL system database: ${database}"
        ;;
esac
[[ "${workers}" =~ ^[1-9][0-9]*$ ]] || die "--workers must be an integer greater than zero"
dotenv_load_missing "${env_file}" "${env_file_explicit}"
require_connection MSSQL_DSN
require_connection PGSQL_DSN
[[ -x "${binary}" ]] || die "binary is not executable: ${binary}; run make binary first"
[[ -f "${tables_config}" ]] || die "tables configuration not found: ${tables_config}"

for command_name in flock tee awk date mkdir chmod psql; do
    require_command "${command_name}"
done
[[ -x /usr/bin/time ]] || die "required executable not found: /usr/bin/time"

if [[ -e "${log_dir}" ]]; then
    [[ -d "${log_dir}" && ! -L "${log_dir}" ]] || die "log path must be a real directory: ${log_dir}"
    [[ -O "${log_dir}" ]] || die "log directory must be owned by the current user: ${log_dir}"
else
    mkdir -p -- "${log_dir}"
fi
chmod 0700 -- "${log_dir}"
[[ -w "${log_dir}" ]] || die "log directory is not writable: ${log_dir}"

if [[ -z "${lock_file}" ]]; then
    lock_file="${log_dir}/daily-replication.lock"
fi
[[ ! -d "${lock_file}" && ! -L "${lock_file}" ]] || die "lock path must be a regular file path: ${lock_file}"

exec 9>"${lock_file}"
if ! flock -n 9; then
    die "another daily replication is already running (lock: ${lock_file})"
fi

target_identity="$(PGDATABASE="${PGSQL_DSN}" psql -X -w -v ON_ERROR_STOP=1 -F $'\t' -Atqc \
    "SELECT COALESCE(host(inet_server_addr()), 'local-socket'), current_setting('port'), current_database(), current_user, pg_get_userbyid(datdba) FROM pg_database WHERE datname = current_database()")"
IFS=$'\t' read -r target_server target_port target_database target_user target_owner <<<"${target_identity}"
[[ -n "${target_server}" && -n "${target_port}" && -n "${target_database}" && -n "${target_user}" && -n "${target_owner}" ]] || \
    die "cannot determine the PostgreSQL target identity"
[[ "${target_database}" == "${database}" ]] || die "PGSQL_DSN targets database ${target_database}, expected ${database}"
[[ "${target_user}" == "${owner}" ]] || die "PGSQL_DSN uses role ${target_user}, expected ${owner}"
[[ "${target_owner}" == "${owner}" ]] || die "database ${database} is owned by ${target_owner}, expected ${owner}"

run_timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
log_file="${log_dir}/daily-${run_timestamp}-$$.log"
[[ ! -e "${log_file}" ]] || die "refusing to overwrite log: ${log_file}"

binary_version="$("${binary}" --version)"
if [[ "${binary_version}" == *dirty* ]]; then
    echo "warning: binary version reports a dirty build; use a clean release binary for reproducible production evidence" >&2
fi

echo "daily_replication_status=running log=${log_file}"
set +e
{
    echo "daily_replication_started_at=${run_timestamp}"
    echo "binary_version=${binary_version}"
    echo "target_identity=${target_server}:${target_port}/${target_database} user=${target_user} owner=${target_owner}"
    echo "tables_config=${tables_config} workers=${workers} progress_interval=${progress_interval}"
    /usr/bin/time -v "${binary}" \
        --all-tables \
        "--tables-config=${tables_config}" \
        --sync-mode=replace \
        --apply \
        "--workers=${workers}" \
        "--progress-interval=${progress_interval}"
} 2>&1 | tee "${log_file}" | compact_console
pipeline_status=("${PIPESTATUS[@]}")
set -e

if [[ "${pipeline_status[0]}" -ne 0 ]]; then
    echo "daily_replication_status=failed downstream_ready=false log=${log_file}" >&2
    exit "${pipeline_status[0]}"
fi
if [[ "${pipeline_status[1]}" -ne 0 ]]; then
    die "daily replication log write failed with status ${pipeline_status[1]}"
fi
if [[ "${pipeline_status[2]}" -ne 0 ]]; then
    die "daily replication console filter failed with status ${pipeline_status[2]}"
fi

if ! assert_apply_success "${log_file}"; then
    echo "daily_replication_status=failed downstream_ready=false log=${log_file}" >&2
    exit 1
fi
success_line="daily_replication_status=succeeded downstream_ready=true log=${log_file}"
echo "${success_line}" | tee -a "${log_file}"
