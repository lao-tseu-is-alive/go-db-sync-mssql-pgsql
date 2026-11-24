#!/usr/bin/env bash
# Recreate a disposable transit database and verify first-load and replacement runs.
set -euo pipefail

umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "${script_dir}/.." && pwd)"
cd "${repo_root}"

source "${script_dir}/lib/dotenv.sh"

binary="${repo_root}/bin/go-db-sync-mssql-pgsql"
tables_config="${repo_root}/config/tables.yaml"
workers=2
database=""
owner=""
maintenance_database="postgres"
confirmation=""
log_dir=""
env_file="${repo_root}/.env"
env_file_explicit=0
current_stage="initialization"
completed=0
database_mutation_started=0

usage() {
    cat <<'EOF'
Purpose:
  Certify the complete full-snapshot replication path against the exact
  PostgreSQL transit database and tables configuration intended for real use.
  This is a destructive rehearsal: it recreates the target database, performs
  an initial load, then repeats the load to exercise the daily replacement path.

Usage:
  scripts/recreate_and_verify_transit_replication.sh \
    --database <disposable-database> \
    --owner <target-role> \
    [--tables-config <path>] [--workers <n>] [--binary <path>] \
    [--log-dir <new-directory>] [--env-file <path>] \
    [--maintenance-database <database>] \
    [--confirm-recreate <same-database>]

Required options:
  --database NAME       Exact PostgreSQL database to drop and recreate.
  --owner ROLE          Role that will own the recreated database and that must
                        also be the user selected by PGSQL_DSN.

Optional options:
  --tables-config PATH  Exact-name exclusion configuration.
                        Default: config/tables.yaml
  --workers N           Maximum concurrent table jobs. Default: 2.
  --binary PATH         CLI binary. Default: bin/go-db-sync-mssql-pgsql.
  --log-dir PATH        New private log directory. The path must not exist.
                        Default: a new mode-0700 directory below /tmp.
  --env-file PATH       Dotenv file used for missing connection values.
                        Default: optional ./.env.
  --maintenance-database NAME
                        Database used for DROP/CREATE on the same server and
                        with the same role as PGSQL_DSN. Default: postgres.
  --confirm-recreate NAME
                        Non-interactive confirmation. NAME must exactly equal
                        --database. Without this option, a terminal prompt asks
                        the operator to type: RECREATE <database>

Required connection variables:
  MSSQL_DSN        Read-only SQL Server source connection.
  PGSQL_DSN        Target connection whose database must equal --database and
                   whose user must equal --owner. It must use a postgres:// or
                   postgresql:// URI so the maintenance connection can be
                   derived without duplicating credentials.

  Existing exported values take precedence. Missing values are loaded from
  ./.env when it exists, or from --env-file when supplied. Values are parsed as
  data and are never expanded, executed or printed.

Exact workflow:
  1. Load missing connection values from dotenv without overriding exports.
  2. Derive the maintenance connection from PGSQL_DSN by changing only its
     database name, so the same server, credentials and TLS options are used.
  3. Verify that --owner is the connected role and has LOGIN, NOSUPERUSER,
     CREATEDB, NOCREATEROLE, NOREPLICATION and NOBYPASSRLS.
  4. If the target exists, prove that it is on the same server and owned by the
     proposed role before any destructive action.
  5. Display the server, database, owner, tables configuration, worker count,
     logs and every destructive/copy action, then require explicit confirmation.
  6. Force-disconnect clients, drop the target database and recreate it from
     template0 with --owner as its owner.
  7. Reconnect through PGSQL_DSN and prove server, database, user and ownership.
  8. Run the complete --all-tables dry-run. Any collision, invalid mapping or
     empty selection stops the rehearsal before table writes.
  9. Run replace mode into the empty database under /usr/bin/time -v.
 10. Run the same replace mode again against the populated database.
 11. Report downstream_ready=true only when both aggregate applies report no
     failed or cancelled table.

Safety rules:
  - Interactive confirmation is the default. Non-interactive confirmation must
    exactly match --database.
  - postgres, template0, and template1 are always refused.
  - The maintenance connection must use another database on the same server.
  - Existing target identity is checked before DROP; recreated identity is
    checked again before COPY.
  - Exported DSNs override dotenv values; connection strings are never printed.
  - Logs are private and written outside the repository by default.
  - Multi-table apply is not database-wide atomic. After any failure, the target
    may be empty or partially populated and must not be used downstream.

Required PostgreSQL role attributes:
  Run this once as a role allowed to alter --owner. The rehearsal verifies these
  exact attributes and prints the same corrective statement if they differ:

  ALTER ROLE mon_user WITH
      LOGIN
      NOSUPERUSER
      CREATEDB
      NOCREATEROLE
      NOREPLICATION
      NOBYPASSRLS;

Example:
  # MSSQL_DSN and PGSQL_DSN may be stored in ./.env.
  scripts/recreate_and_verify_transit_replication.sh \
    --database goeland \
    --owner goeland_sync \
    --tables-config config/tables.yaml \
    --workers 2

  # The script displays its complete action plan and asks for:
  # RECREATE goeland

Non-interactive example:
  scripts/recreate_and_verify_transit_replication.sh \
    --database goeland \
    --owner goeland_sync \
    --tables-config config/tables.yaml \
    --workers 2 \
    --confirm-recreate goeland
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
        --confirm-recreate)
            need_value "$1" "${2-}"
            confirmation="$2"
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
        --env-file)
            need_value "$1" "${2-}"
            env_file="$2"
            env_file_explicit=1
            shift 2
            ;;
        --maintenance-database)
            need_value "$1" "${2-}"
            maintenance_database="$2"
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

