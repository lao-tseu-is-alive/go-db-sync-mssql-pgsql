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
