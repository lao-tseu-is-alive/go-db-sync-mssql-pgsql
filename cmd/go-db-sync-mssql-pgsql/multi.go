package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/config"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/mssql"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/multisync"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/postgres"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/syncer"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/tableconfig"
)

type preparedTable struct {
	plan   multisync.TablePlan
	report syncer.Report
	err    error
}

func runAllTables(ctx context.Context, options config.Options, stdout, stderr io.Writer) int {
	overallStarted := time.Now()
	stopCPUProfile, err := startCPUProfile(options.CPUProfile)
	if err != nil {
		fmt.Fprintf(stderr, "error: start CPU profile: %v\n", err)
		return 2
	}
	var progressLock sync.Mutex
	var progressForTable func(string) func(syncer.Progress)
	if options.ProgressInterval > 0 {
		fmt.Fprintln(stderr, "phase=discovering-tables")
		progressForTable = func(table string) func(syncer.Progress) {
			return multiProgressPrinter(stderr, &progressLock, options.SourceSchema+"."+table)
		}
	}
	plan, prepared, err := prepareAllTables(ctx, options, progressForTable)
	if err != nil {
		_ = stopCPUProfile()
		fmt.Fprintf(stderr, "error: %v\n", redact(options, err))
		return 1
	}
	printPlan(stdout, options, plan, prepared, time.Since(overallStarted))
	if planInvalid(plan, prepared) {
		_ = stopCPUProfile()
		fmt.Fprintln(stderr, "error: multi-table plan is invalid; no PostgreSQL writes performed")
		return 1
	}
	if options.DryRun {
		profileErr := stopCPUProfile()
		if profileErr != nil {
			fmt.Fprintf(stderr, "error: finish CPU profile: %v\n", profileErr)
			return 1
		}
		printCPUProfile(stdout, options.CPUProfile)
		return 0
	}

	jobs := make([]multisync.Job, 0, len(prepared))
	for _, table := range prepared {
		if table.plan.Excluded {
			continue
		}
		request := tableRequest(options, table.plan)
		if options.ProgressInterval > 0 {
			request.Progress = multiProgressPrinter(stderr, &progressLock, options.SourceSchema+"."+table.plan.SourceName)
		}
		jobs = append(jobs, multisync.Job{Request: request, Prepared: table.report})
	}
	summary, runErr := multisync.Run(ctx, jobs, options.Workers, workerFactory(options))
	profileErr := stopCPUProfile()
	printApplySummary(stdout, options, summary, time.Since(overallStarted))
	if profileErr != nil {
		fmt.Fprintf(stderr, "error: finish CPU profile: %v\n", profileErr)
		return 1
	}
	printCPUProfile(stdout, options.CPUProfile)
	if runErr != nil {
		fmt.Fprintf(stderr, "error: %v\n", redact(options, runErr))
		return 1
	}
	if summary.Failed > 0 || summary.Cancelled > 0 {
		fmt.Fprintln(stderr, "error: multi-table run incomplete; downstream imports must not start")
		return 1
	}
	return 0
}

func prepareAllTables(
	ctx context.Context,
	options config.Options,
	progressForTable func(string) func(syncer.Progress),
) (multisync.Plan, []preparedTable, error) {
	exclusions := []string(nil)
	if options.TablesConfig != "" {
		selection, err := tableconfig.Load(options.TablesConfig)
		if err != nil {
			return multisync.Plan{}, nil, err
		}
		exclusions = selection.Exclude
	}
	source, target, closeClients, err := openClientPair(ctx, options)
	if err != nil {
		return multisync.Plan{}, nil, err
	}
	defer closeClients()
	discovered, err := source.Tables(ctx, options.SourceSchema)
	if err != nil {
		return multisync.Plan{}, nil, err
	}
	plan, err := multisync.BuildPlan(options.SourceSchema, options.TargetSchema, discovered, exclusions)
	if err != nil {
		return multisync.Plan{}, nil, err
	}
	service := syncer.New(source, target)
	prepared := make([]preparedTable, 0, len(plan.Tables))
	for _, table := range plan.Tables {
		item := preparedTable{plan: table}
		if !table.Excluded {
			request := tableRequest(options, table)
			if progressForTable != nil {
				request.Progress = progressForTable(table.SourceName)
			}
			item.report, item.err = service.Prepare(ctx, request)
		}
		prepared = append(prepared, item)
	}
	return plan, prepared, nil
}

func tableRequest(options config.Options, table multisync.TablePlan) syncer.Request {
	return syncer.Request{
		SourceSchema:     options.SourceSchema,
		SourceTable:      table.SourceName,
		TargetSchema:     options.TargetSchema,
		TargetTable:      table.TargetName,
		Apply:            options.Apply,
		SyncMode:         options.SyncMode,
		ProgressInterval: options.ProgressInterval,
	}
}

func openClientPair(ctx context.Context, options config.Options) (*mssql.Client, *postgres.Client, func(), error) {
	connectCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	source, err := mssql.Open(connectCtx, options.MSSQLDSN)
	if err != nil {
		return nil, nil, nil, err
	}
	source.EnablePerformanceMetrics(options.PerformanceMetrics)
	target, err := postgres.Open(connectCtx, options.PGSQLDSN)
	if err != nil {
		_ = source.Close()
		return nil, nil, nil, err
	}
	return source, target, func() {
		target.Close()
		_ = source.Close()
	}, nil
}

