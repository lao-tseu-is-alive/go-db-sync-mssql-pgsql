// Package config parses the guarded single-table CLI contract and secret-bearing DSNs.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/ident"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/naming"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/syncmode"
)

const usageText = `Usage:
  go-db-sync-mssql-pgsql --source-table NAME --dry-run [options]
  go-db-sync-mssql-pgsql --source-table NAME --apply [options]
  go-db-sync-mssql-pgsql --version

Exactly one table is processed. --dry-run discovers and validates without
PostgreSQL writes. --apply explicitly authorizes a transactional full refresh.

Connection settings are read from exported MSSQL_DSN and PGSQL_DSN values,
then from an optional ./.env fallback. Exported values take precedence. DSNs
are never printed. The SQL Server credential should be read-only.

Options:
`

// Options is the validated CLI and environment configuration.
type Options struct {
	// SourceSchema is the selected SQL Server schema.
	SourceSchema string
	// SourceTable is the required SQL Server table.
	SourceTable string
	// TargetSchema is the selected PostgreSQL schema.
	TargetSchema string
	// TargetTable is the explicit or legacy-transformed PostgreSQL table.
	TargetTable string
	// Apply authorizes PostgreSQL mutation.
	Apply bool
	// DryRun permits discovery and validation only.
	DryRun bool
	// SyncMode selects direct replacement or dependency-preserving reconciliation.
	SyncMode syncmode.Mode
	// Version requests version output without database configuration.
	Version bool
	// ProgressInterval controls COPY progress output; zero disables it.
	ProgressInterval time.Duration
	// PerformanceMetrics enables detailed MSSQL row and COPY pipeline timings.
	PerformanceMetrics bool
	// CPUProfile is an optional new file path for a runtime/pprof CPU profile.
	CPUProfile string
	// MSSQLDSN is the secret-bearing source connection string and must not be logged.
	MSSQLDSN string
	// PGSQLDSN is the secret-bearing target connection string and must not be logged.
	PGSQLDSN string
}

// ErrMissingDSN reports that one or both required database DSNs are unavailable.
var ErrMissingDSN = errors.New("MSSQL_DSN and PGSQL_DSN are required")

// Parse validates command-line arguments and reads DSNs through getenv.
func Parse(arguments []string, getenv func(string) string, output io.Writer) (Options, error) {
	var options Options
	syncModeValue := string(syncmode.Replace)
	flags := flag.NewFlagSet("go-db-sync-mssql-pgsql", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.Usage = func() {
		fmt.Fprint(output, usageText)
		flags.PrintDefaults()
	}
	flags.StringVar(&options.SourceSchema, "source-schema", "dbo", "SQL Server source schema")
	flags.StringVar(&options.SourceTable, "source-table", "", "SQL Server source table (required)")
	flags.StringVar(&options.TargetSchema, "target-schema", "public", "PostgreSQL target schema")
	flags.StringVar(&options.TargetTable, "target-table", "", "PostgreSQL target table (default: legacy name transform)")
	flags.BoolVar(&options.Apply, "apply", false, "authorize the transactional PostgreSQL refresh")
	flags.BoolVar(&options.DryRun, "dry-run", false, "discover and validate without PostgreSQL writes")
	flags.StringVar(&syncModeValue, "sync-mode", syncModeValue, "publication mode: replace or merge")
	flags.BoolVar(&options.Version, "version", false, "print version and exit")
	flags.DurationVar(&options.ProgressInterval, "progress-interval", 5*time.Second, "COPY progress interval (0 disables progress)")
	flags.BoolVar(&options.PerformanceMetrics, "perf-metrics", false, "report detailed MSSQL and COPY pipeline timings")
	flags.StringVar(&options.CPUProfile, "cpu-profile", "", "write an optional CPU profile to a new file")
	if err := flags.Parse(arguments); err != nil {
		return Options{}, err
	}
	if flags.NArg() != 0 {
		return Options{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	if options.Version {
		return options, nil
	}
	if options.SourceTable == "" {
		return Options{}, errors.New("--source-table is required")
	}
	if options.Apply == options.DryRun {
		return Options{}, errors.New("choose exactly one of --dry-run or --apply")
	}
	if options.ProgressInterval < 0 {
		return Options{}, errors.New("--progress-interval cannot be negative")
	}
	mode, err := syncmode.Parse(syncModeValue)
	if err != nil {
		return Options{}, fmt.Errorf("--sync-mode: %w", err)
	}
	options.SyncMode = mode
	if options.TargetTable == "" {
		options.TargetTable = naming.TableName(options.SourceTable)
	}
	if err := ident.ValidateSource(options.SourceSchema); err != nil {
		return Options{}, fmt.Errorf("--source-schema: %w", err)
	}
	if err := ident.ValidateSource(options.SourceTable); err != nil {
		return Options{}, fmt.Errorf("--source-table: %w", err)
	}
	if err := ident.ValidateTarget(options.TargetSchema); err != nil {
		return Options{}, fmt.Errorf("--target-schema: %w", err)
	}
	if err := ident.ValidateTarget(options.TargetTable); err != nil {
		return Options{}, fmt.Errorf("--target-table: %w", err)
	}
	options.MSSQLDSN = getenv("MSSQL_DSN")
	options.PGSQLDSN = getenv("PGSQL_DSN")
	if options.MSSQLDSN == "" || options.PGSQLDSN == "" {
		return Options{}, ErrMissingDSN
	}
	return options, nil
}

var (
	urlUserInfoPattern = regexp.MustCompile(`://[^/@\s]+@`)
	passwordPattern    = regexp.MustCompile(`(?i)(password|pwd)\s*=\s*([^;\s]+)`)
)

// RedactError removes configured DSNs and common embedded password forms.
func RedactError(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "<redacted-dsn>")
		}
	}
	message = urlUserInfoPattern.ReplaceAllString(message, "://<redacted>@")
	message = passwordPattern.ReplaceAllString(message, "$1=<redacted>")
	return errors.New(message)
}
