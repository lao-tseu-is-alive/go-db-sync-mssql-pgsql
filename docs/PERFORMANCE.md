# Performance evidence and profiling

This document separates observed measurements from inference and future
recommendations. Per-table correctness and atomic publication remain
non-negotiable. Phase 2 introduces only fixed, operator-selected table-level
concurrency; dynamic concurrency and unrelated optimization require new
comparable evidence.

## Existing operator baseline

On 2026-10-28 an operator synchronized `dbo.goelandtextindex` with 5,607,406
rows using both implementations:

| Measurement | Legacy Python | Go v0.1.0 |
| --- | ---: | ---: |
| Wall time | 102.832 s | 92.510 s |
| User CPU | 71.874 s | 148.158 s |
| System CPU | 3.775 s | 29.543 s |
| Staging/COPY | Not separately measured | 86.894 s |
| Transactional live publication | Not applicable | 5.256 s |

Observed: Go reduced wall time by 10.0% but consumed substantially more client
CPU. Within the Go CLI duration of 92.443 seconds, staging/COPY accounted for
94.0%, live publication for 5.7%, and other work for about 0.3%.

The comparison is informative rather than controlled: peak RSS and repeat
trials were not captured, and Python copied into an existing empty table while
Go created its target. Python also writes directly to the live target in
partially committed batches; Go first stages and verifies all rows to preserve
the previous target on failure.

## Same-snapshot replace versus merge

On 2026-10-29, the operator-authorized `dbo.Document` table contained 2,361,511
rows and 47 columns. One instrumented `replace` run was followed immediately by
one instrumented `merge` run against the same restored SQL Server snapshot.
Before each run, the compatible PostgreSQL target already contained the same
snapshot, so target structure and contents were equivalent.

| Measurement | Replace | Merge |
| --- | ---: | ---: |
| Source/COPY/published rows | 2,361,511 | 2,361,511 |
| Wall time (`/usr/bin/time`) | 81.12 s | 97.82 s |
| CLI duration | 81.065 s | 97.757 s |
| COPY | 80.617 s direct to target | 77.760 s to staging |
| Staging, including COPY | 0 s | 78.063 s |
| Live publication | 80.807 s, including direct COPY | 19.508 s after staging |
| TRUNCATE | 3.9 ms | 0 s |
| Final count verification | 186 ms | 988 ms |
| User CPU | 191.75 s | 185.55 s |
| System CPU | 34.48 s | 29.71 s |
| Peak RSS | 19,216 KiB | 22,896 KiB |
| Overall rows/s | 29,131.2 | 24,156.9 |

Replace reduced observed wall time by 16.70 seconds (17.1%) and increased
overall throughput by 20.6%. Its direct COPY was 2.857 seconds (3.7%) slower
than COPY into merge's unindexed temporary table, but it eliminated the second
staging-to-target pass. Merge spent 16.801 seconds inserting/upserting, 1.719
seconds deleting absent keys and 0.988 seconds verifying the live target.

The publication durations are intentionally not additive with COPY: replace
publication contains direct COPY, while merge publication begins after staging.
Both runs used `--perf-metrics`, whose per-row clock reads perturb performance.
These are comparable single trials, not an uninstrumented multi-run median.
Correctness evidence is count-level for the live benchmark: source, COPY and
committed target counts matched in both modes. Exact typed values and rollback
semantics are covered separately by deterministic and opt-in integration tests;
no whole-table checksum was captured.

## First complete multi-table transit run

On 2026-11-23, the operator authorized the first complete Phase 2 run against
the disposable PostgreSQL transit database. The v0.3.0 CLI used default
`replace`, two fixed workers and `--perf-metrics`. The exact-name exclusion
configuration selected 23 of 1,417 discovered MSSQL base tables and excluded
1,394. A separate dry-run completed in 1.996 seconds with no collisions,
invalid mappings or PostgreSQL writes.

| Measurement | Result |
| --- | ---: |
| Selected / succeeded tables | 23 / 23 |
| Failed / cancelled tables | 0 / 0 |
| Source and committed rows | 2,052,690 |
| Apply-command preflight | 0.390 s |
| Apply phase | 16.869 s |
| CLI duration | 17.260 s |
| Aggregate throughput | 118,930.5 rows/s |
| Newly created targets | 1 |
| Downstream readiness | true |

`dbo.ActeurRole` accounted for 1,549,721 rows (75.5% of the selected total)
and dominated elapsed time. Its table duration was 13.543 seconds, including
13.452 seconds in direct COPY. Instrumentation attributed 9.021 seconds to
SQL Server row acquisition, scan and conversion, leaving 4.431 seconds for pgx
encoding, client/server I/O and PostgreSQL COPY processing.

Every table's expected source count matched the copied and committed count,
including an empty table, and the new
`public.aff_opc_dem_pc_dico_charge_unite` target was created successfully. This
is count-level operational evidence, not a database-wide atomicity or
whole-table equality claim. Detailed timers add per-row overhead, and this run
did not capture process CPU or peak RSS, so it is not an uninstrumented capacity
baseline and should not be compared directly with the single-table trials.

Immediately afterward, the operator repeated the same 23-table replacement
without `--perf-metrics`, against the same source snapshot and equivalently
populated targets. `/usr/bin/time -v` captured the process measurements:

| Measurement | Detailed metrics | Uninstrumented repeat |
| --- | ---: | ---: |
| Source and committed rows | 2,052,690 | 2,052,690 |
| Apply phase | 16.869 s | 15.226 s |
| CLI duration | 17.260 s | 15.924 s |
| Wall time | Not separately captured | 15.93 s |
| Aggregate throughput | 118,930.5 rows/s | 128,904.9 rows/s |
| User CPU | Not captured | 21.52 s |
| System CPU | Not captured | 2.95 s |
| Peak RSS | Not captured | 26,332 KiB |
| Successful / failed / cancelled | 23 / 0 / 0 | 23 / 0 / 0 |

