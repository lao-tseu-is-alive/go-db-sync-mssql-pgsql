# go-db-sync-mssql-pgsql

Current version: **v0.3.0**.

`go-db-sync-mssql-pgsql` is a small Go CLI for verified full-snapshot refreshes
from one SQL Server schema into PostgreSQL. It supports one explicitly selected
table and a deterministic `--all-tables` plan. It has no scheduler, CDC,
distributed orchestration, API or deployment layer.

The source is the nightly restored SQL Server snapshot (production backup at
20:00, restored at 21:30), not a live change stream. Data newer than that
backup is outside this tool's view.

## Safety model

- SQL Server credentials are separate and should be read-only. PostgreSQL uses
  its own write-enabled credential.
- `--list-tables` is source-only: it requires `MSSQL_DSN`, never opens a
  PostgreSQL connection and performs metadata reads only.
- The CLI requires exactly one of `--dry-run` or `--apply`; no database write
  can occur without `--apply`.
- A dry run discovers SQL Server metadata, maps and validates it, counts source
  rows and validates an existing PostgreSQL target without mutation.
- An apply streams typed rows with bounded memory. Default `replace` mode uses
  one PostgreSQL transaction to truncate an existing compatible target with
  `RESTRICT`, COPY directly into it, verify counts and commit.
- `--sync-mode=merge` retains the original temporary staging, count verification
  and UPSERT/DELETE reconciliation when existing row identity must be preserved.
- A COPY, constraint, dependency or verification failure rolls back the whole
  PostgreSQL transaction for that table and preserves its previous live target.
- Existing targets must match ordered columns, types, nullability and primary
  key exactly. The CLI never drops or alters them.
- DSNs and source row values are never logged.

Multi-table runs validate every selected mapping and target before the first
write, then use a bounded worker pool with one independent MSSQL/PostgreSQL
connection pair and one PostgreSQL transaction per active table. They are not
database-wide atomic: a table committed before a later failure stays committed.
The first failure cancels in-flight transactions and leaves queued tables
unstarted. The command exits nonzero, reports every table, and marks downstream
imports ready only after complete success.

Replace mode never uses `TRUNCATE CASCADE`. It takes an `ACCESS EXCLUSIVE` lock
and fails explicitly when incoming foreign keys prevent `TRUNCATE ... RESTRICT`;
use merge mode for those targets. In merge mode, a primary key enables upsert
plus deletion of absent keys, preserving referenced rows whose keys remain.
Without a primary key, merge uses transactional delete-and-insert.

## Build and test

Go 1.27.3 or newer is required.

```sh
make check
make build
./bin/go-db-sync-mssql-pgsql --version
```

Normal tests never connect to a database. PostgreSQL integration tests are
opt-in and must point only at a disposable database; each test creates and
removes a unique schema:

```sh
DBSYNC_TEST_PGSQL_DSN='postgres://<test-user>:<db-password>@127.0.0.1:5432/<disposable-db>?sslmode=disable' \
  make integration-test
```

## Configuration and use

Copy [`.env.example`](.env.example) to `.env`, or export the variables. The CLI
uses exported values first and falls back to `./.env` only when a required DSN
is missing. It never overwrites an exported value or prints loaded values.

```sh
export MSSQL_DSN='sqlserver://<source-user>:<db-password>@<source-host>:1433?database=<source-db>&encrypt=true'
export PGSQL_DSN='postgres://<target-user>:<db-password>@<target-host>:5432/<target-db>?sslmode=require'

# List exact MSSQL base-table names, one per line. PGSQL_DSN is not required.
./bin/go-db-sync-mssql-pgsql --list-tables

# Explore a non-default source schema.
./bin/go-db-sync-mssql-pgsql --list-tables --source-schema archive

# Read-only discovery and validation.
./bin/go-db-sync-mssql-pgsql --source-table Employe --dry-run

# Explicitly authorized one-table replacement (default).
./bin/go-db-sync-mssql-pgsql --source-table Employe --apply

# Dependency-preserving staged reconciliation.
./bin/go-db-sync-mssql-pgsql \
  --source-table Employe --apply --sync-mode=merge

# Validate a deterministic all-table plan without writes.
cp config/config_Sample.yaml config/tables.yaml
# Edit config/tables.yaml with exact names from --list-tables.
./bin/go-db-sync-mssql-pgsql \
  --all-tables --tables-config=config/tables.yaml --dry-run

# Apply the validated plan with at most two table transactions at once.
./bin/go-db-sync-mssql-pgsql \
  --all-tables --tables-config=config/tables.yaml --apply --workers=2
```

The dotenv parser accepts `NAME=value`, optional `export`, and single- or
double-quoted values. It does not execute or expand shell expressions.

