# Changelog

All notable changes are documented here. The format follows Keep a Changelog
and this pre-1.0 project uses semantic versioning.

## [Unreleased]

### Added

- Added a guarded full-transit rehearsal script that verifies the real target
  and maintenance server/database/user identities, displays every destructive
  action and requires `RECREATE <database>` interactively or an exact
  non-interactive confirmation. It recreates the target, validates the complete
  plan, then measures both first-load and populated-target replace runs before
  reporting downstream readiness. Missing connection values may be loaded from
  `.env` without overriding exports or executing content; no-argument
  invocation displays purpose, workflow, parameters, role requirements, safety
  rules and examples. PostgreSQL maintenance access is derived from the target
  DSN, eliminating a separate admin secret; the script verifies CREATEDB and
  the other least-privilege role attributes and prints corrective SQL when they
  differ.
- Added a separate daily production wrapper that never recreates the database,
  prevents overlapping runs, verifies mandatory target database/user/owner
  identity, pins one all-table replace apply, writes complete private
  timestamped resource-usage logs behind a compact console and permits
  downstream processing only after complete aggregate success. Its offline
  tests cover identity mismatch, output filtering, exact CLI arguments, lock
  refusal and failure propagation.

### Documentation

- Recorded the first successful operator-authorized Phase 2 transit apply: all
  23 selected tables and 2,052,690 rows completed with no failure or
  cancellation, and the aggregate report marked downstream readiness true. An
  immediate uninstrumented repeat established a 15.924-second CLI baseline with
  21.52 seconds user CPU, 2.95 seconds system CPU and 26,332 KiB peak RSS.
- Recorded the expanded 60-table rehearsal and first daily-wrapper run. The
  daily wrapper verified 19,894,444 rows in 144.827 seconds at 137,367.4 rows/s
  with no failure or cancellation; raw operational logs remain private in the
  ignored local `log/` directory.

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
- Added a commented `config/config_Sample.yaml`; real `config/tables.yaml`,
  `config/tables_*.yaml` and dated exclusion working files are ignored as
  environment-specific operator configuration.
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
