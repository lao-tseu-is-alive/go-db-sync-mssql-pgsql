# go-db-sync-mssql-pgsql — Codex agent instructions

## Mission and scope

Build a small, reliable Go CLI that reproduces the **functional behavior** of
the existing Python `mssql2pgsql` utility while improving streaming efficiency,
error handling and safe publishing. Phase 1 established one-table behavior.
Phase 2 adds deterministic full-snapshot replication for all base tables in one
configured MSSQL schema while retaining the same single-table engine. No
scheduler, CDC, Change Tracking, Kafka, Airbyte, distributed orchestration,
dynamic concurrency, web API or Kubernetes deployment belongs in this phase.

Source MSSQL is a Linux-hosted SQL Server database restored nightly at 21:30 from a production backup taken at 20:00. It is a snapshot, not a live change stream. The CLI must access MSSQL read-only. PostgreSQL is the target. A complete refresh is the intended operation. Do not assume data newer than the backup.

Work in the current repository `go-db-sync-mssql-pgsql`; use Go module `github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql`. Do not modify either reference repository.

## Current state and durable hand-off

As of 2026-10-29, `v0.3.0` is the current release baseline. It retains guarded
single-table synchronization and adds Phase 2 `--all-tables`, source-only table
listing, exact-name YAML exclusions, a bounded worker pool, compact plans and
exact decimal/numeric mapping without changing the default replace mode.

Post-v0.2.0, the default branch also owns `.github/workflows/release.yml`. A
semantic-version tag triggers the repository release gate, static Linux
amd64/arm64 builds, SHA-256 checksums and a GitHub release whose notes come from
the matching changelog section. Manual dispatch accepts only an existing tag
that matches the source version. The existing `v0.2.0` tag was successfully
published through that manual path because its tag predates the workflow.

The authoritative current-state documents are:

- [`docs/IMPLEMENTATION_STATUS.md`](docs/IMPLEMENTATION_STATUS.md) for completed
  behavior, planned work, assumptions and known gaps;
- [`docs/PERFORMANCE.md`](docs/PERFORMANCE.md) for metric definitions, measured
  evidence and recommendations;
- [`docs/ROADMAP.md`](docs/ROADMAP.md) for task state and the next authorized
  action;
- [`CHANGELOG.md`](CHANGELOG.md) for released and unreleased behavior.

The current operator-facing diagnostic options and behavior are:

- missing DSNs fall back to `./.env`, while exported environment variables keep
  precedence and secret values are never logged;
- apply progress is reported every five seconds by default;
  `--progress-interval=<duration>` changes the cadence and
  `--progress-interval=0` disables it;
- `--perf-metrics` enables per-row SQL Server `Rows.Next`, `Rows.Scan` and value
  conversion timings plus COPY and PostgreSQL phase attribution;
- `--cpu-profile=<new-file>` writes a local `runtime/pprof` CPU profile without
  an HTTP listener and refuses to overwrite an existing file.

Multi-table behavior is deliberately narrow:

- `--list-tables` performs a source-only metadata inventory, requires only
  `MSSQL_DSN`, never connects to PostgreSQL and prints exact original base-table
  names one per line;
- `--all-tables` discovers user base tables only in `--source-schema`;
  `--tables-config` contains exact, case-sensitive original MSSQL names to
  exclude before target-name conversion;
- `config/config_Sample.yaml` is the tracked public template;
  `config/tables.yaml` and dated exclusion working files are local operator
  state and must remain ignored;
- the complete plan is sorted and validated before any write, and selected
  sources that map to one target make the plan fail closed;
- all-table plans suppress individual excluded-table lines by default while
  preserving their count; `--show-excluded` restores full detail;
- `--workers` defaults to two and bounds both goroutines and independently
  connected MSSQL/PostgreSQL client pairs;
- each table uses the unchanged replace/merge transaction, so a multi-table run
  is not database-wide atomic;
- the first failure cancels in-flight work and leaves queued jobs unstarted;
  committed tables remain committed, the command exits nonzero and downstream
  imports must wait for complete aggregate success.

`--sync-mode=replace|merge` selects PostgreSQL publication. `replace` is the
default for the disposable daily transit database: after target compatibility
validation it starts a transaction, truncates an existing target with
`RESTRICT`, copies directly into the target, verifies counts and commits. It
never uses `CASCADE`; COPY, count, constraint and dependency failures roll the
transaction back. `merge` preserves the original verified staging plus
UPSERT/DELETE implementation for targets whose row identity or dependencies
must remain stable.

