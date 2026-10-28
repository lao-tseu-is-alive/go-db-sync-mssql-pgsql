// Command go-db-sync-mssql-pgsql performs a guarded full refresh of one SQL Server table.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/config"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/mssql"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/postgres"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/syncer"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/pkg/version"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	options, err := config.Parse(arguments, os.Getenv, stderr)
	if errors.Is(err, config.ErrMissingDSN) {
		loaded, loadErr := config.LoadDotEnv(".env")
		if loadErr != nil {
			fmt.Fprintf(stderr, "error: load .env: %v\n", loadErr)
			return 2
		}
		if loaded {
			options, err = config.Parse(arguments, os.Getenv, stderr)
		}
	}
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	if options.Version {
		fmt.Fprintln(stdout, version.String())
		return 0
	}

	connectCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	source, err := mssql.Open(connectCtx, options.MSSQLDSN)
	if err != nil {
		cancel()
		fmt.Fprintf(stderr, "error: %v\n", config.RedactError(err, options.MSSQLDSN, options.PGSQLDSN))
		return 1
	}
	defer source.Close()
	target, err := postgres.Open(connectCtx, options.PGSQLDSN)
	cancel()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", config.RedactError(err, options.MSSQLDSN, options.PGSQLDSN))
		return 1
	}
	defer target.Close()

	var progress func(syncer.Progress)
	if options.ProgressInterval > 0 {
		progress = progressPrinter(stderr)
	}
	report, err := syncer.New(source, target).Run(ctx, syncer.Request{
		SourceSchema:     options.SourceSchema,
		SourceTable:      options.SourceTable,
		TargetSchema:     options.TargetSchema,
		TargetTable:      options.TargetTable,
		Apply:            options.Apply,
		ProgressInterval: options.ProgressInterval,
		Progress:         progress,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", config.RedactError(err, options.MSSQLDSN, options.PGSQLDSN))
		return 1
	}

	mode := "dry-run"
	if report.Applied {
		mode = "applied"
	}
	fmt.Fprintf(stdout, "mode=%s source=%s.%s target=%s.%s columns=%d source_rows=%d duration=%s\n",
		mode, report.Table.SourceSchema, report.Table.SourceName, report.Table.TargetSchema,
		report.Table.TargetName, len(report.Table.Columns), report.SourceRows, report.Duration.Round(time.Millisecond))
	if !report.Applied {
		fmt.Fprintf(stdout, "target_exists=%t target_compatible=%t no PostgreSQL writes performed\n",
			report.TargetStatus.Exists, report.TargetStatus.Compatible)
		return 0
	}
	rate := float64(report.Publication.PublishedRows) / report.Duration.Seconds()
	if report.Duration <= 0 {
		rate = 0
	}
	fmt.Fprintf(stdout, "staged_rows=%d published_rows=%d target_created=%t stage_duration=%s publish_duration=%s rows_per_second=%.1f\n",
		report.Publication.CopiedRows, report.Publication.PublishedRows, report.Publication.CreatedTarget,
		report.Publication.StageDuration.Round(time.Millisecond), report.Publication.PublishDuration.Round(time.Millisecond), rate)
	return 0
}

func progressPrinter(output io.Writer) func(syncer.Progress) {
	return func(progress syncer.Progress) {
		if progress.Phase != "staging" {
			fmt.Fprintf(output, "phase=%s\n", progress.Phase)
			return
		}
		percent := 0.0
		if progress.TotalRows > 0 {
			percent = float64(progress.Rows) * 100 / float64(progress.TotalRows)
		}
		rate := 0.0
		if progress.Elapsed > 0 {
			rate = float64(progress.Rows) / progress.Elapsed.Seconds()
		}
		fmt.Fprintf(output, "phase=staging rows=%d/%d percent=%.1f elapsed=%s rows_per_second=%.1f\n",
			progress.Rows, progress.TotalRows, percent, progress.Elapsed.Round(time.Second), rate)
	}
}
