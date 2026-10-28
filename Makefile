SHELL := /bin/bash
GOCACHE := /tmp/go-db-sync-mssql-pgsql-gocache
export GOCACHE

VERSION_FILE := pkg/version/version.go
APP_NAME := go-db-sync-mssql-pgsql
APP_VERSION := $(shell sed -n 's/^\s*Version\s*=\s*"\([^"]*\)"/\1/p' $(VERSION_FILE))
MODULE := github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql
REVISION := $(shell git describe --always --dirty 2>/dev/null || echo unknown)
BUILD_STAMP := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-X $(MODULE)/pkg/version.Revision=$(REVISION) -X $(MODULE)/pkg/version.BuildStamp=$(BUILD_STAMP)"

.DEFAULT_GOAL := help

.PHONY: help
## help: show available targets
help:
	@sed -n 's/^## /  /p' Makefile

.PHONY: fmt
## fmt: format all repository Go files
fmt:
	gofmt -w $$(git ls-files --cached --others --exclude-standard '*.go')

.PHONY: fmt-check
## fmt-check: fail when a Go file is not gofmt-formatted
fmt-check:
	@unformatted="$$(gofmt -l $$(git ls-files --cached --others --exclude-standard '*.go'))"; \
		test -z "$$unformatted" || { echo "fmt-check: run gofmt on:"; echo "$$unformatted"; exit 1; }

.PHONY: lint
## lint: run Go's static analyzer over all packages
lint:
	go vet ./...

.PHONY: test
## test: run normal tests with race detection and coverage (no database connections)
test:
	go test -race -covermode=atomic -coverprofile=coverage.out ./...

.PHONY: integration-test
## integration-test: run opt-in tests against DBSYNC_TEST_PGSQL_DSN (disposable DB only)
integration-test:
	go test -race -tags integration ./internal/postgres

.PHONY: binary
## binary: compile the CLI into bin/
binary:
	mkdir -p bin
	CGO_ENABLED=0 go build $(LDFLAGS) -o bin/$(APP_NAME) ./cmd/$(APP_NAME)

.PHONY: build
## build: run normal tests and compile the CLI
build: test binary

.PHONY: godoc-check
## godoc-check: require package and exported API contract comments
godoc-check:
	go run ./cmd/doccheck --scope go

.PHONY: atlas-check
## atlas-check: require exactly one atlas entry per non-ignored repository file
atlas-check:
	go run ./cmd/doccheck --scope atlas

.PHONY: docs-assert
## docs-assert: verify stable security and operational prose claims
docs-assert:
	bash scripts/check_documentation_claims.sh

.PHONY: docs-check
## docs-check: run GoDoc, atlas and stable-claim checks
docs-check: godoc-check atlas-check docs-assert

.PHONY: check
## check: run formatting, lint, tests, documentation and whitespace gates
check: fmt-check lint test docs-check
	git diff --check

.PHONY: vuln-check
## vuln-check: run the pinned govulncheck release on reachable code
vuln-check:
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

.PHONY: version-check
## version-check: require README and docs to match the source version
version-check:
	@test -n "$(APP_VERSION)" || { echo "version-check: cannot read $(VERSION_FILE)"; exit 1; }
	@grep -qF 'Current version: **v$(APP_VERSION)**.' README.md
	@grep -qF 'Tracked version: **v$(APP_VERSION)**.' docs/ROADMAP.md
	@grep -qF 'Tracked version: **v$(APP_VERSION)**.' docs/atlas.md
	@echo "version-check: v$(APP_VERSION)"

.PHONY: changelog-check
## changelog-check: require one dated section for the source version
changelog-check:
	@duplicates="$$(sed -n 's/^## \[\([0-9][0-9.]*\)\].*/\1/p' CHANGELOG.md | sort | uniq -d)"; \
		test -z "$$duplicates" || { echo "changelog-check: duplicate versions: $$duplicates"; exit 1; }
	@grep -qE '^## \[$(APP_VERSION)\] - [0-9]{4}-[0-9]{2}-[0-9]{2}$$' CHANGELOG.md
	@echo "changelog-check: v$(APP_VERSION) documented"

.PHONY: roadmap-check
## roadmap-check: require version, unique task IDs and a next action
roadmap-check:
	@grep -qF 'Tracked version: **v$(APP_VERSION)**.' docs/ROADMAP.md
	@ids="$$(grep -oE '^- \[[ x~-]\] \*\*SYNC-[0-9]{3}' docs/ROADMAP.md | grep -oE 'SYNC-[0-9]{3}')"; \
		test -n "$$ids" || { echo 'roadmap-check: no task IDs'; exit 1; }; \
		duplicates="$$(printf '%s\n' "$$ids" | sort | uniq -d)"; \
		test -z "$$duplicates" || { echo "roadmap-check: duplicate IDs: $$duplicates"; exit 1; }
	@grep -q '^## Next action$$' docs/ROADMAP.md
	@echo 'roadmap-check: OK'

.PHONY: release-check
## release-check: run normal gates, vulnerability/release checks and versioned binary verification
release-check: check vuln-check version-check changelog-check roadmap-check binary
	@bin/$(APP_NAME) --version | grep -qF '$(APP_NAME) v$(APP_VERSION) (revision '
	@echo "release-check: v$(APP_VERSION) OK"
