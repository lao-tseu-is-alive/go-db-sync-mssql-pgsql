# Documentation quality contract

This document is the normative documentation contract for
`go-db-sync-mssql-pgsql`. Documentation is part of implementation and must be
updated in the same change as behavior, repository inventory or release state.

The words **MUST**, **MUST NOT**, **SHOULD** and **MAY** express requirement
levels. Automated checks prove structure and selected stable facts; review is
still responsible for semantic accuracy.

## Sources of truth

| Source | Responsibility | Enforcement |
| --- | --- | --- |
| Go source | Package purpose and exported caller contracts. | `make godoc-check` |
| `docs/COMPATIBILITY.md` | Exact legacy naming/schema compatibility and deliberate corrections. | Tests and `make docs-assert` |
| `docs/atlas.md` | Exactly one responsibility entry for every tracked or new non-ignored file. | `make atlas-check` |
| `docs/ROADMAP.md` | Unique `SYNC-NNN` task order and state. | `make roadmap-check` |
| `docs/IMPLEMENTATION_STATUS.md` | Honest completed/planned/known-gap snapshot. | Review and assertions |
| `CHANGELOG.md` | Released behavior by version and date. | `make changelog-check` |
| `Makefile` | Composed local and release controls. | The controls themselves |

`AGENTS.md` and `README.md` link here; summaries in those files do not
supersede this contract.

## Go source contract

Every production Go package MUST have a purpose-driven package comment. A
library package starts with `Package <name>`; a main package starts with
`Command` or `Package main`. Every exported type, function, method, constant
and variable MUST have useful GoDoc beginning with the exact identifier. Every
named exported struct field follows the same rule.

Comments describe facts the type system cannot: defaults, units, nullability,
ordering, transaction/cancellation/resource boundaries, secret handling,
failure conditions and whether data is streamed. English is used for source
comments and documentation.

`cmd/doccheck --scope go` checks non-generated production Go files returned by
the canonical Git inventory. Tests are excluded mechanically but remain
subject to review.

## Repository atlas contract

`docs/atlas.md` MUST have exactly one canonical entry for every path returned
by:

```sh
git ls-files -z --cached --others --exclude-standard
```

Entries use this exact form:

```text
- `relative/path` — Responsibility and authority note.
```

The atlas banner `Tracked version: **vX.Y.Z**.` MUST match the `Version`
constant in `pkg/version/version.go`. Additions, removals and renames update
the atlas in the same change. Generated artifacts, examples and authoritative
inputs are distinguished in descriptions.

## Executable claims

Stable security and operational claims shared across code and prose SHOULD be
asserted in `scripts/check_documentation_claims.sh`. Assertions are deliberately
literal and visible. They cover the explicit apply guard, one-table boundary,
schema defaults, publication-mode default, prohibition on `TRUNCATE CASCADE`,
secret handling, snapshot assumption and control-chain links; they do not
replace behavioral tests.

Operator documentation MUST identify destructive behavior and its transaction
boundary. In particular, the default `replace` mode is described as
transactional `TRUNCATE ... RESTRICT` followed by direct COPY and count
verification, while `merge` is described as the staging-based compatibility
mode. Documentation MUST NOT imply that `--dry-run` or a failed transaction
publishes partial data.

A checker change MUST include tests showing accepted and rejected cases where
practical. `cmd/doccheck` has unit tests for atlas drift and GoDoc omissions.

## Roadmap, version and changelog

`pkg/version/version.go` is the single version source. `Version` is a constant;
only revision and build timestamp may be injected by linker flags. README,
roadmap, atlas and the current dated changelog section agree at a release
boundary.

Roadmap IDs are unique `SYNC-NNN` values and are never reused. A task is marked
complete only after its code and relevant gates pass. The `Next action` section
identifies the single best next slice. Implementation status distinguishes
verified work from planned work and known gaps.

## Control chain

```text
make docs-check
  ├─ make godoc-check
  ├─ make atlas-check
  └─ make docs-assert

make check
  ├─ make fmt-check
  ├─ make lint
  ├─ make test
  ├─ make docs-check
  └─ git diff --check

make release-check
  ├─ make check
  ├─ make vuln-check
  ├─ make version-check
  ├─ make changelog-check
  ├─ make roadmap-check
  └─ build and verify --version
```

CI runs the repository-owned `make release-check`, not a weaker parallel set
of rules. Integration tests remain opt-in because they require explicitly
authorized disposable databases.

## Definition of done

A change is complete when public contracts describe caller-relevant behavior,
the atlas exactly matches inventory, stable cross-file claims agree, roadmap
and status are honest, and `make docs-check` plus the broader appropriate gate
pass. Before ordinary handoff, run `make check`; before release, run
`make release-check`.
