# Changelog

All notable changes are documented here. The format follows Keep a Changelog
and this pre-1.0 project uses semantic versioning.

## [Unreleased]

## [0.3.0] - 2026-10-29

### Added

- Added a tag-triggered and manually dispatchable GitHub release workflow that
  verifies source/tag version agreement, runs `make release-check`, publishes
  static Linux amd64/arm64 archives and SHA-256 checksums, and derives release
  notes from the matching changelog section.
- Added `--all-tables`, optional strict `--tables-config` YAML exclusions and a
  deterministic dry-run plan covering original source names, mapped targets,
  exclusions, row counts, target compatibility and naming collisions.
- Added source-only `--list-tables`, which requires no PostgreSQL DSN or
  connection and prints deterministically sorted exact MSSQL base-table names
  for exclusion-file preparation.
- Added a commented `config/config_Sample.yaml`; real `config/tables.yaml` and
  dated exclusion working files are ignored as environment-specific operator
  configuration.
- Added a fixed `--workers` pool (default two) with independent connection pairs,
  per-table transactions, per-table summaries and a final aggregate report.
- Added exact SQL Server `decimal(p,s)` and `numeric(p,s)` support. PostgreSQL
  targets use `numeric(p,s)`, precision/scale metadata is validated, driver
  bytes are parsed as typed numerics and floating-point inputs are refused.

### Changed

- Refactored the existing single-table service into reusable preparation and
  application phases. Single-table CLI behavior remains compatible; multi-table
  apply validates every selected mapping before the first write.
- Multi-table application now fails fast: the first failure cancels in-flight
  transactions and leaves queued jobs unstarted. Already committed tables remain
  committed, the run exits nonzero and downstream readiness is false.
- All-table plans now suppress individual excluded-table lines by default while
  retaining the exclusion count. `--show-excluded` restores full detail for
  dry-run or apply.

### Security

- Exact exclusions are matched against original MSSQL names before target-name
  conversion. Unknown/duplicate exclusions and target-name collisions fail
  closed; workers never disable foreign keys or use `TRUNCATE CASCADE`.

## [0.2.0] - 2026-10-29

### Changed

- The CLI now loads an optional `./.env` when either DSN is absent from the
  exported environment. Exported values retain precedence, and dotenv values
  are neither expanded nor logged.
- Applied refreshes now report their phase and, every five seconds, staged rows,
  percentage, elapsed time and throughput. `--progress-interval=0` disables it.
- Added `--perf-metrics` for source/COPY/PostgreSQL phase attribution and
  `--cpu-profile=<new-file>` for opt-in local `runtime/pprof` CPU profiles.
  Instrumentation does not alter the selected publication strategy.
- Added `--sync-mode=replace|merge`. `replace` is the new default for disposable
  transit targets and performs rollback-safe `TRUNCATE ... RESTRICT` plus direct
  COPY; `merge` preserves the previous staged UPSERT/DELETE implementation.

### Fixed

- Existing PostgreSQL targets can now be validated with pgx binary results:
  catalog identity/generated markers use PostgreSQL's internal `"char"` codec
  instead of being scanned into incompatible Go strings.

### Known limitations

- Destructive PostgreSQL integration scenarios compiled under the race detector
  but skipped because no separate `DBSYNC_TEST_PGSQL_DSN` was supplied. The
  operator-authorized `Document` replace and merge runs both completed with
  matching source, copied and committed counts.

## [0.1.0] - 2026-10-28

### Added

- Exact legacy table naming and schema compatibility contract with tests for
  every ordered acronym, uppercase runs, lowercase-only columns, supported
  mappings and checked rowversion conversion (`SYNC-001`).
- Guarded one-table CLI with separate DSNs, explicit dry-run/apply modes,
  read-only SQL Server discovery, bounded typed COPY, compatible target creation
  or validation, verified staging and atomic PostgreSQL publication (`SYNC-002`).
- Documentation contract and atlas, repository gates, exact-value unit tests,
  opt-in disposable-PostgreSQL integration tests and reproducible benchmark
  protocol (`SYNC-003`).

### Security

- pgx 5.9.2 and `x/text` 0.41.0 include fixes for all reachable findings
  reported by the pinned govulncheck 1.9.0 gate.
