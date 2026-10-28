# Implementation status

## Implemented in the Phase 1 slice

- One explicitly selected SQL Server table per invocation; no table loop.
- Exact legacy table naming and lowercase-only column mapping with collision
  refusal.
- SQL Server metadata discovery for ordered columns, nullability and ordered
  primary-key columns.
- Explicit supported type mapping and exact typed value conversion, including
  money, UUID and checked eight-byte rowversion conversion.
- PostgreSQL target schema validation or missing-table creation, temporary
  staging, count verification, transactional publication and final count check.
- Explicit `--dry-run`/`--apply` guard and separate secret-bearing DSNs.
- Optional `./.env` fallback that never overrides exported environment values
  and is not read for `--help` or `--version`.
- Non-sensitive phase output and time-based staging progress, enabled every five
  seconds by default and explicitly disableable.
- Unit tests and opt-in PostgreSQL integration scenarios for creation, exact
  values, incompatibility, empty source and rollback after COPY failure.

`make check`, `make docs-check`, the race-enabled integration test package and
the pinned `govulncheck` gate pass on 2026-10-28. The integration package was
run without `DBSYNC_TEST_PGSQL_DSN`, so its real-database cases compiled and
skipped; no database was contacted.

## Planned

- Execute the benchmark protocol against one operator-approved table and the
  same restored snapshot used by Python.
- Add a rejected SQL Server type only after the selected table demonstrates a
  need and its semantics are explicitly decided.

## Known gaps and assumptions

- No live database was contacted while implementing Phase 1. Integration tests
  skip unless `DBSYNC_TEST_PGSQL_DSN` names a disposable database.
- Count verification assumes the nightly restored source remains static from
  the initial `COUNT_BIG(*)` through streaming. The CLI does not create a SQL
  Server snapshot transaction.
- Row counts detect omission/duplication but are not a proof of whole-table
  equality. Exact-value tests cover targeted null, text, binary, decimal, UUID,
  timestamp, boolean and rowversion cases.
- PostgreSQL `bigint` cannot represent rowversion values above `2^63-1`; the
  CLI fails instead of wrapping the unsigned eight-byte value.
- Existing target defaults and additional constraints are left intact. They can
  reject publication, which safely rolls back.
- No multi-table orchestration, scheduler, incremental replication, API or
  deployment assets exist.