Choose exactly one of `--source-table` and `--all-tables`. Defaults are `--source-schema=dbo`, `--target-schema=public`,
`--sync-mode=replace`, and
`--workers=2`. A single target name uses the exact legacy Python transform;
`--target-table` may override it only in single-table mode.

`--list-tables` is an exploration command outside that synchronization
selection. Its stdout contains only the deterministically sorted original MSSQL
base-table names. These are the exact case-sensitive values used by YAML
`exclude` entries, so the output can be reviewed or redirected without parsing
status text.

[`config/config_Sample.yaml`](config/config_Sample.yaml) is the tracked,
commented template for optional all-table exclusions. Copy it to the ignored
local `config/tables.yaml`, then replace its examples with names reported by
`--list-tables`. Each `exclude` entry is matched exactly and case-sensitively
against the original MSSQL table name before snake_case conversion. Duplicate
and unknown exclusions fail validation, preventing a misspelled exclusion from
silently selecting a table. Operator-specific files such as
`config/tables_poc.yaml` and dated exclusion files under `config/` are
intentionally not published.

The dry-run plan sorts original source names, lists selected and excluded
tables, their destination names and row counts, and reports naming collisions.
Two selected sources that map to the same PostgreSQL name make the plan invalid;
no jobs can concurrently target one destination. Run `--help` for the complete
interface.

To keep output readable, all-table plans print selected and invalid tables plus
the aggregate exclusion count, but suppress individual excluded-table lines.
`--show-excluded` restores the full exclusion detail for either dry-run or
apply.

### Full transit rehearsal

[`scripts/recreate_and_verify_transit_replication.sh`](scripts/recreate_and_verify_transit_replication.sh)
recreates only an explicitly confirmed transit database, then runs the
complete certification sequence: dry-run, first apply into the empty database,
and a second apply into the populated database. Both applies use replace mode,
two workers by default and `/usr/bin/time -v`; detailed per-row metrics remain
disabled so the logs provide usable CPU, RSS and wall-time baselines.

The script is intentionally destructive. It does not impose a special test
database name: the same transit database and `config/tables.yaml` intended for
real operation can therefore be certified without configuration drift. It
loads missing connection values
from `./.env` when that file exists, without overriding exported values,
expanding expressions or executing its contents. `--env-file` selects another
dotenv file explicitly. Two DSNs are required: the read-only source and the
PostgreSQL target. The script derives a maintenance connection from
`PGSQL_DSN` by changing only the database name, so it uses the same server,
role, credentials and TLS settings without another secret. DSNs are never
printed. PostgreSQL system databases are always refused.

The PostgreSQL role named by `--owner` and `PGSQL_DSN` must own the target and
have the exact least-privilege attributes below. The script verifies them before
displaying the destructive confirmation. When they differ, it stops and prints
this statement for a suitably privileged operator to run; it never changes role
attributes itself.

```sql
ALTER ROLE mon_user WITH
    LOGIN
    NOSUPERUSER
    CREATEDB
    NOCREATEROLE
    NOREPLICATION
    NOBYPASSRLS;
```

`CREATEDB` is the only database-creation capability needed by the rehearsal;
`CREATEROLE` and superuser access are deliberately refused. The same role must
own an existing target so it can be dropped. If other roles still own active
sessions, forced disconnection can fail safely and the rehearsal stops before
recreation.

```sh
# These values may instead be stored in the ignored ./.env file.
export MSSQL_DSN='<read-only-mssql-dsn>'
export PGSQL_DSN='<target-dsn-for-goeland>'

scripts/recreate_and_verify_transit_replication.sh \
  --database goeland \
  --owner goeland_sync \
  --tables-config config/tables.yaml \
  --workers 2
```

Before mutation, the script verifies the real server/database/user identity,
prints the complete destructive plan and asks the operator to type
`RECREATE goeland`. For unattended operation only,
`--confirm-recreate goeland` supplies the equivalent exact confirmation.

Private logs are created in a new mode-0700 directory under `/tmp` unless a
new `--log-dir` is supplied. On any failure the database must not be used
downstream; running the script again recreates it before retrying. A successful
run ends with `rehearsal_status=succeeded` and `downstream_ready=true`. Invoke
the script without arguments, or with `--help`, to display its purpose,
parameters, connection variables, safety rules and a complete example.

### Daily production replication

[`scripts/run_daily_transit_replication.sh`](scripts/run_daily_transit_replication.sh)
runs the normal daily refresh without dropping or recreating PostgreSQL. It
acquires a non-blocking process lock, performs exactly one complete all-table
`replace` apply, records `/usr/bin/time -v` measurements in a private
timestamped log and exits zero only when the aggregate reports no failed or
cancelled table and `downstream_ready=true`.