derive_maintenance_dsn() {
    local target_dsn="$1"
    local maintenance_name="$2"
    local scheme rest query="" authority

    case "${target_dsn}" in
        postgres://*|postgresql://*) ;;
        *) die "PGSQL_DSN must use a postgres:// or postgresql:// URI so the maintenance database can be derived safely" ;;
    esac

    scheme="${target_dsn%%://*}"
    rest="${target_dsn#*://}"
    if [[ "${rest}" == *\?* ]]; then
        query="?${rest#*\?}"
        rest="${rest%%\?*}"
    fi
    [[ "${rest}" == */* ]] || die "PGSQL_DSN URI must include an explicit target database path"
    authority="${rest%%/*}"
    printf '%s://%s/%s%s' "${scheme}" "${authority}" "${maintenance_name}" "${query}"
}

print_role_correction() {
    cat >&2 <<EOF
Run this statement once as a role allowed to alter ${owner}:
ALTER ROLE "${owner}" WITH
    LOGIN
    NOSUPERUSER
    CREATEDB
    NOCREATEROLE
    NOREPLICATION
    NOBYPASSRLS;
EOF
}

on_exit() {
    local status=$?
    if [[ "${status}" -ne 0 && "${completed}" -ne 1 ]]; then
        echo "rehearsal failed during: ${current_stage}" >&2
        if [[ "${database_mutation_started}" -eq 1 ]]; then
            echo "the disposable database may be empty or partially populated; do not use it downstream" >&2
        else
            echo "no database mutation was started" >&2
        fi
        if [[ -n "${log_dir}" ]]; then
            echo "private logs: ${log_dir}" >&2
        fi
    fi
}
trap on_exit EXIT

[[ -n "${database}" ]] || die "--database is required"
[[ -n "${owner}" ]] || die "--owner is required"
[[ "${database}" =~ ^[a-z][a-z0-9_]{0,62}$ ]] || die "database must match ^[a-z][a-z0-9_]{0,62}$"
[[ "${owner}" =~ ^[a-z][a-z0-9_]{0,62}$ ]] || die "owner must match ^[a-z][a-z0-9_]{0,62}$"
[[ "${maintenance_database}" =~ ^[a-z][a-z0-9_]{0,62}$ ]] || die "maintenance database must match ^[a-z][a-z0-9_]{0,62}$"

case "${database}" in
    postgres|template0|template1)
        die "refusing PostgreSQL system database: ${database}"
        ;;
esac

[[ "${maintenance_database}" != "${database}" ]] || die "maintenance database must differ from target database"
if [[ -n "${confirmation}" && "${database}" != "${confirmation}" ]]; then
    die "--confirm-recreate must exactly match --database"
fi

[[ "${workers}" =~ ^[1-9][0-9]*$ ]] || die "--workers must be an integer greater than zero"
dotenv_load_missing "${env_file}" "${env_file_explicit}"
require_connection MSSQL_DSN
require_connection PGSQL_DSN
maintenance_dsn="$(derive_maintenance_dsn "${PGSQL_DSN}" "${maintenance_database}")"

[[ -x "${binary}" ]] || die "binary is not executable: ${binary}; run make binary first"
[[ -f "${tables_config}" ]] || die "tables configuration not found: ${tables_config}"

for command_name in psql dropdb createdb tee; do
    require_command "${command_name}"
done
[[ -x /usr/bin/time ]] || die "required executable not found: /usr/bin/time"

if [[ -n "${log_dir}" ]]; then
    [[ ! -e "${log_dir}" ]] || die "--log-dir must not already exist: ${log_dir}"
    mkdir -m 0700 -- "${log_dir}"
else
    log_dir="$(mktemp -d "${TMPDIR:-/tmp}/go-db-sync-transit-rehearsal.XXXXXX")"
    chmod 0700 "${log_dir}"
fi

maintenance_identity="$(psql "${maintenance_dsn}" -X -w -v ON_ERROR_STOP=1 -F $'\t' -Atqc \
    "SELECT COALESCE(inet_server_addr()::text, 'local-socket'), current_setting('port'), current_database(), current_user")"
IFS=$'\t' read -r maintenance_server maintenance_port actual_maintenance_database maintenance_user <<<"${maintenance_identity}"
[[ -n "${maintenance_server}" && -n "${maintenance_port}" && -n "${actual_maintenance_database}" && -n "${maintenance_user}" ]] || die "cannot determine the maintenance connection identity"
[[ "${actual_maintenance_database}" == "${maintenance_database}" ]] || die "derived maintenance connection reached ${actual_maintenance_database}, expected ${maintenance_database}"
[[ "${maintenance_user}" == "${owner}" ]] || die "PGSQL_DSN uses role ${maintenance_user}, expected --owner ${owner}"

role_attributes="$(psql "${maintenance_dsn}" -X -w -v ON_ERROR_STOP=1 -F $'\t' -Atqc \
    "SELECT rolcanlogin, rolsuper, rolcreatedb, rolcreaterole, rolreplication, rolbypassrls FROM pg_roles WHERE rolname = current_user")"
IFS=$'\t' read -r role_login role_superuser role_createdb role_createrole role_replication role_bypassrls <<<"${role_attributes}"
if [[ "${role_login}" != "t" || "${role_superuser}" != "f" || "${role_createdb}" != "t" || \
      "${role_createrole}" != "f" || "${role_replication}" != "f" || "${role_bypassrls}" != "f" ]]; then
    echo "error: role ${owner} does not have the required least-privilege attributes" >&2
    echo "observed: login=${role_login} superuser=${role_superuser} createdb=${role_createdb} createrole=${role_createrole} replication=${role_replication} bypassrls=${role_bypassrls}" >&2
    print_role_correction
    exit 1
fi

target_existed="$(psql "${maintenance_dsn}" -X -w -v ON_ERROR_STOP=1 -Atqc \
    "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = '${database}')")"
[[ "${target_existed}" == "t" || "${target_existed}" == "f" ]] || die "cannot determine whether target database exists"

read_target_identity() {
    psql "${PGSQL_DSN}" -X -w -v ON_ERROR_STOP=1 -F $'\t' -Atqc \
        "SELECT COALESCE(inet_server_addr()::text, 'local-socket'), current_setting('port'), current_database(), current_user, pg_get_userbyid(datdba) FROM pg_database WHERE datname = current_database()"
}

verify_target_identity() {
    local identity="$1"
    local phase="$2"
    local target_server target_port target_database target_user target_owner
    IFS=$'\t' read -r target_server target_port target_database target_user target_owner <<<"${identity}"
    [[ "${target_server}" == "${maintenance_server}" && "${target_port}" == "${maintenance_port}" ]] || \
        die "${phase}: target and maintenance connections reach different PostgreSQL servers"
    [[ "${target_database}" == "${database}" ]] || die "${phase}: PGSQL_DSN targets ${target_database}, expected ${database}"
    [[ "${target_user}" == "${owner}" ]] || die "${phase}: PGSQL_DSN uses ${target_user}, expected owner ${owner}"
    if [[ "${target_owner}" != "${owner}" ]]; then
        echo "error: ${phase}: database ${database} is owned by ${target_owner}, expected ${owner}" >&2
        echo "Run this statement once as the current database owner or a suitably privileged role:" >&2
        echo "ALTER DATABASE \"${database}\" OWNER TO \"${owner}\";" >&2
        exit 1
    fi
}

if [[ "${target_existed}" == "t" ]]; then
    verify_target_identity "$(read_target_identity)" "before recreation"
fi

echo
echo "DESTRUCTIVE FULL-TRANSIT REHEARSAL"
echo "postgres_server=${maintenance_server}:${maintenance_port}"
echo "maintenance_database=${actual_maintenance_database} maintenance_user=${maintenance_user}"
echo "target_database=${database} target_owner=${owner} target_existed=${target_existed}"
echo "tables_config=${tables_config} workers=${workers}"
echo "private_logs=${log_dir}"
echo "actions:"
echo "  1. force-disconnect clients and drop database ${database}"
echo "  2. recreate database ${database} from template0 with owner ${owner}"
echo "  3. validate the complete all-table plan without table writes"
echo "  4. apply the full snapshot into the empty database"
echo "  5. apply the same full snapshot again into the populated database"
echo "  6. allow downstream use only after both aggregate applies succeed"
echo

if [[ -n "${confirmation}" ]]; then
    echo "non_interactive_confirmation=accepted database=${database}"
else
    [[ -t 0 ]] || die "interactive confirmation requires a terminal; use --confirm-recreate ${database} for non-interactive execution"
    expected_confirmation="RECREATE ${database}"
    printf 'Type exactly "%s" to continue: ' "${expected_confirmation}"
    IFS= read -r operator_confirmation
    [[ "${operator_confirmation}" == "${expected_confirmation}" ]] || die "confirmation did not match; no database mutation was started"
fi

run_logged() {
    local label="$1"
    local destination="$2"
    shift 2

    current_stage="${label}"
    echo "stage=${label} log=${destination}"
    set +e
    "$@" 2>&1 | tee "${destination}"
    local pipeline_status=("${PIPESTATUS[@]}")
    set -e
    [[ "${pipeline_status[0]}" -eq 0 ]] || die "${label} command failed with status ${pipeline_status[0]}"
    [[ "${pipeline_status[1]}" -eq 0 ]] || die "${label} log write failed with status ${pipeline_status[1]}"
}

last_summary() {
    local prefix="$1"
    local file="$2"
    awk -v prefix="${prefix}" 'index($0, prefix) == 1 { line=$0 } END { print line }' "${file}"
}

assert_plan_success() {
    local summary
    summary="$(last_summary 'mode=plan ' "$1")"
    [[ -n "${summary}" ]] || die "dry-run produced no aggregate plan"
    [[ "${summary}" == *"requested_mode=dry-run"* ]] || die "unexpected dry-run aggregate: ${summary}"
    [[ "${summary}" == *"collisions=0"* ]] || die "dry-run reported a naming collision"
    [[ "${summary}" == *"invalid=0"* ]] || die "dry-run reported an invalid table"
    [[ "${summary}" == *"no_PostgreSQL_writes=true"* ]] || die "dry-run did not confirm its no-write boundary"
    [[ "${summary}" =~ selected=([1-9][0-9]*) ]] || die "dry-run selected no tables"
}

assert_apply_success() {
    local summary
    summary="$(last_summary 'mode=applied all_tables=true ' "$1")"
    [[ -n "${summary}" ]] || die "$2 produced no aggregate apply result"
    [[ "${summary}" == *"sync_mode=replace"* ]] || die "$2 did not use replace mode"
    [[ "${summary}" == *"failed=0"* ]] || die "$2 reported a failed table"
    [[ "${summary}" == *"cancelled=0"* ]] || die "$2 reported a cancelled table"
    [[ "${summary}" == *"downstream_ready=true"* ]] || die "$2 is not ready for downstream use"
}

echo "rehearsal_database=${database} owner=${owner} workers=${workers}"
echo "WARNING: dropping and recreating database ${database}"
"${binary}" --version

current_stage="database recreation"
database_mutation_started=1
dropdb --maintenance-db="${maintenance_dsn}" --no-password --force --if-exists "${database}"
createdb --maintenance-db="${maintenance_dsn}" --no-password --template=template0 --encoding=UTF8 --owner="${owner}" "${database}"

verify_target_identity "$(read_target_identity)" "after recreation"

common_args=(
    --all-tables
    "--tables-config=${tables_config}"
    --sync-mode=replace
)

plan_log="${log_dir}/01-dry-run.log"
first_apply_log="${log_dir}/02-empty-database-apply.log"
second_apply_log="${log_dir}/03-populated-database-apply.log"

run_logged "dry-run" "${plan_log}" "${binary}" "${common_args[@]}" --dry-run
assert_plan_success "${plan_log}"

run_logged "empty-database apply" "${first_apply_log}" \
    /usr/bin/time -v "${binary}" "${common_args[@]}" --apply "--workers=${workers}"
assert_apply_success "${first_apply_log}" "empty-database apply"

run_logged "populated-database apply" "${second_apply_log}" \
    /usr/bin/time -v "${binary}" "${common_args[@]}" --apply "--workers=${workers}"
assert_apply_success "${second_apply_log}" "populated-database apply"

completed=1
current_stage="complete"
echo "rehearsal_status=succeeded database=${database} downstream_ready=true"
echo "private_logs=${log_dir}"
