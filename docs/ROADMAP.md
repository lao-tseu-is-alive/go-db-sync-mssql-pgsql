# Roadmap

Tracked version: **v0.3.0**.

This file owns implementation order and task state. `[ ]` is planned, `[~]` is
in progress, `[x]` is complete and verified, and `[-]` is dropped with a reason.
IDs are stable and never reused.

## Phase 1 — one table

- [x] **SYNC-001 — Legacy compatibility contract**: record and test exact table
  naming, lowercase-only columns, supported type mappings and deliberate safety
  corrections.
- [x] **SYNC-002 — Guarded one-table CLI**: separate DSNs, explicit dry-run/apply,
  metadata discovery, target creation/validation, typed streaming, verified
  staging and transactional publication.
- [x] **SYNC-003 — Quality and documentation controls**: unit and opt-in
  integration tests, documentation contract, atlas, Makefile gates and benchmark
  protocol.
- [x] **SYNC-007 — Daily snapshot replacement mode**: default to transactional
  `TRUNCATE ... RESTRICT` plus direct COPY while retaining the original staged
  reconciliation as explicit `merge`, with rollback/FK tests and a same-snapshot
  `Document` comparison.
- [x] **SYNC-008 — Release artifact workflow**: validate semantic-version tags,
  run the repository release gate, cross-compile Linux amd64/arm64 archives,
  publish checksums and create GitHub releases from matching changelog notes.

## Phase 2 — multi-table full snapshots

- [x] **SYNC-009 — Deterministic all-table planning**: discover source-schema
  base tables, apply exact original-name YAML exclusions, preserve legacy target
  naming, sort deterministically and reject target-name collisions.
- [x] **SYNC-010 — Bounded table workers**: reuse the prepared one-table engine
  with independent connection pairs, per-table transactions, a fixed worker
  bound and fail-fast cancellation of in-flight and queued work.
- [x] **SYNC-011 — Multi-table reporting and guards**: validate every selected
  target before writing, report each table and the aggregate, preserve dry-run,
  metrics, profiling and replace/merge behavior, and explicitly prevent
  downstream handoff after incomplete runs.
- [x] **SYNC-012 — Phase 2 verification and documentation**: cover planning,
  exclusions, collisions, worker bounds, cancellation and failure propagation;
  update operator/documentation contracts and pass `make check`.
- [x] **SYNC-013 — Source table inventory**: add a PostgreSQL-independent,
  read-only `--list-tables` command that emits exact sorted MSSQL base-table
  names for exclusion planning.
- [x] **SYNC-014 — Exact decimal mapping and compact plans**: preserve SQL Server
  decimal/numeric precision and scale with exact value conversion and tests;
  suppress all-table exclusion detail unless explicitly requested.

## Later work requiring a new instruction

- [x] **SYNC-006 — Performance evidence**: add lightweight phase timings,
  opt-in detailed MSSQL read/scan/conversion measurements and opt-in CPU
  profiles, then collect one comparable run before proposing any optimization.
- [ ] **SYNC-004 — Authorized benchmark execution**: run Go and Python against
  the same table and restored snapshot, recording rows, elapsed time, rows/sec,
  peak RSS and CPU.
- [ ] **SYNC-005 — Additional type decisions**: add only explicitly selected
  mappings backed by exact-value tests, beginning with the first selected table's
  actual unsupported metadata if any.

Scheduling, CDC, Change Tracking, dynamic concurrency, distributed orchestration
and deployment infrastructure remain outside the project scope.

## Next action

Run the first complete operator-authorized `--all-tables --apply` against the
disposable transit database, resolve any newly exposed unsupported source type
through the same exact-value process, and record aggregate timings before
considering further optimization.
