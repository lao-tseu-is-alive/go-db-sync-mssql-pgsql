#!/usr/bin/env bash
# Exercise rehearsal argument and dotenv guards without connecting to databases.
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
rehearsal="${script_dir}/recreate_and_verify_transit_replication.sh"
temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/go-db-sync-rehearsal-test.XXXXXX")"
trap 'rm -rf -- "${temp_dir}"' EXIT

fail() {
    echo "rehearsal-script-test: $*" >&2
    exit 1
}

run_expect_failure() {
    local expected="$1"
    local forbidden="$2"
    shift 2
    local output status

    set +e
    output="$("$@" 2>&1)"
    status=$?
    set -e
    [[ "${status}" -ne 0 ]] || fail "command unexpectedly succeeded"
    [[ "${output}" == *"${expected}"* ]] || fail "missing expected error: ${expected}"
    if [[ -n "${forbidden}" && "${output}" == *"${forbidden}"* ]]; then
        fail "error exposed forbidden input"
    fi
}

no_argument_help="$("${rehearsal}")"
[[ "${no_argument_help}" == *'Purpose:'* ]] || fail "no-argument help lacks its purpose"
[[ "${no_argument_help}" == *'Usage:'* ]] || fail "no-argument help lacks usage"
[[ "${no_argument_help}" == *'Required connection variables:'* ]] || fail "no-argument help lacks connection variables"
[[ "${no_argument_help}" == *'Required PostgreSQL role attributes:'* ]] || fail "no-argument help lacks role requirements"
[[ "${no_argument_help}" == *'Example:'* ]] || fail "no-argument help lacks an example"

valid_env="${temp_dir}/valid.env"
printf '%s\n' \
    'MSSQL_DSN=sqlserver://example.invalid/source' \
    'PGSQL_DSN=postgres://example.invalid/goeland' >"${valid_env}"

common_args=(
    --database goeland
    --owner goeland_sync
    --confirm-recreate goeland
    --binary "${temp_dir}/missing-binary"
)

run_expect_failure 'binary is not executable' '' \
    env -u MSSQL_DSN -u PGSQL_DSN \
    "${rehearsal}" "${common_args[@]}" --env-file "${valid_env}"

execution_marker="${temp_dir}/dotenv-content-was-executed"
literal_env="${temp_dir}/literal.env"
printf 'MSSQL_DSN=$(touch %s)\n' "${execution_marker}" >"${literal_env}"
printf '%s\n' \
    "PGSQL_DSN='postgres://example.invalid/goeland'" >>"${literal_env}"
run_expect_failure 'binary is not executable' '' \
    env -u MSSQL_DSN -u PGSQL_DSN \
    "${rehearsal}" "${common_args[@]}" --env-file "${literal_env}"
[[ ! -e "${execution_marker}" ]] || fail "dotenv content was executed"

run_expect_failure 'PGSQL_DSN must be set in the environment or dotenv file' '' \
    env -u MSSQL_DSN PGSQL_DSN= \
    "${rehearsal}" "${common_args[@]}" --env-file "${valid_env}"

keyword_env="${temp_dir}/keyword.env"
printf '%s\n' \
    'MSSQL_DSN=sqlserver://example.invalid/source' \
    'PGSQL_DSN=host=example.invalid dbname=goeland user=goeland_sync' >"${keyword_env}"
run_expect_failure 'PGSQL_DSN must use a postgres:// or postgresql:// URI' '' \
    env -u MSSQL_DSN -u PGSQL_DSN \
    "${rehearsal}" "${common_args[@]}" --env-file "${keyword_env}"

malformed_env="${temp_dir}/malformed.env"
printf '%s\n' \
    'MSSQL_DSN=sqlserver://example.invalid/source' \
    'PGSQL_DSN=postgres://example.invalid/goeland' \
    'not-an-assignment-with-secret' >"${malformed_env}"

run_expect_failure 'parse dotenv line 3' 'secret' \
    env -u MSSQL_DSN -u PGSQL_DSN \
    "${rehearsal}" "${common_args[@]}" --env-file "${malformed_env}"

run_expect_failure 'refusing PostgreSQL system database: postgres' '' \
    "${rehearsal}" --database postgres --owner goeland_sync \
    --confirm-recreate postgres --env-file "${valid_env}"

run_expect_failure '--confirm-recreate must exactly match --database' '' \
    "${rehearsal}" --database goeland --owner goeland_sync \
    --confirm-recreate another_database --env-file "${valid_env}"

fake_bin_dir="${temp_dir}/fake-bin"
mkdir -p "${fake_bin_dir}"

cat >"${fake_bin_dir}/psql" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
query="${*: -1}"
case "${query}" in
    *rolcanlogin*) printf 't\tf\tf\tf\tf\tf\n' ;;
    *inet_server_addr*) printf '127.0.0.1\t5432\tpostgres\tgoeland_sync\n' ;;
    *) printf 'f\n' ;;
esac
EOF

cat >"${fake_bin_dir}/dropdb" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
touch "${MUTATION_MARKER:?}"
EOF

cat >"${fake_bin_dir}/createdb" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
touch "${MUTATION_MARKER:?}"
EOF

fake_binary="${fake_bin_dir}/go-db-sync-mssql-pgsql"
cat >"${fake_binary}" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod 0700 "${fake_bin_dir}/psql" "${fake_bin_dir}/dropdb" \
    "${fake_bin_dir}/createdb" "${fake_binary}"

test_tables_config="${temp_dir}/tables.yaml"
printf 'version: 1\nexclude: []\n' >"${test_tables_config}"
mutation_marker="${temp_dir}/database-mutated"
run_expect_failure 'ALTER ROLE "goeland_sync" WITH' '' \
    env -u MSSQL_DSN -u PGSQL_DSN \
    PATH="${fake_bin_dir}:${PATH}" MUTATION_MARKER="${mutation_marker}" \
    "${rehearsal}" --database goeland --owner goeland_sync \
    --confirm-recreate goeland --binary "${fake_binary}" \
    --tables-config "${test_tables_config}" \
    --log-dir "${temp_dir}/role-check-logs" --env-file "${valid_env}"
[[ ! -e "${mutation_marker}" ]] || fail "role verification allowed database mutation"

echo 'rehearsal-script-test: OK (operator script: scripts/recreate_and_verify_transit_replication.sh)'
