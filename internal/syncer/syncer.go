// Package syncer coordinates one guarded source discovery and PostgreSQL refresh.
package syncer

import (
	"context"
	"fmt"
	"time"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/postgres"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/schema"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/stream"
)

// Source discovers, counts and streams one SQL Server table.
type Source interface {
	Describe(context.Context, string, string, string, string) (schema.Table, error)
	Count(context.Context, schema.Table) (int64, error)
	Rows(context.Context, schema.Table) (stream.Source, error)
}

// Target validates and publishes one PostgreSQL table.
type Target interface {
	Inspect(context.Context, schema.Table) (postgres.Status, error)
	Publish(context.Context, schema.Table, stream.Source, int64) (postgres.Result, error)
}

// Request identifies the only table and whether mutation is explicitly authorized.
type Request struct {
	// SourceSchema is the SQL Server schema.
	SourceSchema string
	// SourceTable is the SQL Server table.
	SourceTable string
	// TargetSchema is the PostgreSQL schema.
	TargetSchema string
	// TargetTable is the PostgreSQL table.
	TargetTable string
	// Apply authorizes publication; false is a read-only dry run.
	Apply bool
	// ProgressInterval is the minimum interval between staging row reports.
	ProgressInterval time.Duration
	// Progress receives phase and staging-count updates; nil disables reporting.
	Progress func(Progress)
}

// Progress describes one non-sensitive synchronization progress event.
type Progress struct {
	// Phase identifies the current operation.
	Phase string
	// Rows is the number of source rows staged so far.
	Rows int64
	// TotalRows is the source count measured before streaming.
	TotalRows int64
	// Elapsed is the time spent streaming source rows.
	Elapsed time.Duration
}

// Report contains discovery, validation and optional publication evidence.
type Report struct {
	// Table is the mapped metadata contract.
	Table schema.Table
	// SourceRows is the pre-stream source count.
	SourceRows int64
	// TargetStatus is the dry-run-compatible target inspection.
	TargetStatus postgres.Status
	// Publication is populated only for an applied refresh.
	Publication postgres.Result
	// Applied reports whether PostgreSQL publication ran.
	Applied bool
	// Duration is the total orchestration duration.
	Duration time.Duration
}

// Service coordinates the source and target adapters sequentially.
type Service struct {
	source Source
	target Target
}

// New constructs a one-table synchronization service.
func New(source Source, target Target) *Service {
	return &Service{source: source, target: target}
}

// Run discovers and validates one table, then publishes only when Apply is true.
func (service *Service) Run(ctx context.Context, request Request) (Report, error) {
	started := time.Now()
	emitProgress(request.Progress, Progress{Phase: "discovering"})
	table, err := service.source.Describe(ctx, request.SourceSchema, request.SourceTable, request.TargetSchema, request.TargetTable)
	if err != nil {
		return Report{}, fmt.Errorf("discover source table: %w", err)
	}
	emitProgress(request.Progress, Progress{Phase: "validating-target"})
	status, err := service.target.Inspect(ctx, table)
	if err != nil {
		return Report{}, fmt.Errorf("validate target table: %w", err)
	}
	emitProgress(request.Progress, Progress{Phase: "counting-source"})
	count, err := service.source.Count(ctx, table)
	if err != nil {
		return Report{}, err
	}
	report := Report{Table: table, SourceRows: count, TargetStatus: status}
	if !request.Apply {
		report.Duration = time.Since(started)
		return report, nil
	}
	emitProgress(request.Progress, Progress{Phase: "staging", TotalRows: count})
	rows, err := service.source.Rows(ctx, table)
	if err != nil {
		return Report{}, err
	}
	if request.Progress != nil && request.ProgressInterval > 0 {
		rows = newProgressSource(rows, count, request.ProgressInterval, request.Progress)
	}
	result, err := service.target.Publish(ctx, table, rows, count)
	if err != nil {
		return Report{}, err
	}
	report.Publication = result
	report.Applied = true
	report.Duration = time.Since(started)
	return report, nil
}

func emitProgress(callback func(Progress), progress Progress) {
	if callback != nil {
		callback(progress)
	}
}