Run `--perf-metrics` and `--cpu-profile` in separate refreshes against
equivalent target state: per-row clock reads can pollute a CPU profile, and
both diagnostic modes perturb the run rather than define a baseline. Preserve
the existing typed streaming, transactional publication and data correctness.
Do not add concurrency beyond the fixed Phase 2 table-worker bound or make a
major optimization before reporting concrete measurements.

Existing operator evidence for `dbo.goelandtextindex` (5,607,406 rows) shows
Go `v0.1.0` at 92.510 seconds versus Python at 102.832 seconds. Go staging/COPY
accounted for 86.894 seconds (94.0% of CLI time), while transactional
publication took 5.256 seconds (5.7%). This points investigation at the
source-to-stage pipeline but does not identify whether SQL Server reading,
driver decoding, pgx encoding/I/O or PostgreSQL staging insertion dominates.
Instrumented `dbo.Document` runs established that COPY/encoding and merge
publication dominate; detailed replace-versus-merge evidence belongs in
`docs/PERFORMANCE.md` and must be kept comparable by using the same snapshot
and equivalent populated target state. The 2026-10-29 matched trials copied and
published 2,361,511 rows: replace took 81.12 seconds and merge 97.82 seconds.
Treat the 17.1% observed reduction as single-trial evidence, not a general
speedup claim.

## Mandatory references — read before coding