func workerFactory(options config.Options) multisync.WorkerFactory {
	return func(ctx context.Context, _ int) (multisync.Worker, func(), error) {
		source, target, cleanup, err := openClientPair(ctx, options)
		if err != nil {
			return nil, nil, err
		}
		return syncer.New(source, target), cleanup, nil
	}
}

func printPlan(output io.Writer, options config.Options, plan multisync.Plan, prepared []preparedTable, duration time.Duration) {
	selected := 0
	excluded := 0
	invalid := 0
	var rows int64
	for _, table := range prepared {
		status := "ready"
		errorText := ""
		switch {
		case table.plan.Excluded:
			status = "excluded"
			excluded++
		case table.plan.Collision:
			status = "collision"
			invalid++
			selected++
			errorText = "multiple selected source tables map to this target"
		case table.err != nil:
			status = "invalid"
			invalid++
			selected++
			errorText = redact(options, table.err).Error()
		default:
			selected++
			rows += table.report.SourceRows
		}
		if table.plan.Excluded && !options.ShowExcluded {
			continue
		}
		fmt.Fprintf(output, "plan source=%s.%s target=%s.%s status=%s rows=%d",
			plan.SourceSchema, table.plan.SourceName, plan.TargetSchema, table.plan.TargetName, status, table.report.SourceRows)
		if !table.plan.Excluded && table.err == nil {
			fmt.Fprintf(output, " target_exists=%t target_compatible=%t",
				table.report.TargetStatus.Exists, table.report.TargetStatus.Compatible)
		}
		if errorText != "" {
			fmt.Fprintf(output, " error=%q", errorText)
		}
		fmt.Fprintln(output)
		if options.PerformanceMetrics && options.DryRun && !table.plan.Excluded && table.err == nil {
			fmt.Fprintf(output, "perf_table source=%s.%s target=%s.%s phase=preflight\n",
				plan.SourceSchema, table.plan.SourceName, plan.TargetSchema, table.plan.TargetName)
			printPerformance(output, table.report)
		}
	}
	for _, collision := range plan.Collisions {
		fmt.Fprintf(output, "collision target=%s.%s sources=%s\n",
			plan.TargetSchema, collision.TargetName, strings.Join(collision.SourceNames, ","))
	}
	requestedMode := "dry-run"
	if options.Apply {
		requestedMode = "apply-preflight"
	}
	fmt.Fprintf(output, "mode=plan requested_mode=%s all_tables=true source_schema=%s target_schema=%s discovered=%d selected=%d excluded=%d excluded_details=%t collisions=%d invalid=%d source_rows=%d duration=%s no_PostgreSQL_writes=true\n",
		requestedMode, plan.SourceSchema, plan.TargetSchema, len(plan.Tables), selected, excluded,
		options.ShowExcluded, len(plan.Collisions), invalid, rows, duration.Round(time.Millisecond))
}

func planInvalid(plan multisync.Plan, prepared []preparedTable) bool {
	if len(plan.Collisions) > 0 {
		return true
	}
	for _, table := range prepared {
		if table.err != nil {
			return true
		}
	}
	return false
}

func printApplySummary(output io.Writer, options config.Options, summary multisync.Summary, totalDuration time.Duration) {
	for _, result := range summary.Results {
		fmt.Fprintf(output, "table source=%s.%s target=%s.%s status=%s rows=%d duration=%s",
			options.SourceSchema, result.SourceName, options.TargetSchema, result.TargetName,
			result.Status, result.Rows, result.Duration.Round(time.Millisecond))
		if result.Err != nil {
			fmt.Fprintf(output, " error=%q", redact(options, result.Err))
		}
		fmt.Fprintln(output)
		if options.PerformanceMetrics && result.Report.Applied {
			fmt.Fprintf(output, "perf_table source=%s.%s target=%s.%s\n",
				options.SourceSchema, result.SourceName, options.TargetSchema, result.TargetName)
			printPerformance(output, result.Report)
		}
	}
	rate := 0.0
	if totalDuration > 0 {
		rate = float64(summary.Rows) / totalDuration.Seconds()
	}
	fmt.Fprintf(output, "mode=applied all_tables=true sync_mode=%s workers=%d succeeded=%d failed=%d cancelled=%d rows=%d apply_duration=%s duration=%s rows_per_second=%.1f database_wide_atomic=false downstream_ready=%t\n",
		options.SyncMode, options.Workers, summary.Succeeded, summary.Failed, summary.Cancelled,
		summary.Rows, summary.Duration.Round(time.Millisecond), totalDuration.Round(time.Millisecond), rate,
		summary.Failed == 0 && summary.Cancelled == 0)
}

func multiProgressPrinter(output io.Writer, lock *sync.Mutex, table string) func(syncer.Progress) {
	printer := progressPrinterWithPrefix(output, "table="+table+" ")
	return func(progress syncer.Progress) {
		lock.Lock()
		defer lock.Unlock()
		printer(progress)
	}
}

func printCPUProfile(output io.Writer, path string) {
	if path != "" {
		fmt.Fprintf(output, "cpu_profile=%s\n", path)
	}
}

func redact(options config.Options, err error) error {
	return config.RedactError(err, options.MSSQLDSN, options.PGSQLDSN)
}
