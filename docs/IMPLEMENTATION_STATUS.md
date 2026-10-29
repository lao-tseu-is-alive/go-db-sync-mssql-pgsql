# Implementation status

Release baseline: **v0.3.0** (2026-10-29).

## Implemented

- Backward-compatible single-table selection plus deterministic discovery of all
  user base tables in one configured SQL Server schema.
- Source-only `--list-tables` exploration requiring only `MSSQL_DSN` and
  returning sorted exact base-table names without PostgreSQL access.
- Strict versioned YAML exclusions matched exactly and case-sensitively against
  original MSSQL names before legacy target-name conversion; unknown and
  duplicate exclusions are rejected.
- A tracked commented YAML sample separated from ignored operator-specific
  exclusion files, so source inventory policy is not published accidentally.
- A read-only multi-table plan listing selected/excluded mappings, counts,
  target compatibility and naming collisions before any PostgreSQL write.
- Compact all-table plan output that reports selected/invalid tables and the
  exclusion total; `--show-excluded` restores full exclusion details.
- Exact legacy table naming and lowercase-only column mapping with collision
  refusal.
- SQL Server metadata discovery for ordered columns, nullability and ordered
  primary-key columns.
- Explicit supported type mapping and exact typed value conversion, including
  money, precision/scale-preserving decimal/numeric, UUID and checked eight-byte
  rowversion conversion.
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
- Tag-triggered and manually dispatchable GitHub release automation for static
  Linux amd64/arm64 archives, checksums and changelog-derived release notes.
- A fixed `--workers` pool (default two, minimum one), with one independently
  connected source/target client pair per worker, per-table transactions and no
  duplicate target jobs. The existing one-table engine is split into reusable
  prepare/apply phases rather than duplicated.
- Fail-fast multi-table cancellation: the first table failure cancels in-flight
  transactions and leaves queued jobs unstarted. Every selected table receives
  a deterministic terminal summary and incomplete runs exit nonzero.

Phase 2 unit tests cover planning, exact exclusions, naming collisions, worker
bounds, cancellation and failure propagation. The race-enabled integration
test package remains isolated behind the `integration` build tag and a
separately supplied `DBSYNC_TEST_PGSQL_DSN`. Operator-authorized Phase 1 replace
and merge runs contacted disposable transit databases for the `Document`
benchmark; both verified 2,361,511 source, copied and committed rows.

`make check` passes on 2026-10-29 with race detection, coverage, configuration,
GoDoc, atlas, operational-claim and whitespace gates.

## Planned

- Run the first operator-authorized all-table dry-run and disposable-transit
  apply against the nightly snapshot; no production/shared target is used by
  automated tests.
- Execute the full multi-trial benchmark protocol, including an uninstrumented
  Python comparison, when a release performance claim is needed.
- Add a rejected SQL Server type only after the selected table demonstrates a
  need and its semantics are explicitly decided.

## Known gaps and assumptions

- Integration tests skip unless `DBSYNC_TEST_PGSQL_DSN` names a disposable
  database; the transit benchmark is evidence for end-to-end publication but
  does not replace isolated destructive integration cases.
- Count verification assumes the nightly restored source remains static from
  each initial `COUNT_BIG(*)` through its later stream. Multi-table preparation
  can lengthen that interval. The CLI does not create a SQL Server snapshot
  transaction.
- Row counts detect omission/duplication but are not a proof of whole-table
  equality. Exact-value tests cover targeted null, text, binary, decimal, UUID,
  timestamp, boolean and rowversion cases.
- PostgreSQL `bigint` cannot represent rowversion values above `2^63-1`; the
  CLI fails instead of wrapping the unsigned eight-byte value.
- Existing target defaults and additional constraints are left intact. Replace
  mode rejects incoming foreign-key dependencies rather than cascading; merge
  remains available when existing row identity must be preserved. Either mode
  safely rolls back a rejected publication.
- A multi-table run is not database-wide atomic. Already committed tables stay
  committed after a later table fails; downstream imports must wait for a fully
  successful aggregate report.
- No scheduler, incremental replication, dynamic concurrency, distributed
  orchestration, API or deployment assets exist.