1. Legacy Python implementation, available locally at `/home/cgil/PycharmProjects/mssql2pgsql` (public: https://github.com/lao-tseu-is-alive/mssql2pgsql). Read **actual files**, especially:
   - `sqlalchemy_mssql.py`: `standard_table_names`, `convert_to_snake_case`, `get_pgsqltype_from_mssql`, `get_postgresql_create_sql`, `get_select_for_postgresql`, schema discovery, PKs and nullability.
   - `sqlalchemy_pgsql.py`: existence checks, counting, `bulk_copy` and `truncate_table`.
   - `copy_Mssql_Table_to_Postgresql.py`: end-to-end refresh, 20,000-row buffering, control flow and output.
   - `config.py`, `README.md`, and tests/tools if useful. **Never expose or copy credential values.**
2. Engineering conventions from `/home/cgil/cgdev/golang/go-cloud-k8s-poc-2026` (public: https://github.com/lao-tseu-is-alive/go-cloud-k8s-poc-2026), especially `docs/DOCUMENTATION.md`, `AGENTS.md`, `Makefile`, `docs/atlas.md`, `docs/ROADMAP.md`, `CHANGELOG.md` and relevant documentation checker scripts. Read and understand before adapting.
3. The reference repository is a larger Go/ConnectRPC/Vue/K8s application. **Reuse its applicable Go quality and documentation rules, not its unrelated frontend/proto/infrastructure or deployment machinery.** Adapt paths, package names, tool checks and release gates to this small CLI. Do not merely copy a Makefile whose targets will not work here.

If a referenced local path is inaccessible, look at the corresponding GitHub source when possible, report the limitation, and do not invent implementation details.

## Compatibility contract with legacy Python

Analyze actual Python behavior and record a short compatibility matrix in `docs/COMPATIBILITY.md` before implementing changes. Treat Python as the reference **for semantics, not for bugs or inefficient transfer mechanics**.

- Discover source table and columns via SQL Server metadata, default MSSQL schema `dbo`; discover destination in PostgreSQL, default schema `public`.
- For **table names**, reproduce `standard_table_names()` followed by `convert_to_snake_case()` exactly, including acronym replacements and their order. Port to Go with table-driven tests covering ordinary names, multiple uppercase runs and every special acronym. Do not replace with a generic snake_case library.
- IMPORTANT: for **column names**, the Python `get_postgresql_create_sql` uses `c['name'].lower()`; it does **not** use `convert_to_snake_case`. Preserve lowercasing of column names by default. Explicitly document any collision/unsupported identifier cases; refuse ambiguous mappings rather than silently merging columns.
- Preserve source column order; preserve SQL Server primary-key column order where defined; copy PK, nullability and supported type mapping to the destination when creating a table. Do not add new constraints or silently alter existing destination schema.
- Review the legacy type mapping carefully: TINYINT/SMALLINT/INTEGER to integer; BIGINT to bigint; BIT to boolean; VARCHAR/NVARCHAR/NCHAR/TEXT/NTEXT to text; CHAR(n) to char(n); BINARY/VARBINARY to bytea; SMALLDATETIME/DATETIME to timestamp; MONEY/SMALLMONEY to decimal(20,4); SQL Server DECIMAL/NUMERIC to PostgreSQL `numeric(p,s)` with exact metadata and byte-to-numeric conversion; UNIQUEIDENTIFIER to uuid; SQL Server `timestamp`/`rowversion` is **not** a datetime: legacy maps it to PostgreSQL bigint. Explicitly implement its 8-byte conversion and test byte order/unsigned range. When mappings remain unsupported, incorrect or ambiguous (e.g. datetime2, datetimeoffset, SQL_VARIANT), fail clearly or document a deliberate correction with tests; **never silently coerce data**.
- SQL NULL must remain a real PostgreSQL NULL, distinct from the literal text `\\N`; preserve multiline text, backslashes, control characters, UTF-8, decimal precision, UUIDs, binary values and timestamps accurately. Avoid hand-built delimited text and the Python CSV-like escaping behavior.
- If the destination table is absent, create it from discovered metadata. If present, validate expected columns/types before modifying data; reject incompatible structures with an actionable error. No automatic DROP/ALTER in Phase 1.
- Full refresh semantics: replace the selected table's contents, never append duplicates. An empty source must produce an empty destination **only after successful transfer and verification**.
- Support read-only source and write-enabled target credentials separately. Validate and safely quote SQL identifiers; SQL parameters cannot bind identifiers. No arbitrary SQL supplied by command-line arguments.

## Established Phase 1 implementation

- Drivers: `github.com/microsoft/go-mssqldb` via `database/sql` for MSSQL and `github.com/jackc/pgx/v5` for PostgreSQL. Prefer direct typed row streaming into `pgx.CopyFrom`, implementing a small `CopyFromSource` adapter over SQL Server rows. No ORM, no ODBC, no CSV/temp files, no full-table buffering.
- Keep modules/packages minimal and cohesive, e.g. `cmd/go-db-sync-mssql-pgsql`, `internal/mssql`, `internal/postgres`, `internal/sync`, `internal/naming`, `internal/config`; adjust only if a simpler structure works better.
- The CLI chooses exactly one of `--source-table` and `--all-tables`. Optional
  `--tables-config` applies only to all-table mode; `--target-table` applies only
  to single-table mode. `--list-tables` is a separate source-only exploration
  action and accepts neither dry-run nor apply. Schema defaults, version and help
  behavior remain common to all modes.
- Credentials and connection settings via environment variables (or secret-free config references), e.g. `MSSQL_DSN` and `PGSQL_DSN`; support `.env.example` with placeholders only. Do not log DSNs. Protect and redact errors containing connection details.
- Prefer a bounded-memory, one-table transfer. Default `replace` mode uses one transaction to `TRUNCATE ... RESTRICT`, COPY directly into the compatible target, verify counts and commit; rollback restores prior contents on any failure. It never uses `CASCADE`. Optional `merge` mode preserves verified temporary staging followed by UPSERT/DELETE reconciliation for dependency-sensitive targets. Never commit partial batches to the live table.
- COUNT(*) comparison is necessary but insufficient for data correctness; add deterministic targeted tests comparing exact row values. The database snapshot is assumed static during each test/sync; document the consistency assumption and avoid mismatching different source snapshots for verification.
- Use `context.Context` for cancellation, timeouts and cleanup. Always inspect `rows.Err()` and `CopyFrom` errors. On failure, preserve the existing live target and return nonzero status; clean merge staging safely. Avoid retaining or logging personally identifiable source rows. Log counts, table identifier, COPY/stage timings, duration, rows/sec and clear redacted errors.
- The reusable synchronization engine handles exactly one prepared table. Phase
  2 orchestration may invoke it through a fixed worker pool but must not duplicate
  its discovery, streaming or publication logic. Favor readable, maintainable Go
  code, small functions and explicit errors (`%w`).
- Database write operations **must require explicit execution**. `--dry-run` performs discovery and validates metadata without mutating PostgreSQL; a normal sync may modify the selected target table, so provide an explicit `--apply` flag or equivalent guard. Describe this behavior in help and README.

## Tests, evidence, and performance

- Unit tests for naming parity (including all acronym special cases), identifier quoting/validation, column mapping and SQL Server-to-PostgreSQL value conversions: NULL vs literal `\\N`, Unicode, multiline text, backslashes, booleans, money/decimal, UUID, timestamps, rowversion, binary.
- Tests for missing target creation, target-schema incompatibility, empty table, replacement of existing data, failure during `COPY`, count mismatch, foreign-key restriction, rollback and preservation of prior target data; separate integration tests requiring real test databases from normal unit tests. No automatic writes to production or shared databases.
- Implement a simple benchmark procedure in `docs/BENCHMARK.md` comparing **the same table/snapshot** against the current Python implementation: row count, elapsed time, rows/sec, peak RSS and CPU if available. Do not claim any speedup until measured; capture exact commands and reproducibility conditions. Avoid copying sensitive row samples into documentation.
- Keep integration tests opt-in and isolated from production/shared databases.
  Multi-table orchestration is covered with deterministic fakes; PostgreSQL
  transaction semantics remain covered by the existing disposable-database
  integration suite.

## Documentation and Makefile contract

[`docs/DOCUMENTATION.md`](docs/DOCUMENTATION.md) is the normative documentation
contract for this repository; this section is an entry point and does not
supersede it.

Adapt the applicable conventions in the reference `docs/DOCUMENTATION.md` as a **normative contract** for this repository, and link it from `README.md` and this `AGENTS.md` rather than duplicating its rules. Preserve these requirements:

- English for source code comments and docs. Every Go package has a purpose-driven package comment (`Package <name>`; `Command`/`Package main` for main). Every exported identifier has a useful GoDoc comment starting with its exact name, including exported struct fields.
- `docs/atlas.md` is a complete file-by-file inventory: exactly one canonical entry per tracked or new non-ignored file. Enumerate using `git ls-files -z --cached --others --exclude-standard`. Format: `- \`relative/path\` — Responsibility and authority note.` Keep it synchronized in the same change as each file addition/removal/rename. Use `Tracked version: **vX.Y.Z**.` matching `pkg/version/version.go`.
- Establish a single version source in `pkg/version/version.go`, with `Version` as a constant and build metadata (revision, timestamp) optionally injected via ldflags. CLI `--version` must reflect it; README, atlas, roadmap and changelog must agree at release boundaries.
- Create and maintain `docs/DOCUMENTATION.md`, `docs/ROADMAP.md` (project-specific task identifiers such as `SYNC-001`, unique IDs and a `Next action` section), `CHANGELOG.md`, plus a concise `docs/IMPLEMENTATION_STATUS.md` or equivalent that distinguishes completed code, planned work and known gaps. Do not copy Goéland-domain GLD tasks or immutable domain specifications.
- Implement **working** documentation checks, adapted from reference project where appropriate: `make godoc-check`, `make atlas-check`, `make docs-assert`, `make docs-check`. The atlas checker must reject missing/duplicate/stale paths; GoDoc checker must cover non-generated production files; stable security/operational claims should have targeted deterministic assertions. Include checker self-tests where reasonable.
- Adapt standard Makefile workflow: `make help`, `make fmt`, `make fmt-check`, `make lint` (go vet and relevant lint only), `make test` (race + coverage), `make build`/`binary`, `make check` (format + lint + tests + docs + `git diff --check`), `make vuln-check` using `govulncheck` when available/pinned, `make version-check`, `make changelog-check`, `make roadmap-check` and `make release-check` as appropriate. CI should run repository-owned gates (not separate weaker rules). No proto, Vue, buf, frontend or Kubernetes tasks.
- Preserve reference emphasis on cognitive complexity <= 15 for production functions when practical; include a reproducible check with a pinned tool if adopted. Do not use `go tool` for a checker without declaring/pinning the tool in `go.mod`.
- Keep generated artifacts distinguished from authoritative files; never commit secrets, real data exports, `.env`, connection strings, temporary COPY files, binaries or coverage reports.
- Update README, atlas, roadmap, implementation status and changelog honestly in the same slice. Do not mark roadmap items done before functionality and tests actually pass.

## Current acceptance boundary

Phase 2 is limited to all base tables in one configured source schema. Planning
must retain original names for exact exclusions, preserve legacy mapping, reject
collisions, validate all targets before writes and sort deterministically. Apply
uses a fixed worker pool, independent connections and per-table transactions.
Cancellation is fail-fast and reported for every selected table. Never claim
database-wide atomicity; downstream consumers start only after complete success.

Run `make check` for each completed slice and report opt-in integration tests
that were skipped because no disposable DSN was authorized. Do not connect to
real services without explicit operator permission. Stop after the focused
multi-table implementation; do not add scheduling, CDC or infrastructure.

**Priority:** correct output and safety first; then bounded throughput; then
minimal operational ergonomics. Implement the current task, not a speculative
platform.
