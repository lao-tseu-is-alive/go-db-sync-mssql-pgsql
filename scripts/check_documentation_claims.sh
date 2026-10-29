#!/usr/bin/env bash
# Verify stable security and operational claims across their owning surfaces.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
failed=0

require_literal() {
    local file="$1"
    local literal="$2"
    local claim="$3"
    if ! grep -Fq -- "$literal" "$file"; then
        echo "docs-assert: ${claim}: ${file} lacks: ${literal}" >&2
        failed=1
    fi
}

# Explicit mutation authorization and Phase 2 selection scope.
require_literal internal/config/config.go 'choose exactly one of --dry-run or --apply' 'explicit mode guard'
require_literal README.md 'can occur without `--apply`' 'operator apply guard'
require_literal internal/config/config.go 'choose exactly one of --source-table or --all-tables' 'table selection guard'
require_literal internal/config/config.go 'list exact SQL Server base-table names without connecting to PostgreSQL' 'source-only listing flag'
require_literal README.md 'PostgreSQL connection and performs metadata reads only.' 'source-only listing boundary'
require_literal internal/config/config.go '"workers", 2' 'bounded worker default'
require_literal internal/config/config.go '"show-excluded", false' 'compact apply-plan default'
require_literal internal/schema/mapping.go 'return fmt.Sprintf("numeric(%d,%d)", precision, scale), ConversionDecimal, nil' 'exact decimal type mapping'
require_literal README.md 'without floating-point conversion' 'documented exact decimal conversion'
require_literal README.md 'They are not' 'multi-table transaction boundary'
require_literal README.md 'database-wide atomic' 'database-wide atomicity warning'
require_literal README.md 'imports ready only after complete success' 'downstream readiness guard'
require_literal config/config_Sample.yaml 'version: 1' 'tracked exact-exclusion sample schema'

# Defaults and naming behavior must agree.
require_literal internal/config/config.go '"source-schema", "dbo"' 'source schema default'
require_literal internal/config/config.go '"target-schema", "public"' 'target schema default'
require_literal README.md 'Defaults are `--source-schema=dbo`, `--target-schema=public`' 'documented schema defaults'
require_literal docs/COMPATIBILITY.md "\`c['name'].lower()\`" 'legacy column naming'

# Secret handling and static-snapshot assumptions are operationally load-bearing.
require_literal internal/config/config.go 'are never printed' 'CLI help secret claim'
require_literal README.md 'DSNs and source row values are never logged.' 'operator secret claim'
require_literal internal/config/dotenv.go 'if _, exists := os.LookupEnv(name); exists {' 'exported environment precedence'
require_literal README.md 'falls back to `./.env` only when a required DSN' 'dotenv fallback'
require_literal internal/config/config.go '"progress-interval", 5*time.Second' 'progress default'
require_literal README.md '`--progress-interval=0` to disable' 'progress control'
require_literal internal/config/config.go 'syncModeValue := string(syncmode.Replace)' 'replace mode default'
require_literal internal/postgres/sql.go '"TRUNCATE TABLE " + qualifiedName(table.TargetSchema, table.TargetName) + " RESTRICT"' 'replace uses truncate restrict'
require_literal README.md 'Replace mode never uses `TRUNCATE CASCADE`.' 'cascade prohibition'
require_literal README.md '`--sync-mode=merge` retains the original temporary staging' 'merge compatibility mode'
require_literal internal/config/config.go '"perf-metrics", false' 'performance metrics opt-in'
require_literal internal/config/config.go '"cpu-profile", ""' 'CPU profile opt-in'
require_literal README.md 'refuses to overwrite an existing file' 'profile overwrite safety'
require_literal AGENTS.md '`--perf-metrics` enables per-row SQL Server' 'agent performance metrics hand-off'
require_literal AGENTS.md '`--cpu-profile=<new-file>` writes a local `runtime/pprof` CPU profile' 'agent CPU profile hand-off'
require_literal AGENTS.md 'Run `--perf-metrics` and `--cpu-profile` in separate refreshes' 'agent measurement separation'
require_literal AGENTS.md '`--sync-mode=replace|merge` selects PostgreSQL publication.' 'agent publication-mode hand-off'
require_literal .gitignore '.env.*' 'environment files ignored'
require_literal docs/IMPLEMENTATION_STATUS.md 'source remains static' 'source consistency assumption'

# Documentation governance uses one composed control chain.
require_literal AGENTS.md '[`docs/DOCUMENTATION.md`](docs/DOCUMENTATION.md) is the normative documentation' 'agent documentation entry point'
require_literal README.md '[documentation quality contract](docs/DOCUMENTATION.md) is normative' 'README documentation entry point'
require_literal Makefile 'docs-check: godoc-check atlas-check docs-assert' 'documentation gate composition'
require_literal Makefile 'check: fmt-check lint test config-check docs-check' 'normal gate composition'
require_literal .github/workflows/ci.yml 'run: make release-check' 'CI repository gate'
require_literal .github/workflows/release.yml 'contents: write' 'release contents permission'
require_literal .github/workflows/release.yml 'run: make release-check' 'release repository gate'
require_literal .github/workflows/release.yml 'sha256sum "${APP_NAME}"-linux-*.tar.gz >SHA256SUMS' 'release checksums'
require_literal .github/workflows/release.yml 'tag ${release_version} does not match source version' 'release tag/version guard'

if [[ "$failed" -ne 0 ]]; then
    exit 1
fi
echo 'docs-assert: OK'
