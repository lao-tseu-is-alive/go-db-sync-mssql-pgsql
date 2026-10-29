// Package version provides the audited application version and optional build metadata.
package version

const (
	// AppName is the CLI executable name.
	AppName = "go-db-sync-mssql-pgsql"
	// Version is the single source of truth for the release version.
	Version = "0.2.0"
	// Repository is the Go module path used by build-time linker flags.
	Repository = "github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql"
)

var (
	// Revision is the source revision injected at build time.
	Revision = "unknown"
	// BuildStamp is the UTC build timestamp injected at build time.
	BuildStamp = "unknown"
)

// String returns the operator-facing version string.
func String() string {
	return AppName + " v" + Version + " (revision " + Revision + ", built " + BuildStamp + ")"
}
