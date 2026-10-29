# Repository atlas

Tracked version: **v0.3.0**.

Every tracked or new non-ignored file has exactly one canonical responsibility
entry below. `make atlas-check` validates this list in both directions.

- `.env.example` — Secret-free connection-variable template for operators.
- `.github/workflows/ci.yml` — CI entry point that runs the repository-owned release gate.
- `.github/workflows/release.yml` — Tag/manual workflow that validates, builds and publishes checksummed GitHub release assets.
- `.gitignore` — Excludes credentials, binaries, coverage and local Go artifacts.
- `AGENTS.md` — Authoritative task scope, compatibility and contributor instructions.
- `CHANGELOG.md` — Versioned record of delivered behavior.
- `config/config_Sample.yaml` — Tracked, commented template for local exact-name exclusion configuration.
- `LICENSE` — Repository license terms.
- `Makefile` — Composed build, test, documentation and release controls.
- `README.md` — Operator-facing purpose, safety model, setup and usage guide.
- `cmd/doccheck/main.go` — Repository-specific GoDoc and atlas checker command.
- `cmd/doccheck/main_test.go` — Checker self-tests for missing GoDoc and atlas drift.
- `cmd/go-db-sync-mssql-pgsql/main.go` — CLI parsing, mode dispatch, single-table execution and shared metrics output.
- `cmd/go-db-sync-mssql-pgsql/main_test.go` — CLI tests for version and pre-connection guard behavior.
- `cmd/go-db-sync-mssql-pgsql/list.go` — Source-only exact MSSQL base-table inventory command.
- `cmd/go-db-sync-mssql-pgsql/multi.go` — All-table preparation, worker composition, progress and aggregate reporting.
- `docs/BENCHMARK.md` — Reproducible same-table/snapshot Python-versus-Go benchmark protocol.
- `docs/COMPATIBILITY.md` — Normative legacy naming, schema mapping and deliberate correction matrix.
- `docs/DOCUMENTATION.md` — Normative documentation and control-chain contract.
- `docs/IMPLEMENTATION_STATUS.md` — Living completed/planned/gap snapshot for Phase 1.
- `docs/PERFORMANCE.md` — Measured baseline, instrumentation semantics and evidence-based optimization guidance.
- `docs/ROADMAP.md` — Ordered project tasks, state and next action.
- `docs/atlas.md` — Complete machine-checked responsibility inventory for this repository.
- `go.mod` — Go version plus direct and indirect module requirements.
- `go.sum` — Checksums for reproducible Go module downloads.
- `internal/config/config.go` — Single/all-table CLI parsing, worker/apply guards and error redaction.
- `internal/config/config_test.go` — Selection, worker, naming-default and secret-redaction tests.
- `internal/config/dotenv.go` — Optional non-overriding dotenv fallback parser.
- `internal/config/dotenv_test.go` — Dotenv precedence, syntax and secret-safe error tests.
- `internal/ident/ident.go` — Dialect-aware identifier validation and quoting.
- `internal/ident/ident_test.go` — Identifier limit and quote-escaping tests.
- `internal/mssql/mssql.go` — Read-only SQL Server base-table discovery, metadata, count and SELECT adapter.
- `internal/mssql/mssql_test.go` — SQL Server SELECT projection and identifier-quoting unit tests.
- `internal/mssql/rows.go` — Forward-only SQL rows to typed pgx COPY stream adapter.
- `internal/multisync/plan.go` — Deterministic exact-exclusion and target-collision planning.
- `internal/multisync/plan_test.go` — Planning, exact-exclusion, ordering and collision tests.
- `internal/multisync/run.go` — Fixed worker-pool execution, cancellation policy and aggregate result model.
- `internal/multisync/run_test.go` — Worker-bound, ordering, cancellation and failure-propagation tests.
- `internal/naming/naming.go` — Exact ordered port of the legacy Python table-name transform.
- `internal/naming/naming_test.go` — Table-driven parity tests including every legacy acronym.
- `internal/postgres/catalog.go` — PostgreSQL catalog discovery and exact target compatibility validation.
- `internal/postgres/catalog_test.go` — Target validation and publication-SQL unit tests.
- `internal/postgres/postgres.go` — Single-connection transactional replace/merge, verification and atomic publication adapter.
- `internal/postgres/postgres_integration_test.go` — Opt-in disposable-database publication and rollback scenarios.
- `internal/postgres/sql.go` — Safely quoted target DDL and publication statement construction.
- `internal/schema/mapping.go` — Explicit SQL Server-to-PostgreSQL type mapping.
- `internal/schema/mapping_test.go` — Supported/rejected type, collision and PK-order tests.
- `internal/schema/schema.go` — Ordered table/column metadata contracts and ambiguity validation.
- `internal/schema/value.go` — Typed money, UUID and rowversion conversion for COPY.
- `internal/schema/value_benchmark_test.go` — Allocation-aware microbenchmarks for supported value conversions.
- `internal/schema/value_test.go` — Exact NULL/text/binary/decimal/UUID/time/rowversion value tests.
- `internal/stream/stream.go` — Shared closeable bounded-memory COPY stream interface.
- `internal/syncer/syncer.go` — Reusable one-table preparation and transactional application engine.
- `internal/syncer/progress.go` — Low-overhead time-based COPY progress wrapper.
- `internal/syncer/progress_test.go` — Staging progress final-count reporting test.
- `internal/syncer/syncer_test.go` — Preparation reuse, dry-run, mapping and incompatibility orchestration tests.
- `internal/syncmode/mode.go` — Validated replace/merge publication-mode contract and defaults.
- `internal/syncmode/mode_test.go` — Publication-mode acceptance and rejection tests.
- `internal/tableconfig/config.go` — Strict versioned YAML loader for exact source-table exclusions.
- `internal/tableconfig/config_test.go` — YAML schema, exact-name and repository configuration tests.
- `pkg/version/version.go` — Single audited version source and optional build metadata.
- `scripts/check_documentation_claims.sh` — Deterministic cross-file security and operational assertions.