`--database` and `--owner` are mandatory. Before synchronization, the wrapper
connects through `PGSQL_DSN` and proves the actual server, database, connected
role and database owner; a mismatch fails before table writes. It safely loads
missing DSNs from `./.env` without overriding exports. Unlike the destructive
rehearsal, it does not need `CREATEDB`; the PostgreSQL role needs only the
schema and table privileges used by normal synchronization, including creation
of missing targets, `TRUNCATE` and `COPY`.

The default console is compact: verified identity, aggregate plans, periodic
progress for long copies, failures, final aggregate and key resource measures.
The private timestamped log retains the complete output for every table. Logs
are not removed automatically; production deployment must apply its normal
retention policy to `--log-dir`. Repository-local operational evidence belongs
under the ignored `log/` directory and must never be committed.

```sh
scripts/run_daily_transit_replication.sh \
  --database go_poc_replica \
  --owner go_db_sync_goeland \
  --tables-config config/tables_poc.yaml \
  --workers 2 \
  --progress-interval 30s \
  --log-dir log/go-db-sync-mssql-pgsql
```

Schedule this command only after the nightly MSSQL restore has completed. A
nonzero exit means the refresh is incomplete: because table transactions are
independent, previously committed tables remain committed. No downstream import
may start. Invoke the script without arguments for its complete
operational contract and options.

During an apply, the CLI reports the current phase and COPY progress every five
seconds: table, rows copied, percentage, elapsed time and rows/second. Replace reports
`phase=copying`; merge reports `phase=staging`. Use
`--progress-interval=30s` to reduce output or `--progress-interval=0` to disable
progress reporting. Progress contains counts and timings only, never row data.

### Performance evidence

Use opt-in detailed timings to separate SQL Server row fetching/decoding,
`Rows.Scan`, value conversion, the remaining `pgx.CopyFrom` pipeline, optional
merge staging, replace truncation and transactional live publication:

```sh
./bin/go-db-sync-mssql-pgsql \
  --source-table goelandtextindex --apply --perf-metrics
```

An optional CPU profile uses Go's local `runtime/pprof` writer; it does not
start a network listener and refuses to overwrite an existing file:

```sh
./bin/go-db-sync-mssql-pgsql \
  --source-table goelandtextindex --apply \
  --cpu-profile=/tmp/goelandtextindex.cpu.pprof

go tool pprof -top \
  ./bin/go-db-sync-mssql-pgsql /tmp/goelandtextindex.cpu.pprof
```

Run detailed timers and CPU profiling in separate refreshes: otherwise timer
calls can pollute the CPU profile. Both modes perturb the run, so neither is a
baseline benchmark. Measurement definitions, the existing 5.6-million-row
baseline and recommendations are in [PERFORMANCE.md](docs/PERFORMANCE.md).

## Compatibility and supported types

The exact naming behavior, type matrix, deliberate corrections and unresolved
types are recorded in [the compatibility contract](docs/COMPATIBILITY.md).
Columns are lowercased only; they do not use the table snake-case function.
Ambiguous lowercase collisions are rejected.

Supported SQL Server types are integer families, `bigint`, `bit`, legacy text
families, `char(n)`, binary families, `smalldatetime`/`datetime`, money
families, `decimal(p,s)`/`numeric(p,s)`, `uniqueidentifier`, and eight-byte
`timestamp`/`rowversion` within PostgreSQL signed `bigint`. Decimal precision
and scale are preserved in PostgreSQL `numeric(p,s)` and values are parsed from
the driver's exact bytes without floating-point conversion. Timezone/date and
other unlisted types remain explicitly unsupported until their semantics and
exact-value tests are agreed.

## Project controls

The [documentation quality contract](docs/DOCUMENTATION.md) is normative.
The [roadmap](docs/ROADMAP.md), [implementation status](docs/IMPLEMENTATION_STATUS.md),
[file atlas](docs/atlas.md), [benchmark protocol](docs/BENCHMARK.md), and
[changelog](CHANGELOG.md) are maintained with the code. `make docs-check`
enforces GoDoc, the complete atlas and stable operational claims; `make check`
is the normal handoff gate.

## Releases

Pushing a semantic-version tag such as `v0.3.0` runs the release workflow. It
verifies that the tag matches `pkg/version/version.go`, runs
`make release-check`, cross-compiles static Linux amd64 and arm64 binaries,
packages them as `.tar.gz`, publishes `SHA256SUMS`, and creates the GitHub
release from that version's changelog section.

The workflow can also be started manually for an existing tag; that path
successfully published `v0.2.0`, whose tag predates the workflow. It never
builds from an arbitrary branch: the requested tag must exist and match the
source version at that tag.
