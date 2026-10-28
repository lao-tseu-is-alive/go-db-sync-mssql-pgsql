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

# Explicit mutation authorization and Phase 1 scope.
require_literal internal/config/config.go 'choose exactly one of --dry-run or --apply' 'explicit mode guard'
require_literal README.md 'can occur without `--apply`' 'operator apply guard'
require_literal AGENTS.md '**Phase 1 must implement and test ONE explicitly selected MSSQL table only.**' 'one-table scope'
require_literal README.md 'stops at one table per invocation' 'one-table operator scope'

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
require_literal .gitignore '.env.*' 'environment files ignored'
require_literal docs/IMPLEMENTATION_STATUS.md 'source remains static' 'source consistency assumption'

# Documentation governance uses one composed control chain.
require_literal AGENTS.md '[`docs/DOCUMENTATION.md`](docs/DOCUMENTATION.md) is the normative documentation' 'agent documentation entry point'
require_literal README.md '[documentation quality contract](docs/DOCUMENTATION.md) is normative' 'README documentation entry point'
require_literal Makefile 'docs-check: godoc-check atlas-check docs-assert' 'documentation gate composition'
require_literal Makefile 'check: fmt-check lint test docs-check' 'normal gate composition'
require_literal .github/workflows/ci.yml 'run: make release-check' 'CI repository gate'

if [[ "$failed" -ne 0 ]]; then
    exit 1
fi
echo 'docs-assert: OK'