The uninstrumented CLI duration was 1.336 seconds (7.7%) lower and reported
8.4% higher throughput than the detailed-metrics run. This consecutive pair
shows the expected instrumentation cost in this workload, but one
uninstrumented trial is still not a multi-run capacity distribution. The
largest table, `dbo.ActeurRole`, completed in 11.454 seconds without detailed
timers versus 13.543 seconds with them.

## Expanded 60-table rehearsal

On 2026-11-24, the operator ran the guarded full-transit rehearsal with an
expanded private table policy. The restored source exposed 1,417 base tables;
60 were selected and 1,357 excluded. The dry-run found no collision or invalid
mapping and counted 19,894,444 selected rows. The rehearsal then applied the
same snapshot once to the newly recreated empty database and once to the
populated database.

| Measurement | Empty target | Populated target |
| --- | ---: | ---: |
| Selected / succeeded tables | 60 / 60 | 60 / 60 |
| Failed / cancelled tables | 0 / 0 | 0 / 0 |
| Source and committed rows | 19,894,444 | 19,894,444 |
| Apply phase | 151.664 s | 150.685 s |
| CLI / wall duration | 152.990 s | 152.052 s |
| Aggregate throughput | 130,037.8 rows/s | 130,839.7 rows/s |
| User CPU | 426.78 s | 428.22 s |
| System CPU | 65.86 s | 66.99 s |
| Peak RSS | 21,724 KiB | 21,872 KiB |
| Downstream readiness | true | true |

The populated replacement was 0.938 seconds (0.6%) faster in wall time and its
throughput was 0.6% higher. That difference is small enough to treat empty and
populated replace performance as equivalent in this single consecutive pair.
`dbo.Document` was the longest table at 103.445 and 102.551 seconds for
2,370,906 rows; narrow relationship tables frequently exceeded 180,000 rows/s,
showing that row width and conversion/encoding work matter more than row count
alone.

Both passes matched expected, copied and committed counts for every table,
including an empty table. This remains count-level operational evidence rather
than whole-table equality proof. The tested binary identified itself as a
dirty build, so preserve these measurements as capacity evidence but repeat
with a clean release binary before making a release-specific performance claim.

## Instrumentation model

Coarse phase timings use only a few clock reads per synchronization and are
always collected. `--perf-metrics` additionally times each SQL Server row's:

- `Rows.Next`: network wait plus driver decoding;
- `Rows.Scan`: assignment into generic Go values;
- conversion: money, UUID, rowversion and direct-value mapping.

Detailed per-row timing intentionally remains opt-in because clock reads add
measurement overhead. The CLI also reports total `pgx.CopyFrom` time and its
remainder after subtracting measured source work. That remainder includes pgx
encoding, client/server I/O and PostgreSQL direct or staging insertion; it is
not claimed as pure database execution time.

Optional `--cpu-profile=<path>` writes a Go CPU profile without enabling an
HTTP listener. The destination must not already exist. Profiles can contain
function names and paths but no source row values; keep them outside the
repository and review them before sharing.

## Local conversion microbenchmark

On the implementation host (Linux amd64, Intel i9-10980XE), five runs of the
allocation-aware conversion benchmark produced these representative values:

| Conversion | Approximate time/op | Bytes/op | Allocations/op |
| --- | ---: | ---: | ---: |
| Direct integer | 4.6 ns | 0 | 0 |
| Money to exact numeric | 1.17 µs | 192 | 9 |
| UUID string to typed UUID | 173 ns | 16 | 1 |
| Rowversion to checked bigint | 57 ns | 8 | 1 |

`goelandtextindex` has 14 columns using direct integer/text/boolean/timestamp
paths. At 5,607,406 rows, 78.5 million direct conversion dispatches extrapolate
to roughly 0.36 seconds in isolation. That does not include row-buffer
allocation, driver decoding or pgx encoding, but it is evidence against
optimizing the direct conversion switch before profiling the complete stream.

Reproduce with:

```sh
go test -run '^$' -bench '^BenchmarkConvertValue$' \
  -benchmem -count=5 ./internal/schema
```

## Measurement command

Against an operator-authorized table and target:

```sh
./bin/go-db-sync-mssql-pgsql \
  --source-table goelandtextindex \
  --apply \
  --sync-mode=replace \
  --perf-metrics

./bin/go-db-sync-mssql-pgsql \
  --source-table goelandtextindex \
  --apply \
  --cpu-profile=/tmp/goelandtextindex.cpu.pprof

go tool pprof -top \
  ./bin/go-db-sync-mssql-pgsql \
  /tmp/goelandtextindex.cpu.pprof
```

Run detailed timers and CPU profiling in separate refreshes against equivalent
target state, so timer calls do not pollute the CPU profile. Both modes perturb
the result, so neither run is the performance baseline.

## Recommendations before optimization

1. Use default replace for disposable daily transit tables. Use merge only when
   incoming dependencies or stable live row identity require reconciliation.
2. If `Rows.Next` dominates, investigate SQL Server/network/driver decoding. If
   conversion dominates, use the profile and allocation evidence to identify a
   specific type path. If the COPY remainder dominates, inspect pgx encoding,
   socket writes and PostgreSQL COPY processing.
3. Keep `--workers` fixed and operator-selected. Do not add dynamic concurrency
   or concurrency inside one table; retain exact values, bounded per-table
   streaming and transactional publication while measuring database pressure.
