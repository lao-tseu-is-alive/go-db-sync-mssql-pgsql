// Package config parses the guarded table-selection CLI contract and secret-bearing DSNs.
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
  go-db-sync-mssql-pgsql --all-tables --dry-run [options]
  go-db-sync-mssql-pgsql --all-tables --apply [options]
  go-db-sync-mssql-pgsql --list-tables [--source-schema NAME]
  go-db-sync-mssql-pgsql --version

Choose one source table or all base tables in the source schema. --dry-run
prints and validates the plan without PostgreSQL writes. --apply explicitly
authorizes independent transactional refreshes. A multi-table run is not
database-wide atomic.

Synchronization settings are read from exported MSSQL_DSN and PGSQL_DSN
values, then from an optional ./.env fallback. --list-tables requires only
MSSQL_DSN and never connects to PostgreSQL. Exported values take precedence.
DSNs are never printed. The SQL Server credential should be read-only.

Options:
`

// Options is the validated CLI and environment configuration.
type Options struct {
	// SourceSchema is the selected SQL Server schema.
	SourceSchema string
	// SourceTable is the selected SQL Server table in single-table mode.
	SourceTable string
	// AllTables selects all discovered base tables in SourceSchema.
	AllTables bool
	// ListTables requests a read-only source-table inventory without PostgreSQL.
	ListTables bool
	// TablesConfig is an optional YAML file containing exact source-name exclusions.
	TablesConfig string
	// Workers bounds concurrent multi-table jobs and database connection pairs.
	Workers int
	// ShowExcluded prints every excluded table in an all-table plan.
	ShowExcluded bool
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

var (
	// ErrMissingDSN reports that one or both synchronization DSNs are unavailable.
	ErrMissingDSN = errors.New("MSSQL_DSN and PGSQL_DSN are required")
	// ErrMissingMSSQLDSN reports that source-only table listing lacks its DSN.
	ErrMissingMSSQLDSN = errors.New("MSSQL_DSN is required")
)

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
	flags.StringVar(&options.SourceTable, "source-table", "", "one SQL Server source table")
	flags.BoolVar(&options.AllTables, "all-tables", false, "select all base tables in the source schema")
	flags.BoolVar(&options.ListTables, "list-tables", false, "list exact SQL Server base-table names without connecting to PostgreSQL")
	flags.StringVar(&options.TablesConfig, "tables-config", "", "YAML file with exact MSSQL table-name exclusions (requires --all-tables)")
	flags.IntVar(&options.Workers, "workers", 2, "maximum concurrent table jobs in --all-tables mode (minimum 1)")
	flags.BoolVar(&options.ShowExcluded, "show-excluded", false, "show excluded table details in an --all-tables plan")
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
	if options.ListTables {
		return parseListTables(options, getenv)
	}
	if (options.SourceTable == "" && !options.AllTables) || (options.SourceTable != "" && options.AllTables) {
		return Options{}, errors.New("choose exactly one of --source-table or --all-tables")
	}
	if options.TablesConfig != "" && !options.AllTables {
		return Options{}, errors.New("--tables-config requires --all-tables")
	}
	if options.ShowExcluded && !options.AllTables {
		return Options{}, errors.New("--show-excluded requires --all-tables")
	}
	if options.AllTables && options.TargetTable != "" {
		return Options{}, errors.New("--target-table cannot be used with --all-tables")
	}
	if options.Workers < 1 {
		return Options{}, errors.New("--workers must be at least 1")
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
	if !options.AllTables && options.TargetTable == "" {
		options.TargetTable = naming.TableName(options.SourceTable)
	}
	if err := ident.ValidateSource(options.SourceSchema); err != nil {
		return Options{}, fmt.Errorf("--source-schema: %w", err)
	}
	if options.SourceTable != "" {
		if err := ident.ValidateSource(options.SourceTable); err != nil {
			return Options{}, fmt.Errorf("--source-table: %w", err)
		}
	}
	if err := ident.ValidateTarget(options.TargetSchema); err != nil {
		return Options{}, fmt.Errorf("--target-schema: %w", err)
	}
	if options.TargetTable != "" {
		if err := ident.ValidateTarget(options.TargetTable); err != nil {
			return Options{}, fmt.Errorf("--target-table: %w", err)
		}
	}
	options.MSSQLDSN = getenv("MSSQL_DSN")
	options.PGSQLDSN = getenv("PGSQL_DSN")
	if options.MSSQLDSN == "" || options.PGSQLDSN == "" {
		return Options{}, ErrMissingDSN
	}
	return options, nil
}

func parseListTables(options Options, getenv func(string) string) (Options, error) {
	if options.SourceTable != "" || options.AllTables {
		return Options{}, errors.New("--list-tables cannot be combined with --source-table or --all-tables")
	}
	if options.Apply || options.DryRun {
		return Options{}, errors.New("--list-tables does not accept --apply or --dry-run")
	}
	if options.TablesConfig != "" || options.TargetTable != "" || options.ShowExcluded {
		return Options{}, errors.New("--list-tables does not accept --tables-config, --target-table or --show-excluded")
	}
	if err := ident.ValidateSource(options.SourceSchema); err != nil {
		return Options{}, fmt.Errorf("--source-schema: %w", err)
	}
	options.MSSQLDSN = getenv("MSSQL_DSN")
	if options.MSSQLDSN == "" {
		return Options{}, ErrMissingMSSQLDSN
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
