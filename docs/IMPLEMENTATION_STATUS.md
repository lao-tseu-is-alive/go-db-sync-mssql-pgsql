# Implementation status

## Implemented in the Phase 1 slice

- One explicitly selected SQL Server table per invocation; no table loop.
- Exact legacy table naming and lowercase-only column mapping with collision
  refusal.
- SQL Server metadata discovery for ordered columns, nullability and ordered
  primary-key columns.
- Explicit supported type mapping and exact typed value conversion, including
  money, UUID and checked eight-byte rowversion conversion.
- PostgreSQL target schema validation or missing-table creation, default direct
  transactional replacement with `TRUNCATE ... RESTRICT`, optional unchanged
  staged reconciliation, COPY count verification and final count check.
- Explicit `--dry-run`/`--apply` guard and separate secret-bearing DSNs.
- Optional `./.env` fallback that never overrides exported environment values
  and is not read for `--help` or `--version`.
- Non-sensitive phase output and time-based COPY progress, enabled every five
  seconds by default and explicitly disableable.
- Coarse phase timings plus opt-in detailed MSSQL read/scan/conversion metrics,
  COPY pipeline attribution and local CPU profiles using `runtime/pprof`; no
  concurrency is introduced.
- Unit tests and opt-in PostgreSQL integration scenarios for creation, exact
  values, incompatibility, empty source, existing-data replacement, COPY/count
  rollback and foreign-key restriction.

`make check` passes on 2026-10-29. The race-enabled integration test package
compiles, but its real-database cases skip because no separately supplied
`DBSYNC_TEST_PGSQL_DSN` is present. Operator-authorized replace and merge runs
did contact the disposable transit databases for the `Document` benchmark;
both verified 2,361,511 source, copied and committed rows.

## Planned

- Execute the full multi-trial benchmark protocol, including an uninstrumented
  Python comparison, when a release performance claim is needed.
- Add a rejected SQL Server type only after the selected table demonstrates a
  need and its semantics are explicitly decided.

## Known gaps and assumptions

- Integration tests skip unless `DBSYNC_TEST_PGSQL_DSN` names a disposable
  database; the transit benchmark is evidence for end-to-end publication but
  does not replace isolated destructive integration cases.
- Count verification assumes the nightly restored source remains static from
  the initial `COUNT_BIG(*)` through streaming. The CLI does not create a SQL
  Server snapshot transaction.
- Row counts detect omission/duplication but are not a proof of whole-table
  equality. Exact-value tests cover targeted null, text, binary, decimal, UUID,
  timestamp, boolean and rowversion cases.
- PostgreSQL `bigint` cannot represent rowversion values above `2^63-1`; the
  CLI fails instead of wrapping the unsigned eight-byte value.
- Existing target defaults and additional constraints are left intact. Replace
  mode rejects incoming foreign-key dependencies rather than cascading; merge
  remains available when existing row identity must be preserved. Either mode
  safely rolls back a rejected publication.
- No multi-table orchestration, scheduler, incremental replication, API or
  deployment assets exist.
