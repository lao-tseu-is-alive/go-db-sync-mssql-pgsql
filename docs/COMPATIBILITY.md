# Legacy compatibility contract

This Phase 1 implementation uses the local Python `mssql2pgsql` repository as
the semantic reference. It deliberately does not preserve its unsafe transfer
mechanics.

| Concern | Python behavior | Phase 1 behavior |
| --- | --- | --- |
| Source/destination schema | SQL Server `dbo`; PostgreSQL `public` | Same defaults; both are explicit CLI options. |
| Table name | Ordered `standard_table_names()` replacements, then `re.sub('(?!^)([A-Z]+)', r'_\1', ...).lower()` | Exact ordered port, covered for every replacement and uppercase-run edge cases. |
| Column name | `c['name'].lower()` | Exact lowercase mapping; case-fold collisions are rejected. |
| Column and PK order | Reflection order; PK output is accidentally column order | Source column order is preserved; SQL Server PK key order is preserved deliberately. |
| Nullability | Reflected into `NOT NULL` | Same for a newly created target and validated on an existing target. |
| Data transfer | Values converted to delimited text, `\N` overloaded as NULL, 20,000-row partial commits | Typed, bounded-memory `pgx.CopyFrom`; SQL NULL and literal `\N` remain distinct. |
| Refresh | Live table truncated before transfer | Default `replace` truncates with `RESTRICT`, direct-COPYs and verifies in one rollback-safe transaction; optional `merge` preserves verified staging and reconciliation. |
| Existing target | Structure is not validated | Exact ordered columns, mapped types, nullability and PK order are required. No DROP/ALTER. |
| Empty source | Truncates the target before copy | Publishes an empty verified stage atomically. |

## Ordered table-name replacements

The replacements are literal, case-sensitive, global, and occur in this
order: `AGF`, `EGID`, `ARCH`, `ASST`, `CID`, `CIL`, `ESTRID`, `ES`, `GC`,
`GEO`, `CCJP`, `CH`, `CN`, `RCB`, `DMZ`, `ISO`, `NT`, `OPC`, `POLC`, `QSE`,
`RM`, `SAP`, `SCC`, `SPD`, `URB`. Each is changed to the mixed-case spelling
used by the Python function before its unusual uppercase-run regex runs.

The regex inserts one underscore before each non-initial run of ASCII capital
letters. If a capital run starts at byte zero, the first capital is excluded
by `(?!^)`, but the rest of that same run can still match. The Go port preserves
that detail rather than substituting a conventional snake-case algorithm.

## Type mapping

| SQL Server metadata type | PostgreSQL type | Status |
| --- | --- | --- |
| `tinyint`, `smallint`, `int`/`integer` | `integer` | Legacy-compatible. |
| `bigint` | `bigint` | Legacy-compatible. |
| `bit` | `boolean` | Legacy-compatible, transferred as a typed boolean. |
| `varchar`, `nvarchar`, `nchar`, `text`, `ntext` | `text` | Legacy-compatible. Length limits are intentionally not introduced. |
| `char(n)` | `char(n)` | Legacy-compatible; a missing/invalid length is rejected instead of using the legacy fallback of 5. |
| `binary`, `varbinary` | `bytea` | Legacy-compatible target type, transferred as bytes. |
| `smalldatetime`, `datetime` | `timestamp without time zone` | Legacy-compatible. |
| `money`, `smallmoney` | `numeric(20,4)` | Legacy-compatible target type; decimal text is preserved without float conversion. |
| `uniqueidentifier` | `uuid` | Legacy-compatible. |
| `timestamp`, `rowversion` | `bigint` | Interprets exactly 8 bytes as an unsigned big-endian value and rejects values above PostgreSQL signed `bigint`. |

Phase 1 rejects `decimal`/`numeric`, `datetime2`, `datetimeoffset`, `date`,
`time`, `float`, `real`, `image`, `xml`, `sql_variant`, spatial/CLR types and
all unlisted types. The Python fallback emitted the SQLAlchemy type spelling,
which is not a safe or unambiguous contract for precision, timezone or binary
semantics. Adding any rejected type requires a documented mapping and exact
value tests.

The SQL Server driver exposes `uniqueidentifier` bytes in a configurable wire
order. To avoid a DSN-dependent UUID permutation, the generated source `SELECT`
projects only that type through SQL Server's canonical 36-character rendering,
then parses it into a typed UUID before binary COPY. Other supported values are
selected in their native driver representation.

## Deliberate safety corrections

- Identifiers are validated and quoted for their database dialect. CLI input
  can select only schema/table identifiers, never an SQL expression.
- A missing target is created from source metadata. An existing target is
  validated before source rows are copied or live rows are touched.
- Default `replace` publication uses `TRUNCATE TABLE ... RESTRICT` and direct
  COPY in one transaction. It never uses `CASCADE`; unsupported incoming
  foreign-key dependencies fail and rollback. Optional `merge` upserts by the
  source primary key and removes absent keys, preserving referenced rows where
  keys remain; without a primary key it uses transactional delete-and-insert.
- Row count is checked after COPY and again after publication; merge also
  checks its staging table. The restored
  SQL Server database is assumed not to change between the initial count and
  the streaming query; Phase 1 does not create a SQL Server snapshot.
- Source rows and DSNs are never logged. Driver errors are passed through a
  redactor before operator-facing output.

## Known identifier limits

SQL Server source identifiers may be at most 128 Unicode code points and may
not contain NUL/control characters. PostgreSQL identifiers must additionally
fit 63 UTF-8 bytes. A lowercased column collision, an over-length transformed
name, or an otherwise ambiguous mapping is refused before target mutation.
