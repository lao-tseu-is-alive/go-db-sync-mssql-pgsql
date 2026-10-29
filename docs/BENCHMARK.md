# Benchmark protocol

No performance result or speedup is claimed until this protocol is run against
the same explicitly selected table and the same static SQL Server snapshot.
Never record DSNs, credentials or source row samples.

## Preconditions

1. Obtain operator approval for the source table and disposable PostgreSQL
   target.
2. Record SQL Server backup timestamp, restore timestamp, table identifier,
   source row count, host/VM resources, Go version, Python revision, Go revision,
   PostgreSQL version and SQL Server version.
3. Confirm the source snapshot cannot change during both runs.
4. Start both implementations from equivalent compatible target structures and
   contents. Do not benchmark against production PostgreSQL.
5. Run one warm-up outside the recorded trials if storage-cache effects matter.

## Commands

Run at least three recorded trials of each implementation. Supply credentials
through the environment without echoing them.

```sh
/usr/bin/time -v ./bin/go-db-sync-mssql-pgsql \
  --source-schema dbo --source-table <selected-table> \
  --target-schema public --apply --sync-mode=replace

/usr/bin/time -v ./bin/go-db-sync-mssql-pgsql \
  --source-schema dbo --source-table <selected-table> \
  --target-schema public --apply --sync-mode=merge

cd /home/cgil/PycharmProjects/mssql2pgsql
/usr/bin/time -v uv run python copy_Mssql_Table_to_Postgresql.py <selected-table>
```

The legacy Python program truncates before copying and may leave partial data;
use only the disposable benchmark target. Reset equivalent target state between
trials.

## Record

For every trial record implementation/revision, source and destination counts,
wall elapsed seconds, calculated rows/sec, maximum resident set size, user CPU,
system CPU, success/failure and any non-sensitive observation. Use the median
successful trial for comparison and retain individual values. A count mismatch
invalidates the trial.

Peak RSS and CPU come from `/usr/bin/time -v`; rows/sec is source rows divided
by wall elapsed seconds. Do not compare runs across different snapshots,
tables, host load or database locations.

## Operator measurement — 2026-10-28

An operator ran both implementations sequentially for `dbo.goelandtextindex`
with the same reported source and destination count of 5,607,406 rows:

| Implementation | Real | User | System | Reported detail |
| --- | ---: | ---: | ---: | --- |
| Legacy Python | 102.832 s | 71.874 s | 3.775 s | Direct 20,000-row COPY batches to an empty existing target. |
| Go v0.1.0 | 92.510 s | 148.158 s | 29.543 s | 86.894 s staging; 5.256 s transactional publication; 60,658.1 rows/s overall. |

The observed wall-time reduction is 10.0%, not a general speedup claim. The Go
run consumed more client CPU and performed an additional verified staging and
publication step to preserve the previous live table on failure. Peak RSS,
exact host load and a multi-trial median were not captured, and the target setup
differed (Python reported an existing empty table; Go created its target), so
this is useful operator evidence rather than the complete controlled protocol.

Detailed attribution and reproducible conversion microbenchmarks live in
[`PERFORMANCE.md`](PERFORMANCE.md).

## Replace versus merge measurement — 2026-10-29

One operator-authorized instrumented trial of each mode ran sequentially on
`dbo.Document` with the same reported source count of 2,361,511 rows. Both
started with the compatible target populated from the same snapshot.

| Mode | Real | User | System | Peak RSS | COPY | Stage | Publication | Correctness |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| Replace | 81.12 s | 191.75 s | 34.48 s | 19,216 KiB | 80.617 s direct | 0 s | 80.807 s including COPY | source = copied = published = 2,361,511 |
| Merge | 97.82 s | 185.55 s | 29.71 s | 22,896 KiB | 77.760 s staged | 78.063 s | 19.508 s after staging | source = copied = published = 2,361,511 |

Replace was 16.70 seconds (17.1%) faster in wall time and its overall
throughput was 20.6% higher. Both commands used `--perf-metrics`, so this is a
same-conditions diagnostic comparison rather than the required three-trial,
uninstrumented release benchmark. Publication and COPY overlap in replace and
must not be summed. See [`PERFORMANCE.md`](PERFORMANCE.md) for phase attribution
and limitations.
