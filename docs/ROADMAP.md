# Roadmap

Tracked version: **v0.2.0**.

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

Multi-table synchronization, scheduling, CDC, Change Tracking and deployment
infrastructure are outside Phase 1 and have no roadmap task.

## Next action

After v0.2.0, run the opt-in PostgreSQL integration suite with a separately
supplied `DBSYNC_TEST_PGSQL_DSN` before the next feature release; do not reuse
an unconfirmed shared database for destructive tests.
