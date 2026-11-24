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
- A guarded full-transit rehearsal script that recreates only an explicitly
  confirmed database after verifying its real PostgreSQL server/database/user
  identity, validates the full plan, measures first-load and populated-target
  replacement runs, and exposes downstream readiness only after both aggregates
  succeed. It safely loads missing connections from an optional dotenv file
  while preserving exported values, derives maintenance access from the target
  DSN, and verifies rather than grants the owner's exact least-privilege role
  attributes.
- A daily production wrapper that never recreates PostgreSQL, prevents
  overlapping executions with a non-blocking lock, pins one all-table replace
  apply, verifies mandatory target database/user/owner identity, stores the
  complete output in a private timestamped resource-usage log, presents a
  compact console and accepts success only when the final aggregate declares
  complete downstream readiness.

Phase 2 unit tests cover planning, exact exclusions, naming collisions, worker
bounds, cancellation and failure propagation. The race-enabled integration
test package remains isolated behind the `integration` build tag and a
separately supplied `DBSYNC_TEST_PGSQL_DSN`. Operator-authorized Phase 1 replace
and merge runs contacted disposable transit databases for the `Document`
benchmark; both verified 2,361,511 source, copied and committed rows.

On 2026-11-23, the first operator-authorized Phase 2 apply completed against
the disposable transit database: 23 of 1,417 discovered tables were selected,
1,394 were excluded, and all 23 succeeded with 2,052,690 verified rows. There
were no failures, cancellations, naming collisions or invalid mappings; one
missing target was created. Default replace mode with two workers and detailed
metrics completed the apply phase in 16.869 seconds and the CLI run in 17.260
seconds, reporting 118,930.5 rows/s and `downstream_ready=true`.

An immediate equivalent repeat without detailed metrics also completed all 23
tables and 2,052,690 rows. It took 15.226 seconds for apply and 15.924 seconds
for the CLI, reporting 128,904.9 rows/s; `/usr/bin/time -v` measured 21.52
seconds user CPU, 2.95 seconds system CPU and 26,332 KiB peak RSS.

On 2026-11-24, the guarded rehearsal selected and completed 60 of 1,417 tables
twice, verifying 19,894,444 rows in both the empty-target and populated-target
runs. CLI durations were 152.990 and 152.052 seconds respectively, with
130,037.8 and 130,839.7 rows/s, about 21.3 MiB peak RSS and no failed or
cancelled table. The binary reported a dirty revision, so this is strong
operational evidence but not release-reproducible benchmark evidence.

The first daily-wrapper run then targeted the intentionally restored older test
database and completed the same 60 tables and 19,894,444 rows in 144.827
seconds at 137,367.4 rows/s, with zero failures/cancellations and downstream
readiness true. This occurred before the final target-identity and compact
console guards and used the dirty binary, so a clean guarded run remains due.

`make check` passes on 2026-11-24 with race detection, coverage, configuration,
GoDoc, atlas, operational-claim and whitespace gates.

## Planned

- Build a clean release binary and exercise the daily wrapper once under the
  intended scheduler account before enabling the external nightly schedule.
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
