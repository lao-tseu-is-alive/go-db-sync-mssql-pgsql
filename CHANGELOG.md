# Changelog

All notable changes are documented here. The format follows Keep a Changelog
and this pre-1.0 project uses semantic versioning.

## [Unreleased]

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
