# go-db-sync-mssql-pgsql — Codex agent instructions

## Mission and scope

Build a small, reliable Go CLI that reproduces the **functional behavior** of the existing Python `mssql2pgsql` utility while improving streaming efficiency, error handling, and safe publishing. **Phase 1 must implement and test ONE explicitly selected MSSQL table only.** No whole-database synchronization, scheduler, CDC, Change Tracking, Kafka, Airbyte, web API, or Kubernetes deployment in this phase. Do not generalize prematurely.

Source MSSQL is a Linux-hosted SQL Server database restored nightly at 21:30 from a production backup taken at 20:00. It is a snapshot, not a live change stream. The CLI must access MSSQL read-only. PostgreSQL is the target. A complete refresh is the intended operation. Do not assume data newer than the backup.

Work in the current repository `go-db-sync-mssql-pgsql`; use Go module `github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql`. Do not modify either reference repository.

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
- Review the legacy type mapping carefully: TINYINT/SMALLINT/INTEGER to integer; BIGINT to bigint; BIT to boolean; VARCHAR/NVARCHAR/NCHAR/TEXT/NTEXT to text; CHAR(n) to char(n); BINARY/VARBINARY to bytea; SMALLDATETIME/DATETIME to timestamp; MONEY/SMALLMONEY to decimal(20,4); UNIQUEIDENTIFIER to uuid; SQL Server `timestamp`/`rowversion` is **not** a datetime: legacy maps it to PostgreSQL bigint. Explicitly implement its 8-byte conversion and test byte order/unsigned range. When legacy mappings are unsupported, incorrect or ambiguous (e.g. precision, datetime2, decimal, datetimeoffset, binary, SQL_VARIANT), fail clearly or document a deliberate correction with tests; **never silently coerce data**.
- SQL NULL must remain a real PostgreSQL NULL, distinct from the literal text `\\N`; preserve multiline text, backslashes, control characters, UTF-8, decimal precision, UUIDs, binary values and timestamps accurately. Avoid hand-built delimited text and the Python CSV-like escaping behavior.
- If the destination table is absent, create it from discovered metadata. If present, validate expected columns/types before modifying data; reject incompatible structures with an actionable error. No automatic DROP/ALTER in Phase 1.
- Full refresh semantics: replace the selected table's contents, never append duplicates. An empty source must produce an empty destination **only after successful transfer and verification**.
- Support read-only source and write-enabled target credentials separately. Validate and safely quote SQL identifiers; SQL parameters cannot bind identifiers. No arbitrary SQL supplied by command-line arguments.

## Proposed implementation for Phase 1

- Drivers: `github.com/microsoft/go-mssqldb` via `database/sql` for MSSQL and `github.com/jackc/pgx/v5` for PostgreSQL. Prefer direct typed row streaming into `pgx.CopyFrom`, implementing a small `CopyFromSource` adapter over SQL Server rows. No ORM, no ODBC, no CSV/temp files, no full-table buffering.
- Keep modules/packages minimal and cohesive, e.g. `cmd/go-db-sync-mssql-pgsql`, `internal/mssql`, `internal/postgres`, `internal/sync`, `internal/naming`, `internal/config`; adjust only if a simpler structure works better.
- Minimal CLI interface: `--source-table` (required for a sync), optional `--source-schema=dbo`, `--target-schema=public`, `--dry-run`, `--version` and `--help`. Target table defaults to the legacy transformation; any explicit override must be validated. Provide a way to list source tables if it is easy and useful, but do not expand into all-table execution.
- Credentials and connection settings via environment variables (or secret-free config references), e.g. `MSSQL_DSN` and `PGSQL_DSN`; support `.env.example` with placeholders only. Do not log DSNs. Protect and redact errors containing connection details.
- Prefer a bounded-memory, one-table transfer. Stage rows into a temporary or dedicated staging table of compatible structure, verify counts, then publish atomically in a PostgreSQL transaction. Choose a strategy that works for a table with existing constraints/dependencies; explain locking and performance tradeoffs. Never TRUNCATE the live target **before** a potentially failing transfer and never commit partial batches to the live table.
- COUNT(*) comparison is necessary but insufficient for data correctness; add deterministic targeted tests comparing exact row values. The database snapshot is assumed static during each test/sync; document the consistency assumption and avoid mismatching different source snapshots for verification.
- Use `context.Context` for cancellation, timeouts and cleanup. Always inspect `rows.Err()` and `CopyFrom` errors. On failure, preserve the existing live target and return nonzero status; clean staging safely. Avoid retaining or logging personally identifiable source rows. Log counts, table identifier, stage timings, duration, rows/sec and clear redacted errors.
- Start sequentially and with **one table only**. Do not implement workers/concurrent-table transfer until measurements justify it. Favor readable, maintainable Go code, small functions and explicit errors (`%w`).
- Database write operations **must require explicit execution**. `--dry-run` performs discovery and validates metadata without mutating PostgreSQL; a normal sync may modify the selected target table, so provide an explicit `--apply` flag or equivalent guard. Describe this behavior in help and README.

## Tests, evidence, and performance

- Unit tests for naming parity (including all acronym special cases), identifier quoting/validation, column mapping and SQL Server-to-PostgreSQL value conversions: NULL vs literal `\\N`, Unicode, multiline text, backslashes, booleans, money/decimal, UUID, timestamps, rowversion, binary.
- Tests for missing target creation, target-schema incompatibility, empty table, failure during `COPY`, rollback and preservation of prior target data; separate integration tests requiring real test databases from normal unit tests. No automatic writes to production or shared databases.
- Implement a simple benchmark procedure in `docs/BENCHMARK.md` comparing **the same table/snapshot** against the current Python implementation: row count, elapsed time, rows/sec, peak RSS and CPU if available. Do not claim any speedup until measured; capture exact commands and reproducibility conditions. Avoid copying sensitive row samples into documentation.
- After one table works end to end, **stop and report findings**, including unresolved mapping differences and measurements. Do not start all-table replication without a new instruction.

## Documentation and Makefile contract

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

## Immediate task and acceptance criteria

1. Inspect both local reference repositories, summarize naming/type-mapping semantics and applicable documentation/Makefile rules; identify any ambiguity rather than guessing.
2. Set up the minimal Go CLI, configuration, version metadata, Makefile, documentation contract/atlas and practical checks.
3. Implement **one-table** introspection, safe table creation or validation, streamed full copy to staging, transactional publication, and row-count verification.
4. Add unit and opt-in integration tests; implement a benchmark protocol. Do not connect to real services unless the operator explicitly permits it.
5. Run `make check`, `make docs-check`, and relevant tests; report actual results and any checks blocked by environment.
6. Stop. Provide a short summary: changed files, precise naming compatibility, supported/unresolved types, measured behavior if databases were authorized, limitations and the single best next step.

**Priority:** correct output and safety first; then streaming efficiency; then minimal operational ergonomics. Implement the immediate task, not a speculative platform.
