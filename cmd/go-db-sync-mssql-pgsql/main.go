// Command go-db-sync-mssql-pgsql performs guarded SQL Server snapshot refreshes.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/pprof"
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
	if errors.Is(err, config.ErrMissingDSN) || errors.Is(err, config.ErrMissingMSSQLDSN) {
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
	if options.ListTables {
		return runListTables(ctx, options, stdout, stderr)
	}
	if options.AllTables {
		return runAllTables(ctx, options, stdout, stderr)
	}

	connectCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	source, err := mssql.Open(connectCtx, options.MSSQLDSN)
	if err != nil {
		cancel()
		fmt.Fprintf(stderr, "error: %v\n", config.RedactError(err, options.MSSQLDSN, options.PGSQLDSN))
		return 1
	}
	defer source.Close()
	source.EnablePerformanceMetrics(options.PerformanceMetrics)
	target, err := postgres.Open(connectCtx, options.PGSQLDSN)
	cancel()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", config.RedactError(err, options.MSSQLDSN, options.PGSQLDSN))
		return 1
	}
	defer target.Close()
	stopCPUProfile, err := startCPUProfile(options.CPUProfile)
	if err != nil {
		fmt.Fprintf(stderr, "error: start CPU profile: %v\n", err)
		return 2
	}
	defer func() { _ = stopCPUProfile() }()

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
		SyncMode:         options.SyncMode,
		ProgressInterval: options.ProgressInterval,
		Progress:         progress,
	})
	profileErr := stopCPUProfile()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", config.RedactError(err, options.MSSQLDSN, options.PGSQLDSN))
		return 1
	}
	if profileErr != nil {
		fmt.Fprintf(stderr, "error: finish CPU profile: %v\n", profileErr)
		return 1
	}
	if options.CPUProfile != "" {
		fmt.Fprintf(stdout, "cpu_profile=%s\n", options.CPUProfile)
	}

	mode := "dry-run"
	if report.Applied {
		mode = "applied"
	}
	fmt.Fprintf(stdout, "mode=%s sync_mode=%s source=%s.%s target=%s.%s columns=%d source_rows=%d duration=%s\n",
		mode, options.SyncMode, report.Table.SourceSchema, report.Table.SourceName, report.Table.TargetSchema,
		report.Table.TargetName, len(report.Table.Columns), report.SourceRows, report.Duration.Round(time.Millisecond))
	if !report.Applied {
		fmt.Fprintf(stdout, "target_exists=%t target_compatible=%t no PostgreSQL writes performed\n",
			report.TargetStatus.Exists, report.TargetStatus.Compatible)
		if options.PerformanceMetrics {
			printPerformance(stdout, report)
		}
		return 0
	}
	rate := float64(report.Publication.PublishedRows) / report.Duration.Seconds()
	if report.Duration <= 0 {
		rate = 0
	}
	fmt.Fprintf(stdout, "copied_rows=%d published_rows=%d target_created=%t copy_duration=%s stage_duration=%s publish_duration=%s rows_per_second=%.1f\n",
		report.Publication.CopiedRows, report.Publication.PublishedRows, report.Publication.CreatedTarget,
		report.Publication.Performance.CopyFromDuration.Round(time.Millisecond), report.Publication.StageDuration.Round(time.Millisecond),
		report.Publication.PublishDuration.Round(time.Millisecond), rate)
	if options.PerformanceMetrics {
		printPerformance(stdout, report)
	}
	return 0
}

func startCPUProfile(path string) (func() error, error) {
	if path == "" {
		return func() error { return nil }, nil
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create %q without overwriting: %w", path, err)
	}
	if err := pprof.StartCPUProfile(file); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("start runtime/pprof: %w", err)
	}
	stopped := false
	return func() error {
		if stopped {
			return nil
		}
		stopped = true
		pprof.StopCPUProfile()
		if err := file.Close(); err != nil {
			return fmt.Errorf("close %q: %w", path, err)
		}
		return nil
	}, nil
}

func printPerformance(output io.Writer, report syncer.Report) {
	performance := report.Performance
	fmt.Fprintf(output, "perf orchestration discovery=%s target_validation=%s source_count=%s source_query=%s\n",
		performance.DiscoveryDuration.Round(time.Microsecond),
		performance.TargetValidationDuration.Round(time.Microsecond),
		performance.SourceCountDuration.Round(time.Microsecond),
		performance.SourceQueryDuration.Round(time.Microsecond))
	if !report.Applied {
		return
	}
	publication := performance.Publication
	source := publication.Source
	fmt.Fprintf(output, "perf mssql rows=%d next=%s scan=%s conversion=%s total=%s\n",
		source.Rows, source.NextDuration.Round(time.Microsecond), source.ScanDuration.Round(time.Microsecond),
		source.ConversionDuration.Round(time.Microsecond), source.TotalDuration().Round(time.Microsecond))
	fmt.Fprintf(output, "perf copy copy_from=%s source_accounted=%s pipeline_remainder=%s\n",
		publication.CopyFromDuration.Round(time.Microsecond), source.TotalDuration().Round(time.Microsecond),
		publication.CopyPipelineRemainder().Round(time.Microsecond))
	fmt.Fprintf(output, "perf postgres target_prepare=%s truncate=%s stage_create=%s stage_verify=%s live_insert=%s live_delete=%s final_verify=%s commit=%s\n",
		publication.TargetPreparationDuration.Round(time.Microsecond), publication.TruncateDuration.Round(time.Microsecond),
		publication.StageCreateDuration.Round(time.Microsecond), publication.StageVerifyDuration.Round(time.Microsecond), publication.LiveInsertDuration.Round(time.Microsecond),
		publication.LiveDeleteDuration.Round(time.Microsecond), publication.FinalVerifyDuration.Round(time.Microsecond),
		publication.CommitDuration.Round(time.Microsecond))
}

func progressPrinter(output io.Writer) func(syncer.Progress) {
	return progressPrinterWithPrefix(output, "")
}

func progressPrinterWithPrefix(output io.Writer, prefix string) func(syncer.Progress) {
	return func(progress syncer.Progress) {
		if progress.Phase != "staging" && progress.Phase != "copying" {
			fmt.Fprintf(output, "%sphase=%s\n", prefix, progress.Phase)
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
		fmt.Fprintf(output, "%sphase=%s rows=%d/%d percent=%.1f elapsed=%s rows_per_second=%.1f\n",
			prefix, progress.Phase, progress.Rows, progress.TotalRows, percent, progress.Elapsed.Round(time.Second), rate)
	}
}
