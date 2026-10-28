# Roadmap

Tracked version: **v0.1.0**.

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

## Later work requiring a new instruction

- [ ] **SYNC-004 — Authorized benchmark execution**: run Go and Python against
  the same table and restored snapshot, recording rows, elapsed time, rows/sec,
  peak RSS and CPU.
- [ ] **SYNC-005 — Additional type decisions**: add only explicitly selected
  mappings backed by exact-value tests, beginning with the first selected table's
  actual unsupported metadata if any.

Multi-table synchronization, scheduling, CDC, Change Tracking and deployment
infrastructure are outside Phase 1 and have no roadmap task.

## Next action

Finish the Phase 1 quality gates, then obtain operator authorization for one
disposable end-to-end database run on the explicitly selected table.
