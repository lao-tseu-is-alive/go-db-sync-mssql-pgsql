# Changelog

All notable changes are documented here. The format follows Keep a Changelog
and this pre-1.0 project uses semantic versioning.

## [Unreleased]

### Changed

- The CLI now loads an optional `./.env` when either DSN is absent from the
  exported environment. Exported values retain precedence, and dotenv values
  are neither expanded nor logged.
- Applied refreshes now report their phase and, every five seconds, staged rows,
  percentage, elapsed time and throughput. `--progress-interval=0` disables it.

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
